package extension

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// State is one Extension the lock records, set against what is on disk.
type State struct {
	// Key is the lock's key: a full path, or the id of a local Extension.
	Key   string
	ID    string
	Entry Entry
	// Dir is the Extension's directory: the skills directory joined with ID.
	Dir string
	// Absent says the directory is not there at all.
	Absent bool
	// Changed are files edited since they were written, or there and not
	// written by asgard-cli.
	Changed []Conflict
	// Missing are files the lock records that are gone.
	Missing []string
}

// Clean says the directory holds exactly what was written.
func (s State) Clean() bool { return !s.Absent && len(s.Changed) == 0 && len(s.Missing) == 0 }

// Inspect sets every Extension the lock records against the skills directory,
// and lists the `asgard-ext-*` directories there that the lock does not record:
// an Extension committed without an entry, or a skill of this repository's own
// using the prefix, which would collide with an Extension of the same name.
func Inspect(skillsRoot string, lock *Lock) ([]State, []string, error) {
	keys := make([]string, 0, len(lock.Extensions))
	for k := range lock.Extensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	recorded := map[string]bool{}
	var states []State
	for _, k := range keys {
		e := lock.Extensions[k]
		id := path.Base(k)
		recorded[id] = true
		s := State{Key: k, ID: id, Entry: e, Dir: filepath.Join(skillsRoot, id)}
		if info, err := os.Stat(s.Dir); err != nil || !info.IsDir() {
			s.Absent = true
			states = append(states, s)
			continue
		}
		changed, err := Conflicts(s.Dir, e.Files)
		if err != nil {
			return nil, nil, err
		}
		s.Changed = changed
		for rel := range e.Files {
			if _, err := os.Stat(filepath.Join(s.Dir, filepath.FromSlash(rel))); os.IsNotExist(err) {
				s.Missing = append(s.Missing, rel)
			}
		}
		sort.Strings(s.Missing)
		states = append(states, s)
	}

	var unrecorded []string
	entries, err := os.ReadDir(skillsRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	for _, d := range entries {
		if d.IsDir() && strings.HasPrefix(d.Name(), Prefix) && !recorded[d.Name()] {
			unrecorded = append(unrecorded, d.Name())
		}
	}
	return states, unrecorded, nil
}
