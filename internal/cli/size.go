package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/size"
)

func newSizeCmd() *cobra.Command {
	var in size.Inputs

	cmd := &cobra.Command{
		Use:         "size [shape]",
		Annotations: touchesNoNetwork(),
		Short:       "What one capability is made of, before it is written",
		Long: `Estimate a capability's shape and size from what the interview established.

"How many agents, how many projects" is the first question a proposal is asked
and the basis of a quote. The stage prompts say how the split is decided and
the extracts say what one shape contains; this command adds them up.

    asgard-cli size                       the shapes, and what each costs empty
    asgard-cli size flow-agent-single --databases 2 --queries 4 --writes 1 --knowledge 1

Estimate one capability at a time. A request covering two audiences is two requests
and two estimates - they share no entry point and no read path, so adding their
CRs together describes nothing that will be built.

The counts come from deployments in production rather than from reasoning.
Note that the flow-agent shapes contain no Agent CR at all.

There are two outputs. The CR table is for the estimate. The plain reading is
what may go in front of the customer, because a proposal deck must not show CR
kinds; without it, somebody has to translate the table under time pressure.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if len(args) == 0 {
				fmt.Fprintf(out, "Shapes an engagement chooses between, by who is on the other end:\n\n")
				for _, s := range size.Shapes {
					fmt.Fprintf(out, "  %s\n", s)
					fmt.Fprintf(out, "  %-24s base: %s\n", "", s.BaseSummary())
					fmt.Fprintf(out, "  %-24s seen in %s\n\n", "", s.SeenIn)
				}
				fmt.Fprintf(out, "One of them: `asgard-cli size <shape> --databases N --queries N ...`\n"+
					"The audience decides which - `asgard-cli guide requirements`, question 2.\n")
				return nil
			}

			shape, ok := size.Find(args[0])
			if !ok {
				return fmt.Errorf("no shape named %q; one of: %s", args[0], strings.Join(size.Names(), ", "))
			}

			e := size.Of(shape, in)

			fmt.Fprintf(out, "%s\n%s\n\n", shape.Name, shape.Audience)
			if shape.Note != "" {
				fmt.Fprintf(out, "%s\n\n", shape.Note)
			}

			fmt.Fprintf(out, "One project. A second audience is a second project and a second estimate.\n\n")
			fmt.Fprintf(out, "CRs, for the estimate - not for a customer's screen:\n\n")
			var missing []string
			for _, k := range e.Sorted() {
				fmt.Fprintf(out, "  %-18s %d", k, e.CRs[k])
				if url, ok := size.Docs[k]; ok {
					fmt.Fprintf(out, "   %s", url)
				} else if _, gap := size.Undocumented[k]; gap {
					fmt.Fprintf(out, "   (no documentation page - see below)")
					missing = append(missing, k)
				} else if note, plain := size.NotACR[k]; plain {
					fmt.Fprintf(out, "   %s", strings.SplitN(note, "\n", 2)[0])
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "  %-18s %d\n", "TOTAL", e.Total)
			if e.CRs["Agent"] == 0 {
				fmt.Fprintf(out, "\n  Agents: 0. This shape has none.\n")
			}

			fmt.Fprintf(out, "\nThe same thing, said the way a customer can check:\n\n")
			for _, line := range e.Plain() {
				fmt.Fprintf(out, "  - %s\n", line)
			}

			if len(e.Warnings) > 0 {
				fmt.Fprintf(out, "\nWhat makes this number conditional:\n\n")
				for _, w := range e.Warnings {
					fmt.Fprintf(out, "  %s\n\n", strings.ReplaceAll(w, "\n", "\n  "))
				}
			}

			if len(missing) > 0 {
				fmt.Fprintf(out, "\nParts with nothing to link:\n\n")
				for _, k := range missing {
					fmt.Fprintf(out, "  %s - %s\n\n", k,
						strings.ReplaceAll(size.Undocumented[k], "\n", "\n  "))
				}
			}

			fmt.Fprintf(out, "Do not state this number while an open question could double it.\n"+
				"`asgard-cli question` prints what is still unanswered.\n")
			return nil
		},
	}

	f := cmd.Flags()
	f.IntVar(&in.Databases, "databases", 0, "systems read through a semantic layer")
	f.IntVar(&in.APIs, "apis", 0, "systems reached over HTTP")
	f.IntVar(&in.Queries, "queries", 0, "fixed query tools, where a layer is the wrong surface")
	f.IntVar(&in.Writes, "writes", 0, "actions with a side effect, each gated")
	f.IntVar(&in.Knowledge, "knowledge", 0, "document sources - manuals, FAQs, a site")
	f.IntVar(&in.Agents, "agents", 0, "specialisms, for the shapes that carry Agent CRs")
	f.IntVar(&in.Consoles, "consoles", 0, "systems with no database and no API")
	f.IntVar(&in.Schedules, "schedules", 0, "scheduled runs")
	return cmd
}
