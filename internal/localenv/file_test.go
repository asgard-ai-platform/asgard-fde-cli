package localenv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAddPlaceholders(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	before := "# the read-only account\nUOF_DB_USER=reader\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	added, err := f.AddPlaceholders([]string{"UOF_DB_USER", "UOF_DB_PASSWORD", "GITHUB_PAT"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"UOF_DB_PASSWORD", "GITHUB_PAT"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("added = %v, want %v", added, want)
	}

	// The form sees them, empty; the key already there keeps its value and note.
	var keys []string
	for _, e := range f.Entries() {
		keys = append(keys, e.Key)
		switch e.Key {
		case "UOF_DB_USER":
			if e.Value != "reader" || e.Note != "the read-only account" {
				t.Errorf("existing key changed: %+v", e)
			}
		default:
			if e.Value != "" {
				t.Errorf("%s = %q, want empty", e.Key, e.Value)
			}
		}
	}
	if want := []string{"UOF_DB_USER", "UOF_DB_PASSWORD", "GITHUB_PAT"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("entries = %v, want %v", keys, want)
	}

	// Nothing is written until the form is saved.
	if data, _ := os.ReadFile(path); string(data) != before {
		t.Fatalf("file changed before Save:\n%s", data)
	}

	f.Set("GITHUB_PAT", "ghp_x")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := before + "UOF_DB_PASSWORD=\nGITHUB_PAT=ghp_x\n"; string(data) != want {
		t.Fatalf("saved:\n%s\nwant:\n%s", data, want)
	}
}

func TestAddPlaceholdersRejectsANonKey(t *testing.T) {
	f, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"GITHUB-PAT", "1ST_KEY", "A B"} {
		if _, err := f.AddPlaceholders([]string{bad}); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	if len(f.Entries()) != 0 {
		t.Fatalf("a rejected name left entries: %+v", f.Entries())
	}
}
