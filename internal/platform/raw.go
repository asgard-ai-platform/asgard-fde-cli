package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// rawRequest is a call whose body or answer is not the JSON envelope: a
// multipart upload, or a stream the caller reads as it arrives.
type rawRequest struct {
	method      string
	path        string
	query       url.Values
	body        io.Reader
	contentType string
	accept      string
	// viaAssistant marks the request as the Workbench assistant's.
	viaAssistant bool
	// noTimeout lifts the per-call timeout, for an answer that streams.
	noTimeout bool
	// sideEffect: as request.sideEffect.
	sideEffect bool
	// project: as request.project.
	project string
}

// doRaw makes one call and returns the response for a 2xx, which the caller
// closes. A non-2xx is decoded into an *APIError the same way do decodes one.
func (c *Client) doRaw(ctx context.Context, req rawRequest) (*http.Response, error) {
	endpoint := c.profile.PlatformAPI + req.path
	if len(req.query) > 0 {
		endpoint += "?" + req.query.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.method, endpoint, req.body)
	if err != nil {
		return nil, fmt.Errorf("build the request: %w", err)
	}
	if c.workspace == "" {
		return nil, errors.New("no workspace selected; pass --workspace, or record one with `asgard-cli workspace use <id>`")
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set(ClientHeader, clientValue())
	httpReq.Header.Set(WorkspaceHeader, c.workspace)
	if req.project != "" {
		httpReq.Header.Set(ProjectHeader, req.project)
	}
	if req.contentType != "" {
		httpReq.Header.Set("Content-Type", req.contentType)
	}
	accept := req.accept
	if accept == "" {
		accept = "application/json"
	}
	httpReq.Header.Set("Accept", accept)
	if req.viaAssistant || c.assistant {
		httpReq.Header.Set(ViaAssistantHeader, "true")
	}

	httpClient := c.http
	if req.noTimeout {
		copied := *c.http
		copied.Timeout = 0
		httpClient = &copied
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call %s %s: %w", req.method, endpoint, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if req.sideEffect {
			NoteSideEffect()
		}
		return resp, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	apiErr := &APIError{Status: resp.StatusCode, Method: req.method, Path: req.path, Message: truncate(string(raw), 200)}
	var env envelope
	if json.Unmarshal(raw, &env) == nil && (env.Message != "" || len(env.Details) > 0) {
		apiErr.Message, apiErr.ReasonCode, apiErr.ErrorCode = env.Message, env.ReasonCode, errorCode(env.Details)
	}
	return nil, apiErr
}

// decodeEnvelope reads a 2xx response's envelope into out.
func decodeEnvelope(resp *http.Response, out any) error {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read the response: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("the platform answered with something that is not its response envelope: %s", truncate(string(raw), 200))
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}
