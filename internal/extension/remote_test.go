package extension

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name string
	body string
	mode int64
	kind byte
}

func tarGz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: e.name, Mode: mode, Size: int64(len(e.body)), Typeflag: kind}
		if kind != tar.TypeReg {
			hdr.Size = 0
			hdr.Linkname = "/etc/passwd"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFromArchiveTakesOnlyShipped(t *testing.T) {
	data := tarGz(t,
		tarEntry{name: "README.md", body: "readme"},
		tarEntry{name: "routes/", kind: tar.TypeDir},
		tarEntry{name: "routes/build-time.none.md", body: "route"},
		tarEntry{name: "scripts/probe.py", body: "print()", mode: 0o755},
		tarEntry{name: "AGENTS.md", body: "the author's own"},
		tarEntry{name: "newthing/x.md", body: "a future entry"},
	)
	files, skipped, err := FromArchive(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || string(files["routes/build-time.none.md"].Data) != "route" {
		t.Fatalf("files = %v", files)
	}
	if !files["scripts/probe.py"].Exec || files["README.md"].Exec {
		t.Errorf("exec bits not kept: %+v", files)
	}
	if strings.Join(skipped, ",") != "AGENTS.md,newthing" {
		t.Errorf("skipped = %v", skipped)
	}
}

func TestFromArchiveRefusesWhatWouldEscape(t *testing.T) {
	for name, entry := range map[string]tarEntry{
		"parent":   {name: "routes/../../evil.md", body: "x"},
		"absolute": {name: "/etc/evil", body: "x"},
		"symlink":  {name: "assets/logo.svg", kind: tar.TypeSymlink},
		"hardlink": {name: "assets/logo.svg", kind: tar.TypeLink},
	} {
		if _, _, err := FromArchive(bytes.NewReader(tarGz(t, entry))); err == nil {
			t.Errorf("%s: an archive holding %q was accepted", name, entry.name)
		}
	}
	if _, _, err := FromArchive(strings.NewReader("not gzip")); err == nil {
		t.Error("a body that is not gzip was accepted")
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		want               bool
	}{
		{"v1.2.0", "v1.1.9", true},
		{"v1.10.0", "v1.9.0", true},
		{"v1.3.0", "v1.3.0-rc.1", true},
		{"v1.3.0-rc.2", "v1.3.0-rc.1", true},
		{"v1.3.0-rc.10", "v1.3.0-rc.9", true},
		{"v1.3.0-rc.1", "v1.3.0", false},
		{"v1.2.0", "v1.2.0", false},
		{"v1.2.0", "", false},
		{"latest", "v1.0.0", false},
	} {
		if got := Newer(tc.candidate, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	for _, tc := range []struct {
		version, constraint string
		ok, understood      bool
	}{
		{"0.3.0", ">=0.3.0", true, true},
		{"0.2.9", ">=0.3.0", false, true},
		{"0.4.0", ">=0.3.0 <1.0.0", true, true},
		{"1.0.0", ">=0.3.0, <1.0.0", false, true},
		{"0.3.0", "0.3.0", true, true},
		{"0.3.0", "^0.3.0", false, false},
		{"0.3.0", "", false, false},
	} {
		ok, understood := satisfies(tc.version, tc.constraint)
		if ok != tc.ok || understood != tc.understood {
			t.Errorf("satisfies(%q, %q) = %v, %v; want %v, %v", tc.version, tc.constraint, ok, understood, tc.ok, tc.understood)
		}
	}
}

func TestCheckRequires(t *testing.T) {
	skillsRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(skillsRoot, "db-query"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "db-query", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := &Lock{Extensions: map[string]Entry{"github.com/a/asgard-ext-base": {}}}
	req := Requires{
		AsgardCLI:  ">=0.9.0",
		Skills:     []string{"db-query", "semantic-layer-modeling"},
		Extensions: []string{"asgard-ext-base", "github.com/a/asgard-ext-other"},
		Platform:   []string{"connector"},
	}
	got := strings.Join(CheckRequires(req, skillsRoot, lock, "v0.1.8-main"), "\n")
	for _, want := range []string{"asgard-cli >=0.9.0 and this is v0.1.8-main", "skill semantic-layer-modeling", "Extension github.com/a/asgard-ext-other", "platform's connector"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings do not mention %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"skill db-query", "asgard-ext-base,"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("warnings mention %q, which is satisfied:\n%s", unwanted, got)
		}
	}
	if w := CheckRequires(Requires{AsgardCLI: ">=0.1.0"}, skillsRoot, lock, "v0.1.8-main"); len(w) != 0 {
		t.Errorf("a development build named after 0.1.8 does not meet >=0.1.0: %v", w)
	}
	if w := CheckRequires(Requires{AsgardCLI: ">=0.1.0"}, skillsRoot, lock, "dev"); len(w) != 1 || !strings.Contains(w[0], "no release version") {
		t.Errorf("a build with no version: %v", w)
	}
}

func TestInspect(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("asgard-ext-clean/SKILL.md", "a")
	write("asgard-ext-edited/SKILL.md", "edited")
	write("asgard-ext-edited/extra.md", "mine")
	write("asgard-ext-stray/SKILL.md", "not recorded")
	write("db-query/SKILL.md", "not an Extension")
	lock := &Lock{Extensions: map[string]Entry{
		"github.com/a/asgard-ext-clean":  {Version: "v1.0.0", Files: map[string]string{"SKILL.md": Digest([]byte("a"))}},
		"github.com/a/asgard-ext-edited": {Version: "v1.0.0", Files: map[string]string{"SKILL.md": Digest([]byte("b")), "gone.md": Digest([]byte("c"))}},
		"asgard-ext-absent":              {Source: SourceLocal},
	}}
	states, unrecorded, err := Inspect(root, lock)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]State{}
	for _, s := range states {
		byID[s.ID] = s
	}
	if !byID["asgard-ext-clean"].Clean() {
		t.Errorf("clean: %+v", byID["asgard-ext-clean"])
	}
	edited := byID["asgard-ext-edited"]
	if len(edited.Changed) != 2 || strings.Join(edited.Missing, ",") != "gone.md" {
		t.Errorf("edited: changed %v, missing %v", edited.Changed, edited.Missing)
	}
	if !byID["asgard-ext-absent"].Absent {
		t.Errorf("absent: %+v", byID["asgard-ext-absent"])
	}
	if strings.Join(unrecorded, ",") != "asgard-ext-stray" {
		t.Errorf("unrecorded = %v", unrecorded)
	}
}

func TestLintLogo(t *testing.T) {
	withLogo := func(logo string, extra map[string]string) []Finding {
		files := good()
		files["README.md"] = strings.Replace(goodReadme, "listing: public", "logo: "+logo+"\nlisting: public", 1)
		for k, v := range extra {
			files[k] = v
		}
		findings, _ := Lint(writeTree(t, files))
		return findings
	}
	if f := withLogo("assets/logo.svg", map[string]string{"assets/logo.svg": "<svg/>"}); len(f) != 0 {
		t.Errorf("a good logo: %s", messages(f, Warning))
	}
	for logo, want := range map[string]string{
		"assets/missing.svg": "does not exist",
		"assets/logo.gif":    "neither .svg nor .png",
		"docs/logo.svg":      "not among what the Extension ships",
		"../logo.svg":        "not among what the Extension ships",
	} {
		f := withLogo(logo, map[string]string{"assets/logo.gif": "GIF89a"})
		if got := messages(f, Warning); !strings.Contains(got, want) || messages(f, Error) != "" {
			t.Errorf("logo %s: want a warning saying %q, got warnings:\n%serrors:\n%s", logo, want, got, messages(f, Error))
		}
	}
	big := withLogo("assets/logo.png", map[string]string{"assets/logo.png": strings.Repeat("x", logoLimit+1)})
	if got := messages(big, Warning); !strings.Contains(got, "over the 256 KiB") {
		t.Errorf("a logo over the limit: %s", got)
	}
}

func TestLintSummaryOnOneLine(t *testing.T) {
	for summary, warns := range map[string]bool{
		"summary: 讓 Agent 查 Acme 的紀錄，不需要 OAuth。":                        false,
		"summary: Let the Agent read Acme records.":                     false,
		"summary: >-\n  讓 Agent 查 Acme 的紀錄，\n  不需要 OAuth。":              true,
		"summary: |\n  Let the Agent read\n  Acme records.":             true,
		"summary: 讓 Agent 查 Acme 的紀錄，\n  不需要 OAuth。":                    true,
		"summary: >-\n  Let the Agent read Acme records,\n  and reply.": false,
	} {
		files := good()
		files["README.md"] = strings.Replace(goodReadme, "summary: Let the Agent read Acme records.", summary, 1)
		findings, _ := Lint(writeTree(t, files))
		got := strings.Contains(messages(findings, Warning), "write it on one line")
		if got != warns {
			t.Errorf("%q: warned %v, want %v\n%s", summary, got, warns, messages(findings, Warning))
		}
	}
}
