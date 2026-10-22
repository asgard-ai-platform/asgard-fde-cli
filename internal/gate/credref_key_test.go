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

// **An OAuthProvider's client is two references like any other.** Its
// clientId and clientSecret read keys of the release's Secret under names the
// chart chooses, so a declaration of them is read, and a key nothing declares
// is said to be.
func TestCredentialRefsFollowsTheOAuthProviderClient(t *testing.T) {
	ref := func(key string) map[string]any {
		return map[string]any{"valueFrom": map[string]any{
			"secretKeyRef": map[string]any{"name": "iac-purchasing-dev-app-secret", "key": key},
		}}
	}
	oap := Doc{Kind: "OAuthProvider", Name: "oap-google-drive", Spec: map[string]any{
		"clientId":     ref("google_drive_client_id"),
		"clientSecret": ref("google_drive_client_secret"),
	}}
	opts := Options{Release: "purchasing-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {"google_drive_client_id": true, "google_drive_client_secret": true}, "configMapKeyRef": {},
	}}
	if got := Placeholder(CredentialRefs([]Doc{oap}, opts)); got != "" {
		t.Errorf("a provider whose client is declared was reported:\n%s", got)
	}
	delete(opts.DeclaredKeys["secretKeyRef"], "google_drive_client_secret")
	if got := Placeholder(CredentialRefs([]Doc{oap}, opts)); !strings.Contains(got, `reads key "google_drive_client_secret", and .asgard-pipeline.yaml declares it for no release`) {
		t.Errorf("an undeclared client secret key passed:\n%s", got)
	}
}

// **`secretRef` is read by nothing, on either kind.** asgard-kube v1.2.0
// deprecated both. A provider's own fixed `client_id` and `client_secret` are
// no longer read through it, so declaring them is declaring keys nothing reads;
// a credential's is ignored, and the warning names the Secret the tokens are in.
func TestCredentialRefsWarnsOnADeprecatedSecretRef(t *testing.T) {
	oap := Doc{Kind: "OAuthProvider", Name: "oap-google-drive", Spec: map[string]any{
		"secretRef": map[string]any{"name": "iac-purchasing-dev-app-secret"},
	}}
	oac := Doc{Kind: "OAuthCredential", Name: "oac-google-drive", Spec: map[string]any{
		"providerRef": "oap-google-drive",
		"secretRef":   map[string]any{"name": "oac-google-drive-token"},
	}}
	opts := Options{Release: "purchasing-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {"client_id": true}, "configMapKeyRef": {},
	}}
	r := CredentialRefs([]Doc{oap, oac}, opts)
	got := Placeholder(r)
	for _, want := range []string{
		"OAuthProvider/oap-google-drive: spec.secretRef is deprecated and the platform no longer reads it",
		`OAuthCredential/oac-google-drive: spec.secretRef is deprecated and the platform no longer reads it. The tokens are in the Secret named "oac-google-drive"`,
		`"client_id" and nothing in this render reads it`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "The Platform injects the name on every run") {
		t.Errorf("a deprecated secretRef was given the advice for a literal Secret name:\n%s", got)
	}
	if !strings.Contains(r.Summary, "2 deprecated secretRef(s)") || strings.Contains(r.Summary, "unusable") {
		t.Errorf("summary %q", r.Summary)
	}
}

// **An OAuthCredential's token Secret is the one other Secret a reference may
// name.** The platform keeps the tokens in a Secret of the credential's own
// name, and a Workflow reads access_token from it as a variable. The name is
// known from the render, so it is checked with or without a release name; a key
// the platform does not write is reported, and a credential this render does
// not deploy is not a token Secret.
func TestCredentialRefsKnowsTheOAuthCredentialTokenSecret(t *testing.T) {
	oac := Doc{Kind: "OAuthCredential", Name: "oac-google-drive", Spec: map[string]any{"providerRef": "oap-google-drive"}}
	wf := func(key string) Doc {
		return Doc{Kind: "Workflow", Name: "wf-drive-search", Spec: map[string]any{
			"variables": []any{map[string]any{"name": "driveToken", "valueFrom": map[string]any{
				"secretKeyRef": map[string]any{"name": "oac-google-drive", "key": key},
			}}},
		}}
	}
	opts := Options{Release: "purchasing-dev", DeclaredKeys: map[string]map[string]bool{
		"secretKeyRef": {}, "configMapKeyRef": {},
	}}

	r := CredentialRefs([]Doc{oac, wf("access_token")}, opts)
	if got := Placeholder(r); got != "" {
		t.Errorf("reading the credential's token was reported:\n%s", got)
	}
	r = CredentialRefs([]Doc{oac, wf("access_token")}, Options{})
	if got := Placeholder(r); got != "" || !strings.Contains(r.Summary, "all to this release's own objects") {
		t.Errorf("with no release name the token reference went unchecked: %q\n%s", r.Summary, got)
	}

	if got := Placeholder(CredentialRefs([]Doc{oac, wf("token")}, opts)); !strings.Contains(got, `reads key "token" from the token Secret of OAuthCredential oac-google-drive`) {
		t.Errorf("a key the platform does not write passed:\n%s", got)
	}
	if got := Placeholder(CredentialRefs([]Doc{wf("access_token")}, opts)); !strings.Contains(got, `secretKeyRef.name is "oac-google-drive", and the only Secret this release has is "iac-purchasing-dev-app-secret"`) {
		t.Errorf("a credential this render does not deploy was taken for a token Secret:\n%s", got)
	}
}
