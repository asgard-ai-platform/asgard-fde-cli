package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

// sandboxEnv puts this process in a Workbench sandbox whose session file names
// platformAPI, with a HOME and a git config of its own.
func sandboxEnv(t *testing.T, platformAPI string) string {
	t.Helper()
	home := t.TempDir()
	session := filepath.Join(t.TempDir(), "session.json")
	body := `{"access_token":"member-token","workspace_id":"ws-sb","platform_api":"` + platformAPI +
		`","user":{"id":"u-1","display_name":"Ada Lovelace","email":""},"channel_id":"wb-fde-1"}`
	if err := os.WriteFile(session, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(auth.EnvSandboxMode, "true")
	t.Setenv(auth.EnvSessionFile, session)
	t.Setenv(auth.EnvToken, "")
	t.Setenv(auth.EnvWorkspace, "")
	t.Setenv(auth.EnvPlatformAPI, "")
	t.Setenv(auth.EnvHome, t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("ASGARD_NO_UPDATE_CHECK", "1")
	return home
}

func runCLI(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestRepositoryFromPath(t *testing.T) {
	for in, want := range map[string]string{
		"acme/app.git": "acme/app", "acme/app": "acme/app", "/acme/app.git/": "acme/app",
		"acme": "", "": "", "acme/app/extra": "", "/app": "",
	} {
		if got := repositoryFromPath(in); got != want {
			t.Errorf("repositoryFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// git's credential protocol does not say fetch or push, so the helper asks
// for a push token and, refused by the platform's permissions, settles for a
// read token.
func TestGitCredentialFallsBackToRead(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var via, workspace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		via, workspace = r.Header.Get("X-Asgard-Via-Assistant"), r.Header.Get("x-asgard-workspace")
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/write") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"success":false,"message":"Forbidden"}`)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"token":"ghs_read","repository":"acme/app","permission":"read","expires_at":"2026-09-30T08:00:00Z"}}`)
	}))
	defer srv.Close()
	sandboxEnv(t, srv.URL)

	out, errOut, err := runCLI(t, "protocol=https\nhost=github.com\npath=acme/app.git\n\n", "pipeline", "git-credential", "get")
	if err != nil {
		t.Fatal(err)
	}
	if out != "username=x-access-token\npassword=ghs_read\n" {
		t.Errorf("stdout %q", out)
	}
	if !strings.Contains(errOut, "workspace administration") {
		t.Errorf("stderr does not say why the push will fail: %q", errOut)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/write") || !strings.HasSuffix(paths[1], "/read") {
		t.Errorf("asked %v", paths)
	}
	if via != "true" || workspace != "ws-sb" {
		t.Errorf("via %q workspace %q; want the assistant marker and the session's workspace", via, workspace)
	}

	// Anything but github.com over https gets no answer, and no call.
	paths = nil
	out, _, err = runCLI(t, "protocol=https\nhost=gitlab.com\npath=acme/app.git\n\n", "pipeline", "git-credential", "get")
	if err != nil || out != "" || len(paths) != 0 {
		t.Errorf("gitlab: out %q err %v calls %v", out, err, paths)
	}
	// store and erase do nothing.
	if out, _, err := runCLI(t, "protocol=https\nhost=github.com\n", "pipeline", "git-credential", "store"); err != nil || out != "" {
		t.Errorf("store: %q %v", out, err)
	}
}

func TestGitAuthConfiguresGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := sandboxEnv(t, "https://platform-api.example.test")
	// A helper configured before must not survive as a second source.
	if err := exec.Command("git", "config", "--global", "credential.https://github.com.helper", "store").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "config", "--global", "url.git@github.com:.insteadOf", "https://github.com/").Run(); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runCLI(t, "", "pipeline", "git-auth")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Ada Lovelace <u-1@users.noreply.asgard-ai.com>") {
		t.Errorf("stdout %q", out)
	}
	if !strings.Contains(errOut, "insteadOf") && !strings.Contains(errOut, "insteadof") {
		t.Errorf("no warning about the URL rewrite: %q", errOut)
	}
	raw, _ := exec.Command("git", "config", "--global", "--get-all", "credential.https://github.com.helper").Output()
	helpers := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(helpers) != 2 || helpers[0] != "" || !strings.HasSuffix(helpers[1], " pipeline git-credential") {
		t.Errorf("helpers %q; want the list reset, then this CLI", helpers)
	}
	for key, want := range map[string]string{
		"credential.https://github.com.usehttppath": "true",
		"user.name":  "Ada Lovelace",
		"user.email": "u-1@users.noreply.asgard-ai.com",
	} {
		got, _ := exec.Command("git", "config", "--global", "--get", key).Output()
		if strings.TrimSpace(string(got)) != want {
			t.Errorf("%s = %q, want %q", key, strings.TrimSpace(string(got)), want)
		}
	}
	_ = home
}

func TestSandboxOnlyCommandsRefuseElsewhere(t *testing.T) {
	t.Setenv(auth.EnvSandboxMode, "")
	for _, args := range [][]string{{"pipeline", "git-auth"}, {"pipeline", "git-credential", "get"}} {
		if _, _, err := runCLI(t, "protocol=https\nhost=github.com\npath=a/b\n\n", args...); err == nil || !strings.Contains(err.Error(), "Workbench assistant's sandbox") {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestLoginInTheSandboxSaysThereIsNothingToDo(t *testing.T) {
	sandboxEnv(t, "https://platform-api.example.test")
	for _, c := range []string{"login", "logout"} {
		out, _, err := runCLI(t, "", c)
		if err != nil || !strings.Contains(out, "nothing to") {
			t.Errorf("%s: %q %v", c, out, err)
		}
	}
}

func TestInitRefusesTheSandboxWorkDir(t *testing.T) {
	t.Setenv(auth.EnvSandboxMode, "true")
	if err := refuseObviouslyWrongRoot("/work"); err == nil || !strings.Contains(err.Error(), "side by side") {
		t.Errorf("/work: %v", err)
	}
	if err := refuseObviouslyWrongRoot("/work/acme-asgard-kube"); err != nil {
		t.Errorf("/work/<repo>: %v", err)
	}
	t.Setenv(auth.EnvSandboxMode, "")
	if err := refuseObviouslyWrongRoot("/work"); err != nil {
		t.Errorf("outside a sandbox /work is an ordinary directory: %v", err)
	}
}
