package main

import (
	"fmt"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("sources", check{
		Needs: "the clones",
		What:  "what each upstream clone resolves to and how far behind its remote it is",
		Run:   runSources,
	})
}

func runSources([]string) error {
	width := 0
	for _, s := range src.Sources {
		if len(s.Env) > width {
			width = len(s.Env)
		}
	}
	for _, name := range src.Order {
		s := src.Sources[name]
		dir, err := src.Resolve(name)
		if err != nil {
			fmt.Printf("%-*s  not found   %s\n", width, s.Env, s.What)
			continue
		}
		at := src.Commit(dir)
		if at == "" {
			fmt.Printf("%-*s  not a clone %s\n", width, s.Env, dir)
			continue
		}
		state := "(current)"
		if b := behind(dir); b != "" {
			state = "(" + b + ")"
		}
		fmt.Printf("%-*s  %-10s %-34s %s\n", width, s.Env, at, state, dir)
	}
	fmt.Println("\nNothing here pulls. `git -C <path> pull` before relying on a clone.")
	return nil
}

// behind reports how far a clone is behind its own remote, as a phrase.
//
// **Read, never fetched.** This reports the clone as it stands; if the answer
// matters, pull first. A check that fetched would be answering a different
// question each time it ran.
func behind(dir string) string {
	head := src.Commit(dir)
	for _, ref := range []string{"origin/HEAD", "origin/main", "origin/master"} {
		out, err := src.Git(dir, "rev-parse", "--short", ref)
		if err != nil {
			continue
		}
		up := strings.TrimSpace(out)
		if up == head {
			return ""
		}
		n, err := src.Git(dir, "rev-list", "--count", "HEAD.."+ref)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%s commit(s) behind %s", strings.TrimSpace(n), up)
	}
	return ""
}

func toSet(list []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		out[s] = true
	}
	return out
}
