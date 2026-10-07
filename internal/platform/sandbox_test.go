package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

type recorded struct {
	method, path, via, contentType string
	body                           []byte
	form                           map[string]string
}

func fakePlatform(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]recorded) {
	t.Helper()
	var seen []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorded{method: r.Method, path: r.URL.Path, via: r.Header.Get(ViaAssistantHeader), contentType: r.Header.Get("Content-Type")}
		if strings.HasPrefix(rec.contentType, "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err == nil {
				rec.form = map[string]string{}
				for k, v := range r.MultipartForm.Value {
					rec.form[k] = v[0]
				}
				if fh := r.MultipartForm.File["file"]; len(fh) == 1 {
					f, _ := fh[0].Open()
					b, _ := io.ReadAll(f)
					rec.form["file:"+fh[0].Filename] = string(b)
				}
			}
		} else {
			rec.body, _ = io.ReadAll(r.Body)
		}
		seen = append(seen, rec)
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func sandboxClient(url string) *Client {
	return New(&auth.Session{Profile: auth.Profile{PlatformAPI: url}, Token: "tok", Source: auth.SourceSandbox}, "ws-1")
}

// A session from the sandbox's session file is the assistant acting for the
// member, so every call says so - reads included, not only Workbench writes.
func TestSandboxClientMarksEveryRequest(t *testing.T) {
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	})
	if _, err := sandboxClient(srv.URL).ListPipelines(context.Background()); err != nil {
		t.Fatal(err)
	}
	local := New(&auth.Session{Profile: auth.Profile{PlatformAPI: srv.URL}, Token: "tok", Source: auth.SourceStore}, "ws-1")
	if _, err := local.ListPipelines(context.Background()); err != nil {
		t.Fatal(err)
	}
	if (*seen)[0].via != "true" || (*seen)[1].via != "" {
		t.Errorf("via-assistant: sandbox %q, local %q", (*seen)[0].via, (*seen)[1].via)
	}
}

func TestMintRepositoryToken(t *testing.T) {
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"success":true,"data":{"token":"ghs_secret","repository":"Acme/App","permission":"%s","expires_at":"2026-09-30T08:00:00Z"}}`,
			strings.TrimPrefix(r.URL.Path, "/v1/iac/repository-tokens/"))
	})
	c := sandboxClient(srv.URL)
	for _, write := range []bool{false, true} {
		tok, err := c.MintRepositoryToken(context.Background(), "acme/app", write)
		if err != nil {
			t.Fatal(err)
		}
		if tok.Token != "ghs_secret" || tok.ExpiresAt.IsZero() {
			t.Errorf("token %+v", tok.Repository)
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			if strings.Contains(fmt.Sprintf(verb, tok), "ghs_secret") || strings.Contains(fmt.Sprintf(verb, *tok), "ghs_secret") {
				t.Errorf("%s prints the token", verb)
			}
		}
	}
	if (*seen)[0].path != "/v1/iac/repository-tokens/read" || (*seen)[1].path != "/v1/iac/repository-tokens/write" {
		t.Errorf("paths %s %s", (*seen)[0].path, (*seen)[1].path)
	}
	var body map[string]string
	_ = json.Unmarshal((*seen)[0].body, &body)
	if body["repository"] != "acme/app" {
		t.Errorf("body %s", (*seen)[0].body)
	}
}

// The two 403s read differently: the GitHub App lacking a permission is not
// the member lacking a role, and the remedy is on GitHub, not in the workspace.
func TestForbiddenTellsTheAppFromTheRole(t *testing.T) {
	app := (&APIError{Status: 403, Method: "POST", Path: "/v1/iac/repository-tokens/write", Message: "not granted", ErrorCode: IacAppPermissionNotGranted}).Error()
	role := (&APIError{Status: 403, Method: "POST", Path: "/v1/iac/repository-tokens/write", Message: "Forbidden"}).Error()
	if !strings.Contains(app, "GitHub organization") || strings.Contains(app, "workspace administration") {
		t.Errorf("app 403: %s", app)
	}
	if !strings.Contains(role, "workspace administration") {
		t.Errorf("role 403: %s", role)
	}
	audit := (&APIError{Status: 403, Method: "GET", Path: "/v1/workbench/audit-log/dictionary", Message: "Forbidden"}).Error()
	if !strings.Contains(audit, "audit-log/read") {
		t.Errorf("audit 403: %s", audit)
	}
}

func TestCommentAndAttachment(t *testing.T) {
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/attachments") {
			_, _ = io.WriteString(w, `{"success":true,"data":{"id":"att-1","issue_number":12,"original_filename":"minutes.txt","sha256":"abc"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"id":"c-1","issue_number":12,"body":"hi","via_assistant":true}}`)
	})
	local := New(&auth.Session{Profile: auth.Profile{PlatformAPI: srv.URL}, Token: "tok", Source: auth.SourceStore}, "ws-1")
	if _, err := local.CreateWorkbenchComment(context.Background(), 12, "hi", "done"); err != nil {
		t.Fatal(err)
	}
	a, err := local.UploadWorkbenchAttachment(context.Background(), 12, "minutes.txt", strings.NewReader("the bytes"),
		WorkbenchAttachmentUpload{What: "minutes", From: "customer PM", Dated: "2026-09-29"})
	if err != nil {
		t.Fatal(err)
	}
	c, u := (*seen)[0], (*seen)[1]
	if c.path != "/v1/workbench/issues/12/comments" || c.via != "true" || !strings.Contains(string(c.body), `"status":"done"`) {
		t.Errorf("comment %+v %s", c, c.body)
	}
	// Writes are the assistant's even outside the sandbox, as every Workbench write is.
	if u.path != "/v1/workbench/issues/12/attachments" || u.via != "true" || a.ID != "att-1" {
		t.Errorf("upload %+v", u)
	}
	if u.form["what"] != "minutes" || u.form["from"] != "customer PM" || u.form["dated"] != "2026-09-29" ||
		u.form["file:minutes.txt"] != "the bytes" || u.form["supersedes"] != "" {
		t.Errorf("upload form %v", u.form)
	}
}

func TestAuditLog(t *testing.T) {
	srv, seen := fakePlatform(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/query"):
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, "{\"type\":\"meta\"}\n{\"type\":\"end\",\"rows\":0}\n")
		case strings.Contains(r.URL.Path, "/options/"):
			if r.URL.Query().Get("from") == "" || r.URL.Query().Get("to") == "" {
				t.Errorf("options without a range: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"dimension":"event","values":[{"value":"tool_call","count":3}]}}`)
		default:
			_, _ = io.WriteString(w, `{"success":true,"data":{"events":{"tool_call":"Tool call"},"projects":{"proj-1":{"project_id":"p1","project_name":"App"}}}}`)
		}
	})
	c := sandboxClient(srv.URL)
	ctx := context.Background()
	d, err := c.GetAuditDictionary(ctx)
	if err != nil || d.Events["tool_call"] != "Tool call" || d.Projects["proj-1"].Name != "App" {
		t.Fatalf("dictionary %+v %v", d, err)
	}
	o, err := c.GetAuditOptions(ctx, "event", time.Now().Add(-time.Hour), time.Now())
	if err != nil || len(o.Values) != 1 || o.Values[0].Count != 3 {
		t.Fatalf("options %+v %v", o, err)
	}
	stream, err := c.QueryAuditLog(ctx, json.RawMessage(`{"from":"a","to":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(stream)
	_ = stream.Close()
	if !strings.Contains(string(b), `"type":"end"`) {
		t.Errorf("stream %s", b)
	}
	if got := (*seen)[2]; got.method != "POST" || string(got.body) != `{"from":"a","to":"b"}` {
		t.Errorf("query sent %+v %s", got, got.body)
	}
}
