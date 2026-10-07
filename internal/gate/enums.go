package gate

import (
	"fmt"
	"sort"
	"strings"
)

// Enum values the CRDs accept, extracted from the kubebuilder validation
// markers in asgard-kube `pkg/apis/asgard/v1alpha1/types.go` at 42e8722 on
// 2026-10-07, keyed by json field name.
//
// **Some field names are deliberately absent**: `type`, `format`, `alias` and
// `status` each carry a different enum in different places, and the union of
// two enums accepts values that are wrong wherever they appear - the CRD's
// `type` unions values across processors, blobs, measures and connectors.
// `type` is checked properly by Processors instead, which knows it is looking
// at a processor.
//
// `status` was in this table until 2026-09-03 and is the one worth explaining:
// the Go types give it three values from a Loader's job status, and the
// generated CRD gives it six, because **Kubernetes' own condition schema uses
// the same field name**. The Go source is not the contract; the rendered CRD
// is, and it carries schema the Asgard types file never shows.
//
// Like every pinned copy in this package this can go stale in one direction
// only - the platform adding a value - so **E1 is a warning**, never a failure.
//
// `go run ./hack tables` holds this table against the generated CRDs. Run it
// after regenerating, and when asgard-kube moves.
const enumsRead = "2026-10-07, asgard-kube 42e8722"

var crdEnums = map[string][]string{
	"agentClass":                {"managed"},                                                                                    // 1 declaration(s)
	"authMode":                  {"api-key", "none"},                                                                            // 2 declaration(s)
	"blobStatus":                {"Bound", "Unbound"},                                                                           // 1 declaration(s)
	"botProviderClass":          {"discord", "generic", "line", "slack", "telegram"},                                            // 1 declaration(s)
	"completionModelClass":      {"anthropic", "aoai-chat", "builtin", "gemini", "mistral", "openai-chat"},                      // 1 declaration(s)
	"connectorStatus":           {"Failed", "Pending", "Provisioning", "Ready"},                                                 // 1 declaration(s)
	"dataConnectorClass":        {"athena", "hana", "mssql", "mysql", "netsuite", "oracle", "postgres", "salesforce", "trino"},  // 1 declaration(s)
	"debugMode":                 {"always", "never", "on-demand"},                                                               // 1 declaration(s)
	"documentClass":             {"audio", "docx", "image", "pdf", "pptx", "video"},                                             // 1 declaration(s)
	"effort":                    {"auto", "disabled", "high", "low", "max", "medium", "xhigh"},                                  // 2 declaration(s)
	"embeddingModelClass":       {"aoai", "builtin", "gemini", "mistral", "openai", "voyageai"},                                 // 1 declaration(s)
	"event":                     {"post-tool-call", "pre-tool-call", "session-end", "session-start", "user-prompt-submit"},      // 1 declaration(s)
	"handlerType":               {"command"},                                                                                    // 1 declaration(s)
	"imageGenerationModelClass": {"aoai", "builtin", "gemini", "openai"},                                                        // 1 declaration(s)
	"indexStatus":               {"Complete", "Failed", "Indexing", "Pending"},                                                  // 1 declaration(s)
	"knowledgeBaseClass":        {"asgard-baseline"},                                                                            // 1 declaration(s)
	"linkageType":               {"as-destination", "as-source"},                                                                // 1 declaration(s)
	"loaderClass":               {"bot", "database", "google-drive", "onedrive", "web"},                                         // 1 declaration(s)
	"phase":                     {"Expired", "Failed", "Pending", "Ready", "Refreshing"},                                        // 1 declaration(s)
	"provisionStatus":           {"Provisioned", "Provisioning"},                                                                // 1 declaration(s)
	"relationship":              {"many_to_one", "one_to_many", "one_to_one"},                                                   // 1 declaration(s)
	"scheme":                    {"http", "https"},                                                                              // 1 declaration(s)
	"signatureAlgorithm":        {"ES256", "ES512", "PS256"},                                                                    // 1 declaration(s)
	"sourceClass":               {"audio", "csv", "docx", "image", "json-line", "pdf", "pptx", "video", "xlsx"},                 // 2 declaration(s)
	"sslMode":                   {"disable", "require", "verify-ca", "verify-full"},                                             // 1 declaration(s)
	"storageStatus":             {"Provisioning", "Ready"},                                                                      // 1 declaration(s)
	"syncerClass":               {"bot", "database", "dropbox", "ftp", "git", "google-drive", "onedrive", "sftp", "smb", "web"}, // 1 declaration(s)
	"toolsetClass":              {"mcp-server", "workflow-tooling"},                                                             // 1 declaration(s)
	"transcriptionModelClass":   {"aoai", "builtin", "gemini", "openai"},                                                        // 1 declaration(s)
	"triggerClass":              {"cron"},                                                                                       // 1 declaration(s)
}

// Enums walks every rendered CR and reports a value the CRD's enum does not
// allow.
//
//	E1  a field whose json name carries a kubebuilder Enum marker holds a value
//	    outside it. A warning: the apiserver rejects it, so it is real, but this
//	    copy is pinned and the platform can add a value.
//
// Controller-written fields are not special-cased. A render never carries a
// status block, so there is nothing to skip; a stream captured from a live
// cluster does, which is why the one field name that collides with Kubernetes'
// conditions is out of the table rather than filtered at the call site.
func Enums(docs []Doc, opts Options) Result {
	var warnings []string
	seen := 0

	var walk func(path string, v any, kind, name string)
	walk = func(path string, v any, kind, name string) {
		switch t := v.(type) {
		case map[string]any:
			for k, e := range t {
				child := k
				if path != "" {
					child = path + "." + k
				}
				if s, ok := e.(string); ok {
					if allowed, has := crdEnums[k]; has && !contains(allowed, s) {
						seen++
						warnings = append(warnings, fmt.Sprintf(
							"E1 %s/%s: %s is %q, and the CRD allows only %s (read %s). The apiserver refuses anything else, so either it is a typo or the platform has added a value and this build is behind",
							kind, name, child, s, strings.Join(allowed, ", "), enumsRead))
					}
				}
				walk(child, e, kind, name)
			}
		case []any:
			for _, e := range t {
				walk(path, e, kind, name)
			}
		}
	}

	for _, d := range docs {
		walk("spec", d.Spec, d.Kind, d.Name)
	}

	sort.Strings(warnings)
	return Result{Warnings: warnings, Summary: fmt.Sprintf("%d enum field(s) out of range", seen)}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
