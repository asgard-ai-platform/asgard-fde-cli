package extension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LockName is the record an FDE repository keeps, at its root, of the
// Extensions it holds. It is committed, so whoever clones the repository can
// tell which version of which teaching material the CRs were written from.
const LockName = ".asgard-extensions.json"

// lockVersion is the shape of LockName this build writes.
const lockVersion = 1

// SourceLocal marks an Extension that did not come through a registry: copied
// from a path on this machine while its author tests it, or committed straight
// into this repository as a customer's own.
const SourceLocal = "local"

// Lock is LockName.
type Lock struct {
	Version    int              `json:"version"`
	Extensions map[string]Entry `json:"extensions"`
}

// Entry is one Extension in the lock.
//
// The key is the full path (`github.com/asgard-ai-platform/asgard-ext-x`) for
// one fetched through a registry, and the id for a local one.
type Entry struct {
	// Source is SourceLocal for a local Extension, empty for a registry one.
	Source string `json:"source,omitempty"`
	// Path is where a local copy came from, relative to the repository root
	// when it can be, so `extension update` can copy it again.
	Path string `json:"path,omitempty"`
	// Version is the git tag a registry copy was fetched at.
	Version string `json:"version,omitempty"`
	// Commit is what the tag pointed at, or the source's HEAD for a local
	// copy. Empty when the source is not a git repository.
	Commit string `json:"commit,omitempty"`
	// Dirty says the local source had uncommitted changes when it was copied,
	// so Commit alone does not describe what was copied.
	Dirty     bool   `json:"dirty,omitempty"`
	FetchedAt string `json:"fetched_at,omitempty"`
	// Routes are the routes this repository has taken, written by
	// `extension record`.
	Routes []string `json:"routes,omitempty"`
	// Files maps each copied path, relative to the Extension's directory, to
	// the digest of what was written, so a later copy can tell a hand edit.
	Files map[string]string `json:"files"`
}

// ReadLock reads the lock at the repository root. A missing file is an empty
// lock.
func ReadLock(repoRoot string) (*Lock, error) {
	path := filepath.Join(repoRoot, LockName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lock{Version: lockVersion, Extensions: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LockName, err)
	}
	if l.Version > lockVersion {
		return nil, fmt.Errorf("%s is version %d; this asgard-cli writes version %d - update asgard-cli", LockName, l.Version, lockVersion)
	}
	if l.Extensions == nil {
		l.Extensions = map[string]Entry{}
	}
	return &l, nil
}

// Write writes the lock with keys in a stable order, so a diff shows only
// what changed.
func (l *Lock) Write(repoRoot string) error {
	l.Version = lockVersion
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(l); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repoRoot, LockName), buf.Bytes(), 0o644)
}

// Find returns the key of the entry whose directory is id: the id itself for a
// local Extension, or a full path ending in /id.
func (l *Lock) Find(id string) (string, bool) {
	if _, ok := l.Extensions[id]; ok {
		return id, true
	}
	keys := make([]string, 0, len(l.Extensions))
	for k := range l.Extensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if filepath.Base(k) == id {
			return k, true
		}
	}
	return "", false
}
