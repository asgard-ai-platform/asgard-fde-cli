package stage

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
)

// **The living-spec directory is a fact on disk, not a constant.** An
// engagement may rename `docs/spec/<slug>/`, and a prompt, a task record or a
// decision record that names the default instead sends a reader to a path that
// is not there - `decision add` reported a link failure at the end of a command
// that had otherwise succeeded.
func TestPromptNamesTheSpecDirectoryTheRepositoryActuallyHas(t *testing.T) {
	s, ok := Find("projects")
	if !ok {
		t.Fatal("no guidance named projects")
	}

	got, err := s.Prompt(State{SpecSlug: "acme-asgard"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "docs/spec/acme-asgard/") {
		t.Error("a renamed living-spec directory was not named in the prompt")
	}
	if strings.Contains(got, "docs/spec/"+repo.SpecSlug+"/") {
		t.Error("the default was named in a repository that renamed the directory")
	}

	// **Outside a repository there is nothing to read**, and `guide` answers
	// there by design. An empty path in a prompt is worse than the default.
	got, err = s.Prompt(State{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "docs/spec/"+repo.SpecSlug+"/") {
		t.Error("with no repository the prompt did not fall back to the default slug")
	}
}
