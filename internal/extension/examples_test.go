package extension

import (
	"strings"
	"testing"
)

func TestNeutralize(t *testing.T) {
	src := `metadata:
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
    {{- with .Values.asgard.projectEnvironmentId }}
    asgard-ai.com/project-environment-id: {{ . | quote }}
    {{- end }}
spec:
  url: {{ printf "%s/x" .Values.api | quote }} + id
`
	want := `metadata:
  labels:
    asgard-ai.com/project-environment-id: __helm__
spec:
  url: __helm__ + id
`
	if got := string(neutralize([]byte(src))); got != want {
		t.Errorf("neutralize =\n%s\nwant\n%s", got, want)
	}
}

// The examples that motivated the check: a failure branch off update-context,
// written with the template actions a pseudo chart carries. What must stay
// silent is everything about a value the FDE repository fills in.
func TestCheckExamples(t *testing.T) {
	files := good()
	files["examples/shared-account.oauth-customer/templates/wf-acme.yaml"] = `# EXAMPLE - target projects/<project>/chart/app/templates/wf-acme.yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-acme
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  variables:
    - name: token
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: acme_token
  processors:
    - name: input
      type: update-context
      configs:
        - name: id
          expression: prevPayload.id
    - name: call
      type: http-request
      configs:
        - name: url
          expression: {{ printf "%s/records/" .Values.acme.apiBase | quote }} + id
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
    - from: {processor: input, relationName: failure}
      to: {processor: err}
    - from: {processor: call, relationName: failure}
      to: {processor: err}
`
	// Not a CR, and a range spread over lines that the stand-in cannot fix.
	files["examples/shared-account.oauth-customer/values.yaml"] = "acme:\n  apiBase: https://api.acme.example\n"
	files["examples/shared-account.oauth-customer/templates/many.yaml"] = "# EXAMPLE\n{{- range .Values.items }}\n- kind: {{ .kind }}\n  x: [\n{{- end }}\n"
	dir := writeTree(t, files)

	findings, crs := CheckExamples(dir)
	if crs != 1 {
		t.Errorf("read %d CRs, want 1 (values.yaml is not one, the runtime skill is skipped)", crs)
	}
	warns := messages(findings, Warning)
	if !strings.Contains(warns, `wf-acme.yaml: W7 Workflow/wf-acme processor "input"`) {
		t.Errorf("no W7 for the failure branch off update-context:\n%s", warns)
	}
	if !strings.Contains(warns, "many.yaml: not checked") {
		t.Errorf("an example the stand-in cannot parse is not reported:\n%s", warns)
	}
	if strings.Contains(warns+messages(findings, Error), helmSentinel) {
		t.Errorf("a finding is about the stand-in rather than the example:\n%s", warns)
	}
	if errs := messages(findings, Error); errs != "" {
		t.Errorf("errors = %s, want none", errs)
	}
}
