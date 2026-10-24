package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
)

const gateReadme = `---
id: asgard-ext-acme
name: Acme
summary: Let the Agent read Acme records.
kind: integration
system:
  vendor: Acme
  product: Acme Cloud
  category: crm
routes:
  - id: shared-account.static-secret
listing: public
source: closed
owners: ["fde@example.com"]
---

# Acme
`

// isolateExtensionTest keeps a test off this machine's session and update check:
// nothing an Extension command does needs either.
func isolateExtensionTest(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ASGARD_NO_UPDATE_CHECK", "1")
	t.Setenv(auth.EnvHome, t.TempDir())
	t.Setenv(auth.EnvToken, "")
	t.Setenv(auth.EnvSessionFile, filepath.Join(t.TempDir(), "no-session.json"))
}

func writeExtension(t *testing.T, workflow string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "asgard-ext-acme")
	files := map[string]string{
		"README.md":                              gateReadme,
		"SKILL.md":                               "---\nname: asgard-ext-acme\ndescription: Read this when the customer connects Acme.\n---\n# Acme\n",
		"routes/shared-account.static-secret.md": "# route\nSee ../examples/shared-account.static-secret/.\n",
		"examples/README.md":                     "# examples\n",
		"examples/shared-account.static-secret/README.md":              "# set\n",
		"examples/shared-account.static-secret/templates/wf-acme.yaml": workflow,
	}
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

const acmeWorkflow = `# EXAMPLE - target projects/<project>/chart/app/templates/wf-acme.yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-acme
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  processors:
    - name: input
      type: update-context
    - name: call
      type: http-request
      configs:
        - name: url
          expression: {{ .Values.acme.apiBase | quote }}
        - name: method
          value: GET
    - name: err
      type: push-message
      configs:
        - name: payload
          expression: '({ ok: false, error: String(prevError) })'
  relationships:
    - from: {processor: input, relationName: success}
      to: {processor: call}
    - from: {processor: call, relationName: failure}
      to: {processor: err}
`

func TestExtensionGatePasses(t *testing.T) {
	isolateExtensionTest(t)
	dir := writeExtension(t, acmeWorkflow)
	out, _, err := runCLI(t, "", "extension", "gate", dir)
	if err != nil {
		t.Fatalf("gate failed: %v\n%s", err, out)
	}
	for _, want := range []string{"lint      pass", "examples  pass  1 CR(s)", "All 2 steps passed.", "asgard-cli extension get " + dir} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// A processor missing a required config is broken against every version of
// the platform, so the examples step fails and the command exits non-zero.
func TestExtensionGateFailsOnABrokenExample(t *testing.T) {
	isolateExtensionTest(t)
	broken := strings.Replace(acmeWorkflow, "        - name: method\n          value: GET\n", "", 1)
	dir := writeExtension(t, broken)
	out, _, err := runCLI(t, "", "extension", "gate", dir)
	if err == nil {
		t.Fatalf("gate passed an example with no method:\n%s", out)
	}
	if !strings.Contains(out, "examples  fail") || !strings.Contains(out, "W2 Workflow/wf-acme") {
		t.Errorf("output does not name the broken example:\n%s", out)
	}
}

// An agent used to FDE repositories runs `asgard-cli gate` here by habit.
func TestGateInAnExtensionRepositoryPointsToItsGate(t *testing.T) {
	isolateExtensionTest(t)
	dir := writeExtension(t, acmeWorkflow)
	t.Chdir(filepath.Join(dir, "routes"))
	_, _, err := runCLI(t, "", "gate", "--offline")
	if err == nil || !strings.Contains(err.Error(), "asgard-cli extension gate") || !strings.Contains(err.Error(), "asgard-ext-acme") {
		t.Errorf("err = %v, want it to name the Extension and its gate", err)
	}
}
