package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

func newQuestionCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "question",
		Short: "List what nobody has answered yet, and record answers",
		Long: `List what nobody has answered yet, and record what the answer turned out to be.

With no subcommand it prints every open question in ` + "`" + work.QuestionFile + "`" + `, who each
is waiting on, and when it was raised.

An unanswered question has nowhere else to live. A decision record is for
something settled. A task spec's open questions vanish when that task reaches
done. The living spec describes what is, not what nobody knows. So without
` + "`" + work.QuestionFile + "`" + ` the question is rediscovered by the next person, usually by
making the wrong assumption first.

Read this before designing anything, so you do not design past a question
somebody already knew was open.

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
				return writeJSON(cmd.OutOrStdout(), questionReport(state))
			}
			printQuestions(cmd.OutOrStdout(), state)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.AddCommand(newQuestionAddCmd(), newQuestionAnsweredCmd())

	return cmd
}

// questionJSON is one open question, as a record rather than as a column.
type questionJSON struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	Blocks string `json:"blocks,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Raised string `json:"raised,omitempty"`
}

func questionReport(state stage.State) []questionJSON {
	// An empty slice rather than nil: a caller that iterates should not have to
	// tell "nothing is open" apart from "the field is missing".
	out := []questionJSON{}
	for _, q := range state.Questions {
		out = append(out, questionJSON{Number: q.Number, Text: q.Text, Blocks: q.Blocks, Owner: q.Owner, Raised: q.Raised})
	}
	return out
}

// printQuestions reports what is still unanswered.
func printQuestions(out io.Writer, state stage.State) {
	if len(state.Questions) == 0 {
		fmt.Fprintf(out, "No open questions in %s.\n\nOpen one with `asgard-cli question add \"<question>\" --ask <who>`.\n", work.QuestionFile)
		return
	}

	fmt.Fprintf(out, "%d open question(s), from %s:\n", len(state.Questions), work.QuestionFile)
	for _, q := range state.Questions {
		fmt.Fprintf(out, "  %s. %s\n", q.Number, q.Text)
		if q.Blocks != "" {
			fmt.Fprintf(out, "       blocks: %s\n", q.Blocks)
		}
		if q.Owner != "" {
			fmt.Fprintf(out, "       ask:    %s\n", q.Owner)
		}
		if q.Raised != "" && q.Raised != "-" {
			fmt.Fprintf(out, "       raised: %s\n", q.Raised)
		}
	}
	fmt.Fprintf(out, "\nDo not design past one of these. Either get the answer, or record the\n"+
		"assumption you are proceeding on and which branch it commits you to.\n")

	printWaiting(out, state)
}

// printWaiting reports who the open questions are with.
//
// A tool that reports what is missing implies the next move is ours, and for
// long stretches it is not: an engagement waits on a meeting, on an account, on
// a document, on somebody's internal approval. Told only that no request is
// open, an FDE in that state reads the tool as saying they are behind - and the
// information to say otherwise was already in the file, in the column that
// names who can answer.
//
// Waiting is a state the work is in, not a gap in it.
func printWaiting(out io.Writer, state stage.State) {
	if state.InFlight() {
		return
	}

	byOwner := map[string]int{}
	var unowned int
	for _, q := range state.Questions {
		if q.Owner == "" || q.Owner == "-" {
			unowned++
			continue
		}
		byOwner[q.Owner]++
	}
	if len(byOwner) == 0 && unowned == 0 {
		return
	}

	owners := make([]string, 0, len(byOwner))
	for o := range byOwner {
		owners = append(owners, o)
	}
	sort.Slice(owners, func(i, j int) bool {
		if byOwner[owners[i]] != byOwner[owners[j]] {
			return byOwner[owners[i]] > byOwner[owners[j]]
		}
		return owners[i] < owners[j]
	})

	fmt.Fprintf(out, "\nWAITING - nothing is in flight because the answers are with somebody else:\n\n")
	for _, o := range owners {
		fmt.Fprintf(out, "  %-3d %s\n", byOwner[o], o)
	}
	if unowned > 0 {
		fmt.Fprintf(out, "  %-3d nobody named - a question with no owner is not tracked,\n"+
			"      only written down\n", unowned)
	}

	fmt.Fprintf(out, "\nThat is a normal state. What to check while it lasts:\n\n"+
		"  - the questions that are ours rather than theirs - anything for the\n"+
		"    platform team gets asked before the next meeting, not during it.\n"+
		"    `.agents/skills/asgard-platform/wiki/platform-unknowns.md`\n"+
		"  - whether the meeting has something to take into it - the\n"+
		"    `proposal-deck` skill in `.agents/skills/`\n"+
		"  - whether each question names a person or a role. The ones that do not\n"+
		"    are the ones that come back unanswered\n")
}

func newQuestionAddCmd() *cobra.Command {
	var (
		blocks string
		ask    string
	)

	cmd := &cobra.Command{
		Use:   "add <question>",
		Short: "Add an open question",
		Long: `Add an open question, numbered after the highest one already there and stamped
with today's date.

Add it the moment it blocks or shapes a decision, not later. A question you can
already answer is not one - answer it instead.

    asgard-cli question add "which of the two stock figures is authoritative" \
      --blocks REQ-001 --ask "the warehouse lead"

Always set --ask: a question with no owner does not get answered. --blocks makes
it findable from the work it is holding up.

This is not the place to park a question to avoid asking it. If the customer can
answer it in the next meeting, it belongs in that agenda, not in a table.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := loadRepo()
			if err != nil {
				return err
			}

			question, err := work.AddQuestion(root, work.Question{
				Text:   args[0],
				Blocks: blocks,
				Owner:  ask,
				Raised: today(),
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Added question %s to %s, raised %s\n", question.Number, work.QuestionFile, question.Raised)
			if ask == "" {
				fmt.Fprintf(out, "\nNo owner recorded. A question with nobody to ask does not get answered;\n"+
					"add one by editing the row, or with --ask next time.\n")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&blocks, "blocks", "", "what this holds up - a REQ or TASK id, or a decision in words (defaults to empty)")
	cmd.Flags().StringVar(&ask, "ask", "", "who can answer it (defaults to empty; a question with no owner does not get answered)")

	return cmd
}

func newQuestionAnsweredCmd() *cobra.Command {
	var decision string

	cmd := &cobra.Command{
		Use:   "answered <number> <answer>",
		Short: "Move a question to the answered table",
		Long: `Move a question to the answered table, with the answer and today's date.

The row moves rather than being deleted, because the open question explains
the shape of the design that answered it.

    asgard-cli question answered 3 "location 608 only, the row's own is stale" \
      --decision 2026-09-04-safety-stock-source.md

If the answer settled anything, write the dated record under docs/decisions/ and
name it with --decision. The answer here is one line; the record is where the
reasoning goes.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, answer := args[0], args[1]

			root, err := loadRepo()
			if err != nil {
				return err
			}
			if err := work.AnswerQuestion(root, number, answer, decision, today()); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Question %s answered, as of %s\n", number, today())
			if decision == "" {
				fmt.Fprintf(out, "\nNo decision record named. If this settled how something is built, it needs\n"+
					"one under docs/decisions/ - the answer above is a line, not the reasoning.\n")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&decision, "decision", "", "file name of the dated record under docs/decisions/ (defaults to empty)")

	return cmd
}
