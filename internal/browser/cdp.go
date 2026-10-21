package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The Workbench sandbox's browser (a Neko sidecar in the same pod) is driven
// over the Chrome DevTools Protocol at ASGARD_BROWSER_CDP_URL.
//
// A page is opened in a NEW tab, which becomes the one the member sees, and
// the tab that was in front stays as it was (asgard-odin-pm decision
// 2026-10-19-extension-connector D-33). That tab is often an external system's
// admin console the agent has been working in - one that has just shown a
// secret it will show only once - and navigating it away to a form would lose
// the secret. The sandbox's browser skill keeps working in the first tab, which
// is that one.
//
// Opening, raising and closing a tab are the DevTools HTTP endpoints, so no
// WebSocket is needed.

// EnvCDPURL is where the sandbox's browser listens for CDP.
const EnvCDPURL = "ASGARD_BROWSER_CDP_URL"

// cdpTimeout bounds one call to the browser.
const cdpTimeout = 15 * time.Second

// cdpTarget is what /json/new answers.
type cdpTarget struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// openCDPTab opens target in a new tab of the browser at cdpBase, puts that
// tab in front, and returns its id.
func openCDPTab(ctx context.Context, cdpBase, target string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cdpTimeout)
	defer cancel()
	cdpBase = strings.TrimRight(cdpBase, "/")
	client := &http.Client{Timeout: cdpTimeout}

	// Chromium answers /json/new to PUT only; GET is refused.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, cdpBase+"/json/new?"+url.QueryEscape(target), nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("reach the sandbox browser at %s: %w", cdpBase, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("open a tab in the sandbox browser: %s", resp.Status)
	}
	var tab cdpTarget
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tab); err != nil || tab.ID == "" {
		return "", fmt.Errorf("the sandbox browser opened a tab and did not say which: %v", err)
	}
	// Best effort: a new tab is normally in front already, and a failure to
	// raise it is not a failure to open the page.
	_ = cdpGet(ctx, client, cdpBase+"/json/activate/"+url.PathEscape(tab.ID))
	return tab.ID, nil
}

// CloseSandboxTab closes a tab opened in the sandbox's browser. A tab that is
// already gone - the member closed it - is not an error.
func CloseSandboxTab(ctx context.Context, id string) error {
	cdpBase := strings.TrimRight(lookupCDP(), "/")
	if cdpBase == "" || id == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, cdpTimeout)
	defer cancel()
	return cdpGet(ctx, &http.Client{Timeout: cdpTimeout}, cdpBase+"/json/close/"+url.PathEscape(id))
}

// cdpGet makes one DevTools HTTP call. A 404 is a target that no longer
// exists, which is what the caller wanted anyway.
func cdpGet(ctx context.Context, client *http.Client, endpoint string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%s: %s", endpoint, resp.Status)
	}
	return nil
}
