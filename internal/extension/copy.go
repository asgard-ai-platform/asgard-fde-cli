package extension

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Digest is how a copied file is recorded in the lock.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// File is one file to copy.
type File struct {
	Data []byte
	// Exec keeps a script runnable after the copy.
	Exec bool
}

// Collect reads the files that leave an extension repository at src: the
// Shipped entries and everything under them. Keys are slash-separated paths
// relative to src.
func Collect(src string) (map[string]File, error) {
	out := map[string]File{}
	for _, top := range Shipped {
		root := filepath.Join(src, top)
		info, err := os.Stat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			data, err := os.ReadFile(root)
			if err != nil {
				return nil, err
			}
			out[top] = File{Data: data, Exec: info.Mode()&0o111 != 0}
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("%s: only regular files can be copied; a symlink would point at this machine", path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, path)
			out[filepath.ToSlash(rel)] = File{Data: data, Exec: fi.Mode()&0o111 != 0}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Conflict is a file in the destination that a copy would overwrite or delete
// although asgard-cli did not write it as it now stands.
type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Conflicts compares the destination with what the lock says was written
// there. A file edited since, or one the lock does not know, is a conflict:
// replacing the directory would lose it.
func Conflicts(dst string, recorded map[string]string) ([]Conflict, error) {
	var out []Conflict
	err := filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return filepath.SkipAll
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dst, path)
		rel = filepath.ToSlash(rel)
		want, ok := recorded[rel]
		if !ok {
			out = append(out, Conflict{Path: rel, Reason: "not written by asgard-cli"})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if Digest(data) != want {
			out = append(out, Conflict{Path: rel, Reason: "edited since it was copied"})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// Replace makes dst hold exactly files, and returns the digest of each.
// Whatever dst held before is removed, so the caller checks Conflicts first.
func Replace(dst string, files map[string]File) (map[string]string, error) {
	if err := os.RemoveAll(dst); err != nil {
		return nil, err
	}
	digests := make(map[string]string, len(files))
	for rel, f := range files {
		if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("refusing to write %s outside %s", rel, dst)
		}
		path := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		mode := os.FileMode(0o644)
		if f.Exec {
			mode = 0o755
		}
		if err := os.WriteFile(path, f.Data, mode); err != nil {
			return nil, err
		}
		digests[rel] = Digest(f.Data)
	}
	return digests, nil
}
