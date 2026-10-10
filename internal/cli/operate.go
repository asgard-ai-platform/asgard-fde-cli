package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// The groups `operate` sorts its own commands into. A new kind goes into the
// one that says what it is for, the same rule the root's groups follow.
const operateGroupResources = "resources"

func newOperateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operate",
		Short: "Run and read back what a release has already deployed",
		Long: `Run and read back the CRs a release has already put on the cluster.

Everything else in this tool ends when the CRs are on the cluster. What is
left after that is not a CR's shape but its behaviour: a Syncer has to run
before the agent behind it has anything to read, a Trigger is tested by firing
it rather than by waiting for its cron, and whether any of them ran is in
their own history rather than in the pipeline run that deployed them.

    asgard-cli operate syncer sync <name> --release <r>         run it now
    asgard-cli operate syncer executions <name> --release <r>   what its runs did
    asgard-cli operate skill-set sync <name> --release <r>      the same, for a SkillSet
    asgard-cli operate trigger fire <name> --release <r>        a Trigger's invocation, now
    asgard-cli operate trigger runs <name> --release <r>        its invocations, and their logs
    asgard-cli operate source-set reindex <name> --release <r>  a context index refresh, now
    asgard-cli operate oauth-credential authorize <name>        the grant a chart cannot make

Only what IaC cannot do is here. A CR's spec, its labels, whether it is
published or suspended - those are the chart's, and changing one here would be
reverted by the next run that deploys it. So there is no command that edits a
CR, and none that lists them: "asgard-cli pipeline manifest" already reads back
what a release has.

WHICH CR. A CR is named by its metadata.name as the cluster has it, which is
the rendered name, and found in one of two ways:

    --release <name>    a release of the pipeline this checkout records; the
                        name is checked against what that release deployed,
                        and a wrong one is answered with the ones it has
    --project <x>       a platform Project by id, name or namespace, for when
                        there is no checkout; the platform answers a wrong
                        name with a 404

One of the two is required; neither is derived. The platform scopes these
routes by project, and the project is not the namespace even where the two
look alike, so the release's own record is what supplies it.

What each command needs from the member's role in the project is in its own
help. A refusal says which permission it was.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddGroup(
		&cobra.Group{ID: operateGroupResources, Title: "Resources - one CR kind each:"},
	)
	addTo(cmd, operateGroupResources,
		newOperateSyncerCmd(),
		newOperateSkillSetCmd(),
		newOperateSourceSetCmd(),
		newOperateTriggerCmd(),
		newOperateOAuthCredentialCmd(),
	)
	return cmd
}

// operateFlags are the ones every `operate` command carries.
type operateFlags struct {
	profile   string
	workspace string
	format    string
	pipeline  string
	release   string
	project   string
}

func (f *operateFlags) register(cmd *cobra.Command) {
	addProfileFlag(cmd, &f.profile)
	cmd.Flags().StringVar(&f.workspace, workspaceFlag, "",
		"workspace to act in; defaults to what \"asgard-cli workspace use\" recorded for this repository")
	cmd.Flags().StringVar(&f.format, formatFlag, formatText, formatUsage)
	cmd.Flags().StringVar(&f.release, "release", "",
		"release that deployed the CR; the name is checked against what it deployed. This or --project is required")
	cmd.Flags().StringVar(&f.pipeline, "pipeline", "",
		"pipeline id or name the --release belongs to; defaults to the one recorded in "+binding.FileName)
	cmd.Flags().StringVar(&f.project, "project", "",
		"platform Project by id, name or namespace, for use outside a checkout. This or --release is required")
}

// operateScope is where a CR is looked for: the project every route is scoped
// by, and - when the CR was found through a release - what that release has
// on the cluster.
type operateScope struct {
	ProjectID string
	// Where says which release or project, for messages.
	Where string
	// release is the release named, or nil when the scope is a bare project.
	release *platform.Release
	live    *platform.LiveManifest
}

// liveCR is the part of a deployed CR these commands read.
type liveCR struct {
	Metadata struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec map[string]any `yaml:"spec"`
}

func (c *liveCR) specString(key string) string {
	s, _ := c.Spec[key].(string)
	return s
}

// resolve finds the session and the scope. It prints nothing; the command
// decides whether what it is about to do is a change.
func (f *operateFlags) resolve(cmd *cobra.Command) (*platformContext, *operateScope, error) {
	if err := checkFormat(f.format); err != nil {
		return nil, nil, err
	}
	switch {
	case f.release != "" && f.project != "":
		return nil, nil, fmt.Errorf("pass --release or --project, not both: a release already names its project")
	case f.release == "" && f.project == "":
		return nil, nil, fmt.Errorf("--release <name> or --project <id|name|namespace> is required; " +
			"`asgard-cli pipeline releases` lists the releases of this checkout's pipeline")
	case f.project != "" && f.pipeline != "":
		return nil, nil, fmt.Errorf("--pipeline only says where to find --release, and has no meaning with --project")
	}
	pc, err := resolveContext(cmd, contextOptions{Profile: f.profile, Workspace: f.workspace, NeedWorkspace: true})
	if err != nil {
		return nil, nil, err
	}
	ctx := cmd.Context()

	if f.project != "" {
		id, label, err := resolveProject(ctx, pc, f.project)
		if err != nil {
			return nil, nil, err
		}
		return pc, &operateScope{ProjectID: id, Where: "project " + label}, nil
	}

	p, err := resolvePipeline(ctx, pc, f.pipeline)
	if err != nil {
		return nil, nil, err
	}
	rel, err := resolveRelease(ctx, pc, p, f.release)
	if err != nil {
		return nil, nil, err
	}
	live, err := pc.Client.GetLiveManifest(ctx, rel.ReleaseId, false)
	if err != nil {
		if platform.NotFound(err) {
			return nil, nil, fmt.Errorf("release %s has never deployed, so it has nothing on the cluster to operate", rel.Name)
		}
		return nil, nil, err
	}
	return pc, &operateScope{
		ProjectID: rel.ProjectId,
		Where:     fmt.Sprintf("release %s (%s)", rel.Name, rel.Namespace),
		release:   rel,
		live:      live,
	}, nil
}

// object finds a CR the release deployed. It returns nil and no error when
// the scope is a bare project, where there is nothing to check against and
// the platform's own 404 is the answer.
func (s *operateScope) object(kind, name string) (*liveCR, error) {
	if s.live == nil {
		return nil, nil
	}
	var same []string
	for _, o := range s.live.Objects {
		if o.Kind != kind {
			continue
		}
		if o.Name != name {
			same = append(same, o.Name)
			continue
		}
		switch {
		case o.Error != "":
			return nil, fmt.Errorf("%s %s in %s could not be read back: %s", kind, name, s.Where, o.Error)
		case !o.Found:
			return nil, fmt.Errorf("%s %s was deployed by %s and has since been deleted outside the pipeline; "+
				"the next run that deploys the release puts it back", kind, name, s.Where)
		}
		var cr liveCR
		if err := yaml.Unmarshal([]byte(o.Yaml), &cr); err != nil {
			return nil, fmt.Errorf("read %s %s: %w", kind, name, err)
		}
		return &cr, nil
	}
	if len(same) == 0 {
		return nil, fmt.Errorf("%s deployed no %s; `asgard-cli pipeline manifest --release %s --summary` lists what it did deploy",
			s.Where, kind, s.release.Name)
	}
	sort.Strings(same)
	return nil, fmt.Errorf("%s deployed no %s named %q. The names are the rendered ones, as the cluster has them; it has: %s",
		s.Where, kind, name, strings.Join(same, ", "))
}

// skillSetFor names the SkillSet in the release whose SourceSet is
// sourceSet, or nothing when there is no release to look in or none matches.
func (s *operateScope) skillSetFor(sourceSet string) string {
	if s.live == nil || sourceSet == "" {
		return ""
	}
	for _, o := range s.live.Objects {
		if o.Kind != "SkillSet" || !o.Found {
			continue
		}
		var cr liveCR
		if yaml.Unmarshal([]byte(o.Yaml), &cr) == nil && cr.specString("sourceSetName") == sourceSet {
			return o.Name
		}
	}
	return ""
}

// scopeFlag repeats the flag that found the scope, for a command a message
// tells somebody to run next.
func (f *operateFlags) scopeFlag() string {
	if f.release != "" {
		flag := "--release " + f.release
		if f.pipeline != "" {
			flag += " --pipeline " + f.pipeline
		}
		return flag
	}
	return "--project " + f.project
}

// waitRun reports what waitForNewRun found.
type waitRun struct {
	Run *platform.SyncerExecution
	// TimedOut is set when the wait ran out first; Run is then the newest
	// new run seen, or nil when none appeared.
	TimedOut bool
}

// waitForNewRun polls a run history until a run that was not in before has
// finished, or ctx ends.
//
// The trigger answers with no run id, so "the run this started" is the run
// that was not there before. A run somebody else started in the same window
// is indistinguishable from it, which the platform makes unavoidable; it is
// also rare, because a Syncer refuses a trigger while a run is going.
func waitForNewRun(ctx context.Context, before []platform.SyncerExecution,
	list func(context.Context) ([]platform.SyncerExecution, error), sleep func(context.Context) error) (*waitRun, error) {
	seen := make(map[string]bool, len(before))
	for _, r := range before {
		seen[r.Name] = true
	}
	var latest *platform.SyncerExecution
	for {
		runs, err := list(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return &waitRun{Run: latest, TimedOut: true}, nil
			}
			return nil, err
		}
		for i := range runs {
			r := runs[i]
			if seen[r.Name] {
				continue
			}
			if latest == nil || startOf(r) >= startOf(*latest) {
				latest = &r
			}
		}
		if latest != nil && latest.Status != platform.SyncerRunRunning {
			return &waitRun{Run: latest}, nil
		}
		if err := sleep(ctx); err != nil {
			return &waitRun{Run: latest, TimedOut: true}, nil
		}
	}
}

func startOf(r platform.SyncerExecution) int64 {
	if r.StartTimestamp == nil {
		return 0
	}
	return *r.StartTimestamp
}
