package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

func newTaskCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "task",
		Short: "List the open task specs, and write or move one",
		Long: `List the open task specs, and write or move one.

With no subcommand it prints every task that is not done, from
` + "`" + work.TaskIndex + "`" + `.

A request says what the customer wants; a task spec says how one chart change is
made and how it is verified. Work that touches a ` + "`SemanticLayer`" + ` or a
` + "`DataConnector`" + `, widens which cubes an agent may query, or introduces a write
path needs one before it is implemented - those three point at the customer's
live systems, so they are reviewed before the change, not after.

The record is a file in the customer repository, ` + "`" + work.TaskDir + `/TASK-xxx-<name>.md` + "`" + `,
registered in ` + "`" + work.TaskIndex + "`" + `. Task IDs are global across projects, so two
branches that number from their own project collide.

These are the engagement's own records, and the customer does not see them.
What somebody on the customer's side has to see, answer or supply goes on the
workspace's Workbench (asgard-cli workbench create); a gap in this tool goes
upstream (asgard-cli issue-report). "asgard-cli workbench --help" has the table
for which is which.`,
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
				return writeJSON(cmd.OutOrStdout(), taskReport(state))
			}
			printTasks(cmd.OutOrStdout(), state)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.AddCommand(
		newTaskAddCmd(),
		newTaskStatusCmd("ready", work.Ready,
			"scope and acceptance criteria are complete and every R# maps to an implementation task and a verification entry"),
		newTaskStatusCmd("start", work.InProgress,
			"implementation has begun, which the SDD rules say waits for the user to ask for it"),
		newTaskStatusCmd("done", work.Done,
			"the gate is green, the behaviour delta has reached the living spec, and any decision it settled has its own dated record"),
	)

	return cmd
}

// taskJSON is one open task spec, as a record rather than as a column.
type taskJSON struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

func taskReport(state stage.State) []taskJSON {
	out := []taskJSON{}
	for _, t := range work.ActiveTasks(state.Tasks) {
		out = append(out, taskJSON{ID: t.ID, Status: string(t.Status), Title: t.Title})
	}
	return out
}

func printTasks(out io.Writer, state stage.State) {
	tasks := work.ActiveTasks(state.Tasks)
	if len(tasks) == 0 {
		fmt.Fprintf(out, "No open task specs in %s.\n", work.TaskIndex)
		// A request with no task is work nobody has written down the shape of.
		// It is worth saying here rather than on `request`, because the thing
		// that is missing is a task.
		if requests := work.ActiveRequests(state.Requests); len(requests) > 0 {
			fmt.Fprintf(out, "\nAnything touching a chart wants one first:\n\n    asgard-cli task add \"<title>\" --request %s --project <project>\n", requests[0].ID)
		}
		return
	}

	fmt.Fprintf(out, "Open task specs, from %s:\n", work.TaskIndex)
	for _, t := range tasks {
		fmt.Fprintf(out, "  %-9s %-11s %s\n", t.ID, t.Status, t.Title)
	}
	fmt.Fprintf(out, "\nFinish or park these before opening another. Move a status with\n"+
		"`asgard-cli task ready|start|done <id>`, which changes the index, the\n"+
		"spec's Meta and the spec's log together - by hand it is three places.\n")
}

func newTaskAddCmd() *cobra.Command {
	var (
		slug       string
		project    string
		request    string
		complexity string
		owner      string
	)

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Write a task spec and register it",
		Long: `Write a task spec and register it.

The spec is the single-file SDD shape the generated repo documents in
` + "`docs/spec-driven-development.md`" + `: Meta, 1) Requirements, 2) Design,
3) Implementation Tasks, 4) Execution Log. Every part somebody has to answer is
marked TODO, and nothing is filled in on their behalf, because the next reader
cannot tell a field that looks decided from one that was.

It stamps today's date and ` + "`draft`" + `, registers the row, and points the index's
Next Task section at whatever is now most advanced.

    asgard-cli task add "expose stock levels to the warehouse agent" \
      --request REQ-001 --project erp --complexity M`,
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
			if request != "" {
				requests, err := work.ReadRequests(root)
				if err != nil {
					return err
				}
				known := false
				for _, r := range requests {
					if r.ID == request {
						known = true
						break
					}
				}
				if !known {
					return fmt.Errorf("no %s in %s; open it with `asgard-cli request add`", request, work.RequestIndex)
				}
			}

			if slug == "" {
				slug = work.Slugify(title)
			}

			task := work.Task{
				Title:      title,
				Slug:       slug,
				Status:     work.Draft,
				Owner:      owner,
				Complexity: complexity,
				Project:    project,
				Request:    request,
				Created:    today(),
				SpecSlug:   repo.SpecSlugIn(root),
			}

			task, err = work.AddTask(root, task)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote %s\n", task.File())
			fmt.Fprintf(out, "  status   %s\n  created  %s\n", work.Draft, task.Created)
			if project != "" {
				fmt.Fprintf(out, "  project  %s\n", project)
			}
			if request != "" {
				fmt.Fprintf(out, "  request  %s\n", request)
			}
			fmt.Fprintf(out, "Registered in %s\n", work.TaskIndex)

			fmt.Fprintf(out, `
Read `+"`docs/spec/%s/`"+` for the module this touches before filling it in: the
spec is a delta against what that module says the system does today.

    asgard-cli task ready %s     once scope and acceptance criteria are complete
`, task.SpecSlug, task.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "short name for the file (defaults to one derived from the title; required when the title has no ASCII)")
	cmd.Flags().StringVar(&project, "project", "", "project whose chart this changes (defaults to a TODO in the spec)")
	cmd.Flags().StringVar(&request, "request", "", "request this implements, e.g. REQ-001 (defaults to none, for work no customer asked for)")
	cmd.Flags().StringVar(&complexity, "complexity", "", "S, M or L (defaults to a TODO in the spec)")
	cmd.Flags().StringVar(&owner, "owner", "", "who is doing it (defaults to a TODO in the spec)")

	return cmd
}

// newTaskStatusCmd builds one status transition. They are separate commands
// rather than one taking a status argument because each has its own gate, and
// the gate belongs in the help of the command that crosses it.
func newTaskStatusCmd(verb string, to work.Status, gate string) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <task-id>",
		Short: "Move a task to " + string(to),
		Long: fmt.Sprintf(`Move a task to %s, which means %s.

It rewrites the status in %s, in the task spec's own Meta section, appends a
dated line to the spec's Execution Log, and refreshes the index's Next Task
section. Doing those by hand can leave the repo saying two different things
about the same task, with no way to tell which is current.

    asgard-cli task %s TASK-001`, to, gate, work.TaskIndex, verb),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			root, err := loadRepo()
			if err != nil {
				return err
			}
			if err := work.SetTaskStatus(root, id, to, today()); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s is now %s, as of %s\n", id, to, today())
			if to == work.Done {
				fmt.Fprintf(out, `
Two things remain after the status moves: apply the behaviour delta to the
living spec, and give any decision it settled its own dated record under
docs/decisions/.
`)
			}
			return nil
		},
	}
}
