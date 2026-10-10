package main

import (
	"os"
	"path/filepath"
)

// nestedCheckout reports a directory that is a checkout of its own - a git
// worktree, or a clone kept inside another clone - found while walking a
// clone. A walk skips it: its files are another branch of the same repository
// or another repository altogether, and counting them counts a chart twice.
// A maintainer's clone of a reference deployment is a working copy, so one
// carrying a worktree under .claude/worktrees is ordinary.
func nestedCheckout(fi os.FileInfo, p string) bool {
	if !fi.IsDir() {
		return false
	}
	_, err := os.Lstat(filepath.Join(p, ".git"))
	return err == nil
}
