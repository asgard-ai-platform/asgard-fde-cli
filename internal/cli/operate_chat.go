package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// The group `chat` is listed under in `operate`.
const operateGroupConversation = "conversation"

// chatTimeoutDefault bounds one chat command. It is under the Workbench
// sandbox's cut-off, so a long turn ends with a way to follow it rather than
// with the sandbox killing the command.
const chatTimeoutDefault = 8 * time.Minute

func newOperateChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Talk to what a release deployed: an Agent, a workflow set, a Trigger's run, a context index",
		Long: `Talk to what a release deployed, the way the Console's previews do.

    asgard-cli operate chat send agent/<name> "<text>" --release <r>
    asgard-cli operate chat send workflow-set/<id> "<text>" --release <r>
    asgard-cli operate chat send trigger/<name> "<text>" --invocation <id> --release <r>
    asgard-cli operate chat send context-index/<source-set> "<text>" --release <r>
    asgard-cli operate chat replay|status|stop|reset <target> --release <r>

The target says which conversation:

    agent/<name>                 the Agent's preview, on the project's Agent Hub
    workflow-set/<id>            a flow agent's preview; the id is the
                                 asgard-ai.com/workflow-set-id its Workflows carry
    trigger/<name>               one invocation's conversation - what the
                                 Trigger said and what the agent did - which
                                 --invocation names; a reply continues it
    context-index/<source-set>   the conversation for improving the index, or
                                 with --invocation one refresh's

A preview's channel is the Console's, not one of its own: it is derived from
the agent and the signed-in account the way the Console derives it, so the
member's preview in the Console and this command are one conversation. Every
channel holds a sandbox that outlives the last message, which is why there is
one per account rather than one per run. "send --new" starts it over, and the
Console does the same when its preview page opens, so a conversation here
does not survive somebody opening that page. "reset" releases it when done.

This is for testing what was deployed, not for using it: no attachments, no
feedback, and nothing of the sandbox's own files or browser. Those are the
Console's. A test that needs them is the member's to run there.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newChatSendCmd(), newChatReplayCmd(), newChatStatusCmd(), newChatStopCmd(), newChatResetCmd())
	return cmd
}

// chatFlags are the flags every chat command carries.
type chatFlags struct {
	operateFlags
	invocation string
	timeout    time.Duration
}

func (f *chatFlags) register(cmd *cobra.Command, withTimeout bool) {
	f.operateFlags.register(cmd)
	cmd.Flags().StringVar(&f.invocation, "invocation", "",
		"the invocation whose conversation to use; required for trigger/, and for context-index/ a refresh instead of the improve conversation")
	if withTimeout {
		cmd.Flags().DurationVar(&f.timeout, "timeout", chatTimeoutDefault,
			"how long to follow the run before returning with a way to follow it later")
	}
}

// chatConv is one conversation, located.
type chatConv struct {
	Kind, Name string
	Target     platform.ChatTarget
	Channel    string
	// preview is set for agent/ and workflow-set/, whose channel is the
	// Console's per-account one.
	preview bool
	pc      *platformContext
	scope   *operateScope
	f       *chatFlags
}

// target repeats the target for a command a message names.
func (c *chatConv) target() string { return c.Kind + "/" + c.Name }

// next is a chat command on this conversation, with its scope.
func (c *chatConv) next(verb string) string {
	inv := ""
	if c.f.invocation != "" {
		inv = " --invocation " + c.f.invocation
	}
	return fmt.Sprintf("asgard-cli operate chat %s %s%s %s", verb, c.target(), inv, c.f.scopeFlag())
}

// resolveChat parses the target and finds its conversation.
func resolveChat(cmd *cobra.Command, f *chatFlags, target string) (*chatConv, error) {
	kind, name, ok := strings.Cut(target, "/")
	if !ok || name == "" {
		return nil, fmt.Errorf("the target is <kind>/<name>: agent/, workflow-set/, trigger/ or context-index/; got %q", target)
	}
	switch kind {
	case "agent", "workflow-set":
		if f.invocation != "" {
			return nil, fmt.Errorf("--invocation names a Trigger's or a context index's run; a %s preview has one conversation per account", kind)
		}
	case "trigger":
		if f.invocation == "" {
			return nil, fmt.Errorf("--invocation is required for a Trigger: each invocation is its own conversation, and `asgard-cli operate trigger runs %s` lists them", name)
		}
	case "context-index":
	default:
		return nil, fmt.Errorf("unknown target kind %q; one of agent/, workflow-set/, trigger/, context-index/", kind)
	}

	pc, scope, err := f.resolve(cmd)
	if err != nil {
		return nil, err
	}
	ctx := cmd.Context()
	c := &chatConv{Kind: kind, Name: name, pc: pc, scope: scope, f: f}
	project := scope.ProjectID

	switch kind {
	case "agent":
		if _, err := scope.object("Agent", name); err != nil {
			return nil, err
		}
		user, err := pc.Client.CurrentUserID(ctx)
		if err != nil {
			return nil, err
		}
		c.Target, c.Channel, c.preview = platform.AgentChat(name), platform.PreviewChannelID(platform.AgentPreviewScope(name, user)), true
	case "workflow-set":
		if err := scope.workflowSet(name); err != nil {
			return nil, err
		}
		env, err := scopeEnvironment(ctx, pc, scope)
		if err != nil {
			return nil, err
		}
		// Checked before anything is made: the platform would make a preview
		// bot for a workflow set that has no conversation to have.
		typ, err := pc.Client.WorkflowSetType(ctx, project, name, env)
		if err != nil {
			return nil, err
		}
		if typ != "" && typ != "bot" {
			return nil, fmt.Errorf("workflow set %s is of type %s, which is called as a tool and has no conversation; only a bot - a flow agent - has a preview", name, typ)
		}
		user, err := pc.Client.CurrentUserID(ctx)
		if err != nil {
			return nil, err
		}
		c.Target, c.Channel, c.preview = platform.WorkflowSetChat(name, env), platform.PreviewChannelID(platform.FlowPreviewScope(user)), true
	case "trigger":
		if _, err := scope.object("Trigger", name); err != nil {
			return nil, err
		}
		c.Target, c.Channel = platform.TriggerChat(name), f.invocation
	case "context-index":
		cr, err := scope.object("SourceSet", name)
		if err != nil {
			return nil, err
		}
		if cr != nil {
			if _, ok := cr.Spec["contextIndex"]; !ok {
				return nil, fmt.Errorf("SourceSet %s in %s has no contextIndex, so there is no index to talk about", name, scope.Where)
			}
		}
		c.Target, c.Channel = platform.ContextIndexChat(name), f.invocation
		if c.Channel == "" {
			if c.Channel, err = pc.Client.ContextIndexChannel(ctx, project, name); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

// workflowSet checks the release deployed a Workflow of this workflow set.
func (s *operateScope) workflowSet(id string) error {
	if s.live == nil {
		return nil
	}
	var known []string
	for _, o := range s.live.Objects {
		if o.Kind != "Workflow" || !o.Found {
			continue
		}
		var cr liveCR
		if yaml.Unmarshal([]byte(o.Yaml), &cr) != nil {
			continue
		}
		label := cr.Metadata.Labels["asgard-ai.com/workflow-set-id"]
		if label == id {
			return nil
		}
		if label != "" && !contains(known, label) {
			known = append(known, label)
		}
	}
	if len(known) == 0 {
		return fmt.Errorf("%s deployed no Workflow with a workflow-set-id, so it has no workflow set to preview", s.Where)
	}
	return fmt.Errorf("%s deployed no workflow set %q; the ids its Workflows carry: %s", s.Where, id, strings.Join(known, ", "))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// scopeEnvironment is the environment a workflow-set preview is made for:
// the release's, or the project's main one.
func scopeEnvironment(ctx context.Context, pc *platformContext, scope *operateScope) (string, error) {
	if scope.release != nil && scope.release.ProjectEnvironmentId != "" {
		return scope.release.ProjectEnvironmentId, nil
	}
	env, err := pc.Client.MainEnvironment(ctx, scope.ProjectID)
	if err != nil {
		return "", err
	}
	if env == nil {
		return "", fmt.Errorf("%s has no main environment, and a workflow set's preview is made for one", scope.Where)
	}
	return env.Key, nil
}

func newChatSendCmd() *cobra.Command {
	var (
		f        chatFlags
		fresh    bool
		consents []string
		effort   string
		model    string
	)
	cmd := &cobra.Command{
		Use:   "send <target> [text]",
		Short: "Send one turn and follow the run to its end",
		Long: `Send one turn to a conversation and follow the run until it ends.

    asgard-cli operate chat send agent/support "What is the return policy?" --release internal-dev
    asgard-cli operate chat send agent/support --consent toolu_01=allow-once --release internal-dev

The reply streams to standard output as it arrives. Tool calls, subagents and
sandboxes starting are noted on standard error, and the last lines there name
the channel and what to run next. --format json prints every frame as one JSON
line instead, as the platform sent it.

An Agent's preview runs on the project's Agent Hub, and the first message of a
conversation is sent as "@<alias> <text>", as the Console sends it, so Agent Hub
hands the turn to that agent rather than answering itself. Later turns go as
typed.

The exit code says how the run ended:

    0   the run finished
    1   the run failed - the error names the workflow and processor it came
        from - or the stream ended without saying, or --timeout ran out first.
        A run still going is followed with "replay"
    1   the agent wants permission for a tool call. The pending calls are
        listed with their ids; answer with --consent <id>=allow-once,
        allow-always or deny, one per call, and no text

A turn is sent once. A stream that drops is rejoined from where it stopped,
never sent again, because two of the relays would start the turn twice.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := ""
			if len(args) == 2 {
				text = args[1]
			}
			answers, err := parseConsents(consents)
			if err != nil {
				return err
			}
			switch {
			case len(answers) > 0 && text != "":
				return fmt.Errorf("a consent answer carries no text; send the text as its own turn after the run continues")
			case len(answers) == 0 && strings.TrimSpace(text) == "":
				return fmt.Errorf("nothing to send: give the text, or --consent to answer a pending tool call")
			case (effort != "" || model != "") && !strings.HasPrefix(args[0], "agent/"):
				return fmt.Errorf("--effort and --model choose how an Agent preview runs; they mean nothing to %s", args[0])
			}
			conv, err := resolveChat(cmd, &f, args[0])
			if err != nil {
				return err
			}
			if fresh && conv.Kind == "trigger" {
				return fmt.Errorf("--new cannot start an invocation over; fire the Trigger for a new one")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), f.timeout)
			defer cancel()
			client, project, errOut := conv.pc.Client, conv.scope.ProjectID, cmd.ErrOrStderr()

			actingOn(cmd, conv.pc.Session)
			if conv.Kind == "workflow-set" {
				created, err := client.EnsureWorkflowSetPreview(ctx, project, conv.Name, conv.Target.Environment)
				if err != nil {
					return err
				}
				if created {
					fmt.Fprintf(errOut, "made workflow set %s's preview bot, as the Console does on its first preview\n", conv.Name)
				}
			}
			firstTurn := fresh
			if fresh {
				fmt.Fprintf(errOut, "starting %s over; a conversation with a sandbox takes up to a minute to release\n", conv.target())
				if err := client.DeleteChat(ctx, project, conv.Target, conv.Channel); err != nil && !platform.NotFound(err) {
					return err
				}
			} else if conv.Kind == "agent" {
				if _, err := client.ChannelInfo(ctx, project, conv.Target, conv.Channel); platform.NotFound(err) {
					firstTurn = true
				} else if err != nil {
					return err
				}
			}

			msg := platform.ChatMessage{CustomChannelID: conv.Channel, CustomMessageID: platform.NewMessageID(), Text: text, Action: platform.ChatActionNone}
			if len(answers) > 0 {
				msg.Action, msg.ToolCallConsents = platform.ChatActionConsent, answers
			}
			if conv.Kind == "agent" {
				if firstTurn && msg.Action == platform.ChatActionNone {
					alias, err := client.AgentAlias(ctx, project, conv.Name)
					if err != nil {
						return err
					}
					if alias != "" {
						msg.Text = "@" + alias + " " + text
					}
				}
				msg.Payload = agentPayload(model, effort)
			}

			p := newChatPrinter(cmd, f.format == formatJSON, false)
			err = client.SendChat(ctx, project, conv.Target, msg, p.event)
			return p.finish(ctx, conv, err)
		},
	}
	f.register(cmd, true)
	cmd.Flags().BoolVar(&fresh, "new", false,
		"delete the conversation and start it over first; for a preview this is what the Console does when its page opens")
	cmd.Flags().StringArrayVar(&consents, "consent", nil,
		"answer a pending tool call: <toolCallId>=allow-once, allow-always or deny; repeat for each, with no text")
	cmd.Flags().StringVar(&effort, "effort", "", "agent/ only: the reasoning effort for this turn, as the Console's effort menu offers; the model's default when not given")
	cmd.Flags().StringVar(&model, "model", "", "agent/ only: the CompletionModel CR to run this turn on; Agent Hub's default when not given")
	return cmd
}

// agentPayload is what an Agent preview's turn carries beside the text. The
// platform adds the agent's name itself.
func agentPayload(model, effort string) map[string]any {
	p := map[string]any{}
	if model != "" {
		p["agent_hub"] = map[string]any{"completion_model_name": model}
	}
	if effort != "" {
		p["effort"] = effort
	}
	if len(p) == 0 {
		return nil
	}
	return p
}

// parseConsents reads --consent values.
func parseConsents(values []string) ([]platform.ToolCallConsent, error) {
	var out []platform.ToolCallConsent
	for _, v := range values {
		id, answer, ok := strings.Cut(v, "=")
		result := map[string]string{"allow-once": "ALLOW_ONCE", "allow-always": "ALLOW_ALWAYS", "deny": "DENY_ONCE"}[answer]
		if !ok || id == "" || result == "" {
			return nil, fmt.Errorf("--consent is <toolCallId>=allow-once, allow-always or deny; got %q", v)
		}
		out = append(out, platform.ToolCallConsent{ToolCallID: id, Result: result})
	}
	return out, nil
}

func newChatReplayCmd() *cobra.Command {
	var f chatFlags
	cmd := &cobra.Command{
		Use:   "replay <target>",
		Short: "Print a conversation so far, and follow its run if one is going",
		Long: `Print a conversation so far, and follow its run to its end if one is going.

    asgard-cli operate chat replay trigger/daily-report --invocation 6f1c... --release internal-dev

The history is what each turn ended as: the user's messages, marked with ">",
the replies, and the tool calls on standard error. A run still going is
followed as "send" follows it, with the same exit codes. Nothing is sent, so
this is safe to run on a conversation somebody else is having.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conv, err := resolveChat(cmd, &f, args[0])
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), f.timeout)
			defer cancel()
			if _, err := conv.pc.Client.ChannelInfo(ctx, conv.scope.ProjectID, conv.Target, conv.Channel); err != nil {
				if platform.NotFound(err) {
					return fmt.Errorf("%s has no conversation: it was never started, or it was released or reaped", conv.target())
				}
				return err
			}
			p := newChatPrinter(cmd, f.format == formatJSON, true)
			err = conv.pc.Client.RejoinChat(ctx, conv.scope.ProjectID, conv.Target, conv.Channel, p.event)
			return p.finish(ctx, conv, err)
		},
	}
	f.register(cmd, true)
	return cmd
}

func newChatStatusCmd() *cobra.Command {
	var f chatFlags
	cmd := &cobra.Command{
		Use:   "status <target>",
		Short: "Whether a conversation is running, and whether the agent is waiting on an answer",
		Long: `Whether a conversation is running, and whether the agent is waiting on an answer.

    asgard-cli operate chat status agent/support --release internal-dev

RUNNING means a run is going; IDLE that none is. The agent's own verdict
follows: NEEDS_INPUT is a question waiting, COMPLETED that it considers the
work done, and nothing when it has not said. It exits non-zero when the
conversation does not exist. --format json prints the platform's answer.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conv, err := resolveChat(cmd, &f, args[0])
			if err != nil {
				return err
			}
			info, err := conv.pc.Client.ChannelInfo(cmd.Context(), conv.scope.ProjectID, conv.Target, conv.Channel)
			if err != nil {
				if platform.NotFound(err) {
					return fmt.Errorf("%s has no conversation: it was never started, or it was released or reaped", conv.target())
				}
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, info)
			}
			writeChannel(out, conv, info)
			return nil
		},
	}
	f.register(cmd, false)
	return cmd
}

func writeChannel(out io.Writer, conv *chatConv, info *platform.ChannelMetadata) {
	fmt.Fprintf(out, "%-10s %s\n", "channel", conv.Channel)
	if info.Title != "" {
		fmt.Fprintf(out, "%-10s %s\n", "title", info.Title)
	}
	state := info.RunState
	if info.ConversationStatus != "" {
		state += " " + info.ConversationStatus
	}
	fmt.Fprintf(out, "%-10s %s\n", "state", state)
	if info.LastActivityAt > 0 {
		fmt.Fprintf(out, "%-10s %s\n", "active", time.UnixMilli(info.LastActivityAt).Local().Format("2006-01-02 15:04:05"))
	}
	if len(info.LaunchedSandboxes) > 0 {
		var names []string
		for _, s := range info.LaunchedSandboxes {
			names = append(names, s.SandboxName+" ("+s.SandboxBlueprintName+")")
		}
		fmt.Fprintf(out, "%-10s %s - held until the conversation is reset or idles out\n", "sandboxes", strings.Join(names, ", "))
	}
}

func newChatStopCmd() *cobra.Command {
	var f chatFlags
	cmd := &cobra.Command{
		Use:   "stop <target>",
		Short: "Ask the run going in a conversation to stop",
		Long: `Ask the run going in a conversation to stop.

    asgard-cli operate chat stop agent/support --release internal-dev

The platform answers that the request was accepted; the run ends on its own
stream, which "replay" follows. The conversation stays, and the next "send"
continues it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conv, err := resolveChat(cmd, &f, args[0])
			if err != nil {
				return err
			}
			actingOn(cmd, conv.pc.Session)
			if err := conv.pc.Client.StopChat(cmd.Context(), conv.scope.ProjectID, conv.Target, conv.Channel); err != nil && !platform.NotFound(err) {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Asked the run in %s to stop. Whether it has:\n\n    %s\n", conv.target(), conv.next("status"))
			return nil
		},
	}
	f.register(cmd, false)
	return cmd
}

func newChatResetCmd() *cobra.Command {
	var f chatFlags
	cmd := &cobra.Command{
		Use:   "reset <target>",
		Short: "Delete a conversation and release its sandbox",
		Long: `Delete a conversation: its transcript, its uploads and its sandbox.

    asgard-cli operate chat reset agent/support --release internal-dev

Run it when a test is done. A sandbox otherwise stays up for a while after the
last message, and it is the member's: a preview's conversation is the one the
Console shows them, so this clears it there too. It waits until the sandbox is
gone, which can take most of a minute.

A Trigger's invocation is not reset here: its conversation is the record of
what the Trigger did.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conv, err := resolveChat(cmd, &f, args[0])
			if err != nil {
				return err
			}
			if conv.Kind == "trigger" {
				return fmt.Errorf("a Trigger's invocation is the record of what it did, so it is not reset from here")
			}
			actingOn(cmd, conv.pc.Session)
			if err := conv.pc.Client.DeleteChat(cmd.Context(), conv.scope.ProjectID, conv.Target, conv.Channel); err != nil {
				if platform.NotFound(err) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s has no conversation to reset.\n", conv.target())
					return nil
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Reset %s; its sandbox is released.\n", conv.target())
			return nil
		},
	}
	f.register(cmd, false)
	return cmd
}

// chatPrinter renders frames as they arrive and remembers how the run ended.
type chatPrinter struct {
	out, errOut io.Writer
	json        bool
	replay      bool
	// streamed marks the messages whose text arrived as deltas, so their
	// complete frame is not printed a second time.
	streamed map[string]bool
	// started marks the tool calls already noted from their start frame.
	started   map[string]bool
	midLine   bool
	runError  *chatRunError
	pending   []chatPending
	verdict   string
	writeErr  error
	terminals int
}

type chatRunError struct {
	Message  string `json:"message"`
	Code     string `json:"code"`
	Inner    string `json:"inner"`
	Location struct {
		WorkflowName  string `json:"workflowName"`
		ProcessorName string `json:"processorName"`
		ProcessorType string `json:"processorType"`
	} `json:"location"`
}

type chatPending struct {
	ToolCallID  string `json:"toolCallId"`
	ToolsetName string `json:"toolsetName"`
	ToolName    string `json:"toolName"`
	Reason      string `json:"reason"`
}

type chatMessage struct {
	MessageID       string `json:"messageId"`
	Text            string `json:"text"`
	ParentToolUseID string `json:"parentToolUseId"`
}

type chatTool struct {
	ToolUseID       string `json:"toolUseId"`
	ParentToolUseID string `json:"parentToolUseId"`
	IsError         bool   `json:"isError"`
	ToolCall        struct {
		ToolsetName string `json:"toolsetName"`
		ToolName    string `json:"toolName"`
	} `json:"toolCall"`
}

// chatFrame is the part of a frame the text form reads.
type chatFrame struct {
	Fact struct {
		MessageDelta    *struct{ Message chatMessage } `json:"messageDelta"`
		MessageComplete *struct{ Message chatMessage } `json:"messageComplete"`
		MessageUser     *struct {
			Text string `json:"text"`
		} `json:"messageUser"`
		ToolCallStart    *chatTool `json:"toolCallStart"`
		ToolCallComplete *chatTool `json:"toolCallComplete"`
		ToolCallConsent  *struct {
			PendingCalls []chatPending `json:"pendingCalls"`
		} `json:"toolCallConsent"`
		RunError *struct {
			Error chatRunError `json:"error"`
		} `json:"runError"`
		SubagentStart *struct {
			SubagentType string `json:"subagentType"`
			Description  string `json:"description"`
		} `json:"subagentStart"`
		SandboxLaunch *struct {
			SandboxName string `json:"sandboxName"`
		} `json:"sandboxLaunch"`
		ChannelStatusUpdate *struct {
			Status string `json:"status"`
		} `json:"channelStatusUpdate"`
	} `json:"fact"`
}

func newChatPrinter(cmd *cobra.Command, asJSON, replay bool) *chatPrinter {
	return &chatPrinter{out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), json: asJSON, replay: replay,
		streamed: map[string]bool{}, started: map[string]bool{}}
}

func (p *chatPrinter) event(ev platform.ChatEvent) error {
	var f chatFrame
	_ = json.Unmarshal(ev.Data, &f)
	fact := f.Fact
	switch {
	case fact.RunError != nil:
		e := fact.RunError.Error
		p.runError = &e
	case fact.ToolCallConsent != nil:
		p.pending = fact.ToolCallConsent.PendingCalls
	case fact.ChannelStatusUpdate != nil:
		p.verdict = fact.ChannelStatusUpdate.Status
	}
	if ev.Terminal() {
		p.terminals++
	}
	if p.json {
		compact := strings.ReplaceAll(strings.ReplaceAll(string(ev.Data), "\n", " "), "\r", "")
		_, err := fmt.Fprintln(p.out, compact)
		return err
	}

	switch {
	case fact.MessageDelta != nil && fact.MessageDelta.Message.ParentToolUseID == "":
		m := fact.MessageDelta.Message
		p.streamed[m.MessageID] = true
		p.write(m.Text)
	case fact.MessageComplete != nil:
		m := fact.MessageComplete.Message
		if m.ParentToolUseID != "" {
			p.note("subagent replied: %s", oneLine(m.Text, 160))
			break
		}
		if !p.streamed[m.MessageID] {
			p.write(m.Text)
		}
		p.endLine()
	case fact.MessageUser != nil:
		p.endLine()
		p.write("> " + fact.MessageUser.Text)
		p.endLine()
	case fact.ToolCallStart != nil:
		t := fact.ToolCallStart
		if t.ToolUseID != "" {
			p.started[t.ToolUseID] = true
		}
		p.note("%stool %s", subagentMark(t.ParentToolUseID), toolName(t))
	case fact.ToolCallComplete != nil && fact.ToolCallComplete.IsError:
		t := fact.ToolCallComplete
		p.note("%stool %s returned an error", subagentMark(t.ParentToolUseID), toolName(t))
	case fact.ToolCallComplete != nil && p.replay && (fact.ToolCallComplete.ToolUseID == "" || !p.started[fact.ToolCallComplete.ToolUseID]):
		// History replays what each step ended as, so a tool call arrives
		// only as its completion.
		t := fact.ToolCallComplete
		p.note("%stool %s", subagentMark(t.ParentToolUseID), toolName(t))
	case fact.SubagentStart != nil:
		p.note("subagent %s: %s", fact.SubagentStart.SubagentType, oneLine(fact.SubagentStart.Description, 120))
	case fact.SandboxLaunch != nil:
		p.note("sandbox %s starting", fact.SandboxLaunch.SandboxName)
	}
	return p.writeErr
}

// toolName is a tool as the agent called it, with its toolset when it has one.
func toolName(t *chatTool) string {
	if t.ToolCall.ToolsetName == "" {
		return t.ToolCall.ToolName
	}
	return t.ToolCall.ToolsetName + "." + t.ToolCall.ToolName
}

func subagentMark(parent string) string {
	if parent != "" {
		return "subagent "
	}
	return ""
}

// oneLine collapses s to one line of at most n characters - characters, not
// bytes, so a reply in Chinese is not cut inside one.
func oneLine(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return string(r)
}

func (p *chatPrinter) write(s string) {
	if s == "" {
		return
	}
	if _, err := io.WriteString(p.out, s); err != nil && p.writeErr == nil {
		p.writeErr = err
	}
	p.midLine = !strings.HasSuffix(s, "\n")
}

func (p *chatPrinter) endLine() {
	if p.midLine {
		p.write("\n")
	}
}

func (p *chatPrinter) note(format string, args ...any) {
	fmt.Fprintf(p.errOut, "  "+format+"\n", args...)
}

// finish turns how the stream ended into the command's result.
func (p *chatPrinter) finish(ctx context.Context, conv *chatConv, err error) error {
	if !p.json {
		p.endLine()
	}
	fail := func(e error) error {
		if p.json {
			return ErrSilent
		}
		return e
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return fail(fmt.Errorf("the run in %s is still going after --timeout %s. Follow it:\n\n    %s", conv.target(), conv.f.timeout, conv.next("replay")))
	case errors.Is(err, platform.ErrStreamEnded):
		// Two relays end a failed run by closing the stream; the channel
		// still says how it ended.
		info, infoErr := conv.pc.Client.ChannelInfo(context.Background(), conv.scope.ProjectID, conv.Target, conv.Channel)
		if infoErr == nil {
			return fail(fmt.Errorf("the stream for %s ended without saying the run was done; the channel is %s %s. Read what happened:\n\n    %s",
				conv.target(), info.RunState, info.ConversationStatus, conv.next("replay")))
		}
		return fail(err)
	case err != nil:
		return err
	case p.runError != nil && !p.replay:
		e := p.runError
		where := ""
		if l := e.Location; l.WorkflowName != "" || l.ProcessorName != "" {
			where = fmt.Sprintf(" (workflow %s, processor %s, %s)", l.WorkflowName, l.ProcessorName, l.ProcessorType)
		}
		msg := e.Message
		if e.Inner != "" {
			msg += ": " + e.Inner
		}
		return fail(fmt.Errorf("the run failed%s: %s", where, msg))
	case len(p.pending) > 0:
		if p.json {
			return ErrSilent
		}
		var b strings.Builder
		fmt.Fprintf(&b, "the agent wants permission before calling a tool, and the run is paused until it has an answer:\n")
		var flags []string
		for _, c := range p.pending {
			fmt.Fprintf(&b, "    %s  %s.%s", c.ToolCallID, c.ToolsetName, c.ToolName)
			if c.Reason != "" {
				fmt.Fprintf(&b, "  (%s)", oneLine(c.Reason, 100))
			}
			b.WriteByte('\n')
			flags = append(flags, "--consent "+c.ToolCallID+"=allow-once")
		}
		fmt.Fprintf(&b, "Answer each, allow-once, allow-always or deny:\n\n    asgard-cli operate chat send %s %s %s",
			conv.target(), strings.Join(flags, " "), conv.f.scopeFlag())
		return errors.New(b.String())
	}
	if !p.json {
		verdict := ""
		if p.verdict != "" {
			verdict = ", and the agent says " + p.verdict
		}
		fmt.Fprintf(p.errOut, "\nchannel %s%s. Next turn:\n    %s \"<text>\"\n", conv.Channel, verdict, conv.next("send"))
		if conv.preview {
			fmt.Fprintf(p.errOut, "Done testing: %s\n", conv.next("reset"))
		}
	}
	return nil
}
