package gate

import (
	"fmt"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/render"
)

// CredentialRefs checks that every credential reference points at an object
// that will exist.
//
// A Release owns exactly two: `iac-<release>-app-secret` and
// `iac-<release>-app-config`, both created and maintained by the Platform,
// whose names arrive as `.Values.asgard.appSecretName` and
// `.Values.asgard.appConfigMapName`. A chart may not declare a Secret or a
// ConfigMap of its own - the runner's kind whitelist rejects anything outside
// the asgard-ai.com group - so those two names are the only ones a
// `secretKeyRef` or `configMapKeyRef` can legitimately carry. The one
// exception is an OAuthCredential's tokens: they are in a Secret with the
// credential's own name, which the platform creates and owns, and a Workflow
// reads the token from there.
//
// Nothing else sees this. A name is a valid string, so the CR is valid, the
// CRD's own rules pass, the server dry run passes, apply succeeds, and the
// reference resolves to nothing at runtime with no error on the CR. It shipped
// exactly once and cost a deployment its database password: a chart helper read
// `.Values.appSecretName` instead of `.Values.asgard.appSecretName` and fell
// back to a literal `app-secret` - a name that is real in the
// Terraform-provisioned demo namespaces and absent from every Release
// namespace, which is why it read as correct all the way through.
//
// The local render supplies the real names, so what this catches is a template
// that wrote a name instead of reading one. With no release name - a stream
// rendered elsewhere - there is nothing to compare against and the check says
// so rather than passing quietly.
func CredentialRefs(docs []Doc, opts Options) Result {
	if len(docs) == 0 {
		return Result{Summary: "nothing rendered yet"}
	}
	// With no release name the expected names are unknown. That is reported
	// only when there is in fact something it would have checked: a `Result`
	// carries no "skipped", so a summary saying so prints under `ok` and reads
	// as a pass - and silence on a stream with no credential references at all
	// would be noise, not a finding.
	want := map[string]string{}
	if opts.Release != "" {
		want["secretKeyRef"] = render.AppSecretName(opts.Release)
		want["configMapKeyRef"] = render.AppConfigMapName(opts.Release)
	} else {
		want["secretKeyRef"] = ""
		want["configMapKeyRef"] = ""
	}

	var warnings []string
	checked := 0
	seen := map[string]bool{}
	// Every key this render actually reads, by reference kind - the other half
	// of the question `DeclaredKeys` answers.
	read := map[string]map[string]bool{"secretKeyRef": {}, "configMapKeyRef": {}}

	// **An OAuthCredential's token Secret is named by the render itself.** It
	// has the credential's name, so it is known with or without a release
	// name, and it holds three keys the platform writes - none of them
	// declared, because nothing injects them.
	tokenSecrets := map[string]bool{}
	for _, d := range docs {
		if d.Kind == "OAuthCredential" && d.Name != "" {
			tokenSecrets[d.Name] = true
		}
	}
	tokenRefs := 0
	deprecated := 0

	var walk func(v any, kind, name string)
	walk = func(v any, kind, name string) {
		switch t := v.(type) {
		case map[string]any:
			for ref, expect := range want {
				block, ok := t[ref].(map[string]any)
				if !ok {
					continue
				}
				got, _ := block["name"].(string)
				if got == "" {
					continue
				}
				checked++

				// **The key, not only the object.** A reference to the right
				// Secret with a key the declaration never names is injected by
				// nothing: `variables list` marks it ORPHAN, the run reports
				// `vars/orphan`, and lint, render and the dry run stay green
				// while the CR resolves to nothing at runtime. The declaration
				// says that about itself; `add` generates the reference and
				// leaves the declaring to somebody, and nothing said when it
				// was not done.
				// **The platform's own credential is not this chart's.** It
				// resolves, applies and comes up Ready, so nothing downstream
				// says a word; what it costs is a key the engagement cannot
				// rotate without breaking Agent Hub, shared with every platform
				// function in the namespace.
				if render.PlatformInternalSecrets[got] {
					msg := fmt.Sprintf("%s/%s: %s.name is %q, Agent Hub's own credential - read %s from this release's Secret instead",
						kind, name, ref, got, render.ResourceAPIKey)
					if !seen[msg] {
						seen[msg] = true
						warnings = append(warnings, msg)
					}
					continue
				}

				if ref == "secretKeyRef" && tokenSecrets[got] {
					tokenRefs++
					if key, _ := block["key"].(string); key != "" && !oauthTokenKeys[key] {
						msg := fmt.Sprintf("%s/%s: %s reads key %q from the token Secret of OAuthCredential %s, which holds only access_token, refresh_token and expires_at",
							kind, name, ref, key, got)
						if !seen[msg] {
							seen[msg] = true
							warnings = append(warnings, msg)
						}
					}
					continue
				}

				if key, _ := block["key"].(string); key != "" && (expect == "" || got == expect) {
					read[ref][key] = true
					if opts.DeclaredKeys != nil && !opts.DeclaredKeys[ref][key] {
						msg := fmt.Sprintf("%s/%s: %s reads key %q, and %s declares it for no release",
							kind, name, ref, key, pipelineconfig.FileName)
						if !seen[msg] {
							seen[msg] = true
							warnings = append(warnings, msg)
						}
					}
				}

				if expect == "" || got == expect {
					continue
				}
				msg := fmt.Sprintf("%s/%s: %s.name is %q, and the only %s this release has is %q",
					kind, name, ref, got, refObject(ref), expect)
				if !seen[msg] {
					seen[msg] = true
					warnings = append(warnings, msg)
				}
			}
			for _, child := range t {
				walk(child, kind, name)
			}
		case []any:
			for _, child := range t {
				walk(child, kind, name)
			}
		}
	}

	for _, d := range docs {
		walk(d.Spec, d.Kind, d.Name)
		// **`secretRef` is read by nothing.** asgard-kube v1.2.0 deprecated it
		// on both kinds. A provider's client is `clientId` and `clientSecret`,
		// which the walk above follows like any other reference, and the CRD
		// refuses a provider without them; a credential's tokens are in the
		// Secret of its own name, and its `secretRef` is accepted and ignored,
		// so a Workflow that reads the Secret it names reads nothing.
		if d.Kind == "OAuthProvider" || d.Kind == "OAuthCredential" {
			if mapOf(d.Spec["secretRef"]) != nil {
				deprecated++
				warnings = append(warnings, deprecatedSecretRef(d))
			}
		}
	}

	// **The other direction, and it is the quieter one.** A key that is
	// declared, set, and read by no template is not a mistake anything reports:
	// `variables list` shows it with a value, the run injects it into a Secret
	// nothing mounts, and every local check is green. It happens when a shape
	// changes - the tool that read the key is replaced - and the declaration
	// outlives its reader, which leaves a credential rotating on a schedule for
	// a system this chart no longer talks to.
	//
	// **A warning, not a problem.** A key declared ahead of the template that
	// will read it is ordinary mid-engagement work, and failing on it would
	// leave the gate red through the middle of every onboarding.
	unread := 0
	if opts.DeclaredKeys != nil {
		for _, ref := range []string{"secretKeyRef", "configMapKeyRef"} {
			var keys []string
			for k := range opts.DeclaredKeys[ref] {
				if !read[ref][k] {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				unread++
				warnings = append(warnings, fmt.Sprintf(
					"%s declares %s key %q and nothing in this render reads it. "+
						"A declared key is created and injected whether or not a CR names it, so this is "+
						"invisible: `variables list` shows a value, the run reports nothing, and every check "+
						"here is green. Either a template still has to read it - `%s` with `key: %s` - or the "+
						"shape that read it is gone and the declaration should go with it",
					pipelineconfig.FileName, ref, k, ref, k))
			}
		}
	}

	if len(warnings) > 0 {
		sort.Strings(warnings)
		for i, w := range warnings {
			if strings.Contains(w, "and nothing in this render reads it") ||
				strings.Contains(w, "the platform no longer reads it") ||
				strings.Contains(w, "from the token Secret of OAuthCredential") {
				continue
			}
			if strings.Contains(w, "Agent Hub's own credential") {
				warnings[i] = w + fmt.Sprintf(". It is never rotated because Agent Hub depends on the same value, so a chart reading it "+
					"cannot rotate its own key and shares its blast radius with every platform function in the "+
					"namespace. Write `name: {{ include \"<chart>.appSecretName\" . }}`, `key: %s`, declare the "+
					"key under `appSecret:`, and when no use case calls this resource from outside the platform, "+
					"set a random value: `asgard-cli pipeline variables set "+
					"--release <release> --kind secret %s --random`",
					render.ResourceAPIKey, render.ResourceAPIKey)
				continue
			}
			if strings.Contains(w, "declares it for no release") {
				warnings[i] = w + ". Declare it under `appSecret:` or `appConfigMap:` on the release " +
					"that deploys this chart, and set the value with `asgard-cli pipeline variables set`. " +
					"Setting it without declaring it is the silent half: it is stored, never injected, " +
					"and lint, render and the dry run all stay green"
				continue
			}
			warnings[i] = w + ". The Platform injects the name on every run, so the fix is to read it in the " +
				"template - `{{ include \"<chart>.appSecretName\" . }}` - rather than to create the object or " +
				"rename anything. Nothing downstream reports this: the CR is valid and the failure is at runtime"
		}
		var parts []string
		if n := len(warnings) - unread - deprecated; n > 0 {
			parts = append(parts, fmt.Sprintf("%d unusable", n))
		}
		if deprecated > 0 {
			parts = append(parts, fmt.Sprintf("%d deprecated secretRef(s)", deprecated))
		}
		if unread > 0 {
			parts = append(parts, fmt.Sprintf("%d declared key(s) read by nothing", unread))
		}
		return Result{
			Warnings: warnings,
			Summary:  fmt.Sprintf("%d credential reference(s), %s", checked, strings.Join(parts, ", ")),
		}
	}
	if checked == 0 {
		return Result{Summary: "no credential references"}
	}
	if n := checked - tokenRefs; opts.Release == "" && n > 0 {
		summary := fmt.Sprintf("%d credential reference(s), none checked", checked)
		if tokenRefs > 0 {
			summary = fmt.Sprintf("%d credential reference(s), %d not checked", checked, n)
		}
		return Result{
			Warnings: []string{fmt.Sprintf(
				"%d credential reference(s) went unchecked: with no release name the injected Secret and "+
					"ConfigMap names are unknown, so a reference to an object that will not exist cannot be "+
					"told apart from a correct one. Render through `asgard-cli verify <release>` to check them",
				n)},
			Summary: summary,
		}
	}
	return Result{Summary: fmt.Sprintf("%d credential reference(s), all to this release's own objects", checked)}
}

// The keys the platform writes into an OAuthCredential's token Secret.
var oauthTokenKeys = map[string]bool{"access_token": true, "refresh_token": true, "expires_at": true}

func deprecatedSecretRef(d Doc) string {
	if d.Kind == "OAuthProvider" {
		return fmt.Sprintf("%s/%s: spec.secretRef is deprecated and the platform no longer reads it. The client is "+
			"spec.clientId and spec.clientSecret, each `valueFrom.secretKeyRef` to a key of this release's Secret "+
			"declared under `appSecret:` - the key names are the chart's own, so one release can hold several "+
			"clients - and secretRef should go", d.Kind, d.Name)
	}
	return fmt.Sprintf("%s/%s: spec.secretRef is deprecated and the platform no longer reads it. The tokens are in "+
		"the Secret named %q, which the platform creates and owns; remove secretRef, and read the token with "+
		"`secretKeyRef` `name: %s`, `key: access_token`", d.Kind, d.Name, d.Name, d.Name)
}

func refObject(ref string) string {
	if strings.HasPrefix(ref, "config") {
		return "ConfigMap"
	}
	return "Secret"
}
