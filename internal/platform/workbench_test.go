package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

func workbenchTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{
		profile:   auth.Profile{Name: "test", PlatformAPI: srv.URL},
		token:     "token",
		workspace: "ws-1",
		http:      srv.Client(),
	}
}

// A write says it is the assistant's and a read does not. The header is what
// makes the timeline say "via Asgard AI", and a write without it reads on the
// timeline as the member having done it by hand.
func TestWorkbenchWritesAreMarkedAsTheAssistants(t *testing.T) {
	seen := map[string]string{}
	c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method] = r.Header.Get(ViaAssistantHeader)
		_, _ = w.Write([]byte(`{"success":true,"data":{"number":3}}`))
	})
	ctx := context.Background()
	if _, err := c.GetWorkbenchIssue(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateWorkbenchIssue(ctx, CreateWorkbenchIssueRequest{Title: "t", Type: "task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateWorkbenchIssue(ctx, 3, UpdateWorkbenchIssueRequest{}); err != nil {
		t.Fatal(err)
	}
	if v := seen[http.MethodGet]; v != "" {
		t.Errorf("GET sent %s: %q, want none", ViaAssistantHeader, v)
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch} {
		if v := seen[m]; v != "true" {
			t.Errorf("%s sent %s: %q, want \"true\" - the platform counts only that exact value", m, ViaAssistantHeader, v)
		}
	}
}

// The two Workbench refusals an agent can act on say what to do, rather than
// the generic 403 text, which is about pipeline permissions.
func TestWorkbenchRefusalsSayWhatHappened(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   []string
		reject string
	}{
		{http.StatusConflict, WorkbenchConflictRetry, []string{"nothing was written", "again is safe"}, ""},
		{http.StatusForbidden, WorkbenchAssistantForbidden, []string{"only the member may take", "Nothing was written"}, "pipeline"},
		{http.StatusForbidden, "", []string{"member of it"}, "pipeline"},
	}
	for _, tc := range cases {
		c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			details := `null`
			if tc.code != "" {
				details = `{"error_code":"` + tc.code + `"}`
			}
			_, _ = w.Write([]byte(`{"success":false,"message":"refused","reason_code":3,"details":` + details + `}`))
		})
		_, err := c.UpdateWorkbenchIssue(context.Background(), 1, UpdateWorkbenchIssueRequest{})
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.ErrorCode != tc.code {
			t.Fatalf("%d %s: got %v, want an APIError carrying the code", tc.status, tc.code, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%d %s: %q does not say %q", tc.status, tc.code, err, w)
			}
		}
		if tc.reject != "" && strings.Contains(err.Error(), tc.reject) {
			t.Errorf("%d %s: %q still carries the pipeline text", tc.status, tc.code, err)
		}
	}
}

// The list walks the cursor and stops at the limit, asking for no more than it
// will keep.
func TestWorkbenchListFollowsTheCursor(t *testing.T) {
	var sizes []string
	c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sizes = append(sizes, r.URL.Query().Get("page_size"))
		if r.URL.Query().Get("page_token") == "" {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"number":1},{"number":2}],"paging":{"next_page_token":"p2","total_count":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"number":3}],"paging":{"next_page_token":"","total_count":3}}`))
	})
	got, total, err := c.ListWorkbenchIssues(context.Background(), WorkbenchIssueFilter{Status: []string{"done"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || total != 3 {
		t.Errorf("got %d issues of %d, want 3 of 3", len(got), total)
	}

	sizes = nil
	got, _, err = c.ListWorkbenchIssues(context.Background(), WorkbenchIssueFilter{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(sizes) != 1 || sizes[0] != "2" {
		t.Errorf("limit 2: got %d issues over pages %v, want 2 from one page of size 2", len(got), sizes)
	}
}

// A download that does not hash to the recorded checksum is an error, because
// a reference filed from it would be diffed against the customer's next
// version as though it were the first.
func TestWorkbenchDownloadChecksIntegrity(t *testing.T) {
	body := []byte("the bytes")
	c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Attachment-Sha256", r.URL.Query().Get("sum"))
		_, _ = w.Write(body)
	})
	a := &WorkbenchAttachment{ID: "a1", IssueNumber: 1, OriginalFilename: "x.pdf",
		SHA256: "58a1c3a1fa2cc1b06b6ef2d4ba2b1a6f2c4c1f84b2c54a8bf8b2d6a0c5b0ba41"}
	var buf bytes.Buffer
	if _, err := c.DownloadWorkbenchAttachment(context.Background(), a, &buf); !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("got %v, want ErrChecksumMismatch", err)
	}

	a.SHA256 = ""
	buf.Reset()
	sum, err := c.DownloadWorkbenchAttachment(context.Background(), a, &buf)
	if err != nil || buf.String() != string(body) || len(sum) != 64 {
		t.Errorf("got %q, %q, %v; want the bytes and their sha256", buf.String(), sum, err)
	}
}

// The member lookup takes at most 50 ids a call, so a longer list is split
// rather than refused.
func TestWorkbenchMembersAreLookedUpInBatches(t *testing.T) {
	var calls []int
	c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		calls = append(calls, len(ids))
		var b strings.Builder
		b.WriteString(`{"success":true,"data":[`)
		for i, id := range ids {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"user_id":"` + id + `","display_name":"n","is_member":true}`)
		}
		b.WriteString(`]}`)
		_, _ = w.Write([]byte(b.String()))
	})
	ids := make([]string, 120)
	for i := range ids {
		ids[i] = "u" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}
	got, err := c.GetWorkbenchMembers(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 120 || len(calls) != 3 || calls[0] != 50 || calls[2] != 20 {
		t.Errorf("got %d members over calls %v, want 120 over [50 50 20]", len(got), calls)
	}
}

// The pipeline filter goes out as pipeline_id. The platform ignores a query
// key it does not know, so a wrong one is not an error: it is a list that
// quietly stops being filtered, and a pull that fetches the whole workspace.
func TestWorkbenchPipelineFilterIsSentAsPipelineID(t *testing.T) {
	var got []string
	c := workbenchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+"?"+strings.Join(r.URL.Query()["pipeline_id"], ","))
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	})
	ctx := context.Background()
	if _, _, err := c.ListWorkbenchIssues(ctx, WorkbenchIssueFilter{Pipeline: []string{"p1", "none"}}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListWorkspaceAttachments(ctx, []string{"p1"}, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"/v1/workbench/issues?p1,none", "/v1/workbench/attachments?p1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("sent %v, want %v", got, want)
	}
}
