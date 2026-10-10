package gate

import (
	"fmt"
	"sort"
	"strings"
)

// Deployability reports what CD requires of a chart, as opposed to what the
// apiserver requires.
//
// The distinction matters because a chart can be entirely valid - every CR
// applies, every reference resolves, helm lint is green - and still fail the
// deploy. Nothing local was checking these, so the first anyone knew was a red
// tag.
//
// Everything here is a **warning**, and that is the whole design: each condition
// is correct during an onboarding and a problem once someone tags. A project has
// no Syncer until skills are added. Failing on that would leave the gate red
// through the entire middle of the work, which trains people to ignore it - and
// saying nothing means finding out from a run.
func Deployability(docs []Doc, opts Options) Result {
	if len(docs) == 0 {
		return Result{Summary: "nothing rendered yet"}
	}

	ix := newIndex(docs)

	var warnings []string
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	syncers := ix.countsByKind["Syncer"]

	// The rollout's apply step fires the Syncers of this release that opt in with
	// asgard-ai.com/auto-fire-on-rollout and waits for them, so zero Syncers
	// means nothing after the dry run proves the platform accepted any of it.
	// (This was once a step in the repository's own CD workflow, which polled for
	// CronJobs labelled asgard-ai.com/syncer-name. A repository this tool writes
	// has no such workflow, and no cluster credential to run one with.)
	//
	// A Syncer comes from a SkillSet (the skill set, its own SourceSet and the
	// Syncer that feeds it are generated together) or from a knowledge drive. So
	// this is not obscure: a chart with a read path, an entry point and no
	// skills is exactly the shape a fresh one has.
	if syncers == 0 {
		warnf("no Syncer. The rollout's apply step fires the Syncers this release deploys that carry "+
			"asgard-ai.com/auto-fire-on-rollout and waits for them, so with none there is nothing after "+
			"the dry run that proves the platform accepted any of it - a succeeded run means helm returned. "+
			"`asgard-cli add skillset base --project %s --repo <git url>` creates one, as does "+
			"`asgard-cli add knowledgedrive <name> --project %s`. When the shape genuinely has "+
			"none - a DataConnector and a SemanticLayer is one - the read-back is "+
			"`asgard-cli pipeline manifest --release <name> --status`, which says what is on the "+
			"cluster and which of the kinds there can report anything at all",
			opts.ProjectOr(), opts.ProjectOr())
	}

	// A suspended Syncer that never opts in to auto-fire NEVER RUNS on its own. The two
	// labels read nothing of each other: syncer-suspend stops the scheduler, and
	// the runner fires only what carries auto-fire-on-rollout. Every other check
	// passes - the chart renders, the apiserver accepts the CR, the run succeeds
	// - and the skills or the drive resolve to nothing.
	//
	// Only the suspended ones are reported. A Syncer left on its schedule and not
	// opted in still runs; it just does not run at deploy time, and that is a
	// choice rather than a mistake.
	var unfired []string
	for _, d := range docs {
		if d.Kind != "Syncer" {
			continue
		}
		if d.Labels[annotationPrefix+"auto-fire-on-rollout"] == "true" {
			continue
		}
		if d.Labels[annotationPrefix+"syncer-suspend"] == "true" {
			unfired = append(unfired, d.Name)
		}
	}
	if len(unfired) > 0 {
		sort.Strings(unfired)
		warnf("%s suspended with no asgard-ai.com/auto-fire-on-rollout, so nothing runs them on their own. "+
			"syncer-suspend stops the scheduler, the rollout fires only the Syncers that opt in, and neither "+
			"label reads the other - so the CR applies, the run succeeds, and the skills resolve to zero. "+
			"Add the label, or clear syncer-suspend and let the schedule run it",
			strings.Join(unfired, ", "))
	}

	// The project environment id is injected as .Values.asgard.projectEnvironmentId
	// on every run, so a CR without the label is a chart that does not read it
	// rather than a value nobody has fetched. When the value is absent the chart
	// deliberately renders no label at all, which is why this is invisible to
	// every other check - and a local render supplies a placeholder, so what this
	// catches is a template that never mentions it.
	var unlabelled []string
	for _, d := range docs {
		switch d.Kind {
		case "Workflow", "Trigger", "CompletionModel":
			if d.Labels[annotationPrefix+"project-environment-id"] == "" {
				unlabelled = append(unlabelled, d.Kind+"/"+d.Name)
			}
		}
	}
	if len(unlabelled) > 0 {
		sort.Strings(unlabelled)
		warnf("%s render without a project-environment-id label. "+
			"On a cluster they work - a Trigger fires on schedule - while their editors open as a blank canvas, which is why this is worth clearing before anyone looks. "+
			"The platform injects the id as .Values.asgard.projectEnvironmentId on every run, so the fix is in the template rather than in a value: "+
			"read it there the way the other CRs of this kind do",
			strings.Join(unlabelled, ", "))
	}

	summary := fmt.Sprintf("%d syncer(s)", syncers)
	if syncers > 0 && len(unfired) == 0 {
		// Every Syncer either keeps a schedule or opts in to the rollout, so
		// each of them runs. That is worth saying: the failure this replaces
		// was silent, and an absent warning reads the same as an absent check.
		summary += ", each will run"
	}
	if syncers > 0 && len(unlabelled) == 0 {
		summary += ", environment id set"
	}
	return Result{Warnings: warnings, Summary: summary}
}
