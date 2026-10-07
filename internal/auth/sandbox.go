package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/sandbox"
)

// The Workbench assistant runs this CLI inside a sandbox on the platform, as
// the member who is talking to it. There is no browser to sign in with and no
// credential store worth keeping - the sandbox's home is not the member's - so
// the platform hands the identity over itself: every turn, a hook writes the
// member's access token, the workspace they are in and the platform's address
// to one file, and sandbox mode reads it.
//
// The image sets EnvSandboxMode; nothing else turns it on. Outside a sandbox
// the variable is absent and none of this is reached.
const (
	// EnvSandboxMode is set to "true" by the Workbench assistant's sandbox
	// image. Any other value is as if it were absent. Declared in
	// internal/sandbox, which the browser can import.
	EnvSandboxMode = sandbox.EnvMode
	// EnvSessionFile relocates the session file, for testing.
	EnvSessionFile = "ASGARD_SESSION_FILE"
	// DefaultSessionFile is where the sandbox's hook writes the session. /tmp
	// is the sandbox's own scratch space: it is not the member's /work, which
	// persists and which the member browses.
	DefaultSessionFile = "/tmp/.asgard/session.json"
)

// SourceSandbox is a session read from the sandbox's session file.
const SourceSandbox Source = "sandbox"

// FromSandbox is how a sandbox session's profile came to be the one in effect.
const FromSandbox Origin = "the Workbench sandbox's session file"

// SandboxMode reports whether this process runs in the Workbench assistant's
// sandbox.
func SandboxMode() bool { return sandbox.Enabled() }

// SandboxSession is the session file's content.
//
// AccessToken is a secret. Read it only to put it in an Authorization header.
type SandboxSession struct {
	AccessToken string `json:"access_token"`
	WorkspaceID string `json:"workspace_id"`
	PlatformAPI string `json:"platform_api"`
	User        *struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	} `json:"user"`
	// ChannelID is the assistant conversation this sandbox belongs to. Not a
	// credential.
	ChannelID string `json:"channel_id"`
}

// ErrNoSandboxSession is a sandbox with no usable session file: the hook
// writes it at the start of every turn, so it is absent only outside one.
var ErrNoSandboxSession = errors.New(
	"no Workbench session: " + EnvSandboxMode + " is set, but the session file the assistant's sandbox " +
		"writes at the start of every turn is missing or empty. This CLI runs as the member only inside a " +
		"Workbench assistant turn; outside one, set " + EnvToken + " and " + EnvWorkspace)

// SessionFilePath is where the session file is read from.
func SessionFilePath() string {
	if p := os.Getenv(EnvSessionFile); p != "" {
		return p
	}
	return DefaultSessionFile
}

// LoadSandboxSession reads the session file. It is read on every command,
// never cached: the hook rewrites it every turn, and the token in it is
// replaced when the member's own is.
func LoadSandboxSession() (*SandboxSession, error) {
	raw, err := os.ReadFile(SessionFilePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoSandboxSession
	}
	if err != nil {
		return nil, fmt.Errorf("read the Workbench session: %w", err)
	}
	var s SandboxSession
	if err := json.Unmarshal(raw, &s); err != nil {
		// The content is never quoted: it holds a token.
		return nil, fmt.Errorf("the Workbench session file %s is not valid JSON", SessionFilePath())
	}
	if s.AccessToken == "" {
		return nil, ErrNoSandboxSession
	}
	s.PlatformAPI = strings.TrimRight(s.PlatformAPI, "/")
	return &s, nil
}

// sandboxSession turns the session file into a Session on top of the resolved
// profile. The platform API is the file's - the hook names the platform the
// member is on, which a profile in a fresh sandbox cannot know - unless
// EnvPlatformAPI overrides it, as it overrides everything else.
func sandboxSession(r Resolved) (*Session, error) {
	s, err := LoadSandboxSession()
	if err != nil {
		return nil, err
	}
	p := r.Profile
	if s.PlatformAPI != "" && r.APIFrom != FromEnv {
		p.PlatformAPI = s.PlatformAPI
	}
	session := &Session{
		Profile:          p,
		ProfileFrom:      FromSandbox,
		Token:            s.AccessToken,
		Source:           SourceSandbox,
		SandboxWorkspace: s.WorkspaceID,
	}
	if s.User != nil {
		session.Subject, session.Email, session.Name = s.User.ID, s.User.Email, s.User.DisplayName
	}
	return session, nil
}
