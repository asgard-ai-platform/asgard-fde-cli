package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// auditMaxDays is the widest range Console's Explore answers in one query.
const auditMaxDays = 31

// auditReading is what every audit-log answer says about how to read it,
// because the obvious reading of an audit log is the wrong one here.
const auditReading = `Reading it: rows carry raw keys (namespace, IAM user id, CR name) and the names
above come from Console's dictionary - a key it cannot name is shown as itself.
There is no success/failure dimension: events are named, so never summarise them
as "N failed operations". An account can be absent (an external API caller or a
bot). Debug runs are excluded, and the data lags by up to about 20 minutes.`

func newAuditLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit-log",
		Short: "Read the workspace's audit log (Console's Explore)",
		Long: `Read the workspace's audit log: what agents, users and bots did on the
platform, as Asgard Console's Explore records it. The platform relays it with
your own session, and Console decides who may read it - a workspace owner or a
platform admin (IAM action audit-log/read). Anyone else gets Console's 403;
report the 403 instead of an empty summary.

    asgard-cli audit-log summary                 the last 7 days, counted by dimension
    asgard-cli audit-log summary --days 30
    asgard-cli audit-log query --days 1 --event tool_call --limit 50
    asgard-cli audit-log dictionary              the names behind the raw keys

` + auditReading + `

"asgard-cli audit-material" is a different command: it checks this tool's own
reference material.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newAuditLogSummaryCmd(), newAuditLogQueryCmd(), newAuditLogDictionaryCmd())
	return cmd
}

// auditRange reads --days, or --from and --to, into a range Console accepts.
type auditRange struct {
	days     int
	from, to string
}

func (r *auditRange) register(cmd *cobra.Command, defaultDays int) {
	cmd.Flags().IntVar(&r.days, "days", defaultDays, fmt.Sprintf("the last N days, up to now; at most %d", auditMaxDays))
	cmd.Flags().StringVar(&r.from, "from", "", "range start, RFC 3339 (with --to, instead of --days)")
	cmd.Flags().StringVar(&r.to, "to", "", "range end, RFC 3339; defaults to now")
}

func (r *auditRange) resolve(cmd *cobra.Command) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	if r.to != "" {
		t, err := time.Parse(time.RFC3339, r.to)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--to %q is not an RFC 3339 time", r.to)
		}
		to = t
	}
	var from time.Time
	if r.from != "" {
		if cmd.Flags().Changed("days") {
			return time.Time{}, time.Time{}, errors.New("pass --days or --from, not both")
		}
		t, err := time.Parse(time.RFC3339, r.from)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--from %q is not an RFC 3339 time", r.from)
		}
		from = t
	} else {
		if r.days < 1 || r.days > auditMaxDays {
			return time.Time{}, time.Time{}, fmt.Errorf("--days must be between 1 and %d", auditMaxDays)
		}
		from = to.Add(-time.Duration(r.days) * 24 * time.Hour)
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, errors.New("the range must end after it starts")
	}
	if to.Sub(from) > auditMaxDays*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("the range is longer than %d days, which Console does not answer in one query", auditMaxDays)
	}
	return from, to, nil
}

// ---- summary

func newAuditLogSummaryCmd() *cobra.Command {
	var (
		f   workbenchFlags
		rng auditRange
		top int
	)
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Count the audit log by event, account, project and agent",
		Long: `Count the workspace's audit events over a range - by event, by account, by
project, and by bot provider, agent, completion model, toolset and semantic
model - with Console's display names beside the raw keys. Write the summary a
person reads from this output.

    asgard-cli audit-log summary
    asgard-cli audit-log summary --days 30 --top 5
    asgard-cli audit-log summary --format json

` + auditReading,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			from, to, err := rng.resolve(cmd)
			if err != nil {
				return err
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			dict, err := pc.Client.GetAuditDictionary(ctx)
			if err != nil {
				return err
			}
			byDim := map[string][]platform.AuditOptionValue{}
			for _, d := range platform.AuditDimensions {
				opts, err := pc.Client.GetAuditOptions(ctx, d, from, to)
				if err != nil {
					return err
				}
				byDim[d] = opts.Values
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, map[string]any{
					"workspace":  pc.Workspace,
					"from":       from.Format(time.RFC3339),
					"to":         to.Format(time.RFC3339),
					"dimensions": byDim,
					"dictionary": dict,
				})
			}
			fmt.Fprintf(out, "Audit log of workspace %s, %s to %s\n",
				pc.Workspace, from.Local().Format("2006-01-02 15:04"), to.Local().Format("2006-01-02 15:04"))
			titles := map[string]string{
				"event": "events", "user_identity_hint": "accounts", "namespace": "projects",
				"bot_provider": "bot providers", "agent": "agents", "completion_model": "completion models",
				"toolset": "toolsets", "semantic_model": "semantic models",
			}
			if len(byDim["event"]) == 0 {
				// Said, not left as a blank page: an empty answer and a
				// command that printed nothing read the same.
				fmt.Fprintf(out, "\nNo audit events in this range. Console records the workspace's own projects;\n"+
					"the Workbench assistant's own turns are not among them.\n")
			}
			for _, d := range platform.AuditDimensions {
				values := byDim[d]
				if len(values) == 0 {
					continue
				}
				sort.SliceStable(values, func(i, j int) bool { return values[i].Count > values[j].Count })
				fmt.Fprintf(out, "\n%s\n", titles[d])
				for i, v := range values {
					if top > 0 && i == top {
						fmt.Fprintf(out, "  ... and %d more\n", len(values)-top)
						break
					}
					fmt.Fprintf(out, "  %-48s %d\n", auditName(dict, d, v.Value), v.Count)
				}
			}
			fmt.Fprintf(out, "\n%s\n", auditReading)
			return nil
		},
	}
	f.register(cmd)
	rng.register(cmd, 7)
	cmd.Flags().IntVar(&top, "top", 10, "list at most this many values per dimension; 0 lists all")
	return cmd
}

// auditName renders a raw key with its display name, or as itself.
func auditName(d *platform.AuditDictionary, dim, key string) string {
	named := func(name string) string {
		if name == "" || name == key {
			return key
		}
		return name + " (" + key + ")"
	}
	switch dim {
	case "event":
		return named(d.Events[key])
	case "user_identity_hint":
		if key == "" || key == "primary" {
			return "(no account: an API caller or a bot)"
		}
		if a, ok := d.Accounts[key]; ok {
			n := a.Name
			if a.Email != "" {
				n = strings.TrimSpace(n + " <" + a.Email + ">")
			}
			return named(n)
		}
		return key
	case "namespace":
		if p, ok := d.Projects[key]; ok {
			return named(p.Name)
		}
		return key
	}
	// A CR's raw name is only unique within its namespace, and an option
	// value does not say which namespace it came from; name it only when
	// every namespace that knows it agrees.
	var byNS map[string]map[string]string
	switch dim {
	case "bot_provider":
		byNS = d.BotProviders
	case "agent":
		byNS = d.Agents
	case "completion_model":
		byNS = d.CompletionModels
	case "toolset":
		byNS = d.Toolsets
	case "semantic_model":
		byNS = d.SemanticModels
	}
	name := ""
	for _, names := range byNS {
		if n, ok := names[key]; ok {
			if name != "" && name != n {
				return key
			}
			name = n
		}
	}
	return named(name)
}

// ---- query

func newAuditLogQueryCmd() *cobra.Command {
	var (
		f        workbenchFlags
		rng      auditRange
		events   []string
		accounts []string
		keyword  string
		limit    int
		cursor   string
		bodyFile string
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Stream audit events as JSON Lines",
		Long: `Stream the workspace's audit events, as Console's Explore answers them: JSON
Lines on stdout - a {"type":"meta"} line, one {"type":"row"} per event, then
{"type":"end","nextCursor":...}. A failure after the first line is an
{"type":"error"} line, because the status has already been sent. Pass
nextCursor back as --cursor for the next page.

    asgard-cli audit-log query --days 1 --limit 50
    asgard-cli audit-log query --days 7 --event tool_call --event run_error
    asgard-cli audit-log query --body-file query.json      Console's query body as is

--limit is the page size (default 100, at most 500). --account takes IAM user
ids, or primary for events with no account. Rows carry raw keys only;
"asgard-cli audit-log dictionary" names them.

` + auditReading,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var body json.RawMessage
			if bodyFile != "" {
				var raw []byte
				var err error
				if bodyFile == "-" {
					raw, err = io.ReadAll(cmd.InOrStdin())
				} else {
					raw, err = os.ReadFile(bodyFile)
				}
				if err != nil {
					return fmt.Errorf("--body-file: %w", err)
				}
				if !json.Valid(raw) {
					return errors.New("--body-file is not JSON")
				}
				body = raw
			} else {
				from, to, err := rng.resolve(cmd)
				if err != nil {
					return err
				}
				q := map[string]any{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "limit": limit}
				if len(events) > 0 {
					q["events"] = splitList(events)
				}
				if len(accounts) > 0 {
					q["user_identity_hints"] = splitList(accounts)
				}
				if keyword != "" {
					q["keyword"] = keyword
				}
				if cursor != "" {
					q["cursor"] = cursor
				}
				body, _ = json.Marshal(q)
			}
			f.format = formatText
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			stream, err := pc.Client.QueryAuditLog(cmd.Context(), body)
			if err != nil {
				return err
			}
			defer stream.Close()
			_, err = io.Copy(cmd.OutOrStdout(), stream)
			return err
		},
	}
	addProfileFlag(cmd, &f.profile)
	cmd.Flags().StringVar(&f.workspace, workspaceFlag, "",
		"workspace to act in; defaults to ASGARD_WORKSPACE, then what \"asgard-cli workspace use\" recorded for this repository")
	rng.register(cmd, 1)
	cmd.Flags().StringSliceVar(&events, "event", nil, "only these events (user_prompt, assistant_response, tool_call, subagent_start, subagent_complete, model_usage, run_done, run_error, other); repeat or comma-separate")
	cmd.Flags().StringSliceVar(&accounts, "account", nil, "only these IAM user ids, or primary for events with no account")
	cmd.Flags().StringVar(&keyword, "keyword", "", "case-insensitive substring of the event content")
	cmd.Flags().IntVar(&limit, "limit", 100, "page size, at most 500")
	cmd.Flags().StringVar(&cursor, "cursor", "", "the nextCursor of the previous page")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "send this file as Console's query body instead of building one from the flags; - reads stdin")
	return cmd
}

// ---- dictionary

func newAuditLogDictionaryCmd() *cobra.Command {
	var f workbenchFlags
	cmd := &cobra.Command{
		Use:   "dictionary",
		Short: "Print the names behind the audit log's raw keys",
		Long: `Print Console's dictionary for the workspace, as JSON: product, event,
project and account names, and per namespace the display names of bot
providers, agents, completion models, toolsets and semantic models. A raw key
it does not carry is shown as itself.

    asgard-cli audit-log dictionary`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f.format = formatText
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			dict, err := pc.Client.GetAuditDictionary(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), dict)
		},
	}
	addProfileFlag(cmd, &f.profile)
	cmd.Flags().StringVar(&f.workspace, workspaceFlag, "",
		"workspace to act in; defaults to ASGARD_WORKSPACE, then what \"asgard-cli workspace use\" recorded for this repository")
	return cmd
}
