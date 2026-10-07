package cli

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// TestPrintReleaseTeardown covers what a release says about itself while it is
// being removed. A teardown is the one thing here that can stop half way, and
// "delete_failed" with nothing beside it is the state that sends somebody to
// the Console to read what this call already returned.
func TestPrintReleaseTeardown(t *testing.T) {
	cases := []struct {
		name    string
		release platform.Release
		want    []string
		not     []string
	}{{
		name: "parked mid teardown",
		release: platform.Release{
			Name:        "internal-dev",
			State:       platform.ReleaseDeleteFailed,
			DeleteStep:  "helm_uninstall",
			DeleteError: "release: not found",
		},
		want: []string{
			"teardown step    helm_uninstall",
			"teardown error   release: not found",
			"asgard-cli pipeline release destroy internal-dev --retry",
		},
	}, {
		// Detach is synchronous and takes the record with it, so this field is
		// only ever seen on a release whose detach did not go through - and
		// that release is still active, with no teardown to resume.
		name: "detach refused",
		release: platform.Release{
			Name:        "internal-dev",
			State:       platform.ReleaseActive,
			DetachError: "helm history delete failed",
		},
		want: []string{"detach error     helm history delete failed"},
		not:  []string{"--retry", "teardown step"},
	}, {
		name:    "nothing to say",
		release: platform.Release{Name: "internal-dev", State: platform.ReleaseActive},
		not:     []string{"teardown", "detach error", "--retry"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			printRelease(&b, &c.release)
			got := b.String()
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in:\n%s", n, got)
				}
			}
		})
	}
}
