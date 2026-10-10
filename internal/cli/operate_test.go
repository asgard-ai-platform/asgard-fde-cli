package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// operateFake is a platform with one pipeline, one release in a project whose
// id and namespace differ, and a release that deployed a drive Syncer, a
// SkillSet with its own Syncer, and a Syncer deleted out of band.
type operateFake struct {
	mu       sync.Mutex
	runs     map[string][]platform.SyncerExecution
	triggers []string // "path project-header"
	// after is what a trigger appends to that path's history.
	after platform.SyncerExecution
	// busy makes a trigger answer 409, as a Syncer with a run going does.
	busy bool
	// invocations is each invocation list path's rows, newest first; a fire
	// or a reindex puts nextInvocation in front.
	invocations    map[string][]platform.Invocation
	nextInvocation platform.Invocation
	// queries and logPaths record what was asked for.
	queries, logPaths []string
	// oauth is each read of the credential in turn; the last one repeats.
	oauth      []platform.OAuthCredentialStatus
	oauthReads int
	authorizes int
	// volume records every volume call as "METHOD path?query", and uploads
	// what each PUT carried in its file field.
	volume  []string
	uploads []string
	// chatStream is what a chat send answers; channelExists makes the
	// metadata route answer 200; sent records each send's body.
	chatStream    string
	channelExists bool
	sent          []string
}

const operateManifest = `{"success":true,"data":{"helm_revision":3,"helm_status":"deployed","objects":[
{"kind":"Syncer","name":"kb-docs","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: kb-docs\nspec:\n  sourceSetName: kb\n"},
{"kind":"Syncer","name":"skills-git","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: skills-git\n  labels:\n    asgard-ai.com/managed-by: skill-set\nspec:\n  sourceSetName: skills-ss\n"},
{"kind":"SkillSet","name":"support-skills","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: support-skills\nspec:\n  sourceSetName: skills-ss\n"},
{"kind":"Syncer","name":"gone","namespace":"proj-ns","found":false},
{"kind":"Trigger","name":"daily-report","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: daily-report\nspec: {}\n"},
{"kind":"SourceSet","name":"kb","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: kb\nspec:\n  contextIndex:\n    prompt: x\n"},
{"kind":"SourceSet","name":"plain","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: plain\nspec: {}\n"},
{"kind":"OAuthCredential","name":"gdrive","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: gdrive\nspec: {}\n"},
{"kind":"Agent","name":"support","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: support\nspec: {}\n"}
]}}`

func (f *operateFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	f.runs = map[string][]platform.SyncerExecution{}
	f.invocations = map[string][]platform.Invocation{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		switch {
		case p == "/v1/iac/pipelines":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"pipeline_id":"p-1","name":"app"}]}`)
		case p == "/v1/iac/pipelines/p-1/releases":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"release_id":"r-1","name":"dev","project_id":"proj-uuid","namespace":"proj-ns"}]}`)
		case p == "/v1/iac/releases/r-1/manifest":
			_, _ = io.WriteString(w, operateManifest)
		case p == "/v1/project":
			_, _ = io.WriteString(w, `{"success":true,"data":[{"project_id":"proj-uuid","project_name":"App","k8s_namespace_name":"proj-ns"}]}`)
		case p == "/v1/syncer/kb-docs":
			_, _ = io.WriteString(w, `{"success":true,"data":{"syncer_id":"kb-docs","source_set_id":"kb"}}`)
		case p == "/v1/syncer/skills-git":
			_, _ = io.WriteString(w, `{"success":true,"data":{"syncer_id":"skills-git","source_set_id":"skills-ss","resource_tags":{"managed-by":"skill-set"}}}`)
		case p == "/v1/auth/me":
			_, _ = io.WriteString(w, `{"success":true,"data":{"user_id":"u"}}`)
		case p == "/v1/agent/support":
			_, _ = io.WriteString(w, `{"success":true,"data":{"managed":{"alias_name":"support-bot"}}}`)
		case strings.HasSuffix(p, "/chat/channel/metadata"):
			if !f.channelExists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"success":false,"message":"channel not found"}`)
				return
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"runState":"IDLE"}}`)
		case strings.HasSuffix(p, "/chat/message/sse") && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			f.sent = append(f.sent, string(body))
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, f.chatStream)
		case strings.Contains(p, "/volume/"):
			f.volume = append(f.volume, r.Method+" "+p+"?"+r.URL.RawQuery+" "+r.Header.Get(platform.ProjectHeader))
			switch {
			case strings.HasSuffix(p, "/list"):
				_, _ = io.WriteString(w, `{"success":true,"data":{"entries":[{"name":"b.md","sizeBytes":3},{"name":"z","isDir":true},{"name":"a.md","sizeBytes":1}],"paging":{"total":3}}}`)
			case strings.HasSuffix(p, "/file") && r.Method == http.MethodPut:
				file, _, err := r.FormFile("file")
				if err == nil {
					body, _ := io.ReadAll(file)
					f.uploads = append(f.uploads, string(body))
				}
				_, _ = io.WriteString(w, `{"success":true,"data":{"bytesWritten":5}}`)
			case strings.HasSuffix(p, "/move"):
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"success":false,"message":"the destination already exists"}`)
			default:
				_, _ = io.WriteString(w, `{"success":true,"message":"ok"}`)
			}
		case p == "/v1/integration/oauth/credentials/gdrive/authorize" && r.Method == http.MethodPost:
			f.authorizes++
			_, _ = io.WriteString(w, `{"success":true,"data":{"authorization_url":"https://edge.example/ns/proj-ns/oauth-credential/gdrive/authorize?flow_token=t1"}}`)
		case p == "/v1/integration/oauth/credentials/gdrive":
			st := f.oauth[min(f.oauthReads, len(f.oauth)-1)]
			f.oauthReads++
			body, _ := json.Marshal(platform.OAuthCredential{ID: "gdrive", ProviderID: "preset-google-drive-syncer", Status: st})
			_, _ = io.WriteString(w, `{"success":true,"data":`+string(body)+`}`)
		case (strings.HasSuffix(p, "/force")) && r.Method == http.MethodPost:
			f.triggers = append(f.triggers, p+" "+r.Header.Get(platform.ProjectHeader))
			list := strings.TrimSuffix(p, "/force") + "/invocations"
			f.invocations[list] = append([]platform.Invocation{f.nextInvocation}, f.invocations[list]...)
			_, _ = io.WriteString(w, `{"success":true,"message":"started"}`)
		case strings.HasSuffix(p, "/logs"):
			f.logPaths = append(f.logPaths, p)
			_, _ = io.WriteString(w, `{"success":true,"data":[{"entry":{"timestamp":"2026-10-10T08:00:00Z","level":"info","message":"first\n\nsecond"},"total":1}]}`)
		case strings.HasSuffix(p, "/invocations"):
			f.queries = append(f.queries, r.URL.RawQuery)
			body, _ := json.Marshal(f.invocations[p])
			_, _ = io.WriteString(w, `{"success":true,"data":`+string(body)+`,"paging":{"total":`+strconv.Itoa(len(f.invocations[p]))+`}}`)
		case strings.HasSuffix(p, "/trigger") && r.Method == http.MethodPost && f.busy:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"success":false,"message":"Syncer job still running"}`)
		case strings.HasSuffix(p, "/trigger") && r.Method == http.MethodPost:
			f.triggers = append(f.triggers, p+" "+r.Header.Get(platform.ProjectHeader))
			hist := strings.TrimSuffix(p, "/trigger") + "/executions"
			f.runs[hist] = append(f.runs[hist], f.after)
			_, _ = io.WriteString(w, `{"success":true,"message":"triggered"}`)
		case strings.HasSuffix(p, "/executions"):
			body, _ := json.Marshal(f.runs[p])
			_, _ = io.WriteString(w, `{"success":true,"data":`+string(body)+`}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"message":"not here"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func i64(v int64) *int64 { return &v }

// The project header carries the release's project id, never its namespace,
// and the drive comes from the live Syncer's spec.
func TestOperateSyncerSyncThroughARelease(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, errOut, err := runCLI(t, "", "operate", "syncer", "sync", "kb-docs", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(f.triggers) != 1 || f.triggers[0] != "/v1/source-set/kb/syncer/kb-docs/trigger proj-uuid" {
		t.Errorf("triggers %q", f.triggers)
	}
	if !strings.Contains(out, "Started a run of Syncer kb-docs (drive kb)") ||
		!strings.Contains(out, "operate syncer executions kb-docs --release dev --pipeline app") {
		t.Errorf("out %q", out)
	}
	if !strings.Contains(errOut, "acting on ") {
		t.Errorf("no acting-on line before a change: %q", errOut)
	}
}

func TestOperateSyncerThroughAProject(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	if _, _, err := runCLI(t, "", "operate", "syncer", "sync", "kb-docs", "--project", "proj-ns"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(f.triggers) != 1 || f.triggers[0] != "/v1/source-set/kb/syncer/kb-docs/trigger proj-uuid" {
		t.Errorf("triggers %q", f.triggers)
	}
}

// A name the release did not deploy is answered with the ones it did, and
// nothing is sent.
func TestOperateSyncerWrongNameListsTheRelease(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "syncer", "sync", "kb", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "gone, kb-docs, skills-git") {
		t.Fatalf("wrong name: %v", err)
	}
	_, _, err = runCLI(t, "", "operate", "syncer", "sync", "gone", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "deleted outside the pipeline") {
		t.Fatalf("deleted syncer: %v", err)
	}
	if len(f.triggers) != 0 {
		t.Errorf("triggered %q", f.triggers)
	}
}

// A SkillSet's Syncer is sent to the SkillSet, named, from either scope.
func TestOperateSyncerOwnedByASkillSet(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "syncer", "sync", "skills-git", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "asgard-cli operate skill-set sync support-skills --release dev --pipeline app") {
		t.Fatalf("release scope: %v", err)
	}
	_, _, err = runCLI(t, "", "operate", "syncer", "executions", "skills-git", "--project", "proj-uuid")
	if err == nil || !strings.Contains(err.Error(), "operate skill-set executions <skill-set> --project proj-uuid") {
		t.Fatalf("project scope: %v", err)
	}
	if len(f.triggers) != 0 {
		t.Errorf("triggered %q", f.triggers)
	}
}

func TestOperateSkillSetSyncWaitsForTheNewRun(t *testing.T) {
	f := &operateFake{after: platform.SyncerExecution{Name: "skills-git-2", Status: platform.SyncerRunSucceeded,
		StartTimestamp: i64(100), CompletionTimestamp: i64(142)}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())
	f.runs["/v1/skill-set/support-skills/from-git/executions"] = []platform.SyncerExecution{
		{Name: "skills-git-1", Status: platform.SyncerRunFailed, StartTimestamp: i64(10), CompletionTimestamp: i64(20)}}

	out, _, err := runCLI(t, "", "operate", "skill-set", "sync", "support-skills", "--pipeline", "app", "--release", "dev", "--wait", "30s")
	if err != nil {
		t.Fatalf("sync --wait: %v", err)
	}
	if !strings.Contains(out, "Run skills-git-2 of SkillSet support-skills Succeeded in 42s") {
		t.Errorf("out %q", out)
	}
	if f.triggers[0] != "/v1/skill-set/support-skills/from-git/trigger proj-uuid" {
		t.Errorf("triggers %q", f.triggers)
	}
}

// A failed run fails the command, in json as well as in text.
func TestOperateSyncFailedRunFails(t *testing.T) {
	f := &operateFake{after: platform.SyncerExecution{Name: "kb-docs-9", Status: platform.SyncerRunFailed,
		StartTimestamp: i64(100), CompletionTimestamp: i64(105)}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "syncer", "sync", "kb-docs", "--project", "App", "--wait", "30s", "--format", "json")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("want ErrSilent, got %v", err)
	}
	var res syncResult
	if json.Unmarshal([]byte(out), &res) != nil || res.Run == nil || res.Run.Name != "kb-docs-9" {
		t.Errorf("out %q", out)
	}
}

func TestOperateNeedsAScope(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	for _, args := range [][]string{
		{"operate", "syncer", "executions", "kb-docs"},
		{"operate", "syncer", "executions", "kb-docs", "--release", "dev", "--project", "proj-ns"},
		{"operate", "syncer", "executions", "kb-docs", "--project", "proj-ns", "--pipeline", "app"},
	} {
		if _, _, err := runCLI(t, "", args...); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}

func TestOperateExecutionsNewestFirst(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())
	f.runs["/v1/source-set/kb/syncer/kb-docs/executions"] = []platform.SyncerExecution{
		{Name: "kb-docs-1", Status: platform.SyncerRunSucceeded, StartTimestamp: i64(10), CompletionTimestamp: i64(20)},
		{Name: "kb-docs-2", Status: platform.SyncerRunRunning, StartTimestamp: i64(30)},
	}
	out, _, err := runCLI(t, "", "operate", "syncer", "executions", "kb-docs", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(out, "kb-docs-2") > strings.Index(out, "kb-docs-1") {
		t.Errorf("not newest first: %q", out)
	}
}

// The wait ignores runs that were there before, and gives up when ctx does.
func TestWaitForNewRun(t *testing.T) {
	before := []platform.SyncerExecution{{Name: "a", Status: platform.SyncerRunRunning}}
	calls := 0
	list := func(context.Context) ([]platform.SyncerExecution, error) {
		calls++
		runs := append([]platform.SyncerExecution(nil), before...)
		if calls >= 2 {
			runs = append(runs, platform.SyncerExecution{Name: "b", Status: platform.SyncerRunRunning, StartTimestamp: i64(5)})
		}
		if calls >= 3 {
			runs[1].Status = platform.SyncerRunSucceeded
		}
		return runs, nil
	}
	got, err := waitForNewRun(context.Background(), before, list, func(context.Context) error { return nil })
	if err != nil || got.TimedOut || got.Run.Name != "b" || got.Run.Status != platform.SyncerRunSucceeded {
		t.Fatalf("got %+v %v", got, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = waitForNewRun(ctx, before, func(context.Context) ([]platform.SyncerExecution, error) { return before, nil },
		func(ctx context.Context) error { return ctx.Err() })
	if err != nil || !got.TimedOut || got.Run != nil {
		t.Fatalf("cancelled: %+v %v", got, err)
	}
}

// A trigger refused because a run is going says where to look at that run.
func TestOperateSyncWhileARunIsGoing(t *testing.T) {
	f := &operateFake{busy: true}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "skill-set", "sync", "support-skills", "--project", "App")
	if err == nil || !strings.Contains(err.Error(), "Syncer job still running") ||
		!strings.Contains(err.Error(), "asgard-cli operate skill-set executions support-skills --project App") {
		t.Fatalf("busy: %v", err)
	}
}

func strp(s string) *string { return &s }

// A fire with a wait reports the invocation that was not there before, and
// says when the agent stopped to ask rather than finished.
func TestOperateTriggerFireWaitsForTheNewInvocation(t *testing.T) {
	done := time.Date(2026, 10, 10, 8, 1, 0, 0, time.UTC)
	f := &operateFake{nextInvocation: platform.Invocation{InvocationID: "inv-new", Status: platform.InvocationSucceeded,
		InvokedAt: done.Add(-time.Minute), CompletedAt: &done,
		Channel: &platform.InvocationChannel{RunState: "IDLE", ConversationStatus: platform.ChannelNeedsInput, Title: "Weekly"}}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())
	f.invocations["/v1/trigger/daily-report/invocations"] = []platform.Invocation{
		{InvocationID: "inv-old", Status: platform.InvocationSucceeded, InvokedAt: done.Add(-24 * time.Hour)}}

	out, _, err := runCLI(t, "", "operate", "trigger", "fire", "daily-report", "--pipeline", "app", "--release", "dev", "--wait", "30s")
	if err != nil {
		t.Fatalf("fire --wait: %v", err)
	}
	if !strings.Contains(out, "inv-new") || strings.Contains(out, "inv-old") ||
		!strings.Contains(out, "IDLE NEEDS_INPUT") || !strings.Contains(out, "stopped to ask") {
		t.Errorf("out %q", out)
	}
	if f.triggers[0] != "/v1/trigger/daily-report/force proj-uuid" {
		t.Errorf("triggers %q", f.triggers)
	}
}

func TestOperateTriggerFireFailedNamesTheLog(t *testing.T) {
	f := &operateFake{nextInvocation: platform.Invocation{InvocationID: "inv-bad", Status: platform.InvocationFailed,
		InvokedAt: time.Now(), ErrorMessage: strp("agent not found\nstack")}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "trigger", "fire", "daily-report", "--project", "proj-ns", "--wait", "30s")
	if err == nil || !strings.Contains(err.Error(), "agent not found ...") ||
		!strings.Contains(err.Error(), "asgard-cli operate trigger logs daily-report inv-bad --project proj-ns") {
		t.Fatalf("failed fire: %v", err)
	}
}

func TestOperateReindexNeedsAContextIndex(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "source-set", "reindex", "plain", "--pipeline", "app", "--release", "dev")
	if err == nil || !strings.Contains(err.Error(), "has no contextIndex") || len(f.triggers) != 0 {
		t.Fatalf("plain: %v %q", err, f.triggers)
	}
	if _, _, err := runCLI(t, "", "operate", "source-set", "reindex", "kb", "--pipeline", "app", "--release", "dev"); err != nil {
		t.Fatalf("kb: %v", err)
	}
	if f.triggers[0] != "/v1/source-set/kb/context-index/force proj-uuid" {
		t.Errorf("triggers %q", f.triggers)
	}
}

// A refresh's log is read through the Trigger the platform reports for it.
func TestOperateIndexLogsGoThroughTheDerivedTrigger(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())
	f.invocations["/v1/source-set/kb/context-index/invocations"] = []platform.Invocation{
		{InvocationID: "inv-1", TriggerID: "kb-derived", Status: platform.InvocationSucceeded}}

	out, _, err := runCLI(t, "", "operate", "source-set", "index-logs", "kb", "inv-1", "--project", "proj-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.logPaths) != 1 || f.logPaths[0] != "/v1/trigger/kb-derived/invocations/inv-1/logs" {
		t.Errorf("log paths %q", f.logPaths)
	}
	if !strings.Contains(out, "info  first\n\n    second") {
		t.Errorf("out %q", out)
	}
}

func TestOperateRunsFilters(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	for _, bad := range [][]string{{"--status", "done"}, {"--limit", "101"}, {"--limit", "0"}} {
		args := append([]string{"operate", "trigger", "runs", "daily-report", "--project", "proj-uuid"}, bad...)
		if _, _, err := runCLI(t, "", args...); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
	out, _, err := runCLI(t, "", "operate", "trigger", "runs", "daily-report", "--project", "proj-uuid", "--status", "failed", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 1 || f.queries[0] != "page=0&size=5&status=failed" {
		t.Errorf("queries %q", f.queries)
	}
	if !strings.Contains(out, "no invocations in state failed") {
		t.Errorf("out %q", out)
	}
}

// An invocation whose conversation is still running is not finished, even
// once the trigger's run reports succeeded.
func TestWaitForNewInvocation(t *testing.T) {
	calls := 0
	list := func(context.Context) ([]platform.Invocation, error) {
		calls++
		inv := platform.Invocation{InvocationID: "b", Status: platform.InvocationSucceeded,
			Channel: &platform.InvocationChannel{RunState: platform.ChannelRunning}}
		if calls >= 2 {
			inv.Channel.RunState = "IDLE"
		}
		return []platform.Invocation{inv, {InvocationID: "a", Status: platform.InvocationRunning}}, nil
	}
	got, timedOut, err := waitForNewInvocation(context.Background(), []platform.Invocation{{InvocationID: "a"}}, list,
		func(context.Context) error { return nil })
	if err != nil || timedOut || got.InvocationID != "b" || calls != 2 {
		t.Fatalf("got %+v %v %v after %d calls", got, timedOut, err, calls)
	}
}

// In the sandbox the link is for the member, so it is printed and nothing
// waits, even when --wait is given.
func TestOperateOAuthAuthorizeInTheSandboxHandsTheLinkOver(t *testing.T) {
	f := &operateFake{oauth: []platform.OAuthCredentialStatus{{Phase: platform.OAuthPhasePending}}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-credential", "authorize", "gdrive", "--pipeline", "app", "--release", "dev", "--wait", "5m")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !strings.Contains(out, "Give the member this link") || !strings.Contains(out, "flow_token=t1") ||
		!strings.Contains(out, "It was PENDING before this") ||
		!strings.Contains(out, "operate oauth-credential status gdrive --release dev --pipeline app --wait 10m") {
		t.Errorf("out %q", out)
	}
	if f.authorizes != 1 || f.oauthReads != 1 {
		t.Errorf("authorizes %d, reads %d: it waited", f.authorizes, f.oauthReads)
	}
}

func TestOperateOAuthStatus(t *testing.T) {
	f := &operateFake{oauth: []platform.OAuthCredentialStatus{{Phase: platform.OAuthPhaseExpired, Message: "credential is expired"}}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-credential", "status", "gdrive", "--project", "proj-uuid")
	if err == nil || !strings.Contains(err.Error(), "is EXPIRED") || !strings.Contains(err.Error(), "oauth-credential authorize gdrive --project proj-uuid") {
		t.Fatalf("expired: %v", err)
	}
	if !strings.Contains(out, "credential is expired") {
		t.Errorf("out %q", out)
	}
}

func TestOperateOAuthStatusWaitsForReady(t *testing.T) {
	f := &operateFake{oauth: []platform.OAuthCredentialStatus{{Phase: platform.OAuthPhasePending}, {Phase: platform.OAuthPhaseReady, SyncedSecretVersion: "7"}}}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "oauth-credential", "status", "gdrive", "--project", "proj-uuid", "--wait", "30s", "--format", "json")
	if err != nil {
		t.Fatalf("status --wait: %v", err)
	}
	var c platform.OAuthCredential
	if json.Unmarshal([]byte(out), &c) != nil || c.Status.Phase != platform.OAuthPhaseReady || f.oauthReads != 2 {
		t.Errorf("out %q after %d reads", out, f.oauthReads)
	}
}

// A grant over a credential already READY is new only once the Secret behind
// it changes; a FAILED that was already there is not a new failure.
func TestNewGrant(t *testing.T) {
	cred := func(phase, version, msg string) *platform.OAuthCredential {
		return &platform.OAuthCredential{Status: platform.OAuthCredentialStatus{Phase: phase, SyncedSecretVersion: version, Message: msg}}
	}
	for _, c := range []struct {
		before, after *platform.OAuthCredential
		want          bool
	}{
		{cred("READY", "1", ""), cred("READY", "1", ""), false},
		{cred("READY", "1", ""), cred("PENDING", "1", ""), false},
		{cred("READY", "1", ""), cred("READY", "2", ""), true},
		{cred("EXPIRED", "", ""), cred("READY", "", ""), true},
		{cred("FAILED", "1", "x"), cred("FAILED", "1", "x"), false},
		{cred("FAILED", "1", "x"), cred("FAILED", "1", "y"), true},
		{cred("PENDING", "", ""), cred("FAILED", "", "x"), true},
	} {
		if got := newGrant(c.before, c.after); got != c.want {
			t.Errorf("newGrant(%+v, %+v) = %v", c.before.Status, c.after.Status, got)
		}
	}
}

func TestVolumePath(t *testing.T) {
	for in, want := range map[string]string{"/docs/": "docs", "docs/a.md": "docs/a.md", "a": "a"} {
		if got, err := volumePath(in, false); err != nil || got != want {
			t.Errorf("volumePath(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "/", "a//b", "a/../b", "./a"} {
		if _, err := volumePath(bad, false); err == nil {
			t.Errorf("volumePath(%q) passed", bad)
		}
	}
	if got, err := volumePath("/", true); err != nil || got != "" {
		t.Errorf("root for ls: %q %v", got, err)
	}
}

// A listing goes to the SourceSet's volume with the path cleaned, scoped by
// the release's project, and prints folders first.
func TestOperateVolumeLs(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "operate", "source-set", "ls", "kb", "/docs/", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.volume) != 1 || f.volume[0] != "GET /v1/source-set/kb/volume/list?page=0&page_size=1000&path=docs proj-uuid" {
		t.Errorf("calls %q", f.volume)
	}
	if !(strings.Index(out, "z/") < strings.Index(out, "a.md") && strings.Index(out, "a.md") < strings.Index(out, "b.md")) {
		t.Errorf("not folders first, then by name: %q", out)
	}
	if _, _, err := runCLI(t, "", "operate", "source-set", "ls", "nope", "--pipeline", "app", "--release", "dev"); err == nil ||
		!strings.Contains(err.Error(), "kb, plain") {
		t.Errorf("unknown SourceSet: %v", err)
	}
}

func TestOperateVolumePut(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("faq.md", []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runCLI(t, "", "operate", "skill-set", "put", "support-skills", "faq.md", "skills/faq/SKILL.md",
		"--project", "proj-uuid", "--create-only", "--mode", "644"); err != nil {
		t.Fatal(err)
	}
	if len(f.volume) != 1 || f.volume[0] != "PUT /v1/skill-set/support-skills/volume/file?create_only=true&mode=420&path=skills%2Ffaq%2FSKILL.md proj-uuid" {
		t.Errorf("calls %q", f.volume)
	}
	if len(f.uploads) != 1 || f.uploads[0] != "hello" {
		t.Errorf("uploaded %q", f.uploads)
	}
	if _, _, err := runCLI(t, "", "operate", "skill-set", "put", "support-skills", "faq.md", "x", "--project", "proj-uuid", "--mode", "999"); err == nil {
		t.Errorf("--mode 999 passed")
	}
}

// -r asks first, and with no terminal it needs --yes; nothing is sent until then.
func TestOperateVolumeRmRecursive(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "source-set", "rm", "kb", "old", "-r", "--project", "proj-uuid")
	if err == nil || !strings.Contains(err.Error(), "pass --yes") || len(f.volume) != 0 {
		t.Fatalf("rm -r without --yes: %v %q", err, f.volume)
	}
	if _, _, err := runCLI(t, "", "operate", "source-set", "rm", "kb", "old", "-r", "--yes", "--project", "proj-uuid"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, "", "operate", "source-set", "rm", "kb", "old/a.md", "--project", "proj-uuid"); err != nil {
		t.Fatal(err)
	}
	if len(f.volume) != 2 || !strings.HasPrefix(f.volume[0], "DELETE /v1/source-set/kb/volume/all?path=old ") ||
		!strings.HasPrefix(f.volume[1], "DELETE /v1/source-set/kb/volume/item?path=old%2Fa.md ") {
		t.Errorf("calls %q", f.volume)
	}
}

func TestOperateVolumeMvConflictNamesOverwrite(t *testing.T) {
	f := &operateFake{}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	_, _, err := runCLI(t, "", "operate", "source-set", "mv", "kb", "a.md", "b.md", "--project", "proj-uuid")
	if err == nil || !strings.Contains(err.Error(), "b.md is already there; --overwrite replaces it") {
		t.Fatalf("mv onto existing: %v", err)
	}
}

func sseFrame(id, typ, data string) string {
	return "id: " + id + "\nevent: " + typ + "\ndata: " + data + "\n\n"
}

// The first turn of an Agent preview goes as "@<alias> <text>" on the
// Console's channel for this account, and the reply streams to stdout.
func TestOperateChatSendAgentFirstTurn(t *testing.T) {
	f := &operateFake{chatStream: sseFrame("1", "asgard.run.init", `{}`) +
		sseFrame("2", "asgard.message.delta", `{"fact":{"messageDelta":{"message":{"messageId":"m1","text":"Hel"}}}}`) +
		sseFrame("3", "asgard.message.delta", `{"fact":{"messageDelta":{"message":{"messageId":"m1","text":"lo"}}}}`) +
		sseFrame("4", "asgard.message.complete", `{"fact":{"messageComplete":{"message":{"messageId":"m1","text":"Hello"}}}}`) +
		sseFrame("5", "asgard.run.done", `{}`)}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	out, errOut, err := runCLI(t, "", "operate", "chat", "send", "agent/support", "hi there", "--pipeline", "app", "--release", "dev")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out != "Hello\n" {
		t.Errorf("out %q", out)
	}
	channel := platform.PreviewChannelID(platform.AgentPreviewScope("support", "u"))
	if len(f.sent) != 1 || !strings.Contains(f.sent[0], `"text":"@support-bot hi there"`) || !strings.Contains(f.sent[0], channel) {
		t.Errorf("sent %q", f.sent)
	}
	if !strings.Contains(errOut, "operate chat reset agent/support") {
		t.Errorf("no way to release the sandbox: %q", errOut)
	}

	// A conversation that exists is continued as typed.
	f.channelExists = true
	if _, _, err := runCLI(t, "", "operate", "chat", "send", "agent/support", "again", "--pipeline", "app", "--release", "dev"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.sent[1], `"text":"again"`) {
		t.Errorf("second turn %q", f.sent[1])
	}
}

func TestOperateChatSendReportsHowTheRunEnded(t *testing.T) {
	f := &operateFake{channelExists: true}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())
	args := []string{"operate", "chat", "send", "agent/support", "go", "--project", "proj-uuid"}

	f.chatStream = sseFrame("1", "asgard.run.error",
		`{"fact":{"runError":{"error":{"message":"boom","location":{"workflowName":"wf","processorName":"call-api","processorType":"http-request"}}}}}`)
	if _, _, err := runCLI(t, "", args...); err == nil || !strings.Contains(err.Error(), "workflow wf, processor call-api, http-request") {
		t.Errorf("run error: %v", err)
	}

	f.chatStream = sseFrame("1", "asgard.tool_call.consent",
		`{"fact":{"toolCallConsent":{"pendingCalls":[{"toolCallId":"tc-1","toolsetName":"ts-pos","toolName":"reply_store"}]}}}`) +
		sseFrame("2", "asgard.run.done", `{}`)
	if _, _, err := runCLI(t, "", args...); err == nil || !strings.Contains(err.Error(), "tc-1  ts-pos.reply_store") ||
		!strings.Contains(err.Error(), "--consent tc-1=allow-once") {
		t.Errorf("consent: %v", err)
	}

	// json passes every frame through, one per line, and fails silently.
	out, _, err := runCLI(t, "", append(args, "--format", "json")...)
	if !errors.Is(err, ErrSilent) || len(strings.Split(strings.TrimSpace(out), "\n")) != 2 {
		t.Errorf("json: %v %q", err, out)
	}
}

func TestOperateChatConsentTurn(t *testing.T) {
	f := &operateFake{channelExists: true, chatStream: sseFrame("1", "asgard.run.done", `{}`)}
	sandboxEnv(t, f.server(t).URL)
	t.Chdir(t.TempDir())

	if _, _, err := runCLI(t, "", "operate", "chat", "send", "agent/support", "--consent", "tc-1=deny", "--project", "proj-uuid"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.sent[0], `"action":"RESPONSE_TOOL_CALL_CONSENT"`) || !strings.Contains(f.sent[0], `"result":"DENY_ONCE"`) {
		t.Errorf("sent %q", f.sent)
	}
	for _, bad := range [][]string{
		{"agent/support", "text", "--consent", "tc-1=deny"},
		{"agent/support", "--consent", "tc-1=maybe"},
		{"agent/support"},
		{"trigger/daily-report", "hi"},
		{"agent/support", "hi", "--invocation", "x"},
		{"widget/x", "hi"},
		{"trigger/daily-report", "hi", "--invocation", "x", "--effort", "high"},
	} {
		args := append([]string{"operate", "chat", "send"}, bad...)
		if _, _, err := runCLI(t, "", append(args, "--project", "proj-uuid")...); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
}
