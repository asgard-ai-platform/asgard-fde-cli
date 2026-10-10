package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The calls `asgard-cli operate` makes: running and reading back the CRs a
// release has already put on the cluster.
//
// **Only what IaC cannot do.** A CR's spec, its labels and whether it is
// suspended are the chart's, and the platform's routes that change them are
// deliberately not wrapped here: a change made through them is one the next
// run reverts. What is here acts on a CR without changing it - run it now,
// read what its runs did.
//
// **Every id below is the CR's metadata.name**, and every route is scoped by
// the project header. The project id is not the namespace: the platform reads
// the namespace off the project, so the caller passes the project and never
// the namespace.

// ManagedByLabel marks a CR another CR owns. A SourceSet and a Syncer that
// carry it with the value ManagedBySkillSet belong to a SkillSet, and the
// platform hides them from its drive routes - their sync runs through the
// SkillSet's own routes.
const (
	ManagedByLabel    = "asgard-ai.com/managed-by"
	ManagedBySkillSet = "skill-set"
	// managedByTag is the same label as the platform's resource tags report
	// it, without the domain.
	managedByTag = "managed-by"
)

// SyncerExecution is one run of a Syncer's job, as the Syncer's own status
// records it. The platform keeps no log for one; the job name is all there is
// to go on beyond the outcome.
type SyncerExecution struct {
	// Name is the Kubernetes Job's name.
	Name string `json:"name"`
	// StartTimestamp and CompletionTimestamp are unix seconds. A run still
	// going has no completion.
	StartTimestamp      *int64 `json:"start_timestamp,omitempty"`
	CompletionTimestamp *int64 `json:"completion_timestamp,omitempty"`
	// Status is one of the SyncerRun* values.
	Status string `json:"status"`
}

// The states a Syncer's run reports.
const (
	SyncerRunRunning   = "Running"
	SyncerRunSucceeded = "Succeeded"
	SyncerRunFailed    = "Failed"
)

// Syncer is what this CLI reads of a Syncer the platform holds: which
// SourceSet it fills, and whether something else owns it.
type Syncer struct {
	ID           string            `json:"syncer_id"`
	SourceSetID  string            `json:"source_set_id"`
	ResourceTags map[string]string `json:"resource_tags,omitempty"`
	IsDisabled   bool              `json:"is_disabled"`
}

// ManagedBy reports what owns this Syncer, or nothing.
func (s *Syncer) ManagedBy() string { return s.ResourceTags[managedByTag] }

// GetSyncer reads one Syncer.
//
// It is the top-level route, which the platform keeps for a compatibility
// window, and it is used for one thing: finding the SourceSet a Syncer belongs
// to when no live manifest is at hand to read it from. Triggering and reading
// history go through the drive-scoped routes, which are the ones that stay.
func (c *Client) GetSyncer(ctx context.Context, project, syncer string) (*Syncer, error) {
	var out Syncer
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/syncer/" + url.PathEscape(syncer),
		project: project,
		out:     &out,
	})
	return &out, err
}

// TriggerSyncer starts one run of a Syncer now, outside its schedule.
//
// The answer carries no run id. A run that started shows up in
// SyncerExecutions, which is how a caller finds it.
func (c *Client) TriggerSyncer(ctx context.Context, project, sourceSet, syncer string) error {
	return c.do(ctx, request{
		method:     http.MethodPost,
		path:       syncerPath(sourceSet, syncer) + "/trigger",
		project:    project,
		sideEffect: true,
	})
}

// SyncerExecutions returns the runs a Syncer's status records, in the order
// the platform gives them.
func (c *Client) SyncerExecutions(ctx context.Context, project, sourceSet, syncer string) ([]SyncerExecution, error) {
	var out []SyncerExecution
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    syncerPath(sourceSet, syncer) + "/executions",
		project: project,
		out:     &out,
	})
	return out, err
}

func syncerPath(sourceSet, syncer string) string {
	return "/v1/source-set/" + url.PathEscape(sourceSet) + "/syncer/" + url.PathEscape(syncer)
}

// TriggerSkillSetSync starts one run of the Syncer that fills a SkillSet.
//
// The platform finds that Syncer itself: the one labelled managed-by
// skill-set whose SourceSet is the SkillSet's. A SkillSet with no such Syncer
// is refused.
func (c *Client) TriggerSkillSetSync(ctx context.Context, project, skillSet string) error {
	return c.do(ctx, request{
		method:     http.MethodPost,
		path:       "/v1/skill-set/" + url.PathEscape(skillSet) + "/from-git/trigger",
		project:    project,
		sideEffect: true,
	})
}

// SkillSetSyncExecutions returns the runs of the Syncer that fills a SkillSet.
func (c *Client) SkillSetSyncExecutions(ctx context.Context, project, skillSet string) ([]SyncerExecution, error) {
	var out []SyncerExecution
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/skill-set/" + url.PathEscape(skillSet) + "/from-git/executions",
		project: project,
		out:     &out,
	})
	return out, err
}

// operateErrorText says what a refusal on one of the routes above means. The
// generic 403 is about pipeline permissions, and these are decided by the
// member's role in the project instead.
func operateErrorText(e *APIError, msg string) (string, bool) {
	if e.Status != http.StatusForbidden {
		return "", false
	}
	var needs string
	switch {
	case strings.HasPrefix(e.Path, "/v1/source-set/") && strings.Contains(e.Path, "/volume/"):
		needs = "every route to a SourceSet's files takes source-set/put, reading them included"
	case strings.HasPrefix(e.Path, "/v1/skill-set/") && strings.Contains(e.Path, "/volume/"):
		needs = "every route to a SkillSet's files takes skill-set/put, reading them included"
	case strings.HasPrefix(e.Path, "/v1/source-set/") && strings.Contains(e.Path, "/context-index"):
		needs = "every context-index route takes source-set/put, reading its refreshes included"
	case strings.HasPrefix(e.Path, "/v1/trigger/"):
		needs = "running a Trigger now takes workflow/put, and reading its invocations and their logs takes project-resource/read"
	case strings.HasPrefix(e.Path, "/v1/skill-set/"):
		needs = "running a SkillSet's sync takes skill-set/put, and reading its runs takes project-resource/read"
	case strings.HasPrefix(e.Path, "/v1/source-set/"), strings.HasPrefix(e.Path, "/v1/syncer/"):
		needs = "running a drive's Syncer takes source-set/put, and reading a Syncer or its runs takes project-resource/read"
	default:
		return "", false
	}
	return fmt.Sprintf("not allowed (%d %s); this is the member's role in the project, decided in Asgard Console: %s",
		e.Status, msg, needs), true
}

// Invocation is one run of a Trigger: the moment it fired, and the
// conversation it opened with the agent it addresses. A context index's
// refreshes are invocations of the Trigger the platform derives for it.
//
// Status is the wrong signal on its own. An agent that stops to ask a
// question ends its run as succeeded, the same as one that finished, and only
// Channel.ConversationStatus tells the two apart.
type Invocation struct {
	InvocationID  string             `json:"invocation_id"`
	Namespace     string             `json:"namespace"`
	TriggerID     string             `json:"trigger_id"`
	Status        string             `json:"status"`
	InputPayload  json.RawMessage    `json:"input_payload"`
	OutputPayload json.RawMessage    `json:"output_payload"`
	BlobCount     int32              `json:"blob_count"`
	ErrorMessage  *string            `json:"error_message"`
	InvokedAt     time.Time          `json:"invoked_at"`
	CompletedAt   *time.Time         `json:"completed_at"`
	Channel       *InvocationChannel `json:"channel,omitempty"`
}

// The states an invocation reports.
const (
	InvocationRunning   = "running"
	InvocationSucceeded = "succeeded"
	InvocationFailed    = "failed"
)

// InvocationChannel is the conversation behind an invocation, or nil when it
// was never created or has been reaped.
type InvocationChannel struct {
	CustomChannelID string `json:"custom_channel_id"`
	Title           string `json:"title"`
	// RunState is IDLE, RUNNING or ERROR.
	RunState string `json:"run_state"`
	// ConversationStatus is the agent's own verdict, NEEDS_INPUT or COMPLETED,
	// and empty when it has not given one. Empty is not done.
	ConversationStatus string `json:"conversation_status"`
	// LastActivityAt is unix milliseconds.
	LastActivityAt int64 `json:"last_activity_at"`
}

// The run states and verdicts a conversation reports.
const (
	ChannelRunning    = "RUNNING"
	ChannelNeedsInput = "NEEDS_INPUT"
)

// Finished reports whether an invocation and its conversation have both
// stopped: the trigger's run is over and the agent is not still talking.
func (i *Invocation) Finished() bool {
	if i.Status == InvocationRunning {
		return false
	}
	return i.Channel == nil || i.Channel.RunState != ChannelRunning
}

// InvocationLog is one line of an invocation's log.
type InvocationLog struct {
	Entry *InvocationLogEntry `json:"entry"`
	Total int64               `json:"total"`
}

// InvocationLogEntry is what was logged. SSEEvent is set on the lines that
// record a frame of the conversation.
type InvocationLogEntry struct {
	Timestamp time.Time       `json:"timestamp"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	SSEEvent  json.RawMessage `json:"sse_event,omitempty"`
}

// InvocationQuery narrows a list of invocations. Size is at most 100, which
// the platform enforces.
type InvocationQuery struct {
	// Status is one of the Invocation* values, or empty for all.
	Status string
	Page   int64
	Size   int64
}

func (q InvocationQuery) values() url.Values {
	v := url.Values{}
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	v.Set("page", strconv.FormatInt(q.Page, 10))
	v.Set("size", strconv.FormatInt(q.Size, 10))
	return v
}

// FireTrigger starts one run of a Trigger now, outside its schedule.
//
// The answer carries no invocation id; the new invocation shows up in
// TriggerInvocations, newest first. A Trigger whose last run's Job is still
// going is refused.
func (c *Client) FireTrigger(ctx context.Context, project, trigger string) error {
	return c.do(ctx, request{
		method:     http.MethodPost,
		path:       "/v1/trigger/" + url.PathEscape(trigger) + "/force",
		project:    project,
		sideEffect: true,
	})
}

// TriggerInvocations lists a Trigger's invocations, newest first.
func (c *Client) TriggerInvocations(ctx context.Context, project, trigger string, q InvocationQuery) ([]Invocation, Paging, error) {
	var out []Invocation
	var paging Paging
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/trigger/" + url.PathEscape(trigger) + "/invocations",
		query:   q.values(),
		project: project,
		out:     &out,
		paging:  &paging,
	})
	return out, paging, err
}

// InvocationLogs reads an invocation's log. limit 0 leaves the count to the
// platform.
//
// The route names the Trigger, and the platform reads the invocation by its
// id alone; the Trigger is passed as the invocation reports it, so the path
// stays right if the platform starts checking it.
func (c *Client) InvocationLogs(ctx context.Context, project, trigger, invocation string, limit int64) ([]InvocationLog, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	var out []InvocationLog
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/trigger/" + url.PathEscape(trigger) + "/invocations/" + url.PathEscape(invocation) + "/logs",
		query:   q,
		project: project,
		out:     &out,
	})
	return out, err
}

// ReindexContextIndex starts one refresh of a SourceSet's context index now.
// Like FireTrigger it answers with no invocation id.
func (c *Client) ReindexContextIndex(ctx context.Context, project, sourceSet string) error {
	return c.do(ctx, request{
		method:     http.MethodPost,
		path:       "/v1/source-set/" + url.PathEscape(sourceSet) + "/context-index/force",
		project:    project,
		body:       struct{}{},
		sideEffect: true,
	})
}

// ContextIndexInvocations lists a SourceSet's context-index refreshes,
// newest first. Each carries the derived Trigger's name as TriggerID.
func (c *Client) ContextIndexInvocations(ctx context.Context, project, sourceSet string, q InvocationQuery) ([]Invocation, Paging, error) {
	var out []Invocation
	var paging Paging
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/source-set/" + url.PathEscape(sourceSet) + "/context-index/invocations",
		query:   q.values(),
		project: project,
		out:     &out,
		paging:  &paging,
	})
	return out, paging, err
}

// OAuthCredential is a grant to a third-party service - a Google Drive, a
// OneDrive - that a Syncer or a sandbox reads a token from. The CR declares
// which provider; the token exists only once a person has signed in to the
// service and consented, which no chart can do.
type OAuthCredential struct {
	ProjectID    string                       `json:"project_id"`
	ID           string                       `json:"oauth_credential_id"`
	ProviderID   string                       `json:"provider_id"`
	Name         string                       `json:"credential_name"`
	RefreshToken OAuthCredentialRefreshPolicy `json:"refresh_token"`
	Status       OAuthCredentialStatus        `json:"status"`
}

// OAuthCredentialRefreshPolicy is whether the platform renews the token
// before it expires, and how far ahead.
type OAuthCredentialRefreshPolicy struct {
	AheadSeconds int32 `json:"ahead_seconds"`
	AutoRefresh  bool  `json:"auto_refresh"`
}

// OAuthCredentialStatus is what the reconciler last made of the credential.
type OAuthCredentialStatus struct {
	// Phase is one of the OAuthPhase* values.
	Phase         string `json:"phase"`
	Message       string `json:"message,omitempty"`
	LastUpdatedAt string `json:"last_updated_at"`
	ExpiresAt     string `json:"expires_at"`
	// SyncedSecretVersion is the version of the Secret holding the token that
	// the phase was decided from. A new grant writes a new Secret, so this
	// changes on every grant, including one over a credential already READY.
	SyncedSecretVersion string `json:"synced_secret_version"`
}

// The phases an OAuthCredential reports. PENDING is also where a credential
// sits that nobody has granted yet.
const (
	OAuthPhasePending    = "PENDING"
	OAuthPhaseReady      = "READY"
	OAuthPhaseRefreshing = "REFRESHING"
	OAuthPhaseFailed     = "FAILED"
	OAuthPhaseExpired    = "EXPIRED"
)

// GetOAuthCredential reads one credential. The id is the CR's name.
func (c *Client) GetOAuthCredential(ctx context.Context, project, credential string) (*OAuthCredential, error) {
	var out OAuthCredential
	err := c.do(ctx, request{
		method:  http.MethodGet,
		path:    "/v1/integration/oauth/credentials/" + url.PathEscape(credential),
		project: project,
		out:     &out,
	})
	return &out, err
}

// AuthorizeOAuthCredential starts a grant and returns the URL a person opens
// to complete it. The URL is on the platform's edge server, works once, and
// expires a quarter of an hour after it is issued; nothing in it is tied to
// the session that asked for it.
//
// It is not marked as a side effect: it changes nothing until somebody
// completes the grant in a browser, and the caller that sees that happen
// notes it then.
func (c *Client) AuthorizeOAuthCredential(ctx context.Context, project, credential string) (string, error) {
	var out struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	err := c.do(ctx, request{
		method:  http.MethodPost,
		path:    "/v1/integration/oauth/credentials/" + url.PathEscape(credential) + "/authorize",
		project: project,
		body:    struct{}{},
		out:     &out,
	})
	if err == nil && out.AuthorizationURL == "" {
		return "", fmt.Errorf("the platform started a grant for %s and returned no URL to complete it at", credential)
	}
	return out.AuthorizationURL, err
}
