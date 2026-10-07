package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/browser"
)

// callbackPath is the one path the loopback server answers on.
//
// The Casdoor application registers `http://127\.0\.0\.1:\d{1,5}/callback` as a
// redirect URI, so this string and that pattern have to agree. The port is not
// fixed - a fixed one is taken sooner or later, and the failure is a sign-in
// that cannot be started at all - and Casdoor accepts any loopback port because
// `http://127.0.0.1:` is one of its built-in redirect prefixes.
const callbackPath = "/callback"

// LoginTimeout is how long the loopback server waits for the browser.
//
// It is generous because the wait includes a person finding a password manager,
// and it exists at all because a CLI that never returns leaves a listening
// socket and a confused terminal behind.
const LoginTimeout = 5 * time.Minute

// LoginOptions is one sign-in.
type LoginOptions struct {
	Profile Profile
	// ProfileFrom is how this profile came to be the one in effect, printed so
	// that somebody signing in to the wrong installation can see where to look.
	ProfileFrom Origin
	// NoBrowser suppresses opening a browser. **The URL is printed either
	// way** - see announce - so this is not what makes the URL available; it
	// is what stops a browser opening on the wrong machine, which is the
	// difference that matters over SSH. There the URL is pasted into a browser
	// anywhere that can reach the loopback port, which means `ssh -L`.
	NoBrowser bool
	// Out is where progress and the URL are written. It is stderr for the
	// command, so that a --format json run's stdout stays parseable.
	Out io.Writer
	// Timeout overrides LoginTimeout.
	Timeout time.Duration
}

// ErrLoginTimeout reports that the browser never came back.
var ErrLoginTimeout = errors.New("timed out waiting for the browser")

// callback is what the loopback handler saw.
type callback struct {
	code  string
	state string
	err   string
	desc  string
}

// Login runs the authorization code flow with PKCE and returns the session it
// produced. It does not store anything: the caller decides that, so a sign-in
// that fails at the userinfo check leaves no half-session on disk.
func Login(ctx context.Context, opts LoginOptions) (Credential, Userinfo, error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Timeout <= 0 {
		opts.Timeout = LoginTimeout
	}

	// Bind before anything else: the redirect URI has to contain the port, and
	// the port is not known until the kernel picks one.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Credential{}, Userinfo{}, fmt.Errorf("listen on a loopback port for the sign-in redirect: %w", err)
	}
	defer listener.Close()

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", listener.Addr().(*net.TCPAddr).Port, callbackPath)

	pk, err := newPKCE()
	if err != nil {
		return Credential{}, Userinfo{}, err
	}
	state, err := randomState()
	if err != nil {
		return Credential{}, Userinfo{}, err
	}

	results := make(chan callback, 1)
	server := &http.Server{
		Handler:           http.HandlerFunc(handleCallback(results)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	authURL := opts.Profile.authorizeURL(redirectURI, state, pk)
	announce(opts, authURL)

	var got callback
	select {
	case got = <-results:
	case <-ctx.Done():
		return Credential{}, Userinfo{}, ctx.Err()
	case <-time.After(opts.Timeout):
		return Credential{}, Userinfo{}, ErrLoginTimeout
	}

	switch {
	case got.err != "":
		if got.desc != "" {
			return Credential{}, Userinfo{}, fmt.Errorf("the sign-in was refused: %s: %s", got.err, got.desc)
		}
		return Credential{}, Userinfo{}, fmt.Errorf("the sign-in was refused: %s", got.err)
	case got.state != state:
		// Either another program answered on the port first, or the redirect
		// was not the one this run started. Neither is worth exchanging a code
		// over.
		return Credential{}, Userinfo{}, errors.New("the sign-in redirect carried the wrong state value, so it did not come from this run; try again")
	case got.code == "":
		return Credential{}, Userinfo{}, errors.New("the sign-in redirect carried no authorization code")
	}

	cred, err := exchangeCode(ctx, newHTTPClient(), opts.Profile, got.code, pk.verifier, redirectURI)
	if err != nil {
		return Credential{}, Userinfo{}, fmt.Errorf("exchange the authorization code: %w", err)
	}

	info, err := FetchUserinfo(ctx, opts.Profile, cred.AccessToken)
	if err != nil {
		return Credential{}, Userinfo{}, fmt.Errorf("confirm who the new token belongs to: %w", err)
	}
	cred.Subject = info.Sub
	cred.Email = info.Email
	cred.Name = info.Who()

	return cred, info, nil
}

// announce tells the person what is about to happen, and prints the URL
// whether or not a browser is opened.
//
// The URL is printed even on success, because opening a browser can silently
// open the wrong one - a headless session, a container, a second profile - and
// a URL on the terminal is the only recovery that does not need the command run
// again.
func announce(opts LoginOptions, authURL string) {
	// Both URLs, because they are two different installations' worth of
	// trust: the issuer is what you are about to authenticate against, and the
	// Platform API is where the resulting token gets used. A profile that
	// mixes them is the failure Resolved.Warning covers, and naming only one
	// here is how it stays invisible.
	fmt.Fprintf(opts.Out, "Signing in to %s  (profile %s, from %s)\n", opts.Profile.Issuer, opts.Profile.Name, opts.ProfileFrom)
	fmt.Fprintf(opts.Out, "The token will be used on %s\n\n", opts.Profile.PlatformAPI)

	if !opts.NoBrowser {
		if err := browser.Open(authURL); err != nil {
			fmt.Fprintf(opts.Out, "Could not open a browser (%v).\n", err)
		}
	}
	fmt.Fprintf(opts.Out, "Open this URL to continue:\n\n    %s\n\nWaiting for the browser...\n", authURL)
}

// handleCallback answers the redirect and hands the query to the waiting flow.
func handleCallback(results chan<- callback) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callbackPath {
			http.NotFound(w, r)
			return
		}

		q := r.URL.Query()
		got := callback{
			code:  q.Get("code"),
			state: q.Get("state"),
			err:   q.Get("error"),
			desc:  q.Get("error_description"),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if got.err != "" || got.code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, page("Sign-in failed", "Go back to the terminal; it says what happened."))
		} else {
			fmt.Fprint(w, page("Signed in", "You can close this tab and go back to the terminal."))
		}

		// Non-blocking: the channel is buffered for one, and a second redirect
		// (a refresh of the tab) must not block the handler forever.
		select {
		case results <- got:
		default:
		}
	}
}

// page is the one-screen response the browser lands on. It is plain on purpose:
// it is served from a loopback port with no network access, so it can carry no
// stylesheet, and it has one job, which is to say the terminal is where to look
// next.
func page(title, message string) string {
	return "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + title +
		"</title></head><body style=\"font-family:system-ui,sans-serif;margin:4rem auto;max-width:32rem\">" +
		"<h1 style=\"font-size:1.25rem\">" + title + "</h1><p>" + message + "</p></body></html>"
}
