// Package render renders a release's chart with the native helm binary.
//
// **The native binary, not a shell wrapper.** helm ships a Windows build; a
// bash script with `BASH_SOURCE`, `set -euo pipefail` and a shell-out to yq
// does not run there without WSL.
//
// WHAT IT RENDERS IS NOT WHAT WILL DEPLOY, and the distinction matters enough
// that the command says so every time. A run's plan renders on the platform,
// with the release's real values and the real ids injected, and then checks
// every resulting CR against the cluster's own CRDs. This renders with
// placeholders and checks nothing. It is for the loop that is too fast to
// involve a push - does the template compile, does it produce the objects I
// meant - and the authoritative answer is always the plan report.
//
// It renders only. There is no install path: a Syncer pins its revision to the
// chart's appVersion and only a run stamps a real ref in, so a local helm
// upgrade would write a placeholder version as a git ref that does not exist,
// and the Syncer would fail to clone on every run afterwards.
package render

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/tool"
)

// Placeholder is the stand-in for a value the platform injects at run time.
//
// It is deliberately conspicuous. A chart that renders it into a field which
// has to be a real id produces something a reader can see is wrong, whereas an
// empty string renders as valid-looking YAML with a missing value.
const Placeholder = "PLACEHOLDER"

// Options is one render.
type Options struct {
	// Root is the repository root.
	Root string
	// Release is the release name as the declaration spells it.
	Release string
	// ConfigPath is the declaration, relative to Root; empty means the default.
	ConfigPath string
	// ValuesFiles are extra -f arguments, in overlay order. The chart's own
	// values.yaml applies first regardless; these are for the values a run
	// would take from the platform, supplied by hand for a local render.
	ValuesFiles []string
	// Namespace passed to helm. Empty renders with a placeholder, because a
	// chart that reads .Release.Namespace should show that it does.
	Namespace string
}

// Result describes what was rendered, for the caller to report.
type Result struct {
	Release   string
	Chart     string
	Namespace string
	Values    []string
}

// PlatformInternalSecrets are Secrets the platform provisions in every namespace
// for its own components, which a chart must not reference.
//
// **`preset-agent-hub` is Agent Hub's credential, not an engagement's.** The
// namespace reconciler creates it once with an `api_key` it generates and never
// rotates, because Agent Hub holds the same value outside the cluster. A CR that
// reads it is bound to a key the engagement cannot rotate - rotating it breaks
// Agent Hub - and whose leak exposes the namespace's platform functions rather
// than one chart. It resolves, applies and comes up Ready, so nothing downstream
// reports it; the gate is the one place that can.
//
// **It lives here with the rest of the naming contract** so the gate and the
// generator read one list.
var PlatformInternalSecrets = map[string]bool{
	"preset-agent-hub": true,
}

// ResourceAPIKey is the conventional key in the release's own Secret for a
// platform resource credential - `SourceSet.apiKey`, `Toolset.apiKey`,
// `BotProvider.adminApiKey`. The value is the engagement's own: when no use case
// calls the resource with it, a random value is the whole answer.
const ResourceAPIKey = "asgard_resource_api_key"

// HelmReleaseName is the helm release the Platform installs for a declared
// release name, and AppSecretName / AppConfigMapName are the two objects it
// creates and maintains for that release.
//
// A chart must READ these names, never write them - so the derivation lives
// here once and the gate's credential check uses the same one. Two copies of a
// naming rule is how a chart came to point at `app-secret`, a name that is real
// in the Terraform-provisioned demo namespaces and absent from every Release
// namespace.
func HelmReleaseName(releaseName string) string { return "iac-" + releaseName }

// AppSecretName is the Release's own Secret.
func AppSecretName(releaseName string) string { return HelmReleaseName(releaseName) + "-app-secret" }

// AppConfigMapName is the Release's own ConfigMap.
func AppConfigMapName(releaseName string) string {
	return HelmReleaseName(releaseName) + "-app-config"
}

// AsgardValues is the `asgard` block the platform injects into every run.
//
// The names are the contract - a chart reads them and never writes a namespace,
// a secret name or an environment id down - so they are listed here in full
// rather than assembled, and a local render supplies the shape with placeholder
// contents.
func AsgardValues(releaseName, namespace string) map[string]any {
	helm := HelmReleaseName(releaseName)
	if namespace == "" {
		namespace = Placeholder
	}
	return map[string]any{
		"release":              helm,
		"releaseName":          releaseName,
		"namespace":            namespace,
		"projectId":            Placeholder,
		"projectEnvironmentId": Placeholder,
		"appSecretName":        AppSecretName(releaseName),
		"appConfigMapName":     AppConfigMapName(releaseName),
		"ref":                  Placeholder,
		"commitSha":            Placeholder,
	}
}

// Run renders the release's chart to out.
//
// helm's own stderr goes to errOut, so that `asgard-cli render x | asgard-cli
// verify --rendered -` keeps the pipe clean - the property render.sh had by
// writing its progress to stderr.
func Run(ctx context.Context, opts Options, out, errOut io.Writer) (Result, error) {
	cfg, err := pipelineconfig.LoadFromRepo(opts.Root, opts.ConfigPath)
	if err != nil {
		return Result{}, err
	}
	decl, ok := cfg.Release(opts.Release)
	if !ok {
		return Result{}, fmt.Errorf("%s declares no release %q; it declares %v", cfg.Path, opts.Release, cfg.Names())
	}
	if decl.Chart == "" {
		return Result{}, fmt.Errorf("%s declares release %q with no chart", cfg.Path, opts.Release)
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = Placeholder
	}
	res := Result{
		// The helm release name is the platform's, so a rendered manifest
		// carries the names a deployed one will.
		Release:   "iac-" + opts.Release,
		Chart:     filepath.Join(opts.Root, filepath.FromSlash(decl.Chart)),
		Namespace: namespace,
		Values:    opts.ValuesFiles,
	}

	if err := mustExist(filepath.Join(res.Chart, "Chart.yaml")); err != nil {
		return res, err
	}
	for _, v := range opts.ValuesFiles {
		if err := mustExist(v); err != nil {
			return res, err
		}
	}

	injected, err := writeAsgardValues(opts.Release, namespace)
	if err != nil {
		return res, err
	}
	defer os.Remove(injected)

	args := []string{"template", res.Release, res.Chart, "--namespace", namespace}
	for _, v := range opts.ValuesFiles {
		args = append(args, "-f", v)
	}
	// Last, so the reserved block wins whatever a values file said about it -
	// which is what the platform does, and what makes a chart that defines its
	// own `asgard` key a warning rather than a working override.
	args = append(args, "-f", injected)

	cmd, err := tool.Helm.Command(ctx, args...)
	if err != nil {
		return res, err
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Dir = opts.Root

	if err := cmd.Run(); err != nil {
		// helm has already written its own diagnosis to errOut; repeating it
		// here would print the same thing twice.
		return res, fmt.Errorf("helm template failed for release %s", opts.Release)
	}
	return res, nil
}

// AsgardValuesFile writes the reserved block to a temporary file and returns
// its path, for a caller that wants to hand it to helm itself. The caller
// removes it.
//
// It is exported for `helm lint`, which needs exactly this file and nothing
// else. **A chart cannot declare the `asgard` block in its own values.yaml** -
// the platform overwrites it, and declaring it is a warning on every plan - so
// linting with no values file at all makes every chart that reads
// `.Values.asgard.projectEnvironmentId` fail on a nil pointer. Supplying this
// one file, and only this one, keeps the property the bare form was for: every
// OTHER `.Values.*` a template reads still has to have a default in the
// chart's own values.yaml, or the lint fails.
func AsgardValuesFile(releaseName, namespace string) (string, error) {
	return writeAsgardValues(releaseName, namespace)
}

// writeAsgardValues puts the reserved block in a temporary file for helm's -f.
func writeAsgardValues(releaseName, namespace string) (string, error) {
	body, err := yaml.Marshal(map[string]any{"asgard": AsgardValues(releaseName, namespace)})
	if err != nil {
		return "", fmt.Errorf("encode the asgard values: %w", err)
	}
	f, err := os.CreateTemp("", "asgard-values-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create a temporary values file: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("close %s: %w", f.Name(), err)
	}
	return f.Name(), nil
}

// mustExist turns a missing input into a sentence naming the file, since the
// alternative is helm's own error, which names a temporary path.
func mustExist(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("missing %s", path)
	}
	return nil
}
