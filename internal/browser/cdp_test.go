package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/sandbox"
)

// fakeCDP is a browser that records the DevTools HTTP calls it gets.
type fakeCDP struct {
	mu    sync.Mutex
	calls []string
	srv   *httptest.Server
}

func newFakeCDP(t *testing.T) *fakeCDP {
	t.Helper()
	f := &fakeCDP{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/json/new":
			if r.Method != http.MethodPut {
				// What Chromium answers a GET to /json/new.
				http.Error(w, "Using unsafe HTTP verb GET to invoke /json/new", http.StatusMethodNotAllowed)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "TAB2", "type": "page", "url": r.URL.RawQuery})
		case r.URL.Path == "/json/activate/TAB2", r.URL.Path == "/json/close/TAB2":
			_, _ = w.Write([]byte("Target activated"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// In the sandbox a loopback page opens in a new tab, which is raised, and the
// tab that was in front is not touched; a page on the internet is not opened
// at all.
func TestPresentInTheSandbox(t *testing.T) {
	cdp := newFakeCDP(t)
	t.Setenv(sandbox.EnvMode, "true")
	t.Setenv(EnvCDPURL, cdp.srv.URL)

	h, tab, err := PresentTab(context.Background(), "http://127.0.0.1:43123/?t=abc", false)
	if err != nil || h != OpenedInSandbox || tab != "TAB2" {
		t.Fatalf("loopback: %v %q %v", h, tab, err)
	}
	want := "PUT /json/new?" + url.QueryEscape("http://127.0.0.1:43123/?t=abc") + ",GET /json/activate/TAB2?"
	if got := strings.Join(cdp.calls, ","); got != want {
		t.Errorf("told the browser\n  %s\nwant\n  %s", got, want)
	}

	h, err = Present(context.Background(), "https://github.com/apps/x/installations/new?state=s", false)
	if err != nil || h != ForTheMember || len(cdp.calls) != 2 {
		t.Errorf("internet page: %v %v, browser calls %v", h, err, cdp.calls)
	}

	if err := CloseSandboxTab(context.Background(), "TAB2"); err != nil || cdp.calls[len(cdp.calls)-1] != "GET /json/close/TAB2?" {
		t.Errorf("close: %v, calls %v", err, cdp.calls)
	}
	if err := CloseSandboxTab(context.Background(), "GONE"); err != nil {
		t.Errorf("closing a tab that is already gone: %v", err)
	}

	t.Setenv(EnvCDPURL, "")
	if _, err := Present(context.Background(), "http://localhost:1/", false); err == nil {
		t.Error("no browser in the sandbox must be an error")
	}
}

func TestIsLoopback(t *testing.T) {
	for u, want := range map[string]bool{
		"http://127.0.0.1:8080/": true, "http://localhost:1/x": true, "http://[::1]:9/": true,
		"https://github.com/": false, "http://127.0.0.1.example.com/": false, "not a url %%": false,
	} {
		if isLoopback(u) != want {
			t.Errorf("isLoopback(%q) = %v", u, !want)
		}
	}
}

// TestLiveNavigate drives a real sandbox browser. It runs only inside a
// Workbench sandbox pod, with CDP_LIVE_URL naming the page to open; the tab it
// opens is closed again.
func TestLiveNavigate(t *testing.T) {
	target, cdp := os.Getenv("CDP_LIVE_URL"), os.Getenv(EnvCDPURL)
	if target == "" || cdp == "" {
		t.Skip("CDP_LIVE_URL and " + EnvCDPURL + " not set")
	}
	tab, err := openCDPTab(context.Background(), cdp, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseSandboxTab(context.Background(), tab); err != nil {
		t.Fatal(err)
	}
}
