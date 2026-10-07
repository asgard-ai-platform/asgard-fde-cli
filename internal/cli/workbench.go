package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newWorkbenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workbench",
		Short: "Read and write the workspace's Workbench issues",
		Long: `Read and write the issues on the workspace's Workbench, the tracker the FDE and
the customer work through together: questions to ask, what the customer asked
for, work to do, what is wrong after go-live, and feedback users left on an AI
answer. Each one is ISS-N, numbered across the whole workspace.

    asgard-cli workbench list --status in_progress --label blocked
    asgard-cli workbench show ISS-12
    asgard-cli workbench create --type question --title "Which ERP holds the RMA codes?"
    asgard-cli workbench update ISS-12 --status in_review --add-label data-source
    asgard-cli workbench comment ISS-12 --body "Waiting on the customer's DBA."
    asgard-cli workbench attach ISS-12 minutes.pdf --what "kickoff minutes" --from "customer PM" --dated 2026-09-29

Every write is made as the member's assistant. The platform authorizes it
as the signed-in account, exactly as if the member had made it, and the
timeline marks it "via Asgard AI". Some actions the platform keeps for the
member alone - deleting, pinning or locking an issue, managing labels, deleting
an attachment or a comment - and this command has none of them, because the
platform refuses them from an assistant; the member does those in the
Workbench page.

The Workbench is separate from "asgard-cli question", "request" and "task",
which are the engagement's own records in the customer's repository, and from
"asgard-cli issue-report", which reports a problem with this tool to its
maintainers. The Workbench belongs to the platform, and the customer sees it.

` + trackersHelp + `

The workspace is resolved as for every platform command: --workspace, then
ASGARD_WORKSPACE, then the checkout's binding. Labels, pipelines and
assignees are named the way a person names them - a label's name, a
pipeline's name, an email - and translated to ids here; an id works as well.

--pipeline is what the Workbench page calls a Deployment: the pipeline an
issue is about, the same one "asgard-cli pipeline list" lists. Unlike the
pipeline commands' --pipeline, it is never taken from the checkout's binding.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newWorkbenchListCmd(),
		newWorkbenchShowCmd(),
		newWorkbenchCreateCmd(),
		newWorkbenchUpdateCmd(),
		newWorkbenchCommentCmd(),
		newWorkbenchAttachCmd(),
		newWorkbenchPullCmd(),
	)
	return cmd
}

// workbenchFlags are the flags every workbench subcommand shares.
type workbenchFlags struct {
	profile   string
	workspace string
	format    string
}

func (f *workbenchFlags) register(cmd *cobra.Command) {
	addProfileFlag(cmd, &f.profile)
	cmd.Flags().StringVar(&f.workspace, workspaceFlag, "",
		"workspace to act in; defaults to ASGARD_WORKSPACE, then what \"asgard-cli workspace use\" recorded for this repository")
	cmd.Flags().StringVar(&f.format, formatFlag, formatText, formatUsage)
}

func (f *workbenchFlags) context(cmd *cobra.Command) (*platformContext, error) {
	if err := checkFormat(f.format); err != nil {
		return nil, err
	}
	return resolveContext(cmd, contextOptions{Profile: f.profile, Workspace: f.workspace, NeedWorkspace: true})
}

// The platform's enums, in board order, and what the Workbench page calls each.
var (
	workbenchStatuses = []enumValue{
		{"backlog", "Backlog"}, {"to_do", "To Do"}, {"ready", "Ready"}, {"in_progress", "In progress"},
		{"pending_fix", "Pending fix"}, {"in_review", "In review"}, {"done", "Done"},
	}
	workbenchTypes = []enumValue{
		{"question", "Question"}, {"request", "Request"}, {"task", "Task"}, {"bug", "Bug"}, {"feedback", "Feedback"},
	}
	workbenchPriorities = []enumValue{
		{"urgent", "Urgent"}, {"high", "High"}, {"normal", "Normal"}, {"low", "Low"},
	}
	workbenchSorts = []enumValue{
		{"updated_desc", ""}, {"created_desc", ""}, {"priority", ""}, {"due_date", ""},
	}
)

type enumValue struct{ wire, label string }

// normalizeEnum accepts the wire value or the label as the page shows it -
// "in_progress", "In progress" and "in-progress" are one status.
func normalizeEnum(flag, value string, values []enumValue) (string, error) {
	key := strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(strings.TrimSpace(value)))
	var names []string
	for _, v := range values {
		if v.wire == key {
			return v.wire, nil
		}
		names = append(names, v.wire)
	}
	return "", fmt.Errorf("--%s %q is not one of %s", flag, value, strings.Join(names, ", "))
}

func normalizeEnums(flag string, in []string, values []enumValue) ([]string, error) {
	var out []string
	for _, raw := range splitList(in) {
		v, err := normalizeEnum(flag, raw, values)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func enumLabel(wire string, values []enumValue) string {
	for _, v := range values {
		if v.wire == wire {
			return v.label
		}
	}
	return wire
}

// splitList flattens repeated and comma-separated flag values.
func splitList(in []string) []string {
	var out []string
	for _, v := range in {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// parseIssueNumber reads ISS-12, iss-12 or 12.
func parseIssueNumber(s string) (int64, error) {
	t := strings.TrimSpace(s)
	if len(t) > 4 && strings.EqualFold(t[:4], "ISS-") {
		t = t[4:]
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not an issue; write ISS-12 or 12", s)
	}
	return n, nil
}

func issueRef(n int64) string { return "ISS-" + strconv.FormatInt(n, 10) }

// workbenchNames turns what a person types into ids, and ids back into what a
// person reads. Each lookup is made at most once per command, and only when
// something needs it.
type workbenchNames struct {
	ctx context.Context
	pc  *platformContext

	labels    []*platform.WorkbenchLabel
	pipelines []*platform.Pipeline
	members   map[string]*platform.WorkbenchMember
}

func newWorkbenchNames(ctx context.Context, pc *platformContext) *workbenchNames {
	return &workbenchNames{ctx: ctx, pc: pc, members: map[string]*platform.WorkbenchMember{}}
}

func (n *workbenchNames) loadLabels() error {
	if n.labels != nil {
		return nil
	}
	labels, err := n.pc.Client.ListWorkbenchLabels(n.ctx)
	if err != nil {
		return err
	}
	n.labels = labels
	if n.labels == nil {
		n.labels = []*platform.WorkbenchLabel{}
	}
	return nil
}

func (n *workbenchNames) loadPipelines() error {
	if n.pipelines != nil {
		return nil
	}
	pipelines, err := n.pc.Client.ListPipelines(n.ctx)
	if err != nil {
		return err
	}
	n.pipelines = pipelines
	if n.pipelines == nil {
		n.pipelines = []*platform.Pipeline{}
	}
	return nil
}

// labelIDs resolves label names (case-insensitively, as the platform keeps
// them unique) or ids.
func (n *workbenchNames) labelIDs(flag string, in []string) ([]string, error) {
	values := splitList(in)
	if len(values) == 0 {
		return nil, nil
	}
	if err := n.loadLabels(); err != nil {
		return nil, err
	}
	var out []string
	for _, v := range values {
		id := ""
		for _, l := range n.labels {
			if l.ID == v || strings.EqualFold(l.Name, v) {
				id = l.ID
				break
			}
		}
		if id == "" {
			var names []string
			for _, l := range n.labels {
				names = append(names, strconv.Quote(l.Name))
			}
			return nil, fmt.Errorf("--%s %q: the workspace has no such label. It has %s. "+
				"A new label is made by the member in the Workbench page; the platform does not let an assistant manage labels",
				flag, v, strings.Join(names, ", "))
		}
		out = append(out, id)
	}
	return out, nil
}

func (n *workbenchNames) labelName(id string) string {
	for _, l := range n.labels {
		if l.ID == id {
			return l.Name
		}
	}
	return id
}

// pipelineID resolves a pipeline by id or name. allowNone lets the list's
// "none" through, which selects issues about no pipeline.
func (n *workbenchNames) pipelineID(flag, v string, allowNone bool) (string, error) {
	if allowNone && strings.EqualFold(v, "none") {
		return "none", nil
	}
	if err := n.loadPipelines(); err != nil {
		return "", err
	}
	for _, p := range n.pipelines {
		if p.PipelineId == v || p.Name == v {
			return p.PipelineId, nil
		}
	}
	return "", fmt.Errorf("--%s %q: no pipeline of that id or name in workspace %s; `asgard-cli pipeline list` shows what is there",
		flag, v, n.pc.Workspace)
}

func (n *workbenchNames) pipelineName(id string) string {
	if id == "" || n.loadPipelines() != nil {
		return id
	}
	for _, p := range n.pipelines {
		if p.PipelineId == id {
			return p.Name
		}
	}
	return id
}

// assigneeIDs resolves members by email, display name or user id. "me" is
// passed through: the platform reads it as the signed-in account, in the
// filter and in every write.
func (n *workbenchNames) assigneeIDs(flag string, in []string) ([]string, error) {
	var out []string
	for _, v := range splitList(in) {
		if strings.EqualFold(v, "me") {
			out = append(out, "me")
			continue
		}
		id, err := n.memberID(flag, v)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (n *workbenchNames) memberID(flag, v string) (string, error) {
	found, err := n.pc.Client.SearchWorkbenchMembers(n.ctx, v)
	if err != nil {
		return "", err
	}
	for _, m := range found {
		n.members[m.UserID] = m
		if m.UserID == v || strings.EqualFold(m.Email, v) || strings.EqualFold(m.DisplayName, v) {
			return m.UserID, nil
		}
	}
	switch len(found) {
	case 0:
		// Not a name or email any member has; it may still be an id, and the
		// platform says whether it is a member when it is sent.
		return v, nil
	case 1:
		return found[0].UserID, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--%s %q matches more than one member; name one by email:", flag, v)
	for _, m := range found {
		fmt.Fprintf(&b, "\n  %s  %s", m.Email, m.DisplayName)
	}
	return "", fmt.Errorf("%s", b.String())
}

// loadMembers looks up, in one call, every id not already known.
func (n *workbenchNames) loadMembers(ids ...string) error {
	seen := map[string]bool{}
	var missing []string
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, ok := n.members[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	found, err := n.pc.Client.GetWorkbenchMembers(n.ctx, missing)
	if err != nil {
		return err
	}
	for _, m := range found {
		n.members[m.UserID] = m
	}
	return nil
}

// memberName is what a line calls a person: the name, marked when they have
// left the workspace, or the id when the platform does not know it.
func (n *workbenchNames) memberName(id string) string {
	if id == "" {
		return "the system"
	}
	m, ok := n.members[id]
	if !ok || m.DisplayName == "" {
		return id
	}
	if !m.IsMember {
		return m.DisplayName + " (left)"
	}
	return m.DisplayName
}

// ---- list

func newWorkbenchListCmd() *cobra.Command {
	var (
		f                                        workbenchFlags
		status, types, priority, pipeline, label []string
		assignee                                 []string
		q, sortBy                                string
		limit                                    int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the workspace's issues, filtered",
		Long: `List the workspace's issues, pinned first and then most recently updated.

Every filter takes several values, repeated or comma-separated; values within one
filter are ORed and the filters are ANDed:

    asgard-cli workbench list --status backlog,to_do --type question
    asgard-cli workbench list --pipeline none              about no pipeline
    asgard-cli workbench list --assignee me --label blocked
    asgard-cli workbench list --q ISS-12                   by number, or a title fragment

Statuses and types are accepted as the page writes them or as the platform does
- "In progress" and in_progress are one status. The header says how many match
in all, which is more than are shown when --limit cuts the list.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			names := newWorkbenchNames(ctx, pc)

			filter := platform.WorkbenchIssueFilter{Q: q}
			if filter.Status, err = normalizeEnums("status", status, workbenchStatuses); err != nil {
				return err
			}
			if filter.Type, err = normalizeEnums("type", types, workbenchTypes); err != nil {
				return err
			}
			if filter.Priority, err = normalizeEnums("priority", priority, workbenchPriorities); err != nil {
				return err
			}
			if sortBy != "" {
				if filter.Sort, err = normalizeEnum("sort", sortBy, workbenchSorts); err != nil {
					return err
				}
			}
			for _, p := range splitList(pipeline) {
				id, err := names.pipelineID("pipeline", p, true)
				if err != nil {
					return err
				}
				filter.Pipeline = append(filter.Pipeline, id)
			}
			if filter.Label, err = names.labelIDs("label", label); err != nil {
				return err
			}
			if filter.Assignee, err = names.assigneeIDs("assignee", assignee); err != nil {
				return err
			}

			issues, total, err := pc.Client.ListWorkbenchIssues(ctx, filter, limit)
			if err != nil {
				return err
			}
			if issues == nil {
				issues = []*platform.WorkbenchIssue{}
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, map[string]any{"total": total, "issues": issues})
			}
			if len(issues) == 0 {
				fmt.Fprintf(out, "No issues in workspace %s match.\n", pc.Workspace)
				return nil
			}
			if err := names.loadLabels(); err != nil {
				return err
			}
			fmt.Fprintf(out, "%d of %d matching issues in workspace %s\n\n", len(issues), total, pc.Workspace)
			for _, is := range issues {
				writeIssueRow(out, names, is)
			}
			return nil
		},
	}
	f.register(cmd)
	fl := cmd.Flags()
	fl.StringSliceVar(&status, "status", nil, "status: backlog, to_do, ready, in_progress, pending_fix, in_review, done")
	fl.StringSliceVar(&types, "type", nil, "type: question, request, task, bug, feedback")
	fl.StringSliceVar(&priority, "priority", nil, "priority: urgent, high, normal, low")
	fl.StringSliceVar(&pipeline, "pipeline", nil, "the pipeline an issue is about, by name or id; none for issues about no pipeline")
	fl.StringSliceVar(&label, "label", nil, "label name or id")
	fl.StringSliceVar(&assignee, "assignee", nil, "assignee by email, name or user id; me for the signed-in account")
	fl.StringVar(&q, "q", "", "a title fragment, or an issue number such as ISS-12")
	fl.StringVar(&sortBy, "sort", "", "order after the pinned issues: updated_desc (the default), created_desc, priority, due_date")
	fl.IntVar(&limit, "limit", 50, "at most this many issues; 0 reads every page")
	return cmd
}

func writeIssueRow(out io.Writer, names *workbenchNames, is *platform.WorkbenchIssue) {
	var marks []string
	if is.Pinned {
		marks = append(marks, "pinned")
	}
	for _, id := range is.LabelIDs {
		marks = append(marks, names.labelName(id))
	}
	extra := ""
	if len(marks) > 0 {
		extra = "  [" + strings.Join(marks, ", ") + "]"
	}
	fmt.Fprintf(out, "%-8s %-12s %-9s %-7s %s%s\n",
		issueRef(is.Number), enumLabel(is.Status, workbenchStatuses), enumLabel(is.Type, workbenchTypes),
		enumLabel(is.Priority, workbenchPriorities), is.Title, extra)
}

// ---- show

func newWorkbenchShowCmd() *cobra.Command {
	var f workbenchFlags
	cmd := &cobra.Command{
		Use:   "show <ISS-N>",
		Short: "One issue: its fields, body, attachments and timeline",
		Long: `Show one issue: its fields, its body, the attachments it still has, and its
timeline - the system's events and the comments, merged oldest first.

    asgard-cli workbench show ISS-12
    asgard-cli workbench show 12 --format json

A line marked "via Asgard AI" was written by an assistant acting for the member
named on it. An attachment is source material: attaching a file does not make
its content true. What the issue has established is in the fields and the
comments.

The JSON form carries the issue, its events, its comments and its attachments as
the platform returns them, with the label names and the people beside the ids.
Somebody who has left the workspace is still named, and marked as having left.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parseIssueNumber(args[0])
			if err != nil {
				return err
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c := pc.Client
			is, err := c.GetWorkbenchIssue(ctx, number)
			if err != nil {
				return err
			}
			events, err := c.ListWorkbenchEvents(ctx, number)
			if err != nil {
				return err
			}
			comments, err := c.ListWorkbenchComments(ctx, number)
			if err != nil {
				return err
			}
			attachments, err := c.ListWorkbenchAttachments(ctx, number)
			if err != nil {
				return err
			}
			names := newWorkbenchNames(ctx, pc)
			if err := names.loadLabels(); err != nil {
				return err
			}
			people := append([]string{is.CreatedBy}, is.AssigneeIDs...)
			for _, e := range events {
				people = append(people, e.ActorID)
				if e.Field == "assignee" {
					people = append(people, e.From, e.To)
				}
			}
			for _, c := range comments {
				people = append(people, c.AuthorID)
			}
			for _, a := range attachments {
				people = append(people, a.UploaderID)
			}
			if err := names.loadMembers(people...); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				members := map[string]*platform.WorkbenchMember{}
				for _, id := range people {
					if m, ok := names.members[id]; ok {
						members[id] = m
					}
				}
				labelNames := map[string]string{}
				for _, id := range is.LabelIDs {
					labelNames[id] = names.labelName(id)
				}
				return writeJSON(out, map[string]any{
					"issue":       is,
					"label_names": labelNames,
					"members":     members,
					"events":      nonNil(events),
					"comments":    nonNil(comments),
					"attachments": nonNil(attachments),
				})
			}
			writeIssue(out, names, is, events, comments, attachments)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func writeIssue(out io.Writer, names *workbenchNames, is *platform.WorkbenchIssue,
	events []*platform.WorkbenchEvent, comments []*platform.WorkbenchComment, attachments []*platform.WorkbenchAttachment) {
	fmt.Fprintf(out, "%s  %s\n\n", issueRef(is.Number), is.Title)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(out, "  %-12s %s\n", k, v)
		}
	}
	row("Status", enumLabel(is.Status, workbenchStatuses))
	row("Type", enumLabel(is.Type, workbenchTypes))
	row("Priority", enumLabel(is.Priority, workbenchPriorities))
	if is.PipelineID != "" {
		row("Pipeline", names.pipelineName(is.PipelineID)+" ("+is.PipelineID+")")
	}
	var assignees []string
	for _, id := range is.AssigneeIDs {
		assignees = append(assignees, names.memberName(id))
	}
	row("Assignees", strings.Join(assignees, ", "))
	if is.CreatedBy != "" {
		row("Opened by", names.memberName(is.CreatedBy))
	}
	var labels []string
	for _, id := range is.LabelIDs {
		labels = append(labels, names.labelName(id))
	}
	row("Labels", strings.Join(labels, ", "))
	row("Due", is.DueDate)
	for _, r := range is.Relations {
		title := r.Title
		if r.Deleted {
			title = "(deleted)"
		}
		row(strings.ReplaceAll(r.Type, "_", " "), fmt.Sprintf("%s %s [%s]", issueRef(r.Number), title, enumLabel(r.Status, workbenchStatuses)))
	}
	if is.Source != nil {
		row("Source", fmt.Sprintf("%s feedback, project %s, message %s", is.Source.Product, is.Source.Project, is.Source.MessageID))
	}
	var flags []string
	if is.Pinned {
		flags = append(flags, "pinned")
	}
	if is.Locked {
		flags = append(flags, "locked - only workspace admins may comment")
	}
	row("", strings.Join(flags, "; "))

	if strings.TrimSpace(is.Body) != "" {
		fmt.Fprintf(out, "\n%s\n", indent(strings.TrimRight(is.Body, "\n"), "    "))
	}

	if len(attachments) > 0 {
		fmt.Fprintf(out, "\nAttachments (source material, not established fact):\n")
		for _, a := range attachments {
			fmt.Fprintf(out, "  %s  %s, %d bytes, sha256 %s, from %s\n", a.ID, a.OriginalFilename, a.ByteSize, a.SHA256, names.memberName(a.UploaderID))
			fmt.Fprintf(out, "      what %q  from %q  dated %s\n", a.What, a.From, a.Dated)
			if a.Supersedes != "" {
				fmt.Fprintf(out, "      supersedes %s\n", a.Supersedes)
			}
		}
	}

	type entry struct {
		at   string
		line string
	}
	var timeline []entry
	for _, e := range events {
		timeline = append(timeline, entry{e.At, fmt.Sprintf("%s %s%s", names.memberName(e.ActorID), eventText(names, e), viaTag(e.ViaAssistant))})
	}
	for _, c := range comments {
		edited := ""
		if c.EditedAt != nil {
			edited = " (edited)"
		}
		timeline = append(timeline, entry{c.CreatedAt, fmt.Sprintf("%s commented%s%s:\n%s",
			names.memberName(c.AuthorID), edited, viaTag(c.ViaAssistant), indent(strings.TrimRight(c.Body, "\n"), "      "))})
	}
	sort.SliceStable(timeline, func(i, j int) bool { return timeline[i].at < timeline[j].at })
	if len(timeline) > 0 {
		fmt.Fprintf(out, "\nTimeline:\n")
		for _, t := range timeline {
			fmt.Fprintf(out, "  %s  %s\n", shortTime(t.at), t.line)
		}
	}
}

func viaTag(via bool) string {
	if via {
		return "  (via Asgard AI)"
	}
	return ""
}

func shortTime(at string) string {
	if len(at) >= 16 {
		return strings.Replace(at[:16], "T", " ", 1)
	}
	return at
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

// eventText is the timeline sentence for one event, as the Workbench page
// writes it.
func eventText(names *workbenchNames, e *platform.WorkbenchEvent) string {
	from, to := e.From, e.To
	if e.FromDisplay != nil && *e.FromDisplay != "" {
		from = *e.FromDisplay
	}
	if e.ToDisplay != nil && *e.ToDisplay != "" {
		to = *e.ToDisplay
	}
	switch e.Kind {
	case "created":
		return "created this issue"
	case "status":
		return fmt.Sprintf("moved this from %s to %s", enumLabel(from, workbenchStatuses), enumLabel(to, workbenchStatuses))
	case "pin":
		if to == "" || to == "false" {
			return "unpinned this issue"
		}
		return "pinned this issue"
	case "lock":
		if to == "" || to == "false" {
			return "unlocked this conversation"
		}
		return "locked this conversation"
	case "attachment":
		if to == "" {
			return "removed attachment " + from
		}
		return "added attachment " + to
	case "comment_meta":
		return "deleted a comment"
	case "field":
		switch e.Field {
		case "title":
			return fmt.Sprintf("changed the title from %q", from)
		case "body":
			return "edited the description"
		case "type":
			return fmt.Sprintf("changed type from %s to %s", enumLabel(from, workbenchTypes), enumLabel(to, workbenchTypes))
		case "priority":
			return fmt.Sprintf("changed priority from %s to %s", enumLabel(from, workbenchPriorities), enumLabel(to, workbenchPriorities))
		case "assignee":
			if e.To == "" {
				return "unassigned " + names.memberName(e.From)
			}
			return "assigned " + names.memberName(e.To)
		case "label":
			if to == "" {
				return "removed label " + from
			}
			return "added label " + to
		case "due_date":
			if to == "" {
				return "removed the due date"
			}
			return "set due date to " + to
		case "pipeline":
			if to == "" {
				return "detached it from pipeline " + from
			}
			return "set the pipeline to " + to
		case "relation":
			return relationText(e.From, e.To)
		}
	}
	return fmt.Sprintf("%s %s: %s -> %s", e.Kind, e.Field, orDash(from), orDash(to))
}

// relationText reads a relation event. The platform writes the edge as
// "<type>:<number>" - "parent:1", "blocked_by:7" - in to when it is added and
// in from when it is removed.
func relationText(from, to string) string {
	// A replaced parent or duplicate carries both ends in one event.
	if fk, fn, ok := strings.Cut(from, ":"); ok {
		if tk, tn, ok := strings.Cut(to, ":"); ok && fk == tk {
			return fmt.Sprintf("changed %s from ISS-%s to ISS-%s", strings.ReplaceAll(fk, "_", " "), fn, tn)
		}
	}
	edge, adding := to, true
	if edge == "" {
		edge, adding = from, false
	}
	kind, num, ok := strings.Cut(edge, ":")
	if !ok {
		return fmt.Sprintf("changed a relation: %s -> %s", orDash(from), orDash(to))
	}
	ref := "ISS-" + num
	phrases := map[string][2]string{
		"parent":       {"set parent to " + ref, "removed parent " + ref},
		"sub_issue":    {"added sub-issue " + ref, "removed sub-issue " + ref},
		"blocked_by":   {"marked as blocked by " + ref, "removed blocked by " + ref},
		"blocking":     {"marked as blocking " + ref, "removed blocking " + ref},
		"duplicate_of": {"marked as duplicate of " + ref, "removed duplicate of " + ref},
	}
	p, known := phrases[kind]
	switch {
	case !known:
		return fmt.Sprintf("changed a relation: %s -> %s", orDash(from), orDash(to))
	case adding:
		return p[0]
	}
	return p[1]
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- create

// bodyFlags reads an issue body from --body or --body-file, - being stdin.
type bodyFlags struct {
	body, file string
}

func (b *bodyFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&b.body, "body", "", "the description, in markdown")
	cmd.Flags().StringVar(&b.file, "body-file", "", "read the description from this file; - reads stdin")
}

func (b *bodyFlags) read(cmd *cobra.Command) (string, bool, error) {
	fl := cmd.Flags()
	switch {
	case fl.Changed("body") && fl.Changed("body-file"):
		return "", false, fmt.Errorf("pass --body or --body-file, not both")
	case fl.Changed("body-file"):
		var raw []byte
		var err error
		if b.file == "-" {
			raw, err = io.ReadAll(cmd.InOrStdin())
		} else {
			raw, err = os.ReadFile(b.file)
		}
		if err != nil {
			return "", false, fmt.Errorf("--body-file: %w", err)
		}
		return string(raw), true, nil
	case fl.Changed("body"):
		return b.body, true, nil
	}
	return "", false, nil
}

func newWorkbenchCreateCmd() *cobra.Command {
	var (
		f                                           workbenchFlags
		body                                        bodyFlags
		title, typ, status, priority, pipeline, due string
		labels, assignees                           []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Open an issue, as the member's assistant",
		Long: `Open an issue on the workspace's Workbench. It takes the workspace's next number
and is shown as ISS-N; the timeline records it as the signed-in member's, via
Asgard AI.

    asgard-cli workbench create --type question \
      --title "Which ERP holds the RMA status codes?" \
      --body-file ./q.md --pipeline support-bot --label data-source

--type and --title are required. The type decides what the body is for:

    question   something that has to be answered or supplied before work can
               go on - say what, why it is needed, and who can answer it. A
               blocker on the customer's side during the build is one
    request    one thing the customer wants, in the customer's own words
    task       a unit of work, with scope and acceptance
    bug        what is wrong after go-live in the customer's deployment:
               expected, actual, how to reproduce. Never a failure of this
               tool itself - that is "asgard-cli issue-report"
    feedback   a user's reaction to an AI answer, when it arrived some other way

The customer reads what this opens. Whether a thing belongs here, in the
repository's own records or upstream is decided by who has to act on it:
"asgard-cli workbench --help" has the table.

Everything else is optional: the status defaults to backlog and the priority to
normal. The pipeline is never assumed, not even from the checkout's binding:
an issue can be about none, and nobody looks for an issue under the wrong
pipeline.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("--title is required")
			}
			if typ == "" {
				return fmt.Errorf("--type is required: question, request, task, bug or feedback")
			}
			req := platform.CreateWorkbenchIssueRequest{Title: title, DueDate: due}
			var err error
			if req.Type, err = normalizeEnum("type", typ, workbenchTypes); err != nil {
				return err
			}
			if status != "" {
				if req.Status, err = normalizeEnum("status", status, workbenchStatuses); err != nil {
					return err
				}
			}
			if priority != "" {
				if req.Priority, err = normalizeEnum("priority", priority, workbenchPriorities); err != nil {
					return err
				}
			}
			if req.Body, _, err = body.read(cmd); err != nil {
				return err
			}

			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			names := newWorkbenchNames(ctx, pc)
			if pipeline != "" {
				if req.PipelineID, err = names.pipelineID("pipeline", pipeline, false); err != nil {
					return err
				}
			}
			if req.LabelIDs, err = names.labelIDs("label", labels); err != nil {
				return err
			}
			if req.AssigneeIDs, err = names.assigneeIDs("assignee", assignees); err != nil {
				return err
			}

			actingOn(cmd, pc.Session)
			is, err := pc.Client.CreateWorkbenchIssue(ctx, req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, is)
			}
			fmt.Fprintf(out, "Opened %s in workspace %s: %s\n", issueRef(is.Number), pc.Workspace, is.Title)
			return nil
		},
	}
	f.register(cmd)
	body.register(cmd)
	fl := cmd.Flags()
	fl.StringVar(&title, "title", "", "the title (required)")
	fl.StringVar(&typ, "type", "", "question, request, task, bug or feedback (required)")
	fl.StringVar(&status, "status", "", "the status to open it in; defaults to backlog")
	fl.StringVar(&priority, "priority", "", "urgent, high, normal or low; defaults to normal")
	fl.StringVar(&pipeline, "pipeline", "", "the pipeline it is about, by name or id; defaults to none")
	fl.StringSliceVar(&labels, "label", nil, "a label, by name or id; repeat or comma-separate for several")
	fl.StringSliceVar(&assignees, "assignee", nil, "an assignee by email, name or user id, or me; repeat for several")
	fl.StringVar(&due, "due", "", "the due date, YYYY-MM-DD")
	return cmd
}

// ---- update

func newWorkbenchUpdateCmd() *cobra.Command {
	var (
		f                                                          workbenchFlags
		body                                                       bodyFlags
		title, typ, status, priority, pipeline, due                string
		addLabels, removeLabels, addAssignees, removeAssignees     []string
		parent, duplicateOf                                        string
		addBlockedBy, removeBlockedBy, addSubIssue, removeSubIssue []string
	)
	cmd := &cobra.Command{
		Use:   "update <ISS-N>",
		Short: "Change an issue's fields, labels, assignees and relations",
		Long: `Change any of an issue's fields in one write. What is not named is left as it
is, and the timeline records everything named here as one action.

    asgard-cli workbench update ISS-12 --status in_review
    asgard-cli workbench update ISS-12 --add-label blocked --add-assignee pat@example.com
    asgard-cli workbench update ISS-12 --parent ISS-3 --add-blocked-by ISS-7
    asgard-cli workbench update ISS-12 --due "" --pipeline ""       clear both

There is no close action. An issue ends by moving to done and starts again by
moving out of it; "we will not do this" is the not planned label and a move to
done, and a duplicate is --duplicate-of.

An empty --due removes the due date, an empty --pipeline detaches the issue,
and --parent 0 or --duplicate-of 0 clears that relation. Labels and assignees
change by what is added and what is removed, at most 20 of each; a removed
assignee need not still be a member.

A comment is "asgard-cli workbench comment", a file is "asgard-cli workbench
attach"; what the platform keeps for the member alone is in
"asgard-cli workbench --help".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parseIssueNumber(args[0])
			if err != nil {
				return err
			}
			fl := cmd.Flags()
			var req platform.UpdateWorkbenchIssueRequest
			changed := false
			str := func(name, v string) *string {
				if !fl.Changed(name) {
					return nil
				}
				changed = true
				return &v
			}
			req.Title = str("title", title)
			req.DueDate = str("due", due)
			if fl.Changed("type") {
				v, err := normalizeEnum("type", typ, workbenchTypes)
				if err != nil {
					return err
				}
				req.Type = str("type", v)
			}
			if fl.Changed("status") {
				v, err := normalizeEnum("status", status, workbenchStatuses)
				if err != nil {
					return err
				}
				req.Status = str("status", v)
			}
			if fl.Changed("priority") {
				v, err := normalizeEnum("priority", priority, workbenchPriorities)
				if err != nil {
					return err
				}
				req.Priority = str("priority", v)
			}
			if text, ok, err := body.read(cmd); err != nil {
				return err
			} else if ok {
				req.Body, changed = &text, true
			}
			relation := func(name, v string) (*int64, error) {
				if !fl.Changed(name) {
					return nil, nil
				}
				changed = true
				if strings.TrimSpace(v) == "0" {
					zero := int64(0)
					return &zero, nil
				}
				n, err := parseIssueNumber(v)
				if err != nil {
					return nil, fmt.Errorf("--%s: %w", name, err)
				}
				return &n, nil
			}
			if req.ParentNumber, err = relation("parent", parent); err != nil {
				return err
			}
			if req.DuplicateOfNumber, err = relation("duplicate-of", duplicateOf); err != nil {
				return err
			}
			numbers := func(name string, in []string) ([]int64, error) {
				var out []int64
				for _, v := range splitList(in) {
					n, err := parseIssueNumber(v)
					if err != nil {
						return nil, fmt.Errorf("--%s: %w", name, err)
					}
					out = append(out, n)
				}
				if len(out) > 0 {
					changed = true
				}
				return out, nil
			}
			if req.AddBlockedByNumbers, err = numbers("add-blocked-by", addBlockedBy); err != nil {
				return err
			}
			if req.RemoveBlockedByNumbers, err = numbers("remove-blocked-by", removeBlockedBy); err != nil {
				return err
			}
			if req.AddSubIssueNumbers, err = numbers("add-sub-issue", addSubIssue); err != nil {
				return err
			}
			if req.RemoveSubIssueNumbers, err = numbers("remove-sub-issue", removeSubIssue); err != nil {
				return err
			}
			names := [][]string{addLabels, removeLabels, addAssignees, removeAssignees}
			for _, n := range names {
				if len(splitList(n)) > 0 {
					changed = true
				}
			}
			if !changed && !fl.Changed("pipeline") {
				return fmt.Errorf("nothing to change; name at least one field. \"asgard-cli workbench update --help\" lists them")
			}

			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			res := newWorkbenchNames(ctx, pc)
			if fl.Changed("pipeline") {
				id := ""
				if pipeline != "" {
					if id, err = res.pipelineID("pipeline", pipeline, false); err != nil {
						return err
					}
				}
				req.PipelineID = &id
			}
			if req.AddLabelIDs, err = res.labelIDs("add-label", addLabels); err != nil {
				return err
			}
			if req.RemoveLabelIDs, err = res.labelIDs("remove-label", removeLabels); err != nil {
				return err
			}
			if req.AddAssigneeIDs, err = res.assigneeIDs("add-assignee", addAssignees); err != nil {
				return err
			}
			if req.RemoveAssigneeIDs, err = res.assigneeIDs("remove-assignee", removeAssignees); err != nil {
				return err
			}

			actingOn(cmd, pc.Session)
			is, err := pc.Client.UpdateWorkbenchIssue(ctx, number, req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, is)
			}
			if err := res.loadLabels(); err != nil {
				return err
			}
			fmt.Fprintf(out, "Updated %s in workspace %s:\n", issueRef(is.Number), pc.Workspace)
			writeIssueRow(out, res, is)
			return nil
		},
	}
	f.register(cmd)
	body.register(cmd)
	fl := cmd.Flags()
	fl.StringVar(&title, "title", "", "a new title")
	fl.StringVar(&typ, "type", "", "question, request, task, bug or feedback")
	fl.StringVar(&status, "status", "", "backlog, to_do, ready, in_progress, pending_fix, in_review or done")
	fl.StringVar(&priority, "priority", "", "urgent, high, normal or low")
	fl.StringVar(&pipeline, "pipeline", "", "the pipeline it is about, by name or id; empty detaches it")
	fl.StringVar(&due, "due", "", "the due date, YYYY-MM-DD; empty removes it")
	fl.StringSliceVar(&addLabels, "add-label", nil, "add a label, by name or id; repeat or comma-separate for several")
	fl.StringSliceVar(&removeLabels, "remove-label", nil, "remove a label, by name or id")
	fl.StringSliceVar(&addAssignees, "add-assignee", nil, "add an assignee by email, name or user id, or me")
	fl.StringSliceVar(&removeAssignees, "remove-assignee", nil, "remove an assignee by email, name or user id, or me")
	fl.StringVar(&parent, "parent", "", "make it a sub-issue of this one (ISS-N); 0 clears the parent")
	fl.StringVar(&duplicateOf, "duplicate-of", "", "mark it a duplicate of this one (ISS-N); 0 clears it")
	fl.StringSliceVar(&addBlockedBy, "add-blocked-by", nil, "an issue blocking this one (ISS-N); repeat for several")
	fl.StringSliceVar(&removeBlockedBy, "remove-blocked-by", nil, "an issue that no longer blocks this one")
	fl.StringSliceVar(&addSubIssue, "add-sub-issue", nil, "make this issue (ISS-N) a sub-issue of this one")
	fl.StringSliceVar(&removeSubIssue, "remove-sub-issue", nil, "release a sub-issue")
	return cmd
}
