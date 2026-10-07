package browser

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/sandbox"
)

// fakeCDP is a browser with one visible tab that records what it was told.
type fakeCDP struct {
	mu      sync.Mutex
	methods []string
	urls    []string
	srv     *httptest.Server
}

func newFakeCDP(t *testing.T) *fakeCDP {
	t.Helper()
	f := &fakeCDP{}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		ws := "ws://" + r.Host + "/devtools/page/TAB1"
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"id": "DT", "type": "page", "url": "devtools://devtools/x", "webSocketDebuggerUrl": "ws://" + r.Host + "/devtools/page/DT"},
			{"id": "TAB1", "type": "page", "url": "chrome://newtab/", "webSocketDebuggerUrl": ws},
		})
	})
	mux.HandleFunc("/devtools/page/TAB1", func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Sec-WebSocket-Key")
		sum := sha1.Sum([]byte(key + wsGUID))
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
			base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
		_ = rw.Flush()
		for {
			msg, err := readClientFrame(rw.Reader)
			if err != nil {
				return
			}
			var call struct {
				ID     int               `json:"id"`
				Method string            `json:"method"`
				Params map[string]string `json:"params"`
			}
			_ = json.Unmarshal(msg, &call)
			f.mu.Lock()
			f.methods = append(f.methods, call.Method)
			f.urls = append(f.urls, call.Params["url"])
			f.mu.Unlock()
			// An event first, as a real browser sends them, then the answer.
			writeServerFrame(rw.Writer, []byte(`{"method":"Page.frameStartedLoading","params":{}}`))
			writeServerFrame(rw.Writer, []byte(`{"id":`+itoa(call.ID)+`,"result":{}}`))
			_ = rw.Flush()
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func readClientFrame(r *bufio.Reader) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := uint64(h[1] & 0x7F)
	if n == 126 {
		var ext [2]byte
		_, _ = io.ReadFull(r, ext[:])
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	}
	mask := make([]byte, 4)
	if _, err := io.ReadFull(r, mask); err != nil {
		return nil, err
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, err
	}
	for i := range p {
		p[i] ^= mask[i%4]
	}
	return p, nil
}

func writeServerFrame(w io.Writer, p []byte) {
	h := []byte{0x81}
	if len(p) < 126 {
		h = append(h, byte(len(p)))
	} else {
		h = append(h, 126, byte(len(p)>>8), byte(len(p)))
	}
	_, _ = w.Write(append(h, p...))
}

// In the sandbox a loopback page is opened in the tab the member will see,
// and a page on the internet is not opened at all.
func TestPresentInTheSandbox(t *testing.T) {
	cdp := newFakeCDP(t)
	t.Setenv(sandbox.EnvMode, "true")
	t.Setenv(EnvCDPURL, cdp.srv.URL)

	h, err := Present(context.Background(), "http://127.0.0.1:43123/?t=abc", false)
	if err != nil || h != OpenedInSandbox {
		t.Fatalf("loopback: %v %v", h, err)
	}
	if strings.Join(cdp.methods, ",") != "Page.navigate,Page.bringToFront" || cdp.urls[0] != "http://127.0.0.1:43123/?t=abc" {
		t.Errorf("told the browser %v %v", cdp.methods, cdp.urls)
	}

	h, err = Present(context.Background(), "https://github.com/apps/x/installations/new?state=s", false)
	if err != nil || h != ForTheMember || len(cdp.methods) != 2 {
		t.Errorf("internet page: %v %v, browser calls %v", h, err, cdp.methods)
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
// Workbench sandbox pod, with CDP_LIVE_URL naming the page to open.
func TestLiveNavigate(t *testing.T) {
	target, cdp := os.Getenv("CDP_LIVE_URL"), os.Getenv(EnvCDPURL)
	if target == "" || cdp == "" {
		t.Skip("CDP_LIVE_URL and " + EnvCDPURL + " not set")
	}
	if err := navigateCDP(context.Background(), cdp, target); err != nil {
		t.Fatal(err)
	}
}
