package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/extension"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/skills"
)

// gateExtensions is the gate's extensions step: whether the Extensions this
// repository records are what is on disk, and whether a newer version of one
// is out.
//
// Everything it finds warns and nothing fails. An edited Extension file can be
// on purpose, and a newer version is the author's news, not a defect here; what
// matters is that the agent reading the material knows.
//
// The disk half needs nothing but the repository, so it always runs. The
// newer-version half needs a session, and like the skills step it is skipped
// with --offline or without one.
func gateExtensions(cmd *cobra.Command, root, profile string, offline bool) stepResult {
	res := stepResult{Name: "extensions"}
	lock, err := extension.ReadLock(root)
	if err != nil {
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}
	states, unrecorded, err := extension.Inspect(skills.Root(root, ""), lock)
	if err != nil {
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}
	if len(states) == 0 && len(unrecorded) == 0 {
		res.Status = stepPass
		res.Summary = "no Extension here"
		return res
	}

	var problems []string
	for _, s := range states {
		for _, line := range stateProblems(root, s) {
			problems = append(problems, s.ID+": "+line)
		}
	}
	for _, d := range unrecorded {
		problems = append(problems, fmt.Sprintf("%s: not in %s; an Extension committed here needs an entry (source: local), and a skill of this repository's own must not use the %s prefix",
			d, extension.LockName, extension.Prefix))
	}

	var behind []string
	latest, asked := latestVersions(cmd, profile, offline, states)
	for _, s := range states {
		if v, ok := latest[s.Key]; ok && extension.Newer(v, s.Entry.Version) {
			behind = append(behind, fmt.Sprintf("%s: at %s, and %s is out", s.ID, s.Entry.Version, v))
		}
	}

	res.Details = append(problems, behind...)
	switch {
	case len(problems) > 0:
		res.Status = stepWarn
		res.Summary = fmt.Sprintf("%d Extension file(s) or directory(s) are not what %s records", len(problems), extension.LockName)
		res.Remedy = "asgard-cli extension status"
	case len(behind) > 0:
		res.Status = stepWarn
		res.Summary = fmt.Sprintf("%d Extension(s) have a newer version; read its CHANGELOG.md before updating", len(behind))
		res.Remedy = "asgard-cli extension update <id>"
	default:
		res.Status = stepPass
		res.Summary = fmt.Sprintf("%d Extension(s), each as recorded", len(states))
	}
	// A skipped half is said, not left out: "as recorded" with no word on the
	// versions would read as "and current".
	if asked != "" {
		res.Summary += "; latest versions " + asked
	}
	return res
}
