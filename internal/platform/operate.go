package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
