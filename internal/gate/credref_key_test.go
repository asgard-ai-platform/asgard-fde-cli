package gate

import (
	"strings"
	"testing"
)

// **A reference to the right Secret with a key nothing declares is injected by
// nothing.** The declaration says so about itself: an undeclared key is stored
// and never injected, `variables list` marks it ORPHAN, the run reports
// `vars/orphan`, and lint, render and the dry run all stay green while the CR
// resolves to nothing at runtime.
//
// `asgard-cli add dataconnector` generates the reference and leaves the
// declaring to somebody. Across the nine DataConnector classes that is fourteen
// keys, and before this nothing said they were undeclared - the credential
// check looked at the object's name and passed.
func TestCredentialRefsChecksTheKeyNotOnlyTheObject(t *testing.T) {
	doc := Doc{Kind: "DataConnector", Name: "dc-erp", Spec: map[string]any{
		"postgres": map[string]any{
			"password": map[string]any{
				"valueFrom": map[string]any{
					"secretKeyRef": map[string]any{
						"name": "iac-erp-dev-app-secret",
						"key":  "erp_db_password",
					},
				},
			},
		},
	}}
	opts := Options{Release: "erp-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {}, "configMapKeyRef": {},
	}}

	got := Placeholder(CredentialRefs([]Doc{doc}, opts))
	if !strings.Contains(got, "declares it for no release") {
		t.Errorf("an undeclared key passed:\n%s", got)
	}

	opts.DeclaredKeys["secretKeyRef"]["erp_db_password"] = true
	if got := Placeholder(CredentialRefs([]Doc{doc}, opts)); strings.Contains(got, "declares it for no release") {
		t.Errorf("a declared key was still reported:\n%s", got)
	}

	// **Nil is "the declaration was not read", not "it declares none."** A
	// check that cannot see the declaration must say nothing rather than
	// report every key in the chart.
	opts.DeclaredKeys = nil
	if got := Placeholder(CredentialRefs([]Doc{doc}, opts)); strings.Contains(got, "declares it for no release") {
		t.Errorf("reported an undeclared key with no declaration to compare against:\n%s", got)
	}
}

// Placeholder joins a Result's warnings and problems for assertion.
func Placeholder(r Result) string {
	return strings.Join(append(append([]string{}, r.Problems...), r.Warnings...), "\n")
}

// **The platform's own credential is reported, and says what to read instead.**
//
// `preset-agent-hub` resolves, applies and comes up Ready, so lint, the dry run
// and the plan are all green on a chart that reads it - which binds that chart
// to Agent Hub's key, one the engagement cannot rotate without breaking Agent
// Hub. The gate is the only step that can say so.
func TestPlatformInternalSecretIsReported(t *testing.T) {
	doc := Doc{Kind: "SourceSet", Name: "ss-rma", Spec: map[string]any{
		"apiKey": map[string]any{"valueFrom": map[string]any{
			"secretKeyRef": map[string]any{"name": "preset-agent-hub", "key": "api_key"},
		}},
	}}
	opts := Options{Release: "cs-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {}, "configMapKeyRef": {},
	}}
	got := Placeholder(CredentialRefs([]Doc{doc}, opts))
	for _, want := range []string{"Agent Hub's own credential", "asgard_resource_api_key", "--random"} {
		if !strings.Contains(got, want) {
			t.Errorf("a reference to preset-agent-hub did not say %q:\n%s", want, got)
		}
	}
	// One finding, not a second "declares it for no release" for `api_key`:
	// the fix is to stop reading that Secret, not to declare its key.
	if strings.Contains(got, "declares it for no release") {
		t.Errorf("asked to declare a key in the platform's own Secret:\n%s", got)
	}

	// **The shape to write is clean.**
	doc.Spec["apiKey"].(map[string]any)["valueFrom"].(map[string]any)["secretKeyRef"] = map[string]any{
		"name": "iac-cs-dev-app-secret", "key": "asgard_resource_api_key",
	}
	opts.DeclaredKeys["secretKeyRef"]["asgard_resource_api_key"] = true
	if got := Placeholder(CredentialRefs([]Doc{doc}, opts)); got != "" {
		t.Errorf("the release's own resource key was reported:\n%s", got)
	}
}

// **A key that is declared, set, and read by nothing is the direction nothing
// reported.** `variables list` marks a value with no declaration ORPHAN, and
// the run reports `vars/orphan`; the reverse - a declaration with no reader -
// is created and injected on every run into a Secret no CR names, so it shows
// as a value that is present and working. It arrives when a shape changes and
// the template that read the key is replaced, which leaves a live credential
// declared for a system the chart no longer talks to.
func TestCredentialRefsReportsADeclaredKeyNothingReads(t *testing.T) {
	doc := Doc{Kind: "DataConnector", Name: "dc-erp", Spec: map[string]any{
		"postgres": map[string]any{
			"password": map[string]any{
				"valueFrom": map[string]any{
					"secretKeyRef": map[string]any{
						"name": "iac-erp-dev-app-secret",
						"key":  "erp_db_password",
					},
				},
			},
		},
	}}
	opts := Options{Release: "erp-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef":    {"erp_db_password": true, "old_api_key": true},
		"configMapKeyRef": {"retired_base_url": true},
	}}

	got := Placeholder(CredentialRefs([]Doc{doc}, opts))
	for _, want := range []string{`"old_api_key"`, `"retired_base_url"`} {
		if !strings.Contains(got, want) {
			t.Errorf("a declared key nothing reads went unreported (%s):\n%s", want, got)
		}
	}
	// The one that IS read must not be reported in either direction.
	if strings.Contains(got, `"erp_db_password"`) {
		t.Errorf("a declared key that the render reads was reported:\n%s", got)
	}

	// **Nil is "the declaration was not read".** Same rule as the other
	// direction: with nothing to compare against, say nothing.
	opts.DeclaredKeys = nil
	if got := Placeholder(CredentialRefs([]Doc{doc}, opts)); strings.Contains(got, "nothing in this render reads it") {
		t.Errorf("reported an unread declaration with no declaration to compare against:\n%s", got)
	}
}

// **A key read only through the platform's own Secret is not a reader of the
// release's declaration.** `preset-agent-hub` carries `api_key`; a release that
// also declares its own `api_key` has one nothing reads, and the finding about
// the platform Secret must not hide that.
func TestPlatformInternalReadDoesNotSatisfyADeclaration(t *testing.T) {
	doc := Doc{Kind: "SourceSet", Name: "ss-rma", Spec: map[string]any{
		"apiKey": map[string]any{"valueFrom": map[string]any{
			"secretKeyRef": map[string]any{"name": "preset-agent-hub", "key": "api_key"},
		}},
	}}
	opts := Options{Release: "cs-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {"api_key": true}, "configMapKeyRef": {},
	}}
	if got := Placeholder(CredentialRefs([]Doc{doc}, opts)); !strings.Contains(got, "nothing in this render reads it") {
		t.Errorf("a declaration satisfied only by the platform's own Secret passed:\n%s", got)
	}
}

// **An OAuthProvider reads client_id and client_secret without naming them.**
// Its secretRef names the release's Secret and the platform reads those two
// keys from it, so a chart that declares them for a customer's own Google
// client is not declaring keys "read by nothing".
func TestCredentialRefsCountsTheOAuthProviderKeys(t *testing.T) {
	oap := Doc{Kind: "OAuthProvider", Name: "oap-google-drive", Spec: map[string]any{
		"secretRef": map[string]any{"name": "iac-purchasing-dev-app-secret"},
	}}
	opts := Options{Release: "purchasing-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {"client_id": true, "client_secret": true}, "configMapKeyRef": {},
	}}
	if got := Placeholder(CredentialRefs([]Doc{oap}, opts)); strings.Contains(got, "nothing in this render reads it") {
		t.Errorf("the OAuthProvider's two keys were reported as unread:\n%s", got)
	}
	// Without the OAuthProvider they are unread, and still said to be.
	if got := Placeholder(CredentialRefs([]Doc{{Kind: "Agent", Name: "ag-x", Spec: map[string]any{}}}, opts)); !strings.Contains(got, `"client_id" and nothing in this render reads it`) {
		t.Errorf("an unread client_id was not reported:\n%s", got)
	}
}
