package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/localenv"
)

// --wait reads the background server's outcome: key names only, and "still
// waiting" before the caller's limit rather than a hang.
func TestLocalEnvWaitReadsTheState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the form server runs only in the Workbench sandbox, which is Linux; processAlive is a stub on Windows")
	}
	dir := t.TempDir()
	t.Setenv(auth.EnvSessionFile, filepath.Join(dir, "session.json"))
	root := "/work/acme"
	path := localEnvStatePath(root)
	if filepath.Dir(path) != dir {
		t.Fatalf("state %s is not beside the session file", path)
	}

	// Running (this process stands in for the server), not saved yet.
	st := &localEnvState{PID: os.Getpid(), Root: root, URL: "http://127.0.0.1:1/?t=x", Deadline: time.Now().Add(time.Minute)}
	if err := writeLocalEnvState(path, st); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetContext(context.Background())
	if err := waitLocalEnvInSandbox(cmd, root, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Still waiting") {
		t.Errorf("before the save: %q", out.String())
	}

	// Saved.
	st.Done, st.Result = true, &localenv.Result{Saved: true, Filled: []string{"DB_PASSWORD"}}
	_ = writeLocalEnvState(path, st)
	out.Reset()
	if err := waitLocalEnvInSandbox(cmd, root, time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DB_PASSWORD") {
		t.Errorf("after the save: %q", out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the state outlived the answer")
	}

	// A server that died without answering.
	_ = writeLocalEnvState(path, &localEnvState{PID: 999999, Root: root})
	if err := waitLocalEnvInSandbox(cmd, root, time.Second); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Errorf("dead server: %v", err)
	}
	if err := waitLocalEnvInSandbox(cmd, root, time.Second); err == nil || !strings.Contains(err.Error(), "no form is open") {
		t.Errorf("nothing open: %v", err)
	}
}

// The state file holds no value from the form.
func TestLocalEnvStateCarriesNoValues(t *testing.T) {
	b, _ := json.Marshal(localEnvState{Done: true, Result: &localenv.Result{Saved: true, Filled: []string{"K"}}})
	if strings.Contains(strings.ToLower(string(b)), "value") {
		t.Errorf("state %s has a value field", b)
	}
}

// Step one in the sandbox prints the attach link for the member's own
// browser, records the flow, and opens nothing.
func TestConnectInTheSandboxIsTwoSteps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/iac/connections"):
			_, _ = io.WriteString(w, `{"success":true,"data":[{"connection_id":"c-old","account_login":"acme"}]}`)
		case strings.HasSuffix(r.URL.Path, "/begin-github-attach"):
			_, _ = io.WriteString(w, `{"success":true,"data":{"authorize_url":"https://github.com/login/oauth/authorize?state=S1","state":"S1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"message":"not here"}`)
		}
	}))
	defer srv.Close()
	sandboxEnv(t, srv.URL)

	out, _, err := runCLI(t, "", "pipeline", "connect", "--account", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "THEIR OWN") || !strings.Contains(out, "https://github.com/login/oauth/authorize?state=S1") ||
		!strings.Contains(out, "pipeline connect --continue") {
		t.Errorf("step one said %q", out)
	}
	p, err := loadConnectPending()
	if err != nil {
		t.Fatal(err)
	}
	if p.Stage != "attach" || p.State != "S1" || p.Account != "acme" || p.Workspace != "ws-sb" || len(p.Before) != 1 || p.Before[0] != "c-old" {
		t.Errorf("pending %+v", p)
	}
	// Windows has no Unix permission bits to check.
	if info, _ := os.Stat(connectPendingPath()); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("pending is %v, want 0600", info.Mode().Perm())
	}
	if _, _, err := runCLI(t, "", "pipeline", "connect", "--continue", "--workspace", "other"); err == nil || !strings.Contains(err.Error(), "ws-sb") {
		t.Errorf("continue in another workspace: %v", err)
	}
}
