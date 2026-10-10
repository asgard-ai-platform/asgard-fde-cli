// Package platform calls the Asgard platform API.
//
// It is a wrapper and nothing more. The pipeline the CLI drives - lint, render,
// dry run against the cluster's CRDs, plan, apply - runs on the platform, and
// none of it is reimplemented here: a second copy of a rule is a copy that
// disagrees with the server the first time either changes, and the checks worth
// the most (the apiserver's own CEL, pattern and required validation) need a
// cluster the CLI is deliberately never given credentials for.
//
// So what an agent does with this package is push its work and read back what
// the platform made of it. The local half of the loop is the native tools -
// `helm lint`, `helm template` - plus `asgard-cli verify`, which checks the
// things that pass a dry run and still fail at runtime.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// timeout bounds one API call.
//
// It is longer than a page load because some of these calls wait on the
// platform doing real work, and shorter than a run because nothing here polls
// inside a single request - `runs watch` polls by making more calls, so a
// stalled one fails and is retried rather than holding the terminal.
const timeout = 60 * time.Second

// WorkspaceHeader is the header every workspace-scoped route requires.
const WorkspaceHeader = "x-asgard-workspace"

// ClientHeader says which of the platform's clients is calling, and
// ClientName is what this one is.
//
// It is sent on every request rather than on the one that needs it, because it
// is what this program IS rather than something a call decides. What needs it
// today is `pipeline connect`: the GitHub flow ends on a page in a browser
// that the web console's flow ends on too, and that page offers a way back
// INTO the console. Right for somebody who started there; wrong here, where
// the person is waiting at this terminal. The platform seals this into the
// flow and hands it back to the callback, which is the only thing that can
// tell the page which of the two it is talking to.
const (
	ClientHeader = "x-asgard-client"
	ClientName   = "cli"
)

// clientValue is what the header carries: the client, and the version of it.
//
// **The version rides on every call rather than on an endpoint of its own.**
// A client too old to know about a version endpoint never calls one, which is
// exactly the client worth telling - so the fact travels on the requests every
// version already makes. A development build says so in its own version string
// and is left as it is; there is nothing to gain by hiding it from the server
// that is about to behave differently for it.
func clientValue() string {
	if v := version.Get().Version; v != "" {
		return ClientName + "/" + v
	}
	return ClientName
}

// Client is an authenticated caller of one platform.
type Client struct {
	profile   auth.Profile
	token     string
	workspace string
	http      *http.Client
	// assistant marks every request as the Workbench assistant's: a session
	// from the sandbox's session file is by definition the assistant acting
	// for the member, so every call it makes is one, not only the Workbench
	// writes.
	assistant bool
}

// New builds a client from a resolved session. The workspace may be empty for
// the routes that do not take one - listing workspaces is the one that matters,
// because it is how a workspace is chosen in the first place.
func New(session *auth.Session, workspace string) *Client {
	return &Client{
		profile:   session.Profile,
		token:     session.Token,
		workspace: workspace,
		http:      &http.Client{Timeout: timeout},
		assistant: session.Source == auth.SourceSandbox,
	}
}

// Profile reports which platform this client talks to, for messages that have
// to say which one answered.
func (c *Client) Profile() auth.Profile { return c.profile }

// Workspace reports the workspace id every scoped call is made against.
func (c *Client) Workspace() string { return c.workspace }

// APIError is a non-2xx answer from the platform, decoded.
type APIError struct {
	Status     int
	Method     string
	Path       string
	Message    string
	ReasonCode int32
	// ErrorCode is `details.error_code`, which the Workbench routes set to say
	// which of several reasons behind one status this is. Empty elsewhere.
	ErrorCode string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if text, ok := workbenchErrorText(e, msg); ok {
		return text
	}
	if text, ok := operateErrorText(e, msg); ok {
		return text
	}
	switch e.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("the platform rejected the session (%d %s); run `asgard-cli login`", e.Status, msg)
	case http.StatusForbidden:
		if e.ErrorCode == IacAppPermissionNotGranted {
			// Not the member's role: the GitHub App itself lacks the
			// permission, which nobody in the workspace can grant from here.
			return fmt.Sprintf("the GitHub App was not granted what this needs (%d %s): %s. "+
				"An owner of the GitHub organization has to accept the App's requested permissions on its "+
				"installation (GitHub -> the organization's Settings -> GitHub Apps -> the App -> review the "+
				"permission request); nothing in the workspace changes that", e.Status, IacAppPermissionNotGranted, msg)
		}
		// Members may read a pipeline and edit variables; approving, running and
		// deleting need workspace administration. Saying so here saves reading
		// the permission matrix to find out which half a command needed.
		return fmt.Sprintf("not allowed (%d %s); viewing a pipeline and editing variables are open to workspace members, and running, approving, deleting, pushing and creating repositories need workspace administration", e.Status, msg)
	case http.StatusNotFound:
		return fmt.Sprintf("not found (%d %s): %s %s", e.Status, msg, e.Method, e.Path)
	}
	// **A 5xx on a write does not mean the write did not happen.** The platform
	// answered 500 to two of four `pipeline project create` calls and had
	// created both; `pipeline projects` listed all four, each once. Nothing in
	// the error suggested that, and the obvious response to a 500 is to retry -
	// which makes a duplicate platform Project that only the Console can
	// remove, and removing one removes what is deployed into it.
	//
	// So the outcome is reported as unknown rather than as a failure, for the
	// methods where it matters. A GET can be retried freely and says so by
	// omission.
	if e.Status >= 500 && !idempotent(e.Method) {
		return fmt.Sprintf(
			"the platform answered %d: %s\n"+
				"  **this was a %s, so it may have been applied before the error** - the platform has\n"+
				"  answered 500 to a create that had already created. Read the current state back before\n"+
				"  retrying; a second attempt is a second object, not a retry",
			e.Status, msg, e.Method)
	}
	return fmt.Sprintf("the platform answered %d: %s", e.Status, msg)
}

// idempotent reports whether repeating the request is free. A 5xx on one of
// these is safe to retry; on anything else the outcome is unknown.
func idempotent(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "":
		return true
	}
	return false
}

// NotFound reports whether err is a 404, which several callers treat as an
// answer rather than a failure - a release that has never deployed has no live
// manifest, and that is information.
func NotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// Conflict reports whether err is a 409: what the call would create is
// already there.
func Conflict(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}

// BadRequest reports whether err is a 400: the platform understood the call
// and refused what it asked for.
func BadRequest(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest
}

// Unauthorized reports whether err is the platform refusing the session.
func Unauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

// envelope is the platform's response wrapper. Every route answers in it, and
// an error answers in a shape that overlaps it, so one struct reads both.
//
// Paging is kept raw because the platform pages in two shapes: an index and a
// size on most routes, and a cursor on the Workbench ones.
type envelope struct {
	Data       json.RawMessage `json:"data"`
	Paging     json.RawMessage `json:"paging"`
	Message    string          `json:"message"`
	Success    bool            `json:"success"`
	ReasonCode int32           `json:"reason_code"`
	Details    json.RawMessage `json:"details"`
}

// Paging is the platform's page descriptor.
type Paging struct {
	Total int64 `json:"total"`
	Index int64 `json:"index"`
	Size  int64 `json:"size"`
}

// ProjectHeader is the header the project-scoped routes require, alongside the
// workspace one.
const ProjectHeader = "x-asgard-project"

// EnvironmentHeader names a project environment, for the few routes that find
// something by the environment it was made for - a workflow set's preview.
const EnvironmentHeader = "x-asgard-environment"

// request is one call: method, path below the API root, optional query and
// body, and where to put the decoded `data`.
type request struct {
	method string
	// path is appended to the API base, starting with a slash and including the
	// version segment - "/v1/iac/pipelines".
	path  string
	query url.Values
	body  any
	// out receives the `data` field. Nil discards it.
	out any
	// paging receives the `paging` field when the caller wants it.
	paging *Paging
	// cursor receives the `paging` field of a cursor-paged route.
	cursor *CursorPaging
	// viaAssistant marks the request as the Workbench assistant's; see
	// ViaAssistantHeader.
	viaAssistant bool
	// noWorkspace skips the workspace header, for the routes that take none.
	noWorkspace bool
	// project sets the project header, for the routes scoped to one.
	project string
	// environment sets the environment header, for the routes that find
	// something by the environment it was made for.
	environment string
	// sideEffect marks a call that changes something on the platform; a 2xx
	// answer stamps EnvSideEffectFile. Marked per call rather than inferred
	// from the method: some POSTs only read (an audit query, a repository
	// token for git).
	sideEffect bool
}

// do makes one call and unwraps the envelope.
func (c *Client) do(ctx context.Context, req request) error {
	endpoint := c.profile.PlatformAPI + req.path
	if len(req.query) > 0 {
		endpoint += "?" + req.query.Encode()
	}

	var bodyReader io.Reader
	if req.body != nil {
		encoded, err := json.Marshal(req.body)
		if err != nil {
			return fmt.Errorf("encode the request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.method, endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(ClientHeader, clientValue())
	if req.body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if !req.noWorkspace {
		if c.workspace == "" {
			return errors.New("no workspace selected; pass --workspace, or record one with `asgard-cli workspace use <id>`")
		}
		httpReq.Header.Set(WorkspaceHeader, c.workspace)
	}
	if req.project != "" {
		httpReq.Header.Set(ProjectHeader, req.project)
	}
	if req.environment != "" {
		httpReq.Header.Set(EnvironmentHeader, req.environment)
	}
	if req.viaAssistant || c.assistant {
		httpReq.Header.Set(ViaAssistantHeader, "true")
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", req.method, endpoint, err)
	}
	defer resp.Body.Close()

	// Every response carries the platform's reference-material version, and
	// noting it here is what lets a command say "what this repository has is
	// not what this server enforces" without making a call of its own. Read
	// before the status check on purpose: a 403 still answers the question.
	if version := resp.Header.Get(DocsVersionHeader); version != "" {
		lastDocsVersion.Store(version)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read the response to %s %s: %w", req.method, endpoint, err)
	}

	var env envelope
	decodeErr := json.Unmarshal(raw, &env)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{Status: resp.StatusCode, Method: req.method, Path: req.path}
		if decodeErr == nil {
			apiErr.Message = env.Message
			apiErr.ReasonCode = env.ReasonCode
			apiErr.ErrorCode = errorCode(env.Details)
		} else {
			// A body that is not the envelope is a gateway or proxy answering,
			// not the API. Keeping a slice of it is what tells those apart.
			apiErr.Message = truncate(string(raw), 200)
		}
		return apiErr
	}
	if req.sideEffect {
		NoteSideEffect()
	}
	if decodeErr != nil {
		return fmt.Errorf("%s %s answered %s with something that is not the platform's response envelope: %s",
			req.method, endpoint, resp.Status, truncate(string(raw), 200))
	}

	if len(env.Paging) > 0 && string(env.Paging) != "null" {
		var target any
		switch {
		case req.paging != nil:
			target = req.paging
		case req.cursor != nil:
			target = req.cursor
		}
		if target != nil {
			if err := json.Unmarshal(env.Paging, target); err != nil {
				return fmt.Errorf("%s %s answered with paging this build does not understand: %w", req.method, endpoint, err)
			}
		}
	}
	if req.out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, req.out); err != nil {
		return fmt.Errorf("%s %s answered with a body this build does not understand: %w", req.method, endpoint, err)
	}
	return nil
}

// Workspace is one workspace the signed-in user can reach.
type Workspace struct {
	ID   string `json:"workspace_id"`
	Name string `json:"display_name"`
}

// ListWorkspaces returns every workspace the session can see.
//
// It takes no workspace header, which makes it the one call that works before a
// workspace has been chosen - and the check that a session is good for the API
// and not only for Casdoor.
func (c *Client) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	var out []Workspace
	err := c.do(ctx, request{
		method:      http.MethodGet,
		path:        "/v1/workspace",
		out:         &out,
		noWorkspace: true,
	})
	return out, err
}

// errorCode reads `details.error_code`, or nothing. details is free-form, so a
// shape that is not an object is not an error here: it just carries no code.
func errorCode(details json.RawMessage) string {
	var d struct {
		ErrorCode string `json:"error_code"`
	}
	if len(details) == 0 || json.Unmarshal(details, &d) != nil {
		return ""
	}
	return d.ErrorCode
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
