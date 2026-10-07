package scaffold

import (
	"path/filepath"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
)

// **`--force` means "discard local edits to the skeleton", and some files stop
// being skeleton the moment somebody answers into them.**
//
// The deployment declaration is the one that cost most: `--force`, run to take
// three newer skills, replaced a hand-written twelve-release declaration with
// the twenty-four it generates from the directories under `projects/` - every
// `chartValues` and `appSecret` list gone, in a repository with nothing
// committed to recover from.
func TestAccumulatorsAreTheFilesAnEngagementAnswersInto(t *testing.T) {
	for _, target := range []string{
		pipelineconfig.FileName,
		filepath.Join("docs", "open-questions.md"),
		filepath.Join("requirements", "requests", "_index.md"),
		filepath.Join("requirements", "tasks", "_index.md"),
		filepath.Join("docs", "decisions", "README.md"),
		filepath.Join("docs", "spec", "acme", "README.md"),
		filepath.Join("projects", "erp", "chart", "app", "values.yaml"),
	} {
		if !accumulator(target) {
			t.Errorf("%s is written into after the scaffold runs and --force would overwrite it", target)
		}
	}

	// The skeleton proper. These are meant to be edited and taken again, which
	// is what --force is for; treating one as an accumulator would make the
	// flag stop doing its job.
	for _, target := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		filepath.Join(".agents", "skills", "plain-chinese", "SKILL.md"),
		filepath.Join("assets", "README.md"),
	} {
		if accumulator(target) {
			t.Errorf("%s is skeleton, and --force is what takes the newer copy", target)
		}
	}
}
