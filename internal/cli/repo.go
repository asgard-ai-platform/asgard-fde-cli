package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
)

// loadRepo finds the customer repository from the working directory.
//
// **The declaration is what makes a directory one.** It used to be
// `.asgard-config.json`, this tool's own scaffold record - so a repository that
// had everything the platform needs and none of this tool's bookkeeping was not
// a repository as far as every command was concerned. The declaration is the
// file the platform reads on every run, which is the honest test.
func loadRepo() (root string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	declPath, _, err := binding.Locate(dir)
	if err != nil {
		return "", err
	}
	if declPath == "" {
		return "", fmt.Errorf("no %s at or above %s, so this is not a repository this tool deploys from.\n"+
			"`asgard-cli init` writes one, along with the rest of the skeleton",
			pipelineconfig.FileName, dir)
	}
	return filepath.Dir(declPath), nil
}

// today is the date the records are stamped with. It is a variable so a test can
// pin it: a golden file that changes at midnight is not a test.
var today = func() string { return time.Now().Format("2006-01-02") }

// loadState reads the repository and every record it keeps.
//
// Several commands list what the repo contains, and they used to be one command
// that listed all of it and then said which guidance the shape of it made
// relevant. The listing survived that; the inference did not. Each command now
// reads the whole state and prints only its own part, because the parts are
// separate claims and an agent asking what is unanswered should not have to
// read past what a chart declares to find out.
func loadState() (stage.State, error) {
	root, err := loadRepo()
	if err != nil {
		return stage.State{}, err
	}
	return stage.Inspect(root)
}

// errNotInRepo is the one wording for "you are not in a repository this tool
// deploys from", so the answer reads the same whichever command asked.
func errNotInRepo() error {
	dir, _ := os.Getwd()
	return fmt.Errorf("no %s at or above %s, so this is not a repository this tool deploys from.\n"+
		"`asgard-cli init` writes one, along with the rest of the skeleton",
		pipelineconfig.FileName, dir)
}
