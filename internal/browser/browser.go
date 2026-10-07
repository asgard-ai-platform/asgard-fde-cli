// Package browser opens a URL in the desktop's browser, or - in the Workbench
// assistant's sandbox - in the sandbox's own browser, or not at all.
package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/sandbox"
)

// Open asks the desktop to open a URL.
//
// There is no dependency for this because there is nothing to depend on: each
// platform has one command, they have not changed in a decade, and a library
// would be a third-party package in a binary that currently has two.
//
// A failure here is never fatal to the flow that called it. Every caller
// prints the URL as well, so a machine with no desktop - a container, a CI
// runner, an SSH session - falls back to the person opening it themselves,
// which is exactly what --no-browser asks for explicitly.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// rundll32 rather than `cmd /c start`, which treats the first quoted
		// argument as a window title and mangles a URL containing `&`.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", cmd.Path, err)
	}
	// The browser outlives this process, so the child is deliberately not
	// waited on; releasing it stops the CLI holding a zombie until it exits.
	go func() { _ = cmd.Wait() }()
	return nil
}

// Handoff is what Present did with a URL, which decides what the caller tells
// whoever is reading.
type Handoff int

const (
	// OpenedLocally: the desktop's browser was asked to open it.
	OpenedLocally Handoff = iota
	// NotOpened: --no-browser, or the desktop could not open it; the caller
	// prints the URL for somebody to open.
	NotOpened
	// OpenedInSandbox: in the Workbench sandbox, a page only the sandbox can
	// reach (127.0.0.1) is open in the sandbox's browser. The assistant hands
	// it to the member with the open_sandbox_browser tool.
	OpenedInSandbox
	// ForTheMember: in the Workbench sandbox, a page on the internet. Nothing
	// is opened: the assistant gives the URL to the member as a link, for
	// their own browser, which is already signed in to what the page needs
	// and is a better place to work than a remote stream.
	ForTheMember
)

// Present puts a URL in front of a person, the right way for where this runs
// (asgard-odin-pm decision 2026-09-30-workbench-sandbox-fde-browser-choice).
//
// On a desktop it opens the desktop's browser, as Open does. In the Workbench
// sandbox there is no desktop and two browsers that are not this process's:
// the member's own, which can open anything on the internet, and the sandbox's,
// which is the only one that can reach a server listening on 127.0.0.1 here.
// So a loopback URL is opened in the sandbox's browser over CDP, and anything
// else is left for the member's own. Every caller prints the URL either way.
func Present(ctx context.Context, rawURL string, noBrowser bool) (Handoff, error) {
	if sandbox.Enabled() {
		if !isLoopback(rawURL) {
			return ForTheMember, nil
		}
		cdp := os.Getenv(EnvCDPURL)
		if cdp == "" {
			return NotOpened, errors.New("this sandbox has no browser (" + EnvCDPURL + " is not set); its SandboxBlueprint has to enable one")
		}
		if err := navigateCDP(ctx, cdp, rawURL); err != nil {
			return NotOpened, err
		}
		return OpenedInSandbox, nil
	}
	if noBrowser {
		return NotOpened, nil
	}
	if err := Open(rawURL); err != nil {
		return NotOpened, err
	}
	return OpenedLocally, nil
}

// isLoopback reports whether a URL points at this machine.
func isLoopback(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
