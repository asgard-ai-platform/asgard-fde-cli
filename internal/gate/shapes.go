package gate

import (
	"fmt"
	"sort"
	"strings"
)

// The conditional CEL rules the CRDs carry, from asgard-kube `crd/*.yaml` at
// d0f8a8e, read 2026-10-18 by extracting every `x-kubernetes-validations` rule
// and keeping the two families a rendered object can be held against.
//
// **These were the gap.** `gate.Enums` covers the CRDs' enums and
// `gate.Constraints` their patterns and bounds; what was left was
// `XValidation`, and forty of the seventy-nine are `self == oldSelf`, which
// compares a proposal against an object already on the cluster and cannot be
// seen in a render. The rest are these, and nothing checked them - a chart
// whose credential sets neither `value` nor `valueFrom` renders, lints and
// passes a dry-run, and is refused at apply.
const shapesRead = "2026-10-18, asgard-kube d0f8a8e"

// exactlyOne is a set of sibling fields of which exactly one must be present.
//
// Keyed by the field set rather than by the path it appears at, because the
// same shape recurs in ninety-odd places - every credential in every CR is
// `[value valueFrom]` over `[secretKeyRef configMapKeyRef]` - and a table of
// paths would go stale the first time the platform added a connector. Any
// object carrying at least one member of a set is held to the rule.
var exactlyOne = [][]string{
	// A credential, and the Kubernetes source under it.
	{"value", "valueFrom"},
	{"secretKeyRef", "configMapKeyRef"},
	// Asgard's ValueExprTemplate: a literal, an expression, or a template.
	{"value", "expression", "template"},
	// Shapes that occur once each.
	{"sql", "sqlTable"},        // a SemanticLayer cube
	{"serviceName", "sid"},     // an Oracle DataConnector
	{"urls", "siteMapUrl"},     // a web Loader or Syncer
	{"folderId", "folderPath"}, // a OneDrive Loader or Syncer
	{"processor", "exit"},      // a Workflow relationship's target
}

// specBlocks is the one-of at the top of a spec: the class block. Keyed by kind
// because these names are ordinary words - `web`, `database`, `image` - and a
// path-free rule would fire on anything that happened to carry one.
var specBlocks = map[string][]string{
	"BotProvider":          {"generic", "telegram", "line", "discord", "slack", "msteams"},
	"CompletionModel":      {"aoaiChat", "openaiChat", "gemini", "anthropic", "mistral", "builtin"},
	"DataConnector":        {"postgres", "mysql", "mssql", "oracle", "salesforce", "hana", "netsuite", "trino", "athena"},
	"EmbeddingModel":       {"aoai", "openai", "gemini", "voyageai", "mistral", "builtin"},
	"ImageGenerationModel": {"openai", "aoai", "gemini", "builtin"},
	"Indexer":              {"csv", "xlsx", "pdf", "docx", "pptx", "jsonLine", "audio", "video", "image"},
	"Loader":               {"web", "database", "bot", "googleDrive", "oneDrive"},
	"Source":               {"pdf", "docx", "pptx", "audio", "video", "image", "csv", "xlsx", "jsonLine"},
	"Syncer":               {"web", "database", "bot", "googleDrive", "oneDrive", "git", "dropbox", "ftp", "sftp", "smb", "staticContent"},
	"TranscriptionModel":   {"openai", "aoai", "gemini", "builtin"},
}

// impliesBlock is a discriminator that requires a sibling of its own name.
//
// `toolsetClass: mcp-server` without `mcpServerConfig` is the one worth naming:
// it renders, it lints, and the Toolset it produces reaches the cluster and is
// refused there.
var impliesBlock = []struct {
	field  string
	values []string
}{
	{"toolsetClass", []string{"mcp-server"}},
	{"documentClass", []string{"pdf", "docx", "pptx", "audio", "video", "image"}},
	// The CRD states this one both ways: staticContent is set exactly when the
	// class is static-content. X2 counts class blocks without tying a block to
	// its class, so the class-implies-block half is checked here.
	{"syncerClass", []string{"static-content"}},
}

// blockFor names the sibling a discriminator value requires. Every one of these
// rules is `self.<field> != '<v>' || has(self.<v>)` - the block is named after
// the value - except mcp-server, whose block is `mcpServerConfig`, and
// static-content, whose block is `staticContent`.
func blockFor(field, value string) string {
	switch {
	case field == "toolsetClass" && value == "mcp-server":
		return "mcpServerConfig"
	case field == "syncerClass" && value == "static-content":
		return "staticContent"
	}
	return value
}

// Shapes checks the CRDs' conditional rules against a rendered chart.
//
//	X1  exactly one of a set of sibling fields is set - a credential that is
//	    neither a literal nor a reference, or one that is both.
//	X2  a kind's class block: exactly one of the per-class sub-objects.
//	X3  a discriminator value implies its block.
//
// Warnings rather than failures, for consistency with E1 and C1: this table is
// a copy of a contract that moves, and a rule dropped upstream would otherwise
// fail a chart the platform accepts. What it catches instead is the case that
// passes everything else and is refused at apply.
func Shapes(docs []Doc, opts Options) Result {
	var warnings []string
	checked := 0

	add := func(kind, name, msg string) {
		warnings = append(warnings, fmt.Sprintf("%s/%s: %s (read %s)", kind, name, msg, shapesRead))
	}

	var walk func(path string, v any, kind, name string)
	walk = func(path string, v any, kind, name string) {
		switch t := v.(type) {
		case map[string]any:
			for _, set := range exactlyOne {
				var present []string
				for _, f := range set {
					if _, ok := t[f]; ok {
						present = append(present, f)
					}
				}
				if len(present) == 0 {
					continue
				}
				checked++
				if len(present) > 1 {
					add(kind, name, fmt.Sprintf("X1 %s sets %s, and the CRD allows exactly one of [%s]",
						path, strings.Join(present, " and "), strings.Join(set, " ")))
				}
			}

			for _, r := range impliesBlock {
				value, ok := t[r.field].(string)
				if !ok {
					continue
				}
				for _, v := range r.values {
					if v != value {
						continue
					}
					checked++
					if want := blockFor(r.field, v); t[want] == nil {
						add(kind, name, fmt.Sprintf("X3 %s is %q and %s.%s is absent, which the CRD requires",
							path+"."+r.field, value, path, want))
					}
				}
			}

			for k, e := range t {
				child := k
				if path != "" {
					child = path + "." + k
				}
				walk(child, e, kind, name)
			}
		case []any:
			for i, e := range t {
				walk(fmt.Sprintf("%s[%d]", path, i), e, kind, name)
			}
		}
	}

	for _, d := range docs {
		if blocks, has := specBlocks[d.Kind]; has {
			var present []string
			for _, b := range blocks {
				if _, ok := d.Spec[b]; ok {
					present = append(present, b)
				}
			}
			checked++
			if len(present) != 1 {
				what := "none of"
				if len(present) > 1 {
					what = "both " + strings.Join(present, " and ") + " of"
				}
				add(d.Kind, d.Name, fmt.Sprintf("X2 spec sets %s [%s], and the CRD requires exactly one",
					what, strings.Join(blocks, " ")))
			}
		}
		walk("spec", d.Spec, d.Kind, d.Name)
	}

	sort.Strings(warnings)
	return Result{Warnings: warnings, Summary: fmt.Sprintf("%d conditional rule(s) checked", checked)}
}
