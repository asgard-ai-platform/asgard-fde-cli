package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("doc-paths", check{
		Needs: "this repository",
		What:  "every path and package-qualified Go symbol this repository's own documents name, and **every command in the tree being named in README.md** - the opposite question to `audit-material --commands`, which nothing asked",
		Run:   runDocPaths,
	})
}

// The root documents, plus this directory's own - `hack/README.md` and the
// scripts, which tell somebody what to run and had the last stale reference.
//
// **This file is not in the list.** It names the deleted symbols it was written
// to catch, in the comments explaining why it resolves a symbol inside its own
// package, so checking itself would report its own subject. Same reason
// `internal/cli/self.go` parses Go source instead of grepping it: a note
// recording that something was removed must not read as naming it.
var docs = []string{
	"Goal.md", "README.md", "README.zh-TW.md", "AGENTS.md", "STRUCTURE.md",
	"APPROACH.md", "TASK.md", "CLAUDE.md",
	"hack/README.md",
	".env.example", ".agents/skills/consistency-checks/SKILL.md",
}

// A document this repository names that git does not keep.
//
// **`os.Stat` passing is not the same as the reader having the file**, and the
// difference is invisible to whoever wrote the line: a document under `.out/`
// resolves on the machine that produced it and on no other clone, and
// `AGENTS.md` says that directory may be deleted at any time. `TASK.md` pointed
// at a 294-line design argument there, and the check above reported nothing,
// because the path never matched `pathRe` at all.
//
// **It matches a document and not everything ignored, and the boundary is the
// whole reason it can exist.** Of the paths under `.out/` these documents name,
// all but one are a thing a command WRITES - the built binary, the ndjson a
// `hack` step emits, the record `verified` keeps - and failing on those would
// fire on correct material, which is worse than not checking. What a reader is
// sent to READ is prose, so the rule is a markdown file that git ignores.
//
// What it therefore does not catch: a generated document with another
// extension, and a tracked document that is merely wrong. The first is a gap;
// the second is what reading is for.
var ignoredDocRe = regexp.MustCompile(`([A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+\.md)`)

// ignoredDocs reports the markdown files named in text that exist and that git
// does not track, as `git check-ignore` decides.
func ignoredDocs(root, text string) []string {
	var cand []string
	seen := map[string]bool{}
	for _, m := range ignoredDocRe.FindAllStringSubmatch(text, -1) {
		p := m[1]
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			continue
		}
		cand = append(cand, p)
	}
	if len(cand) == 0 {
		return nil
	}
	// One call rather than one per path: check-ignore reads paths on stdin and
	// prints back the ones it ignores.
	cmd := exec.Command("git", "-C", root, "check-ignore", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(cand, "\n") + "\n")
	out, _ := cmd.Output()
	var bad []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			bad = append(bad, line)
		}
	}
	sort.Strings(bad)
	return bad
}

// A path inside this repository: a directory we own, then a file or directory
// under it.
//
// **Backticks are not required.** The gate section writes its commands inside a
// fenced block, where a path carries none, so the match is on the shape of the
// path - anchored to a directory this repository owns - wherever it appears.
var pathRe = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_./` + "`" + `-])((?:source|hack|internal|cmd|prompts|projects|docs|assets|plugins|\.github)/[A-Za-z0-9_./*<>-]*)`)

// A Go symbol written `pkg.Symbol`. The documents cite these as the place a
// rule is implemented, and a deleted one sends a reader looking for it.
var symbolRe = regexp.MustCompile("`([a-z][a-z0-9]*)\\.([A-Z][A-Za-z0-9_]*)`")

var declRe = "func|type|var|const"

// skip drops placeholders the scaffold expands at write time, globs, and paths
// inside a customer's repository rather than ours. None is a path on disk here
// and all are correct in prose.
func skipPath(p string) bool {
	return strings.ContainsAny(p, "<*") || strings.Contains(p, "__") ||
		strings.HasPrefix(p, "projects/") || strings.HasPrefix(p, "docs/") ||
		strings.HasPrefix(p, "assets/")
}

func goFiles(root string) []string {
	var out []string
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+".out"+string(filepath.Separator)) {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out
}

// goPackages returns the package names a `pkg.Symbol` reference could
// legitimately name: every directory under internal/, plus the last segment of
// every import path in the tree, plus the module root's own package.
//
// A reference to a package outside that set names nothing - which is how
// `config.Find` survived the deletion of the whole `config` package.
func goPackages(root string) map[string]bool {
	names := map[string]bool{"selfsrc": true}
	entries, _ := os.ReadDir(filepath.Join(root, "internal"))
	for _, e := range entries {
		if e.IsDir() {
			names[e.Name()] = true
		}
	}
	for _, f := range goFiles(root) {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "_ "))
			if strings.HasPrefix(line, `"`) && strings.HasSuffix(line, `"`) {
				p := strings.Trim(line, `"`)
				names[p[strings.LastIndex(p, "/")+1:]] = true
			}
		}
	}
	return names
}

// checkCommandsDocumented reports a command the binary answers to that
// `README.md` never names.
//
// **`audit-material --commands` is the mirror and cannot see this.** It
// resolves every command the material *writes* against the tree and fails on
// one that is gone; nothing asked the opposite question, so `local-env` and
// `reference` - a credential form and the way a customer's own document gets
// filed - were in the binary and in no README for as long as they existed.
func checkCommandsDocumented(root, binary string) ([]string, int, error) {
	out, err := exec.Command(binary, "--help").CombinedOutput()
	if err != nil {
		return nil, 0, fmt.Errorf("%s --help failed:\n%s", binary, out)
	}
	// **Each README on its own, not the two concatenated.** A command
	// documented in one and absent from the other is undocumented for whoever
	// reads that one, and the Chinese half drifted the same way the English one
	// did - both were missing the same two commands.
	readmes := map[string]string{}
	for _, name := range []string{"README.md", "README.zh-TW.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, 0, err
		}
		readmes[name] = string(data)
	}
	// cobra's own entries, and the binary's own name in the usage line.
	skip := map[string]bool{"asgard-cli": true, "completion": true, "help": true}
	var missing []string
	seen := 0
	for _, m := range regexp.MustCompile(`(?m)^\s{2}([a-z][a-z-]+)\s{2,}\S`).FindAllStringSubmatch(string(out), -1) {
		name := m[1]
		if skip[name] {
			continue
		}
		seen++
		// On a word boundary: a plain Contains passes `local-env` on the
		// string `local-envX`, which is the same false pass that let a
		// substring test elsewhere accept `region` inside "regional".
		//
		// The Chinese overview writes a subcommand without the binary's name,
		// so a bare name at the start of a line counts too - what is asked is
		// whether a reader of that file meets the command at all.
		named := regexp.MustCompile(`(?m)(?:asgard-cli |^\s+)` + regexp.QuoteMeta(name) + `\b`)
		for _, file := range []string{"README.md", "README.zh-TW.md"} {
			if !named.MatchString(readmes[file]) {
				missing = append(missing, name+" ("+file+")")
			}
		}
	}
	return missing, seen, nil
}

func runDocPaths(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	pkgs := goPackages(root)
	internal := map[string]string{"selfsrc": root}
	entries, _ := os.ReadDir(filepath.Join(root, "internal"))
	for _, e := range entries {
		if e.IsDir() {
			internal[e.Name()] = filepath.Join(root, "internal", e.Name())
		}
	}

	bad, checked := 0, 0
	for _, name := range docs {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			fmt.Printf("missing document  %s\n", name)
			bad++
			continue
		}
		for _, p := range ignoredDocs(root, string(data)) {
			fmt.Printf("untracked  %s -> `%s` exists here and git ignores it, so it is on no other clone\n",
				name, p)
			bad++
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range pathRe.FindAllStringSubmatch(line, -1) {
				p := m[1]
				if skipPath(p) {
					continue
				}
				checked++
				if _, err := os.Stat(filepath.Join(root, strings.TrimRight(p, "/"))); err != nil {
					fmt.Printf("gone  %s:%d -> `%s`\n", name, i+1, p)
					bad++
				}
			}
			for _, m := range symbolRe.FindAllStringSubmatch(line, -1) {
				pkg, sym := m[1], m[2]
				checked++
				if !pkgs[pkg] {
					fmt.Printf("gone  %s:%d -> `%s.%s` (no such package)\n", name, i+1, pkg, sym)
					bad++
					continue
				}
				dir, ours := internal[pkg]
				if !ours {
					// Somebody else's package. Whether it has that symbol is
					// their business and their version's.
					continue
				}
				// **Resolved inside the package that owns it.** A search of the
				// whole tree cannot tell `usecase.Index` from `strings.Index`,
				// and passed the first for months after it was deleted.
				if !declares(dir, sym) {
					fmt.Printf("gone  %s:%d -> `%s.%s`\n", name, i+1, pkg, sym)
					bad++
				}
			}
		}
	}
	binary := filepath.Join(root, ".out/asgard-cli")
	if len(args) > 0 {
		binary = args[0]
	}
	commands := 0
	if _, err := os.Stat(binary); err == nil {
		missing, n, err := checkCommandsDocumented(root, binary)
		if err != nil {
			return err
		}
		commands = n
		for _, c := range missing {
			fmt.Printf("undocumented  `asgard-cli %s` is in the command tree and that README never names it\n", c)
			bad++
		}
	} else {
		fmt.Printf("(no binary at %s, so the command tree is unchecked; go build -o .out/asgard-cli ./cmd/asgard-cli)\n", binary)
	}

	fmt.Printf("\n%d path(s) and symbol(s) named, %d command(s) in the tree, %d that are not there.\n",
		checked, commands, bad)
	if bad > 0 {
		fmt.Println("\nA document that names a file or a symbol this repository does not have")
		fmt.Println("sends a reader to look for it. Fix it, or delete the sentence.")
		return errFailed
	}
	return nil
}

func declares(dir, sym string) bool {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var b strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			b.Write(data)
			b.WriteString("\n")
		}
	}
	own := b.String()
	decl := regexp.MustCompile(`\b(?:` + declRe + `)\s+(?:\([^)]*\)\s*)?` + regexp.QuoteMeta(sym) + `\b`)
	field := regexp.MustCompile(`(?m)^\t` + regexp.QuoteMeta(sym) + `\s`)
	return decl.MatchString(own) || field.MatchString(own)
}
