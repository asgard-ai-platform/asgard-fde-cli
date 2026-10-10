package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
}

const operateManifest = `{"success":true,"data":{"helm_revision":3,"helm_status":"deployed","objects":[
{"kind":"Syncer","name":"kb-docs","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: kb-docs\nspec:\n  sourceSetName: kb\n"},
{"kind":"Syncer","name":"skills-git","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: skills-git\n  labels:\n    asgard-ai.com/managed-by: skill-set\nspec:\n  sourceSetName: skills-ss\n"},
{"kind":"SkillSet","name":"support-skills","namespace":"proj-ns","found":true,"yaml":"metadata:\n  name: support-skills\nspec:\n  sourceSetName: skills-ss\n"},
{"kind":"Syncer","name":"gone","namespace":"proj-ns","found":false}
]}}`

func (f *operateFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	f.runs = map[string][]platform.SyncerExecution{}
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
