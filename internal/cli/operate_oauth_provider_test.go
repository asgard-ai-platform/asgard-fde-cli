package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// providerFake is a platform whose release deployed two OAuthProviders, and
// whose project holds a third the release did not deploy.
type providerFake struct {
	mu sync.Mutex
	// switched records each PUT as "provider open-or-close project-header".
	switched []string
}

const providerManifest = `{"success":true,"data":{"helm_revision":1,"helm_status":"deployed","objects":[
{"kind":"OAuthProvider","name":"oap-drive","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: oap-drive\nspec: {}\n"},
{"kind":"OAuthProvider","name":"oap-intercom","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: oap-intercom\nspec: {}\n"},
{"kind":"OAuthCredential","name":"oac-drive","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: oac-drive\nspec: {}\n"}
]}}`

const providerList = `{"success":true,"data":[
{"oauth_provider_id":"oap-sheet","personal_account_eligible":false},
{"oauth_provider_id":"oap-intercom","personal_account_eligible":true,"individual_credentials":{"status":"not_open","connected_count":0,"needs_reauth_count":0}},
{"oauth_provider_id":"oap-drive","display_name":"Google Drive（測試）","personal_account_eligible":true,"individual_credentials":{"status":"open","connected_count":12,"needs_reauth_count":1}}
]}`

func (f *providerFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		switch {
		case p == "/v1/iac/pipelines":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"pipeline_id":"p-1","name":"app"}]}`)
		case p == "/v1/iac/pipelines/p-1/releases":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"release_id":"r-1","name":"dev","project_id":"proj-uuid","namespace":"proj-ns"}]}`)
		case p == "/v1/iac/releases/r-1/manifest":
			_, _ = io.WriteString(w, providerManifest)
		case p == "/v1/project":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"project_id":"proj-uuid","project_name":"App","k8s_namespace_name":"proj-ns"}]}`)
		case p == "/v1/auth/me":
			_, _ = io.WriteString(w, `{"success":true,"data":{"user_id":"u"}}`)
		case p == "/v1/integration/oauth/providers" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, providerList)
		case strings.HasSuffix(p, "/individual-credentials") && r.Method == http.MethodPut:
			name := strings.TrimSuffix(strings.TrimPrefix(p, "/v1/integration/oauth/providers/"), "/individual-credentials")
			var body struct {
				Open bool `json:"open"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switch {
			case name == "oap-intercom" && body.Open:
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"success":false,"message":"Display name required before opening to members","reason_code":10010}`)
				return
			case name == "oap-drive" && r.Header.Get(platform.ProjectHeader) == "proj-forbidden":
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"success":false,"message":"Permission denied"}`)
				return
			}
			verb := "close"
			if body.Open {
				verb = "open"
			}
			f.switched = append(f.switched, name+" "+verb+" "+r.Header.Get(platform.ProjectHeader))
			status := "closed"
			if body.Open {
				status = "open"
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"status":"`+status+`","connected_count":12,"needs_reauth_count":1}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"message":"not here"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// With --release, only what the release deployed; each provider's MEMBERS
// column says whether it may be used with members' accounts and whether it is
// open now.
func TestOperateOAuthProviderListThroughARelease(t *testing.T) {
	f := &providerFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-provider", "list", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and the release's two providers, got:\n%s", out)
	}
	if !strings.HasPrefix(lines[1], "oap-drive") || !strings.HasSuffix(lines[1], "Google Drive（測試）") ||
		!strings.Contains(lines[1], "open") || !strings.Contains(lines[1], "12 (1 to reconnect)") {
		t.Errorf("oap-drive row %q", lines[1])
	}
	// The display name is last, so a CJK one cannot push the other columns
	// out of line: MEMBERS starts at the same place on every row.
	col := strings.Index(lines[0], "MEMBERS")
	for _, l := range lines[1:] {
		if strings.Index(l, "open") != col && strings.Index(l, "not open") != col {
			t.Errorf("MEMBERS not at column %d in %q", col, l)
		}
	}
	if !strings.HasPrefix(lines[2], "oap-intercom") || !strings.Contains(lines[2], "not open") {
		t.Errorf("oap-intercom row %q", lines[2])
	}
	if strings.Contains(out, "oap-sheet") {
		t.Errorf("listed a provider the release did not deploy:\n%s", out)
	}
}

func TestOperateOAuthProviderListThroughAProjectJSON(t *testing.T) {
	f := &providerFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-provider", "list", "--project", "proj-uuid", "--format", "json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []platform.OAuthProvider
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 3 || got[0].ID != "oap-drive" || got[2].ID != "oap-sheet" {
		t.Fatalf("out %q (%v)", out, err)
	}
	if got[2].PersonalAccountEligible || got[2].IndividualCredentials != nil {
		t.Errorf("oap-sheet %+v", got[2])
	}
}

func TestOperateOAuthProviderEnable(t *testing.T) {
	f := &providerFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-provider", "enable", "oap-drive", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if len(f.switched) != 1 || f.switched[0] != "oap-drive open proj-uuid" {
		t.Errorf("switched %v", f.switched)
	}
	if !strings.Contains(out, "is open to the members of release dev") || !strings.Contains(out, "12 connected, 1 to reconnect") {
		t.Errorf("out %q", out)
	}

	if _, _, err := runCLI(t, "", "operate", "oauth-provider", "disable", "oap-drive", "--project", "proj-uuid"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if len(f.switched) != 2 || f.switched[1] != "oap-drive close proj-uuid" {
		t.Errorf("switched %v", f.switched)
	}
}

// A name the release did not deploy is answered before anything is switched.
func TestOperateOAuthProviderEnableWrongName(t *testing.T) {
	f := &providerFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "oauth-provider", "enable", "oap-sheet", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "oap-drive, oap-intercom") {
		t.Fatalf("err %v", err)
	}
	if len(f.switched) != 0 {
		t.Errorf("switched %v", f.switched)
	}
}

// The platform's refusal is turned into what to add to the chart.
func TestOperateOAuthProviderEnableWithoutDisplayName(t *testing.T) {
	f := &providerFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "oauth-provider", "enable", "oap-intercom", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "asgard-ai.com/display-name") || !strings.Contains(err.Error(), `would see "oap-intercom"`) {
		t.Fatalf("err %v", err)
	}
}

func TestOperateOAuthProviderForbiddenNamesThePermission(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/integration/oauth/providers/oap-drive/individual-credentials": "oauth-provider/put",
		"/v1/integration/oauth/credentials/gdrive/authorize":               "oauth-credential/put",
	} {
		err := &platform.APIError{Status: http.StatusForbidden, Method: http.MethodPut, Path: path, Message: "Permission denied"}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %q lacks %s", path, err.Error(), want)
		}
	}
}
