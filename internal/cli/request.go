package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

func newRequestCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "request",
		Short: "List what the customer asked for, and move a request's status",
		Long: `List what the customer asked for, and move a request's status.

With no subcommand it prints every request that is not done, from
` + "`" + work.RequestIndex + "`" + `.

A request is the unit of work an engagement actually receives: one thing the
customer wants that the agent cannot do today. An onboarding is the first
request, and everything after it arrives the same way.

The record is a file in the customer repository, ` + "`" + work.RequestDir + `/REQ-xxx-<name>.md` + "`" + `,
registered in ` + "`" + work.RequestIndex + "`" + `. This CLI stores nothing; the state lives in
the repo, where the next agent looks.

These are the engagement's own records, and the customer does not see
them. What somebody on the customer's side has to see, answer or supply goes
on the workspace's Workbench (asgard-cli workbench create); a gap in this tool
goes upstream (asgard-cli issue-report). "asgard-cli workbench --help" has the
table for which is which.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			state, err := loadState()
			if err != nil {
				return err
			}
			if format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), requestReport(state))
			}
			printRequests(cmd.OutOrStdout(), state)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.AddCommand(
		newRequestAddCmd(),
		newRequestTargetCmd(),
		newRequestStatusCmd("ready", work.Ready,
			"the background, the audience and the scope are settled and it can be broken into tasks"),
		newRequestStatusCmd("done", work.Done,
			"every task it spawned is done and the delta has reached the living spec"),
	)

	return cmd
}

// requestJSON is one open request, as a record rather than as a column.
type requestJSON struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Project string `json:"project,omitempty"`
	Title   string `json:"title"`
}

func requestReport(state stage.State) []requestJSON {
	out := []requestJSON{}
	for _, r := range work.ActiveRequests(state.Requests) {
		out = append(out, requestJSON{ID: r.ID, Status: string(r.Status), Project: r.Project, Title: r.Title})
	}
	return out
}

func printRequests(out io.Writer, state stage.State) {
	requests := work.ActiveRequests(state.Requests)
	if len(requests) == 0 {
		fmt.Fprintf(out, "No open requests in %s.\n\nOpen one with `asgard-cli request add \"<what the customer asked for>\"`.\n", work.RequestIndex)
		return
	}

	fmt.Fprintf(out, "Open requests, from %s:\n", work.RequestIndex)
	untargeted := 0
	for _, r := range requests {
		target := r.Project
		if target == "" {
			target = "no project yet"
			untargeted++
		}
		fmt.Fprintf(out, "  %-9s %-10s %-16s %s\n", r.ID, r.Status, target, r.Title)
	}
	// **Only when a row needs it.** A remedy printed under a list where every
	// request already names a project tells the reader to do something they
	// have done, and the next line they skim is the one that mattered.
	if untargeted > 0 {
		fmt.Fprintf(out, "\nA request with no project named has not had its audience decided:\n`asgard-cli request target <request-id> <project>`.\n")
	}
}

func newRequestAddCmd() *cobra.Command {
	var (
		slug     string
		audience string
		project  string
		priority string
	)

	cmd := &cobra.Command{
		Use:   "add <what the customer asked for>",
		Short: "Open a request for something the customer asked for",
		Long: `Open a request for something the customer asked for.

Write the title in the customer's own words. The translation into a project, a
read path and an entry point is the request's own job, and keeping the original
wording is what lets the next reader check that the translation was right.

Open one request per thing they asked for. With two capabilities in one file,
a half-finished request can end up marked done.

It writes the spec, registers it, and stamps today's date and ` + "`draft`" + ` on both.
The ID is the first unused REQ number in ` + "`" + work.RequestDir + "`" + `, read from the file
names rather than the index, so a spec written without its row still owns its
number.

    asgard-cli request add "warehouse staff need to ask about stock in chat"
    asgard-cli request add "let visitors ask about products" --project site

--project is optional and usually unknown at this point: it is decided by who is
on the other end, which is section 2 of the spec. Until it is set,
` + "`asgard-cli request`" + ` shows it with no project named, meaning the
interview has not decided it yet.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]

			root, err := loadRepo()
			if err != nil {
				return err
			}

			if project != "" {
				projects, err := repo.Projects(root)
				if err != nil {
					return err
				}
				if !slices.Contains(projects, project) {
					return fmt.Errorf("no project %q in this repository; it has: %s",
						project, strings.Join(projects, ", "))
				}
			}

			if slug == "" {
				slug = work.Slugify(title)
			}

			request := work.Request{
				Title:    title,
				Slug:     slug,
				Status:   work.Draft,
				Priority: priority,
				Audience: audience,
				Project:  project,
				Raised:   today(),
				SpecSlug: repo.SpecSlugIn(root),
			}

			request, err = work.AddRequest(root, request)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote %s\n", request.File())
			fmt.Fprintf(out, "  status   %s\n  raised   %s\n", work.Draft, request.Raised)
			if project != "" {
				fmt.Fprintf(out, "  project  %s\n", project)
			}
			fmt.Fprintf(out, "Registered in %s\n", work.RequestIndex)

			fmt.Fprintf(out, `
Fill in sections 2 and 3 of that file - who is on the other end, and which
systems it has to read. Those two answers decide the project, the read path and
the entry point, and nothing downstream can be designed without them.

    asgard-cli guide requirements     how the interview goes, and what it filters out
`)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "short name for the file (defaults to one derived from the title; required when the title has no ASCII)")
	cmd.Flags().StringVar(&audience, "audience", "", "who is on the other end, if it is already known (defaults to a TODO in the spec)")
	cmd.Flags().StringVar(&project, "project", "", "project this lands in, if the audience is already known (defaults to a TODO, decided later)")
	cmd.Flags().StringVar(&priority, "priority", "", "how urgent it is, in whatever words the engagement uses (defaults to a TODO)")

	return cmd
}

// newRequestStatusCmd builds one status transition. They are separate commands
// rather than one taking a status argument because each transition has its own
// gate, and the gate belongs in the help of the command that crosses it.
func newRequestStatusCmd(verb string, to work.Status, gate string) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <request-id>",
		Short: "Move a request to " + string(to),
		Long: fmt.Sprintf(`Move a request to %s, which means %s.

It rewrites the status in %s, in the request spec's own Meta section, and appends
a dated line to the spec's log. They are done together here because a repo where
two of them disagree gives the next reader no way to tell which one is current.

    asgard-cli request %s REQ-001`, to, gate, work.RequestIndex, verb),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			root, err := loadRepo()
			if err != nil {
				return err
			}
			if err := work.SetRequestStatus(root, id, to, today()); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s is now %s, as of %s\n", id, to, today())
			return nil
		},
	}
}

func newRequestTargetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target <request-id> <project>",
		Short: "Record which project a request lands in",
		Long: `Record which project a request lands in.

This is the answer the interview produces, and it is what moves the onboarding
on: until a request names a project this repository has, ` + "`asgard-cli request`" + `
lists it with no project, meaning the interview has not finished.

The project is decided by the audience, not by where the data is. Same audience as an existing
project means it goes in that project; a new audience means a new project, with
its own read path and its own way in. Putting a public capability into
an internal project because the data happens to be nearby makes a semantic
layer reachable from a public endpoint.

It writes the Spec column of ` + "`" + work.RequestIndex + "`" + `, the Target project line of the
request's Meta, and a dated line in its log.

    asgard-cli request target REQ-001 erp`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, project := args[0], args[1]

			root, err := loadRepo()
			if err != nil {
				return err
			}
			projects, err := repo.Projects(root)
			if err != nil {
				return err
			}
			if !slices.Contains(projects, project) {
				return fmt.Errorf("no project %q in this repository; it has: %s",
					project, strings.Join(projects, ", "))
			}
			if err := work.SetRequestProject(root, id, project, today()); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s targets project %s, as of %s\n", id, project, today())
			return nil
		},
	}

	return cmd
}
