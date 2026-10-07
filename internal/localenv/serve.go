package localenv

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/browser"
)

// Options is one run of the editor.
type Options struct {
	// File is the .env being edited.
	File *File
	// Focus highlights these keys. It never filters: the person filling this
	// in may know about a second database the agent has not heard of, and a
	// filtered form is a form that cannot tell anybody about it.
	Focus []string
	// Timeout closes the server if nobody saves. A browser tab holding a
	// customer's credentials should not outlive the task that opened it.
	Timeout time.Duration
	// NoBrowser prints the URL instead of opening it - a container, an SSH
	// session, or somebody who would rather paste it themselves.
	NoBrowser bool
	// OnReady, when set, is told the page's URL once the server listens, and
	// takes the place of opening a browser and printing the URL: the caller
	// decides who sees it. It is how the Workbench sandbox runs the form in a
	// process of its own and hands the page to the member separately.
	OnReady func(url string)
}

// Result is what changed, by key name.
//
// **There are no values in it, and that is the point of the whole command.**
// Whatever this returns may be printed, logged, or read back by an agent whose
// transcript is kept; a credential that reaches any of those has to be treated
// as disclosed.
type Result struct {
	Saved   bool
	Filled  []string
	Changed []string
	Cleared []string
	Added   []string
	Empty   []string
}

// entryJSON is one row as the page sees it.
type entryJSON struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Note   string `json:"note"`
	Secret bool   `json:"secret"`
	Focus  bool   `json:"focus"`
}

type saveRequest struct {
	Entries []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"entries"`
}

// Serve opens the editor and blocks until it is saved, cancelled or times out.
//
// Progress goes to out - which the caller points at stderr - so that anything
// this command writes to stdout stays the key-name report.
func Serve(ctx context.Context, opts Options, out io.Writer) (Result, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Result{}, fmt.Errorf("listen on 127.0.0.1: %w", err)
	}
	defer ln.Close()

	token, err := newToken()
	if err != nil {
		return Result{}, err
	}
	addr := ln.Addr().String()
	url := fmt.Sprintf("http://%s/?t=%s", addr, token)

	before := snapshot(opts.File)
	focus := make(map[string]bool, len(opts.Focus))
	for _, k := range opts.Focus {
		focus[k] = true
	}

	done := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{
		Handler:           guard(mux, token, addr),
		ReadHeaderTimeout: 5 * time.Second,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})

	mux.HandleFunc("/api/env", func(w http.ResponseWriter, r *http.Request) {
		rows := make([]entryJSON, 0)
		for _, e := range opts.File.Entries() {
			rows = append(rows, entryJSON{
				Key: e.Key, Value: e.Value, Note: e.Note,
				Secret: e.Secret, Focus: focus[e.Key],
			})
		}
		writeJSON(w, map[string]any{"path": opts.File.Path, "entries": rows})
	})

	mux.HandleFunc("/api/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "post only", http.StatusMethodNotAllowed)
			return
		}
		var req saveRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		for _, e := range req.Entries {
			if !validKey(e.Key) {
				http.Error(w, "a key is letters, digits and underscore, and does not start with a digit: "+e.Key,
					http.StatusBadRequest)
				return
			}
		}
		for _, e := range req.Entries {
			opts.File.Set(e.Key, e.Value)
		}
		if err := opts.File.Save(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			done <- err
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		done <- nil
	})

	mux.HandleFunc("/api/cancel", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		done <- errCancelled
	})

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			done <- err
		}
	}()

	if opts.OnReady != nil {
		opts.OnReady(url)
	} else {
		presentForm(ctx, out, opts, url, addr)
	}

	var saveErr error

	select {
	case saveErr = <-done:
	case <-ctx.Done():
		saveErr = ctx.Err()
	case <-time.After(opts.Timeout):
		saveErr = fmt.Errorf("nothing was saved within %s, so nothing was written", opts.Timeout.Round(time.Second))
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)

	if saveErr != nil {
		if saveErr == errCancelled {
			return Result{}, nil
		}
		return Result{}, saveErr
	}
	return diff(before, opts.File), nil
}

// presentForm puts the page in front of whoever fills it in and says how long
// it stays.
func presentForm(ctx context.Context, out io.Writer, opts Options, url, addr string) {
	fmt.Fprintf(out, "editing %s at\n\n    %s\n\n", opts.File.Path, url)
	if opts.NoBrowser {
		fmt.Fprintf(out, "open it yourself; over SSH, forward the port first:\n"+
			"    ssh -L %s:%s <host>\n", portOf(addr), addr)
	} else if _, err := browser.Present(ctx, url, false); err != nil {
		fmt.Fprintf(out, "could not open a browser (%v), so open the URL above yourself\n", err)
	}
	fmt.Fprintf(out, "the page closes this server when you save; it also stops on its own after %s\n",
		opts.Timeout.Round(time.Second))
}

// errCancelled is the page saying it is done without saving.
var errCancelled = fmt.Errorf("cancelled")

// guard is the whole authentication story, and it is short because the surface
// is: a loopback socket, one process, one token, one task.
//
// Two checks rather than one. The token stops another program on this machine
// from reading the customer's credentials off the port. The Host check stops a
// page on the open internet from pointing a name it controls at 127.0.0.1 and
// talking to this server through the browser of whoever visits it - the token
// alone would not, because that attack does not need to read the URL.
func guard(next http.Handler, token, addr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != addr {
			http.Error(w, "this server answers to 127.0.0.1 only", http.StatusForbidden)
			return
		}
		got := r.Header.Get("X-Asgard-Token")
		if got == "" {
			got = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "wrong or missing token", http.StatusForbidden)
			return
		}
		h := w.Header()
		// The page holds credentials, so it is allowed to talk to nothing but
		// the process that served it: no fonts, no analytics, no image that
		// carries a value in its query string.
		h.Set("Content-Security-Policy",
			"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; form-action 'none'")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	return addr
}

// validKey accepts what an environment variable may be called, which is also
// what the db-query scripts match when they read the file.
func validKey(k string) bool {
	if k == "" || (k[0] >= '0' && k[0] <= '9') {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// snapshot records which keys had a value, so the report can say what changed
// without ever holding the values side by side.
func snapshot(f *File) map[string]string {
	out := map[string]string{}
	for _, e := range f.Entries() {
		out[e.Key] = e.Value
	}
	return out
}

// diff turns two snapshots into key names.
func diff(before map[string]string, f *File) Result {
	res := Result{Saved: true}
	for _, e := range f.Entries() {
		old, existed := before[e.Key]
		switch {
		case !existed:
			res.Added = append(res.Added, e.Key)
			if e.Value != "" {
				res.Filled = append(res.Filled, e.Key)
			}
		case old == "" && e.Value != "":
			res.Filled = append(res.Filled, e.Key)
		case old != "" && e.Value == "":
			res.Cleared = append(res.Cleared, e.Key)
		case old != e.Value:
			res.Changed = append(res.Changed, e.Key)
		}
		if e.Value == "" {
			res.Empty = append(res.Empty, e.Key)
		}
	}
	return res
}

// Report writes what changed, by key name only.
func (r Result) Report(w io.Writer) {
	if !r.Saved {
		fmt.Fprintln(w, "nothing was saved")
		return
	}
	section := func(title string, keys []string) {
		if len(keys) == 0 {
			return
		}
		fmt.Fprintf(w, "%s (%d):\n", title, len(keys))
		for _, k := range keys {
			fmt.Fprintf(w, "  %s\n", k)
		}
	}
	section("now have a value", r.Filled)
	section("value replaced", r.Changed)
	section("emptied", r.Cleared)
	section("added by hand in the form", r.Added)
	section("still empty", r.Empty)
	if len(r.Filled)+len(r.Changed)+len(r.Cleared)+len(r.Added) == 0 {
		fmt.Fprintln(w, "saved with nothing changed")
	}
}

// Keys lists every key the file has, for a caller that wants to name them.
func Keys(f *File) []string {
	var out []string
	for _, e := range f.Entries() {
		out = append(out, e.Key)
	}
	return out
}

// MissingFocus reports which of the requested focus keys are not in the file,
// so a typo in --focus is said out loud rather than silently highlighting
// nothing.
func MissingFocus(f *File, focus []string) []string {
	var out []string
	for _, k := range focus {
		if k = strings.TrimSpace(k); k != "" && !f.Has(k) {
			out = append(out, k)
		}
	}
	return out
}
