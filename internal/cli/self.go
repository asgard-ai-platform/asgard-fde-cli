package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli"
)

// goStrings returns every string literal in this repository's Go source, keyed
// by file.
//
// **A printed string makes the same claim a document does**: that this build
// answers to what it names. The material and the scaffold templates are
// checked, and without this the tool's own output is not - a renamed command
// leaves dead names in the help, in `check`'s findings and in the generator's
// warnings. The source is embedded at the module root; see that package for
// why.
//
// **Parsed rather than grepped, so that comments are excluded.** A comment
// recording that a command *was* removed must not read as naming it - the
// package doc above says `asgard-cli add` and must not become a claim that
// `add` prints it - and a regular expression over the source cannot tell a
// comment from a message.
//
// **What it reads inside a string is a code segment**, the same as in the
// material: a backticked command or an indented block. `asgard-cli x y` in
// bare prose is not checked, and widening to that produces false positives -
// "a newer asgard-cli than the one you are running" resolves `than` as a
// subcommand. The convention is the check: write a command the way the
// material does.
func goStrings() (map[string]string, error) {
	out := map[string]string{}
	entries, err := fs.Glob(selfsrc.Go, "internal/*/*.go")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	for _, name := range entries {
		src, err := selfsrc.Go.ReadFile(name)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			// A file this build cannot parse is this build's problem, not the
			// material's, and failing the audit for it would be the worst of
			// both.
			continue
		}
		// **Each literal is written at the line it occupies in the file**, so
		// a finding's line number opens the source. Concatenating them instead
		// numbered the findings by literal, and `needs.go:11` pointed at a
		// comment - a reader following it sees nothing wrong and concludes the
		// checker is.
		lines := make([]string, strings.Count(string(src), "\n")+1)
		any := false
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text := literalText(lit.Value)
			// A raw string spans lines of its own; it starts at its first and
			// carries the rest with it.
			at := fset.Position(lit.Pos()).Line - 1
			if at >= 0 && at < len(lines) {
				lines[at] += text
				any = true
			}
			return true
		})
		if any {
			out[name] = strings.Join(lines, "\n")
		}
	}
	return out, nil
}

// literalText unquotes a Go string literal well enough to read commands out of
// it. `strconv.Unquote` refuses a raw string containing a backquote, which is
// most of the help text here, so this does the two things that matter: drop
// the delimiters, and turn an escaped newline into a real one so a command
// split across concatenated lines is still one line.
func literalText(v string) string {
	if len(v) >= 2 {
		v = v[1 : len(v)-1]
	}
	return strings.ReplaceAll(v, `\n`, "\n")
}

// repoDocs returns this repository's own documentation, keyed by file name.
//
// Not part of material(): these land nowhere, so the pointer and provenance
// rules do not reach them. What does reach them is `--commands` - a command
// named in AGENTS.md is a command an agent is about to run.
func repoDocs() (map[string]string, error) {
	out := map[string]string{}
	entries, err := fs.Glob(selfsrc.Docs, "*.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	for _, name := range entries {
		body, err := selfsrc.Docs.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out[name] = string(body)
	}
	return out, nil
}

// repoHack returns the maintainer's gate - `hack/` - keyed by path.
//
// Read for the same reason as the skills: `go run ./hack list` prints each
// check's `What:` string, and a description that outlives the behaviour it
// describes is what the next maintainer acts on.
func repoHack() (map[string]string, error) {
	out := map[string]string{}
	var entries []string
	for _, pattern := range []string{"hack/*.go", "hack/internal/*/*.go"} {
		found, err := fs.Glob(selfsrc.Hack, pattern)
		if err != nil {
			return nil, err
		}
		entries = append(entries, found...)
	}
	sort.Strings(entries)
	for _, name := range entries {
		body, err := selfsrc.Hack.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out[name] = string(body)
	}
	return out, nil
}

// repoSkills returns this repository's own maintenance skills, keyed by path.
//
// Separate from `repoDocs` only because they live in a directory rather than
// at the root. Same reason for reading them: a command one of them names has
// to exist.
func repoSkills() (map[string]string, error) {
	out := map[string]string{}
	entries, err := fs.Glob(selfsrc.Skills, ".agents/skills/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	for _, name := range entries {
		body, err := selfsrc.Skills.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out[name] = string(body)
	}
	return out, nil
}
