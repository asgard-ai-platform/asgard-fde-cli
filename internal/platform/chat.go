package platform

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The conversations a deployed CR holds, over the platform's chat relays.
//
// Four relays share one protocol and differ only in where they live: an Agent's
// preview, a workflow set's preview, a Trigger's invocations and a SourceSet's
// context index. Each is a base path below which the same sub-paths answer -
// /message/sse to send a turn or rejoin, /channel/metadata, /message/suspend,
// /channel. A conversation is a channel, named by a custom channel id.
//
// This is a small client of the relay rather than asgard-sdk-go's: that SDK
// builds asgard-core's paths, sends an API key rather than the member's
// session, and decodes a different envelope, so it cannot reach these routes.
// Frames are passed through as raw JSON, so a field this build does not know
// survives to a caller printing them.

// ChatTarget is one relay: its base path, and the environment header the
// workflow-set preview finds its bot by.
type ChatTarget struct {
	Base        string
	Environment string
}

// AgentChat is an Agent's preview. It runs on the project's shared Agent Hub,
// and the platform names the agent to it from the path.
func AgentChat(agent string) ChatTarget {
	return ChatTarget{Base: "/v1/agent/" + url.PathEscape(agent) + "/chat"}
}

// WorkflowSetChat is a workflow set's preview, on the preview bot made for it
// in one environment.
func WorkflowSetChat(workflowSet, environment string) ChatTarget {
	return ChatTarget{Base: "/v1/workflow-set/" + url.PathEscape(workflowSet) + "/preview", Environment: environment}
}

// TriggerChat is a Trigger's invocations; each invocation's channel id is the
// invocation id.
func TriggerChat(trigger string) ChatTarget {
	return ChatTarget{Base: "/v1/trigger/" + url.PathEscape(trigger) + "/chat"}
}

// ContextIndexChat is a SourceSet's context index: its refreshes, by
// invocation id, and the one conversation for improving it.
func ContextIndexChat(sourceSet string) ChatTarget {
	return ChatTarget{Base: "/v1/source-set/" + url.PathEscape(sourceSet) + "/context-index/chat"}
}

// PreviewChannelID is the channel id the Console gives a preview: "preview_"
// and the first 32 hex characters of SHA-256 of the scope. It is derived
// rather than invented because every channel holds resources - its sandbox
// outlives the last message by a fixed time - so a member has one preview
// channel per agent, shared between the Console and this CLI.
//
// The rule is asgard-ai-platform-web's src/utils/preview-channel.ts, and the
// two must not drift: a different id is a second channel, and a second sandbox.
func PreviewChannelID(scope string) string {
	sum := sha256.Sum256([]byte(scope))
	return "preview_" + hex.EncodeToString(sum[:])[:32]
}

// AgentPreviewScope and FlowPreviewScope are the scopes the Console hashes.
// An Agent's preview runs on the project's shared Agent Hub, so the agent is
// part of the scope; a workflow set's preview bot is already its own.
func AgentPreviewScope(agent, user string) string { return "agent:" + agent + ":" + user }

// FlowPreviewScope: see AgentPreviewScope.
func FlowPreviewScope(user string) string { return "flow:" + user }

// ChatMessage is one turn sent to a channel.
type ChatMessage struct {
	CustomChannelID string `json:"customChannelId"`
	CustomMessageID string `json:"customMessageId"`
	Text            string `json:"text"`
	// Action is "NONE" for a turn, or "RESPONSE_TOOL_CALL_CONSENT" for an
	// answer to a consent prompt.
	Action           string            `json:"action"`
	Payload          map[string]any    `json:"payload,omitempty"`
	ToolCallConsents []ToolCallConsent `json:"toolCallConsents,omitempty"`
}

// ToolCallConsent answers one pending tool call.
type ToolCallConsent struct {
	ToolCallID string `json:"toolCallId"`
	// Result is ALLOW_ONCE, ALLOW_ALWAYS or DENY_ONCE.
	Result     string `json:"result"`
	DenyReason string `json:"denyReason,omitempty"`
}

// The actions a turn takes.
const (
	ChatActionNone    = "NONE"
	ChatActionConsent = "RESPONSE_TOOL_CALL_CONSENT"
)

// NewMessageID is a fresh id for one message. It names the message, not the
// channel, so it is random.
func NewMessageID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ChatEvent is one frame of a conversation: its event type, its cursor, and
// the frame's data as the relay sent it.
type ChatEvent struct {
	Type string
	ID   string
	Data json.RawMessage
}

// The event types this client acts on. The rest are passed through.
const (
	EventRunDone         = "asgard.run.done"
	EventRunError        = "asgard.run.error"
	EventMessageDelta    = "asgard.message.delta"
	EventMessageComplete = "asgard.message.complete"
	EventMessageUser     = "asgard.message.user"
	EventToolCallStart   = "asgard.tool_call.start"
	EventToolCallDone    = "asgard.tool_call.complete"
	EventToolCallConsent = "asgard.tool_call.consent"
	EventSubagentStart   = "asgard.subagent.start"
	EventSubagentDone    = "asgard.subagent.complete"
	EventSandboxLaunch   = "asgard.sandbox.launch"
	EventChannelStatus   = "asgard.channel.status.update"
)

// Terminal reports whether an event ends a run.
func (e ChatEvent) Terminal() bool {
	return e.Type == EventRunDone || e.Type == EventRunError
}

// ErrStreamEnded is a stream that closed without a terminal event and could
// not be resumed. Two of the relays end a failed run that way instead of
// sending its error, so the caller reads the channel's metadata to say more.
var ErrStreamEnded = errors.New("the stream ended without saying the run was done")

// SendChat sends one turn and calls on for every frame until the run ends.
//
// A stream that drops after it has delivered a cursor is resumed by rejoining
// the channel from that cursor. It is never resumed by sending the turn again:
// two of the relays do not pass the cursor on with a send, so a second send
// would start the turn twice.
func (c *Client) SendChat(ctx context.Context, project string, t ChatTarget, msg ChatMessage, on func(ChatEvent) error) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode the message: %w", err)
	}
	resp, err := c.doRaw(ctx, rawRequest{
		method: http.MethodPost, path: t.Base + "/message/sse", project: project, environment: t.Environment,
		body: strings.NewReader(string(body)), contentType: "application/json", accept: "text/event-stream",
		noTimeout: true,
	})
	if err != nil {
		return err
	}
	return c.follow(ctx, project, t, msg.CustomChannelID, resp, on)
}

// RejoinChat replays a channel's history and follows its live run, if one is
// going, to its end. A channel with no run going ends after the history.
func (c *Client) RejoinChat(ctx context.Context, project string, t ChatTarget, channel string, on func(ChatEvent) error) error {
	resp, err := c.rejoin(ctx, project, t, channel, "")
	if err != nil {
		return err
	}
	return c.follow(ctx, project, t, channel, resp, on)
}

func (c *Client) rejoin(ctx context.Context, project string, t ChatTarget, channel, cursor string) (*http.Response, error) {
	return c.doRaw(ctx, rawRequest{
		method: http.MethodGet, path: t.Base + "/message/sse", query: url.Values{"custom_channel_id": {channel}},
		project: project, environment: t.Environment, accept: "text/event-stream", lastEventID: cursor, noTimeout: true,
	})
}

// resumeBackoff is the wait before each attempt to resume a dropped stream.
var resumeBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// follow reads frames until a terminal one, resuming from the last cursor
// when the stream ends early.
func (c *Client) follow(ctx context.Context, project string, t ChatTarget, channel string, resp *http.Response, on func(ChatEvent) error) error {
	cursor := ""
	for attempt := 0; ; attempt++ {
		done, last, err := readFrames(resp.Body, on)
		resp.Body.Close()
		if last != "" {
			cursor = last
		}
		if done || err != nil && !errors.Is(err, errStreamCut) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cursor == "" || attempt >= len(resumeBackoff) {
			return ErrStreamEnded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(resumeBackoff[attempt]):
		}
		if resp, err = c.rejoin(ctx, project, t, channel, cursor); err != nil {
			return err
		}
	}
}

// errStreamCut is a stream that closed or broke before a terminal frame.
var errStreamCut = errors.New("stream cut")

// readFrames parses server-sent events from r and hands each to on. It
// reports whether a terminal frame was seen and the last cursor delivered.
func readFrames(r io.Reader, on func(ChatEvent) error) (done bool, cursor string, err error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var ev ChatEvent
	var data strings.Builder
	for {
		line, readErr := br.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "" && readErr == nil:
			if data.Len() > 0 || ev.Type != "" {
				ev.Data = json.RawMessage(data.String())
				if ev.Type == "" {
					ev.Type = eventTypeOf(ev.Data)
				}
				if ev.ID != "" {
					cursor = ev.ID
				}
				if err := on(ev); err != nil {
					return false, cursor, err
				}
				if ev.Terminal() {
					return true, cursor, nil
				}
			}
			ev, data = ChatEvent{}, strings.Builder{}
		case strings.HasPrefix(line, ":"):
		default:
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				ev.Type = value
			case "id":
				ev.ID = value
			case "data":
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(value)
			}
		}
		if readErr != nil {
			return false, cursor, errStreamCut
		}
	}
}

// eventTypeOf reads the event type from a frame's data, for a frame whose
// event line was missing.
func eventTypeOf(data json.RawMessage) string {
	var v struct {
		EventType string `json:"eventType"`
	}
	_ = json.Unmarshal(data, &v)
	return v.EventType
}

// ChannelMetadata is what the platform knows about a conversation.
type ChannelMetadata struct {
	CustomChannelID string `json:"customChannelId"`
	Title           string `json:"title"`
	// RunState is IDLE, RUNNING or ERROR.
	RunState string `json:"runState"`
	// ConversationStatus is the agent's own verdict, NEEDS_INPUT or COMPLETED,
	// and empty until it gives one.
	ConversationStatus string `json:"conversationStatus"`
	LastActivityAt     int64  `json:"lastActivityAt"`
	// LaunchedSandboxes are the sandboxes the conversation holds now, live or
	// within the time they outlast its last message.
	LaunchedSandboxes []LaunchedSandbox `json:"launchedSandboxes"`
}

// LaunchedSandbox is one sandbox a conversation holds.
type LaunchedSandbox struct {
	SandboxName          string `json:"sandboxName"`
	SandboxBlueprintName string `json:"sandboxBlueprintName"`
	WorkingDirectory     string `json:"workingDirectory"`
}

// ChannelInfo reads a conversation's metadata, or returns a 404 when the
// channel has never been created or has been reaped.
func (c *Client) ChannelInfo(ctx context.Context, project string, t ChatTarget, channel string) (*ChannelMetadata, error) {
	var out ChannelMetadata
	err := c.do(ctx, request{
		method: http.MethodGet, path: t.Base + "/channel/metadata", query: url.Values{"custom_channel_id": {channel}},
		project: project, environment: t.Environment, out: &out,
	})
	return &out, err
}

// StopChat asks the run going on a channel to stop. The answer only says the
// request was accepted; the run ends on its own stream.
func (c *Client) StopChat(ctx context.Context, project string, t ChatTarget, channel string) error {
	return c.do(ctx, request{
		method: http.MethodPost, path: t.Base + "/message/suspend", query: url.Values{"custom_channel_id": {channel}},
		project: project, environment: t.Environment,
	})
}

// DeleteChat removes a channel: its transcript, its uploads and its sandbox.
// It returns when the sandbox is gone, which can take most of a minute, so it
// has no timeout of its own.
func (c *Client) DeleteChat(ctx context.Context, project string, t ChatTarget, channel string) error {
	resp, err := c.doRaw(ctx, rawRequest{
		method: http.MethodDelete, path: t.Base + "/channel", query: url.Values{"custom_channel_id": {channel}},
		project: project, environment: t.Environment, noTimeout: true,
	})
	if err != nil {
		return err
	}
	return decodeEnvelope(resp, nil)
}

// CurrentUserID is the platform's id for the signed-in account, which the
// Console puts in a preview channel's scope.
func (c *Client) CurrentUserID(ctx context.Context) (string, error) {
	var out struct {
		ID string `json:"user_id"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/auth/me", out: &out, noWorkspace: true}); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("the platform did not say who is signed in")
	}
	return out.ID, nil
}

// AgentAlias reads the alias an Agent answers to in Agent Hub. The Console
// starts a preview with "@<alias>" so Agent Hub hands the turn to that agent.
func (c *Client) AgentAlias(ctx context.Context, project, agent string) (string, error) {
	var out struct {
		Managed *struct {
			AliasName string `json:"alias_name"`
		} `json:"managed"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/agent/" + url.PathEscape(agent), project: project, out: &out}); err != nil {
		return "", err
	}
	if out.Managed == nil {
		return "", nil
	}
	return out.Managed.AliasName, nil
}

// WorkflowSetType reads a workflow set's type off its Workflows' tags: "bot"
// is a flow agent with a conversation; "automation_tool" is called as a tool
// and has none. Empty when none of its Workflows says, and a 404 when it has
// no Workflow in the environment.
func (c *Client) WorkflowSetType(ctx context.Context, project, workflowSet, environment string) (string, error) {
	var out []struct {
		ResourceTags map[string]string `json:"resource_tags"`
	}
	path := "/v1/workflow-set/" + url.PathEscape(workflowSet)
	if err := c.do(ctx, request{method: http.MethodGet, path: path, project: project, environment: environment, out: &out}); err != nil {
		return "", err
	}
	if len(out) == 0 {
		return "", &APIError{Status: http.StatusNotFound, Method: http.MethodGet, Path: path, Message: "no Workflow of this workflow set in the environment"}
	}
	for _, w := range out {
		if t := w.ResourceTags["workflow-set-type"]; t != "" {
			return t, nil
		}
	}
	return "", nil
}

// EnsureWorkflowSetPreview makes sure a workflow set has its preview bot in an
// environment, creating it when it has none, as the Console does. It reports
// whether it created one.
func (c *Client) EnsureWorkflowSetPreview(ctx context.Context, project, workflowSet, environment string) (bool, error) {
	path := "/v1/workflow-set/" + url.PathEscape(workflowSet) + "/preview"
	err := c.do(ctx, request{method: http.MethodGet, path: path, project: project, environment: environment})
	if err == nil {
		return false, nil
	}
	if !NotFound(err) {
		return false, err
	}
	err = c.do(ctx, request{method: http.MethodPost, path: path, project: project, environment: environment, sideEffect: true})
	return err == nil, err
}

// ContextIndexChannel is the one channel for improving a SourceSet's context
// index, which the platform names; the Console's "Improve the index" opens it.
func (c *Client) ContextIndexChannel(ctx context.Context, project, sourceSet string) (string, error) {
	var out *struct {
		EnhancementChannelID string `json:"enhancement_channel_id"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/source-set/" + url.PathEscape(sourceSet) + "/context-index", project: project, out: &out}); err != nil {
		return "", err
	}
	if out == nil || out.EnhancementChannelID == "" {
		return "", fmt.Errorf("SourceSet %s has no context index, so there is nothing to improve", sourceSet)
	}
	return out.EnhancementChannelID, nil
}
