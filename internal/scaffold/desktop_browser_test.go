package scaffold

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const desktopBrowser = skillRoot + "/desktop-browser"

// **The repository travels and the skill goes with it.** An FDE repository is
// committed on a desktop and cloned into the Workbench assistant's sandbox, so
// desktop-browser is in `.agents/skills/` there too, beside the sandbox's own
// `browser` skill - and the sandbox's prompt tells the agent to read a
// repository's skills and lets them take precedence over its general guidance.
// What keeps it from being used there is these three things, and each is held
// here: the first words an agent reads say so, every script refuses to run, and
// the refusal names the skill to use instead.
func TestDesktopBrowserSaysFirstThatItIsNotForTheSandbox(t *testing.T) {
	skill, err := fs.ReadFile(templates, desktopBrowser+"/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	m := frontmatterField.FindAllStringSubmatch(string(skill), -1)
	var description string
	for _, f := range m {
		if f[1] == "description" {
			description = f[2]
		}
	}
	if !strings.HasPrefix(description, "Desktop only. NEVER use this skill when the environment variable ASGARD_SANDBOX_MODE is true") ||
		!strings.Contains(description, `built-in "browser" skill`) {
		t.Errorf("the description does not open with the sandbox guard and the skill to use instead:\n%s", description)
	}
}

func TestEveryDesktopBrowserScriptRefusesInTheSandbox(t *testing.T) {
	err := fs.WalkDir(templates, desktopBrowser, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".py":
		case ".sh", ".bash", ".ps1", ".bat", ".cmd":
			t.Errorf("%s: a shell script; the skill's scripts are Python, so they run on every desktop", path)
			return nil
		default:
			return nil
		}
		body, _ := fs.ReadFile(templates, path)
		// The module refuses at import; any other script must import it before
		// doing anything, which is what the reference teaches.
		if !bytes.Contains(body, []byte("\nrefuse_in_sandbox()\n")) && !bytes.Contains(body, []byte("from desktop_browser import")) {
			t.Errorf("%s neither calls refuse_in_sandbox() at import nor imports desktop_browser", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The refusal itself, run the way an agent would run it. Skipped where there is
// no python3, which is a CI runner rather than a desktop.
func TestDesktopBrowserExitsInTheSandbox(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 on PATH")
	}
	script, err := fs.ReadFile(templates, desktopBrowser+"/scripts/desktop_browser.py")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "desktop_browser.py")
	if err := os.WriteFile(path, script, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, env := range map[string]string{
		"ASGARD_SANDBOX_MODE":    "ASGARD_SANDBOX_MODE=true",
		"ASGARD_BROWSER_CDP_URL": "ASGARD_BROWSER_CDP_URL=http://localhost:9222",
	} {
		for _, args := range [][]string{{"start"}, {"info"}} {
			cmd := exec.Command(python, append([]string{"-I", path}, args...)...)
			cmd.Env = append(os.Environ(), env, "ASGARD_CLI_HOME="+dir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 3 {
				t.Errorf("%s %v: err = %v, want exit status 3", name, args, err)
			}
			if !strings.Contains(stderr.String(), `built-in "browser" skill`) {
				t.Errorf("%s %v: the refusal does not name the skill to use:\n%s", name, args, stderr.String())
			}
		}
	}

	// Outside the sandbox the guard lets it through: info answers, with
	// nothing running, without launching anything.
	cmd := exec.Command(python, "-I", path, "info")
	cmd.Env = append(os.Environ(), "ASGARD_SANDBOX_MODE=", "ASGARD_BROWSER_CDP_URL=", "ASGARD_CLI_HOME="+dir, "ASGARD_DESKTOP_BROWSER_PORT=9")
	out, _ := cmd.Output()
	if !strings.Contains(string(out), "running:   no") {
		t.Errorf("info outside the sandbox = %q, want it to say nothing is running", out)
	}
}
