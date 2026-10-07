package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

// Every call says which client is making it, and the one that matters is three
// hops away from here.
//
// `pipeline connect` ends in a browser, on a page the web console's own connect
// flow ends on too, and that page offers a way back INTO the console. Right for
// somebody who started there, wrong for somebody waiting at this terminal. The
// platform can only tell the two apart because this header said so at the
// start - so a call that stops sending it fails nothing, and quietly walks the
// next person into the wrong flow.
func TestEveryRequestSaysWhichClientIsCalling(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{}}`))
	}))
	defer srv.Close()

	c := &Client{
		profile:   auth.Profile{Name: "test", PlatformAPI: srv.URL},
		token:     "token",
		workspace: "ws-1",
		http:      srv.Client(),
	}
	err := c.do(context.Background(), request{
		method: http.MethodPost,
		path:   "/v1/iac/connections/begin-github-attach",
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if v := got.Get(ClientHeader); v != clientValue() {
		t.Errorf("%s = %q, want %q", ClientHeader, v, clientValue())
	}
	// The call that takes no workspace sends it too: what this program is does
	// not depend on which route it is calling.
	got = nil
	if err := c.do(context.Background(), request{
		method:      http.MethodGet,
		path:        "/v1/workspaces",
		noWorkspace: true,
	}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if v := got.Get(ClientHeader); v != clientValue() {
		t.Errorf("unscoped %s = %q, want %q", ClientHeader, v, clientValue())
	}
}

// **The header carries the version, and the client is still readable without
// parsing it.** The server has to be able to tell a CLI from the web console by
// looking at the front of the value, and it has to be able to tell WHICH CLI
// without a second call - a client too old to know about a version endpoint
// never calls one, so the version travels on the requests every version
// already makes.
func TestTheClientHeaderCarriesTheVersion(t *testing.T) {
	v := clientValue()
	if !strings.HasPrefix(v, ClientName) {
		t.Errorf("%q does not begin with %q, so a prefix match stops recognising the CLI", v, ClientName)
	}
	if v == ClientName {
		t.Errorf("%q carries no version, which is the whole point of the value", v)
	}
	name, ver, ok := strings.Cut(v, "/")
	if !ok || name != ClientName || ver == "" {
		t.Errorf("%q is not <client>/<version>", v)
	}
}
