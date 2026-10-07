package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// The message this replaces named two causes - no pattern matched, or the
// release was never created - and recommended the one command that contradicts
// it. Both were false the one time it fired: the run existed and had already
// planned. What it must never do again is claim nothing was created while
// holding the thing that was.
func TestNoRunAppearedSaysWhatItFound(t *testing.T) {
	runs := []*platform.RunSummary{
		{RunId: "r1", Number: 1, ReleaseName: "internal-dev", State: "awaiting_review", Trigger: "tag", Ref: "dev-0.1.0"},
	}

	t.Run("with runs it lists them and does not claim the push matched nothing", func(t *testing.T) {
		msg := noRunAppeared("dev-9.9.9", "", "internal-dev", time.Minute, runs).Error()
		for _, want := range []string{"dev-0.1.0", "internal-dev", "--run", "--ref", "deliveries"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message omits %q:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, "matched nothing") {
			t.Errorf("claimed the push matched nothing while holding a run:\n%s", msg)
		}
	})

	// --commit cannot match a run that recorded a tag object, so that is worth
	// naming - but only when a commit was what was searched for.
	t.Run("the annotated-tag hint appears only for --commit", func(t *testing.T) {
		withCommit := noRunAppeared("commit c87f9a8", "c87f9a8", "internal-dev", time.Minute, runs).Error()
		if !strings.Contains(withCommit, "ANNOTATED") {
			t.Errorf("no annotated-tag hint for a --commit search:\n%s", withCommit)
		}
		withRef := noRunAppeared("dev-9.9.9", "", "internal-dev", time.Minute, runs).Error()
		if strings.Contains(withRef, "ANNOTATED") {
			t.Errorf("annotated-tag hint on a --ref search, where it cannot apply:\n%s", withRef)
		}
	})

	// The old wording was right for this case, and this is the only case it was
	// right for.
	t.Run("with no runs it says the push matched nothing", func(t *testing.T) {
		msg := noRunAppeared("v1.0.0", "", "internal-prod", time.Minute, nil).Error()
		if !strings.Contains(msg, "matched nothing") || !strings.Contains(msg, "deliveries") {
			t.Errorf("did not report an empty pipeline plainly:\n%s", msg)
		}
		if strings.Contains(msg, "--run") {
			t.Errorf("offered --run with no run to follow:\n%s", msg)
		}
	})
}
