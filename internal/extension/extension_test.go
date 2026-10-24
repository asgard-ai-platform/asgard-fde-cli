package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodReadme = `---
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
  - id: shared-account.oauth-customer
    brakes: [dataconnector-semanticlayer]
oauth:
  customer_client:
    where: Acme Admin > Apps
    setup: setup/acme-client.md
listing: public
source: closed
owners: ["fde@example.com"]
---

# Acme
`

const goodSkill = `---
name: asgard-ext-acme
description: Read this when the customer connects Acme.
---
# Acme
`

// writeTree writes files under a fresh directory named asgard-ext-acme.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "asgard-ext-acme")
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func good() map[string]string {
	return map[string]string{
		"README.md":                          goodReadme,
		"SKILL.md":                           goodSkill,
		"routes/build-time.static-secret.md": "# route\n",
		"routes/shared-account.oauth-customer.md":                                              "# route\nSee ../examples/shared-account.oauth-customer/ and ../examples/shared-account.oauth-customer.runtime-skill/.\n",
		"setup/acme-client.md":                                                                 "# setup\n",
		"examples/README.md":                                                                   "# examples\n",
		"examples/shared-account.oauth-customer/README.md":                                     "# set\n",
		"examples/shared-account.oauth-customer/templates/wf-acme.yaml":                        "# EXAMPLE - target projects/<project>/chart/app/templates/wf-acme.yaml\nkind: Workflow\n",
		"examples/shared-account.oauth-customer.runtime-skill/README.md":                       "# set\n",
		"examples/shared-account.oauth-customer.runtime-skill/assets/skills/acme-api/SKILL.md": "---\nname: acme-api\ndescription: d\n---\n<!-- EXAMPLE - target assets/skills/acme-api/SKILL.md -->\n",
	}
}

func messages(fs []Finding, level Level) string {
	var b strings.Builder
	for _, f := range fs {
		if f.Level == level {
			b.WriteString(f.Path + ": " + f.Message + "\n")
		}
	}
	return b.String()
}

func TestLintClean(t *testing.T) {
	findings, meta := Lint(writeTree(t, good()))
	if len(findings) != 0 {
		t.Fatalf("want no findings, got:\n%s%s", messages(findings, Error), messages(findings, Warning))
	}
	if meta == nil || meta.ID != "asgard-ext-acme" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestLintErrors(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(map[string]string)
		expect string
	}{
		{"oauth-asgard route", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "build-time.static-secret", "personal-account.oauth-asgard", 1)
			f["routes/personal-account.oauth-asgard.md"] = "#\n"
			delete(f, "routes/build-time.static-secret.md")
		}, "is not supported"},
		{"personal static secret", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "build-time.static-secret", "personal-account.static-secret", 1)
			f["routes/personal-account.static-secret.md"] = "#\n"
			delete(f, "routes/build-time.static-secret.md")
		}, "goes through OAuth"},
		{"missing route document", func(f map[string]string) { delete(f, "routes/build-time.static-secret.md") }, "every declared route"},
		{"extra route document", func(f map[string]string) { f["routes/notes.md"] = "#\n" }, "not a declared route"},
		{"version field", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "kind: integration", "kind: integration\nversion: 1.0.0", 1)
		}, "the version is the git tag"},
		{"skill name", func(f map[string]string) {
			f["SKILL.md"] = strings.Replace(f["SKILL.md"], "name: asgard-ext-acme", "name: acme", 1)
		}, "must equal the id"},
		{"bad id", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "id: asgard-ext-acme", "id: acme", 1)
		}, "must be asgard-ext-"},
		{"unknown brake", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "dataconnector-semanticlayer", "use-a-spreadsheet", 1)
		}, "is not a brake"},
		{"customer client required", func(f map[string]string) {
			i := strings.Index(f["README.md"], "oauth:")
			j := strings.Index(f["README.md"], "listing:")
			f["README.md"] = f["README.md"][:i] + f["README.md"][j:]
		}, "oauth.customer_client: required"},
		{"status refused", func(f map[string]string) {
			f["README.md"] = strings.Replace(f["README.md"], "  - id: build-time.static-secret\n", "  - id: build-time.static-secret\n    status: verified\n", 1)
		}, "routes no longer carry a status"},
		{"env file", func(f map[string]string) { f["knowledge/.env"] = "TOKEN=x\n" }, "environment file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := good()
			c.edit(files)
			findings, _ := Lint(writeTree(t, files))
			if !Failed(findings) || !strings.Contains(messages(findings, Error), c.expect) {
				t.Fatalf("want an error containing %q, got:\n%s", c.expect, messages(findings, Error))
			}
		})
	}
}

func TestLintWarnings(t *testing.T) {
	files := good()
	files["examples/shared-account.oauth-customer/templates/ts-acme.yaml"] = "kind: Toolset\n"
	files["knowledge/SKILL.md"] = "---\nname: x\ndescription: d\n---\n"
	files["AGENTS.md"] = "# notes for the author's agent\n"
	files["examples/loose.yaml"] = "# EXAMPLE\n"
	files["examples/personal-account.oauth-customer/README.md"] = "# set\n"
	files["examples/whatever/README.md"] = "# set\n"
	findings, _ := Lint(writeTree(t, files))
	if Failed(findings) {
		t.Fatalf("warnings only, got errors:\n%s", messages(findings, Error))
	}
	w := messages(findings, Warning)
	for _, want := range []string{
		"examples/shared-account.oauth-customer/templates/ts-acme.yaml: first line does not say EXAMPLE",
		"knowledge/SKILL.md: a SKILL.md outside examples/<set>/assets/skills/",
		"examples/loose.yaml: a file directly in examples/",
		"examples/whatever: named after no declared route",
		"examples/personal-account.oauth-customer: named after no declared route",
		"examples/whatever: no document under routes/ refers to it",
	} {
		if !strings.Contains(w, want) {
			t.Errorf("missing warning %q in:\n%s", want, w)
		}
	}
	for _, unwanted := range []string{"acme-api", "AGENTS.md", "examples/shared-account.oauth-customer.runtime-skill: no document", "examples/shared-account.oauth-customer: no document"} {
		if strings.Contains(w, unwanted) {
			t.Errorf("unexpected warning about %q:\n%s", unwanted, w)
		}
	}
}

func TestLintExamplesNeedReadme(t *testing.T) {
	files := good()
	delete(files, "examples/README.md")
	delete(files, "examples/shared-account.oauth-customer/README.md")
	findings, _ := Lint(writeTree(t, files))
	w := messages(findings, Warning)
	if !strings.Contains(w, "examples: no README.md") || !strings.Contains(w, "examples/shared-account.oauth-customer: no README.md") {
		t.Errorf("missing README warnings:\n%s", w)
	}
}

func TestCollectShipsOnlyPublishedEntries(t *testing.T) {
	files := good()
	files["AGENTS.md"] = "x"
	files[".github/workflows/lint.yaml"] = "x"
	files["notes/draft.md"] = "x"
	got, err := Collect(writeTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	for rel := range got {
		top := strings.SplitN(rel, "/", 2)[0]
		if top == "AGENTS.md" || top == ".github" || top == "notes" {
			t.Errorf("%s was collected", rel)
		}
	}
	if _, ok := got["routes/build-time.static-secret.md"]; !ok {
		t.Error("a route document was not collected")
	}
}

func TestReplaceAndConflicts(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "asgard-ext-acme")
	digests, err := Replace(dst, map[string]File{"SKILL.md": {Data: []byte("a")}, "scripts/run.sh": {Data: []byte("b"), Exec: true}})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dst, "scripts/run.sh")); err != nil || info.Mode()&0o100 == 0 {
		t.Errorf("exec bit not kept: %v %v", info, err)
	}
	if c, _ := Conflicts(dst, digests); len(c) != 0 {
		t.Fatalf("fresh copy has conflicts: %+v", c)
	}
	_ = os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("edited"), 0o644)
	_ = os.WriteFile(filepath.Join(dst, "mine.md"), []byte("x"), 0o644)
	c, err := Conflicts(dst, digests)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c[0].Path != "SKILL.md" || c[1].Path != "mine.md" {
		t.Fatalf("conflicts = %+v", c)
	}
	if c, _ := Conflicts(filepath.Join(t.TempDir(), "absent"), nil); len(c) != 0 {
		t.Fatalf("absent destination has conflicts: %+v", c)
	}
}

func TestLockRoundTrip(t *testing.T) {
	root := t.TempDir()
	l, err := ReadLock(root)
	if err != nil {
		t.Fatal(err)
	}
	l.Extensions["github.com/asgard-ai-platform/asgard-ext-drive"] = Entry{Version: "v1.0.0", Files: map[string]string{}}
	l.Extensions["asgard-ext-acme"] = Entry{Source: SourceLocal, Path: "../asgard-ext-acme", Files: map[string]string{"SKILL.md": "sha256:x"}}
	if err := l.Write(root); err != nil {
		t.Fatal(err)
	}
	back, err := ReadLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if back.Version != 1 || back.Extensions["asgard-ext-acme"].Path != "../asgard-ext-acme" {
		t.Fatalf("round trip lost data: %+v", back)
	}
	if k, ok := back.Find("asgard-ext-drive"); !ok || k != "github.com/asgard-ai-platform/asgard-ext-drive" {
		t.Errorf("Find by id = %q %v", k, ok)
	}
	if k, ok := back.Find("asgard-ext-acme"); !ok || k != "asgard-ext-acme" {
		t.Errorf("Find local = %q %v", k, ok)
	}
}

// scripts/ runs on the FDE person's machine; a runtime skill's scripts run in
// the platform's Linux sandbox and may be shell.
func TestLintRefusesShellScriptsInScripts(t *testing.T) {
	files := good()
	files["scripts/read.sh"] = "#!/bin/sh\n"
	files["scripts/read.py"] = "#!/usr/bin/env python3\n"
	files["examples/shared-account.oauth-customer.runtime-skill/assets/skills/acme-api/scripts/fetch.sh"] = "# EXAMPLE\n"
	findings, _ := Lint(writeTree(t, files))
	errs := messages(findings, Error)
	if !strings.Contains(errs, "scripts/read.sh: a shell script in scripts/") {
		t.Errorf("no error for scripts/read.sh:\n%s", errs)
	}
	if strings.Contains(errs, "read.py") || strings.Contains(errs, "fetch.sh") {
		t.Errorf("an error for a Python script or a runtime skill's script:\n%s", errs)
	}
}
