package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/render"
)

func newVerifyCmd() *cobra.Command {
	var (
		rendered string
		tools    bool
		format   string
	)

	cmd := &cobra.Command{
		Use:   "verify [release ...]",
		Short: "Render each project and check the invariants a render cannot see",
		Long: `Render each project and check the invariants that helm lint, CRD validation and a
server-side dry run all pass:

  - every CR that the Platform UI lists carries its <kind>-name annotation,
    without which it applies cleanly and appears with no name
  - every reference between CRs resolves, both halves of it: an entrypoint is
    (workflow, entry), and a wrong entry is as dead as a wrong workflow
  - inside a SemanticLayer, every name resolves: each join's cube is one of
    the layer's cubes[].name, each join dimension is declared on that cube,
    and each primaryKeyDimensions entry is a dimension of its own cube. The
    CRD checks only that a join's two dimension lists are the same length
  - a Workflow has its full set of workflow-set labels, and each set has exactly
    one main; a Trigger's workflow-set-id matches its entrypoint Workflow's, or
    its editor opens blank. The other label an editor needs,
    project-environment-id, is a warning rather than a failure - the platform
    injects that value on every run, so a CR without the label is a template
    that does not read it
  - a Syncer's paths obey the CRD's relative-path rules
  - the CRDs' conditional CEL rules that a render can be held against: exactly
    one of a set of sibling fields (a credential that is neither a literal nor a
    reference, or both; a class block missing or doubled), and a discriminator
    that implies its block. 27 of the 69 ` + "`" + `XValidation` + "`" + ` markers are
    ` + "`" + `self == oldSelf` + "`" + ` and cannot be seen in a render; these are the rest. A
    marker is not a rule: one on a struct several kinds embed is emitted into
    each of their CRDs, which is why the enforced count is far higher
  - the agent split: every Agent has some source of capability, no Agent lists
    the same semantic layer twice, no allowedCubes, and sampleQuestions on
    anything published. Two things it does not check: how
    many layers one Agent mounts, and whether prompt.task and prompt.format are
    identical across the Agents of one render. Both are shapes the CRD and the
    platform accept, both have a chart set that means them, and a rule cannot
    tell either from a mistake -
    ".agents/skills/asgard-platform/usecase/agent-hub.md" argues for the shape
    each one departs from
  - every credential reference reads a key ` + "`.asgard-pipeline.yaml`" + ` declares,
    and every declared key is read by something. Setting a value on the
    platform without declaring the key is stored and never injected -
    ` + "`variables list`" + ` marks it ORPHAN and the run reports ` + "`vars/orphan`" + `. The
    reverse produces no other signal: a declared key with no reader is created
    and injected on every run into a Secret no CR names, ` + "`variables list`" + `
    shows a value, and every other check passes. A shape change leaves this
    behind when whatever read the key is replaced. Both are warnings, and both
    need a release name, because the declaration is per release
  - the generator's own TODOs, still in the render. A warning, never a
    failure, because a chart carries them through most of an onboarding.
    This is the last check between ` + "`asgard-cli add`" + ` and a tag that reports
    them: helm renders the word, the apiserver accepts it, the
    run succeeds, and a published Agent shows "TODO" to the customer as its
    sample questions

These are the checks a server-side dry run passes and runtime still fails: a
reference to a CR that does not exist, an entry name nothing declares, a
Workflow with no set labels, a missing display annotation. Each applies cleanly
and then breaks at run time or renders a blank page in the platform UI, which is
why they need a gate of their own rather than being left to the plan.

With no arguments it does every release the declaration names. It renders in
process, so there is no pipeline and no temporary file:

    asgard-cli verify
    asgard-cli verify internal-dev
    asgard-cli verify --rendered .out/rendered.yaml
    asgard-cli verify --tools               every tool description, side by side

--tools prints and checks nothing. ` + "`tooling.description`" + ` is the field
where a wrong value makes a model call the wrong tool. A description is wrong
when it does not distinguish its tool from the one beside it, which depends on
the whole set, so no rule can check it; a person reading all of them at once
can. Read them as the model does: in one list, with no other context, deciding
which one answers the question.

This is the ` + "`verify`" + ` step of ` + "`asgard-cli gate`" + `, which renders first and
then runs these. It needs only helm on PATH.
Exits non-zero on any problem.

What this cannot see needs an apiserver, and no client is given one: whether
the CRDs accept each object, and whether a field they do not declare is being
silently dropped. That is the platform's plan, and it is the authority.

--format json emits one record per render, with each check named and its
problems and warnings separate. Agents should use it: in text, a warning and a
failure differ only by one word at the left margin.

Run ` + "`gate`" + ` after changing anything; run this one alone while you are
fixing a single finding and do not want the steps in front of it each
time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := checkFormat(format); err != nil {
				return err
			}
			if format == formatJSON && tools {
				return fmt.Errorf("--tools prints tool names for a person to read; it has no --%s json form", formatFlag)
			}
			report := verifyJSON{OK: true, Targets: []verifyTarget{}}
			record := func(label string, docs []gate.Doc, opts gate.Options) bool {
				if format == formatJSON {
					t := target(label, docs, opts)
					report.Targets = append(report.Targets, t)
					if !t.OK {
						report.OK = false
					}
					return t.OK
				}
				return runGates(out, label, docs, opts)
			}
			finish := func(ok bool) error {
				if format != formatJSON {
					if !ok {
						return fmt.Errorf("verification failed")
					}
					return nil
				}
				if err := writeJSON(out, report); err != nil {
					return err
				}
				if !ok {
					return ErrSilent
				}
				return nil
			}

			if rendered != "" {
				if len(args) > 0 {
					return fmt.Errorf("--rendered checks a stream that is already rendered, so it takes no release arguments")
				}
				docs, err := readRendered(cmd, rendered)
				if err != nil {
					return err
				}
				if tools {
					printTools(out, rendered, docs)
					return nil
				}
				return finish(record(rendered, docs, gate.Options{}))
			}

			root, err := loadRepo()
			if err != nil {
				return err
			}

			releases, err := releasesToVerify(root, args)
			if err != nil {
				return err
			}
			ok := true
			checked := 0
			for _, release := range releases {
				var buf bytes.Buffer
				if _, err := render.Run(cmd.Context(), render.Options{
					Root: root, Release: release,
				}, &buf, cmd.ErrOrStderr()); err != nil {
					return err
				}

				docs, err := gate.Read(&buf)
				if err != nil {
					return fmt.Errorf("%s: %w", release, err)
				}

				checked++
				if tools {
					printTools(out, release, docs)
					continue
				}
				// The gate fills a remedy command with this, and those name a
				// project rather than a release, so it has to be the project -
				// a command printed with the wrong one cannot be run as printed.
				if !record(release, docs, gate.Options{
					Project:      projectOfRelease(root, release),
					Release:      release,
					DeclaredKeys: declaredKeys(root, release),
				}) {
					ok = false
				}
			}

			if checked == 0 {
				if format == formatJSON {
					return writeJSON(out, report)
				}
				fmt.Fprintf(out, "Nothing to verify: no project declares an environment yet.\n")
				return nil
			}
			return finish(ok)
		},
	}

	cmd.Flags().BoolVar(&tools, "tools", false, "print every tool name and description for a person to review, instead of checking")
	cmd.Flags().StringVar(&rendered, "rendered", "", "check a file of already-rendered manifests, or - for stdin (defaults to rendering each project)")
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)

	return cmd
}

// readRendered loads manifests from a file or stdin.
func readRendered(cmd *cobra.Command, path string) ([]gate.Doc, error) {
	var r io.Reader = cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()
		r = f
	}
	return gate.Read(r)
}

// releasesToVerify resolves the arguments against the declaration, so a typo is
// reported rather than silently verifying nothing.
func releasesToVerify(root string, args []string) ([]string, error) {
	cfg, err := pipelineconfig.LoadFromRepo(root, "")
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return cfg.Names(), nil
	}
	for _, name := range args {
		if _, ok := cfg.Release(name); !ok {
			return nil, fmt.Errorf("%s declares no release %q; it declares %v", cfg.Path, name, cfg.Names())
		}
	}
	return args, nil
}

// wrap indents a continuation line under the label it belongs to, so a long
// message with a command in it stays readable in a terminal.
func wrap(s string) string { return wrapAt(s, 72, 7) }

// runGates runs every check over one render and reports them under one heading.
// gates runs every check on one render, in the order they are reported.
//
// It is a list rather than a call per check in a loop body so that the text output and
// `--format json` cannot disagree about which checks ran: the pair an agent
// acts on hardest is this one and `check`, and a gate that reports a different
// set of checks depending on how it was asked is the worst kind of wrong.
// declaredKeys reads what `.asgard-pipeline.yaml` declares for one release, by
// reference kind. Nil when the declaration cannot be read, which is not the
// same as a release that declares none - a check that cannot see the
// declaration must say nothing rather than report every key as undeclared.
func declaredKeys(root, release string) map[string]map[string]bool {
	cfg, err := pipelineconfig.LoadFromRepo(root, "")
	if err != nil {
		return nil
	}
	r, ok := cfg.Release(release)
	if !ok {
		return nil
	}
	out := map[string]map[string]bool{
		"secretKeyRef":    {},
		"configMapKeyRef": {},
	}
	for _, k := range r.AppSecret {
		out["secretKeyRef"][k.Key] = true
	}
	for _, k := range r.AppConfigMap {
		out["configMapKeyRef"][k.Key] = true
	}
	return out
}

func gates(docs []gate.Doc, opts gate.Options) []verifyCheck {
	named := []struct {
		name string
		r    gate.Result
	}{
		{"xref", gate.Xref(docs, opts)},
		{"agent-split", gate.AgentSplit(docs, opts)},
		{"processors", gate.Processors(docs, opts)},
		{"enums", gate.Enums(docs, opts)},
		{"constraints", gate.Constraints(docs, opts)},
		{"shapes", gate.Shapes(docs, opts)},
		{"credentials", gate.CredentialRefs(docs, opts)},
		{"deployability", gate.Deployability(docs, opts)},
		{"placeholders", gate.Placeholders(docs, opts)},
	}
	out := make([]verifyCheck, 0, len(named))
	for _, n := range named {
		c := verifyCheck{
			Name:     n.name,
			OK:       n.r.OK(),
			Summary:  n.r.Summary,
			Problems: n.r.Problems,
			Warnings: n.r.Warnings,
		}
		if c.Problems == nil {
			c.Problems = []string{}
		}
		if c.Warnings == nil {
			c.Warnings = []string{}
		}
		out = append(out, c)
	}
	return out
}

// verifyJSON is what `verify --format json` emits, one entry per render.
//
// Problems and warnings are separate arrays for the same reason they are in
// `check`: only one of them fails the gate, and a caller that has to read a
// string to find out which will eventually read it wrong.
type verifyJSON struct {
	OK      bool           `json:"ok"`
	Targets []verifyTarget `json:"targets"`
}

type verifyTarget struct {
	Target string        `json:"target"`
	OK     bool          `json:"ok"`
	Checks []verifyCheck `json:"checks"`
}

type verifyCheck struct {
	Name     string   `json:"name"`
	OK       bool     `json:"ok"`
	Summary  string   `json:"summary"`
	Problems []string `json:"problems"`
	Warnings []string `json:"warnings"`
}

// target runs the gates on one render and records the result.
func target(label string, docs []gate.Doc, opts gate.Options) verifyTarget {
	t := verifyTarget{Target: label, OK: true, Checks: gates(docs, opts)}
	for _, c := range t.Checks {
		if !c.OK {
			t.OK = false
		}
	}
	return t
}

func runGates(out io.Writer, label string, docs []gate.Doc, opts gate.Options) bool {
	fmt.Fprintf(out, "%s\n", label)

	ok := true
	for _, r := range gates(docs, opts) {
		for _, p := range r.Problems {
			fmt.Fprintf(out, "  FAIL %s\n", wrap(p))
		}
		// A warning does not fail the gate: it is a condition that is correct
		// now and fatal at deploy time, and failing on it would leave the gate
		// red through the middle of every onboarding.
		for _, w := range r.Warnings {
			fmt.Fprintf(out, "  warn %s\n", wrap(w))
		}
		switch {
		case !r.OK:
			fmt.Fprintf(out, "  %d problem(s) - %s\n", len(r.Problems), r.Summary)
			ok = false
		case len(r.Warnings) > 0:
			// Not "ok": the check passed and something in it still needs doing,
			// and "ok 0 syncer(s)" under a warning about having none reads as a
			// contradiction.
			fmt.Fprintf(out, "  --   %s\n", r.Summary)
		default:
			fmt.Fprintf(out, "  ok   %s\n", r.Summary)
		}
	}
	return ok
}

// printTools lists every tool a render exposes, with its whole description.
//
// It is deliberately not a check. What makes a tooling.description wrong is
// that it does not separate itself from the tool beside it, which is a fact
// about the set - so the useful thing a program can do is put the set in front
// of somebody, in the order and with the context the model gets, and stop
// there. Truncating would defeat it: the sentence that disambiguates two tools
// is usually not the first one.
func printTools(out io.Writer, label string, docs []gate.Doc) {
	tools := gate.Tools(docs)
	fmt.Fprintf(out, "%s\n", label)
	if len(tools) == 0 {
		fmt.Fprintf(out, "  no entry in this render is exposed as a tool.\n\n")
		return
	}
	for _, t := range tools {
		fmt.Fprintf(out, "\n  %s  (%s, entry %s)\n", t.Name, t.Workflow, t.Entry)
		for _, line := range strings.Split(strings.TrimRight(t.Description, "\n"), "\n") {
			fmt.Fprintf(out, "      %s\n", line)
		}
	}
	fmt.Fprintf(out, "\n  %d tool(s). Read them together, as the model does - one list, no other\n"+
		"  context, deciding which answers the question. The failure this catches is\n"+
		"  two descriptions that are each accurate and do not say which to prefer.\n\n", len(tools))
}

// projectOfRelease recovers the project slug from the chart a release names.
//
// A chart lives at projects/<slug>/chart/app, so the slug is the second segment
// - and it is the project's, not the release's. They differ as soon as one chart
// has a dev and a prod release, which is the normal case.
func projectOfRelease(root, release string) string {
	cfg, err := pipelineconfig.LoadFromRepo(root, "")
	if err != nil {
		return ""
	}
	decl, ok := cfg.Release(release)
	if !ok {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(decl.Chart), "/")
	if len(parts) >= 2 && parts[0] == "projects" {
		return parts[1]
	}
	return ""
}
