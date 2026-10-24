package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/extension"
)

const remoteReadme = `---
id: asgard-ext-acme
name: Acme
summary: Let the Agent read Acme records.
kind: integration
system:
  vendor: Acme
  product: Acme Cloud
  category: crm
routes:
  - id: build-time.static-secret
  - id: shared-account.static-secret
requires:
  skills: [db-query]
listing: unlisted
source: closed
owners: ["fde@example.com"]
---

# Acme
`

const acmePath = "github.com/asgard-ai-platform/asgard-ext-acme"

// fakeExtensionPlatform serves the Extension routes for one Extension with
// the versions in archives, and records whether a call carried a workspace.
type fakeExtensionPlatform struct {
	mu        sync.Mutex
	latest    string
	archives  map[string][]byte
	badSha    bool
	workspace []string
	calls     []string
}

func (f *fakeExtensionPlatform) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.workspace = append(f.workspace, r.Header.Get("x-asgard-workspace"))
		f.calls = append(f.calls, r.URL.Path)
		latest := f.latest
		f.mu.Unlock()
		envelope := func(data any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
		}
		version := map[string]any{"path": acmePath, "version": latest, "commit": "c0ffee" + latest, "prerelease": strings.Contains(latest, "-")}
		meta := map[string]any{"id": "asgard-ext-acme", "name": "Acme", "summary": "Let the Agent read Acme records.",
			"system":   map[string]any{"vendor": "Acme", "product": "Acme Cloud", "category": "crm"},
			"routes":   []any{map[string]any{"id": "build-time.static-secret", "status": "verified"}},
			"requires": map[string]any{"skills": []string{"db-query"}}, "listing": "unlisted"}
		p := r.URL.Path
		switch {
		case p == "/v1/extensions":
			if r.URL.Query().Get("q") == "nothing" {
				envelope([]any{})
				return
			}
			envelope([]any{map[string]any{"path": acmePath, "source": "asgard-official", "latest": version, "metadata": meta}})
		case strings.HasSuffix(p, "/@latest"):
			envelope(version)
		case strings.HasSuffix(p, "/@v/list"):
			envelope(map[string]any{"path": acmePath, "versions": []any{version},
				"refused_versions": []any{map[string]any{"version": "v0.0.1", "reason": "README.md has no frontmatter"}}})
		case strings.HasSuffix(p, ".info"):
			ver := strings.TrimSuffix(p[strings.LastIndex(p, "/")+1:], ".info")
			if _, ok := f.archives[ver]; !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": acmePath + " has no version " + ver})
				return
			}
			v := map[string]any{"path": acmePath, "version": ver, "commit": "c0ffee" + ver}
			envelope(map[string]any{"version": v, "metadata": meta, "readme": "# Acme"})
		case strings.HasSuffix(p, ".tar.gz"):
			ver := strings.TrimSuffix(p[strings.LastIndex(p, "/")+1:], ".tar.gz")
			data, ok := f.archives[ver]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": acmePath + " has no version " + ver})
				return
			}
			sum := sha256.Sum256(data)
			sha := hex.EncodeToString(sum[:])
			if f.badSha {
				sha = strings.Repeat("0", 64)
			}
			w.Header().Set("X-Extension-Sha256", sha)
			w.Header().Set("X-Extension-Path", acmePath)
			w.Header().Set("X-Extension-Version", ver)
			w.Header().Set("X-Extension-Commit", "c0ffee"+ver)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func acmeArchive(t *testing.T, skill string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"README.md":                              remoteReadme,
		"SKILL.md":                               skill,
		"routes/build-time.static-secret.md":     "# route\n",
		"routes/shared-account.static-secret.md": "# route\n",
		"AGENTS.md":                              "the author's own\n",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
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

// fdeRepo makes the current directory an FDE repository and returns it.
func fdeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".asgard-pipeline.yaml"), []byte("projects: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

func TestExtensionGetFromPlatform(t *testing.T) {
	fake := &fakeExtensionPlatform{latest: "v1.0.0", archives: map[string][]byte{"v1.0.0": acmeArchive(t, "---\nname: asgard-ext-acme\n---\nv1\n")}}
	sandboxEnv(t, fake.server(t).URL)
	root := fdeRepo(t)

	out, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme")
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	dir := filepath.Join(root, ".agents", "skills", "asgard-ext-acme")
	if b, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err != nil || !strings.Contains(string(b), "v1") {
		t.Fatalf("SKILL.md = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md is not shipped and was written")
	}
	for _, want := range []string{"fetched asgard-ext-acme v1.0.0 (4 files)", "left out of the archive", "AGENTS.md", "skill db-query", "Next: read .agents/skills/asgard-ext-acme/SKILL.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not say %q:\n%s", want, out)
		}
	}
	lock, err := extension.ReadLock(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Extensions[acmePath]
	if !ok || entry.Version != "v1.0.0" || entry.Commit != "c0ffeev1.0.0" || entry.Source != "" || len(entry.Files) != 4 {
		t.Fatalf("lock entry = %+v (ok %v)", entry, ok)
	}
	for _, ws := range fake.workspace {
		if ws != "" {
			t.Errorf("an Extension call carried the workspace header %q", ws)
		}
	}
}

func TestExtensionGetRefusesEditsAndBadDownloads(t *testing.T) {
	fake := &fakeExtensionPlatform{latest: "v1.0.0", archives: map[string][]byte{
		"v1.0.0": acmeArchive(t, "v1\n"), "v1.1.0": acmeArchive(t, "v1.1\n"),
	}}
	sandboxEnv(t, fake.server(t).URL)
	root := fdeRepo(t)
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme"); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, ".agents", "skills", "asgard-ext-acme", "SKILL.md")
	if err := os.WriteFile(skill, []byte("edited here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runCLI(t, "", "extension", "get", "asgard-ext-acme@v1.1.0")
	if err == nil || !strings.Contains(errOut, "SKILL.md: edited since it was copied") {
		t.Fatalf("get over an edit: err %v, stderr %s", err, errOut)
	}
	if b, _ := os.ReadFile(skill); string(b) != "edited here\n" {
		t.Fatalf("the edit was overwritten: %q", b)
	}
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme@v1.1.0", "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if b, _ := os.ReadFile(skill); string(b) != "v1.1\n" {
		t.Fatalf("--force did not replace: %q", b)
	}

	fake.badSha = true
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme@v1.0.0"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("a download with the wrong digest: %v", err)
	}
	if b, _ := os.ReadFile(skill); string(b) != "v1.1\n" {
		t.Fatalf("a failed download changed the directory: %q", b)
	}
	fake.badSha = false
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme@v9.9.9"); err == nil || !strings.Contains(err.Error(), "has no version v9.9.9 (404)") {
		t.Fatalf("an unknown version: %v", err)
	}
}

func TestExtensionUpdateAndStatus(t *testing.T) {
	fake := &fakeExtensionPlatform{latest: "v1.0.0", archives: map[string][]byte{
		"v1.0.0": acmeArchive(t, "v1\n"), "v1.1.0": acmeArchive(t, "v1.1\n"),
	}}
	sandboxEnv(t, fake.server(t).URL)
	root := fdeRepo(t)
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme"); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runCLI(t, "", "extension", "record", "asgard-ext-acme", "--route", "shared-account.static-secret"); err != nil || !strings.Contains(out, "shared-account.static-secret") {
		t.Fatalf("record: %v %s", err, out)
	}
	if _, _, err := runCLI(t, "", "extension", "record", "asgard-ext-acme", "--route", "personal-account.oauth-customer"); err == nil {
		t.Fatal("record accepted a route the Extension does not declare")
	}

	out, _, err := runCLI(t, "", "extension", "update", "asgard-ext-acme")
	if err != nil || !strings.Contains(out, "nothing to do") {
		t.Fatalf("update at the latest: %v %s", err, out)
	}
	fake.latest = "v1.1.0"
	out, _, err = runCLI(t, "", "extension", "status")
	if err != nil || !strings.Contains(out, "v1.1.0 is out") || !strings.Contains(out, "routes shared-account.static-secret") {
		t.Fatalf("status with a newer version: %v\n%s", err, out)
	}
	out, _, err = runCLI(t, "", "extension", "update", "asgard-ext-acme")
	if err != nil || !strings.Contains(out, "fetched asgard-ext-acme v1.1.0") || !strings.Contains(out, "was v1.0.0") {
		t.Fatalf("update to the newer version: %v\n%s", err, out)
	}
	lock, _ := extension.ReadLock(root)
	if e := lock.Extensions[acmePath]; e.Version != "v1.1.0" || strings.Join(e.Routes, ",") != "shared-account.static-secret" {
		t.Fatalf("after update: %+v", e)
	}

	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "asgard-ext-stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := len(fake.calls)
	out, _, err = runCLI(t, "", "extension", "status", "--offline")
	if err != nil || !strings.Contains(out, "asgard-ext-stray  not in") || !strings.Contains(out, "not asked (--offline)") {
		t.Fatalf("status --offline: %v\n%s", err, out)
	}
	for _, c := range fake.calls[before:] {
		t.Errorf("--offline called %s", c)
	}
}

func TestExtensionGateStep(t *testing.T) {
	fake := &fakeExtensionPlatform{latest: "v1.0.0", archives: map[string][]byte{"v1.0.0": acmeArchive(t, "v1\n")}}
	sandboxEnv(t, fake.server(t).URL)
	root := fdeRepo(t)
	cmd := newGateCmd()
	cmd.SetContext(context.Background())
	if res := gateExtensions(cmd, root, "", false); res.Status != stepPass || res.Summary != "no Extension here" {
		t.Fatalf("empty repository: %+v", res)
	}
	if _, _, err := runCLI(t, "", "extension", "get", "asgard-ext-acme"); err != nil {
		t.Fatal(err)
	}
	if res := gateExtensions(cmd, root, "", false); res.Status != stepPass {
		t.Fatalf("as fetched: %+v", res)
	}
	fake.latest = "v1.1.0"
	if res := gateExtensions(cmd, root, "", false); res.Status != stepWarn || !strings.Contains(strings.Join(res.Details, "\n"), "v1.1.0 is out") {
		t.Fatalf("a newer version: %+v", res)
	}
	res := gateExtensions(cmd, root, "", true)
	if res.Status != stepPass || !strings.Contains(res.Summary, "not asked (--offline)") {
		t.Fatalf("--offline: %+v", res)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "asgard-ext-acme", "SKILL.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := gateExtensions(cmd, root, "", true); res.Status != stepWarn || !strings.Contains(strings.Join(res.Details, "\n"), "SKILL.md: edited since it was copied") {
		t.Fatalf("an edited file: %+v", res)
	}
}

func TestExtensionSearchAndShow(t *testing.T) {
	fake := &fakeExtensionPlatform{latest: "v1.0.0", archives: map[string][]byte{"v1.0.0": acmeArchive(t, "v1\n")}}
	sandboxEnv(t, fake.server(t).URL)
	t.Chdir(t.TempDir())

	out, _, err := runCLI(t, "", "extension", "search", "acme")
	if err != nil || !strings.Contains(out, "asgard-ext-acme") || !strings.Contains(out, "Acme Cloud  crm") || !strings.Contains(out, "build-time.static-secret") || strings.Contains(out, "verified") {
		t.Fatalf("search: %v\n%s", err, out)
	}
	if out, _, err := runCLI(t, "", "extension", "search", "nothing"); err != nil || !strings.Contains(out, "no Extension") {
		t.Fatalf("search with no match: %v\n%s", err, out)
	}
	out, _, err = runCLI(t, "", "extension", "show", "asgard-ext-acme")
	if err != nil || !strings.Contains(out, acmePath+" v1.0.0") || !strings.Contains(out, "v0.0.1  README.md has no frontmatter") || !strings.Contains(out, "requires skills db-query") {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if _, _, err := runCLI(t, "", "extension", "show", "asgard-ext-acme@1.0.0"); err == nil {
		t.Fatal("show accepted a version without the v")
	}
}
