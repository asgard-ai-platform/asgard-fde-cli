package check

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
)

// rel is one declared release, named and triggered.
func rel(name, pattern, chart string) pipelineconfig.Release {
	return pipelineconfig.Release{
		Name:  name,
		On:    &pipelineconfig.Trigger{Type: "tag", Pattern: pattern},
		Chart: chart,
	}
}

// warned reports whether the one-release-per-environment warning fired, and how
// often.
func warned(releases ...pipelineconfig.Release) int {
	c := &checker{}
	c.checkOneReleasePerEnvironment(&pipelineconfig.Config{Releases: releases})
	n := 0
	for _, f := range c.findings {
		if f.Level == Warning && strings.Contains(f.Message, "is named by one release") {
			n++
		}
	}
	return n
}

// The reported case: one release, whose NAME says nothing about environments
// and whose PATTERN says dev. Naming a dev environment and never asking what
// the other one was is the whole mistake, and the pattern is where it was said.
func TestOneReleasePerEnvironment_PatternNamesIt(t *testing.T) {
	if n := warned(rel("internal", `^dev-[0-9]+\.[0-9]+\.[0-9]+$`, "projects/internal/chart/app")); n != 1 {
		t.Errorf("the reported shape must warn once, got %d", n)
	}
}

func TestOneReleasePerEnvironment_NameNamesIt(t *testing.T) {
	if n := warned(rel("shop-dev", `^v[0-9]+$`, "projects/shop/chart/app")); n != 1 {
		t.Errorf("a name saying dev must warn, got %d", n)
	}
}

// The shape this is asking for: two releases, one chart, two patterns.
func TestOneReleasePerEnvironment_PairIsSilent(t *testing.T) {
	n := warned(
		rel("shop-dev", `^dev-[0-9]+\.[0-9]+\.[0-9]+$`, "projects/shop/chart/app"),
		rel("shop-prod", `^[0-9]+\.[0-9]+\.[0-9]+$`, "projects/shop/chart/app"),
	)
	if n != 0 {
		t.Errorf("a release per environment is the answer, not a warning: %d", n)
	}
}

// A lone release that does not say which environment it is has not made this
// mistake. Warning on every single-release chart would make the warning noise,
// and noise is ignored.
func TestOneReleasePerEnvironment_SilentWhenUnsaid(t *testing.T) {
	if n := warned(rel("shop", `^[0-9]+\.[0-9]+\.[0-9]+$`, "projects/shop/chart/app")); n != 0 {
		t.Errorf("no environment named, so nothing to say: %d", n)
	}
}

// Whole words only. "developer-portal" is not a dev release, and a warning that
// fires on it teaches everybody to skip the warning.
func TestOneReleasePerEnvironment_WordsNotSubstrings(t *testing.T) {
	if n := warned(rel("developer-portal", `^[0-9]+\.[0-9]+\.[0-9]+$`, "projects/dp/chart/app")); n != 0 {
		t.Errorf("substring match would be a false positive: %d", n)
	}
}

// Each chart is judged on its own: two charts, each with a lone dev release, is
// two mistakes and reads as two.
func TestOneReleasePerEnvironment_PerChart(t *testing.T) {
	n := warned(
		rel("a-dev", `^dev-[0-9]+$`, "projects/a/chart/app"),
		rel("b-dev", `^dev-[0-9]+$`, "projects/b/chart/app"),
	)
	if n != 2 {
		t.Errorf("one per chart, got %d", n)
	}
}
