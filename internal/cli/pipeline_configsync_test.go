package cli

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// TestConfigSyncHint covers the one config sync failure that gets a sentence
// added to it, and the boundary of what it fires on.
//
// **A wrong answer here is silent either way**, which is why it is a test. A
// hint that does not fire leaves the provider's own words alone and nobody
// learns anything new; a hint that fires on the wrong error tells somebody to
// push when pushing is not the fix, and the raw message above it is the only
// thing that would contradict it.
func TestConfigSyncHint(t *testing.T) {
	// What the platform actually hands over: asgard-iac's config sync wraps the
	// ref it could not resolve around the provider's own body, unparsed.
	const empty = `resolve main: github: GET https://api.github.com/repos/acme/app/commits/main: ` +
		`unexpected status 409: {"message":"Git Repository is empty.","status":"409"}`

	cases := []struct {
		name  string
		err   string
		fires bool
	}{
		{"the empty repository", empty, true},
		{"a missing declaration", ".asgard-pipeline.yaml not found in acme/app at 1a2b3c4", false},
		{"a revoked installation", "connection to acme is revoked; reinstall the GitHub App", false},
		{"a ref that is not there", "resolve release-1: github: GET https://api.github.com/x: unexpected status 404", false},
		{"no default branch", "acme/app reports no default branch; set the declaration source ref", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := configSyncHint(c.err)
			if fired := got != ""; fired != c.fires {
				t.Fatalf("configSyncHint(%q) = %q, want fired=%v", c.err, got, c.fires)
			}
			if c.fires && !strings.Contains(got, "Push") {
				t.Errorf("the hint does not name the fix: %q", got)
			}
		})
	}

	// And the whole line, because the hint is printed under an error that
	// already occupies the column it indents to.
	var b strings.Builder
	printPipelineConfigState(&b, &platform.Pipeline{
		DefaultBranch:  "main",
		ConfigPath:     ".asgard-pipeline.yaml",
		LastConfigSync: &platform.ConfigSync{Error: empty},
	})
	out := b.String()
	for _, want := range []string{"config         ERROR", "Git Repository is empty.", "no commits"} {
		if !strings.Contains(out, want) {
			t.Errorf("the config state does not contain %q:\n%s", want, out)
		}
	}
}
