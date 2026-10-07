package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/browser"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/localenv"
)

// local-env in the Workbench assistant's sandbox (asgard-odin-pm decision
// 2026-09-30-workbench-sandbox-fde-browser-choice FDE-19).
//
// On a desktop the command is one blocking process: it serves the form, opens
// the browser, and returns when the form is saved. That shape does not survive
// the sandbox. The agent reads a command's output only when the command ends,
// so it could not hand the member the browser while the command waits; and one
// command runs at most ten minutes, which is less than the form's fifteen. So
// there it is two steps:
//
//	asgard-cli local-env          the form server starts in a process of its
//	                              own, the sandbox's browser is sent to the page,
//	                              and this ends - the agent then calls
//	                              open_sandbox_browser so the member can take over
//	asgard-cli local-env --wait   waits for the save, at most a few minutes at
//	                              a time, and says "still waiting" before the
//	                              agent's command limit rather than being cut off
//
// What comes back is the same list of key names as on a desktop, never a value.

// localEnvWaitDefault is how long one --wait call waits: under the ten
// minutes one agent command may run, so it reports rather than being killed.
const localEnvWaitDefault = 8 * time.Minute

// localEnvServeFlag is the hidden flag the background process is started with.
const localEnvServeFlag = "serve-in-background"

// localEnvState is what the background form server and --wait share, one
// file per repository. It holds no value from the form: key names only.
type localEnvState struct {
	PID       int       `json:"pid"`
	Root      string    `json:"root"`
	URL       string    `json:"url,omitempty"`
	StartedAt time.Time `json:"started_at"`
	Deadline  time.Time `json:"deadline"`
	Done      bool      `json:"done"`
	// Result is set once the form was saved; Error once it ended without.
	Result *localenv.Result `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// localEnvStatePath is the state file for the repository at root, beside the
// Workbench session file - the sandbox's own scratch space, not /work.
func localEnvStatePath(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(filepath.Dir(auth.SessionFilePath()), "local-env-"+hex.EncodeToString(sum[:])[:12]+".json")
}

func readLocalEnvState(path string) (*localEnvState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s localEnvState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("the form server's state %s is unreadable: %w", path, err)
	}
	return &s, nil
}

// writeLocalEnvState replaces the file whole, so a reader never sees half.
func writeLocalEnvState(path string, s *localEnvState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// startLocalEnvInSandbox is the first step: start the server in the
// background, send the sandbox's browser to it, and say what comes next.
func startLocalEnvInSandbox(cmd *cobra.Command, root string, focus []string, timeout time.Duration) error {
	statePath := localEnvStatePath(root)
	if prev, err := readLocalEnvState(statePath); err == nil && !prev.Done && processAlive(prev.PID) {
		// A second call is a new request - different --focus, or the member
		// closed the tab. One form per repository, the newest.
		stopProcess(prev.PID)
	}
	_ = os.Remove(statePath)

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find this binary: %w", err)
	}
	args := []string{"local-env", "--" + localEnvServeFlag, "--timeout", timeout.String()}
	for _, k := range focus {
		args = append(args, "--focus", k)
	}
	logPath := statePath + ".log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	child := exec.Command(self, args...)
	child.Dir = root
	child.Stdin = nil
	child.Stdout, child.Stderr = logFile, logFile
	if err := startDetached(child); err != nil {
		return fmt.Errorf("start the form server: %w", err)
	}

	// The child writes its URL once it listens.
	var st *localEnvState
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if s, err := readLocalEnvState(statePath); err == nil && (s.URL != "" || s.Done) {
			st = s
			break
		}
	}
	if st == nil {
		return fmt.Errorf("the form server did not start; its log is %s", logPath)
	}
	if st.Done {
		return fmt.Errorf("the form server stopped right after starting: %s", st.Error)
	}

	out := cmd.OutOrStdout()
	// The URL carries the form's one-time token. It is printed only when the
	// browser could not be sent to it: the page is for the member, and the
	// agent's transcript is kept.
	if _, err := browser.Present(cmd.Context(), st.URL, false); err != nil {
		fmt.Fprintf(out, "The form for %s is served, but the sandbox's browser could not be sent to it (%v).\n"+
			"Open this in the sandbox's browser, then hand it over as below:\n\n    %s\n\n",
			filepath.Join(root, localenv.FileName), err, st.URL)
	} else {
		fmt.Fprintf(out, "The form for %s is open in the sandbox's browser.\n\n", filepath.Join(root, localenv.FileName))
	}
	fmt.Fprintf(out, "Next, in this order:\n\n"+
		"  1. Call the open_sandbox_browser tool, so the member can take over the sandbox's\n"+
		"     browser and fill the form in. Tell them what the keys are for; never ask them\n"+
		"     to tell you a value.\n"+
		"  2. Run `asgard-cli local-env --wait`. It returns when they save, or says it is\n"+
		"     still waiting after a few minutes - then run it again.\n\n"+
		"The form stays open until %s, then closes without writing anything.\n",
		st.Deadline.Local().Format("15:04"))
	return nil
}

// serveLocalEnvInBackground is the background process: the form server, with
// its URL and its outcome written to the state file instead of a terminal.
func serveLocalEnvInBackground(cmd *cobra.Command, root string, opts localenv.Options) error {
	statePath := localEnvStatePath(root)
	st := &localEnvState{
		PID: os.Getpid(), Root: root, StartedAt: time.Now(), Deadline: time.Now().Add(opts.Timeout),
	}
	opts.NoBrowser = true
	opts.OnReady = func(url string) {
		st.URL = url
		if err := writeLocalEnvState(statePath, st); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "write the state: %v\n", err)
		}
	}
	res, err := localenv.Serve(cmd.Context(), opts, io.Discard)
	st.Done = true
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Result = &res
	}
	return writeLocalEnvState(statePath, st)
}

// waitLocalEnvInSandbox is the second step: wait for the save, up to wait.
func waitLocalEnvInSandbox(cmd *cobra.Command, root string, wait time.Duration) error {
	statePath := localEnvStatePath(root)
	out := cmd.OutOrStdout()
	deadline := time.Now().Add(wait)
	for {
		st, err := readLocalEnvState(statePath)
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("no form is open for this repository; `asgard-cli local-env` opens one")
		}
		if err != nil {
			return err
		}
		if st.Done {
			_ = os.Remove(statePath)
			_ = os.Remove(statePath + ".log")
			if st.Result == nil {
				return fmt.Errorf("the form ended without saving: %s", st.Error)
			}
			st.Result.Report(out)
			fmt.Fprintf(out, "\nRe-read %s rather than assume it holds exactly what was asked for: the member may have added keys.\n", localenv.FileName)
			return nil
		}
		if !processAlive(st.PID) {
			_ = os.Remove(statePath)
			return errors.New("the form server is gone and left no result; the sandbox may have been stopped while it waited. " +
				"`asgard-cli local-env` opens the form again; nothing was written")
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(out, "Still waiting for the member to save the form (it stays open until %s).\n"+
				"Run `asgard-cli local-env --wait` again.\n", st.Deadline.Local().Format("15:04"))
			return nil
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(time.Second):
		}
	}
}
