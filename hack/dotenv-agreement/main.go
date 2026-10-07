// Command dotenv-agreement checks that the two implementations of the .env
// format read the same file the same way.
//
// There are two because there have to be: `asgard-cli local-env` writes the
// file in Go, and the db-query scripts read it in python. A format with two
// implementations and nothing comparing them drifts silently, and the way it
// shows up is the worst kind - the UI displays one value, the query connects
// with another, and neither says anything is wrong.
//
//	go run ./hack/dotenv-agreement
//
// It needs python3 on PATH. Without it the python half is reported as skipped
// rather than passed, because a skip that prints like a pass is how the old
// gate shipped a red step as green for a month.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/localenv"
)

// cases are raw right-hand sides and what they must decode to.
//
// Every one of them is a rule somebody could reasonably get wrong, and the
// first is the bug that started this: a note written after the `=` was read as
// the value, so a placeholder key looked filled in.
var cases = []struct{ raw, want string }{
	{"   # the bare host, no scheme", ""},
	{"#immediate", ""},
	{"abc#123", "abc#123"},           // no space before #, so not a comment
	{"abc #123", "abc"},              // space before #, so it is
	{"  spaced  ", "spaced"},         // bare values are trimmed
	{`"  keep me  "`, "  keep me  "}, // quotes are how spaces survive
	{`'has # hash'`, "has # hash"},
	{"", ""},
	{"plain", "plain"},
	{"value with spaces", "value with spaces"},
	{`"unclosed`, "unclosed"},
	{"a b c   # tail", "a b c"},
	{`"line1\nline2"`, "line1\nline2"}, // how a PEM lives on one line
	{`"say \"hi\""`, `say "hi"`},
	{`"C:\\Users\\x"`, `C:\Users\x`},
	{`"tab\there"`, "tab\there"},
	{`'literal \n stays'`, `literal \n stays`}, // single quotes are literal
	{`"#hash first"`, "#hash first"},           // only quoting can express a leading #
}

// preserved is a file written by hand, and what must still be in it after a
// save that changes exactly one value.
//
// The comments are the reason: this file is edited by people as well as by the
// form, and a note somebody wrote for the next reader is worth as much as the
// value beside it. Two ways to lose one were shipped and caught here - a
// trailing comment dropped when its line was rewritten, and a quoted value
// re-spelled bare on a line nobody touched.
const preserved = `# a banner nobody should lose
# --- GROUP (postgres) --- with a trailing remark

# the note above a key
A_HOST=old.example.com
A_USER=reader        # read-only account, do not swap in the app account
A_PASSWORD=keep#this

# a key deliberately left commented out
# A_REPLICA=

	# an indented comment
LEGACY='literal $notexpanded'
`

// checkPreservation saves one change and requires everything else to be byte
// for byte what it was.
func checkPreservation() int {
	dir, err := os.MkdirTemp("", "dotenv-agreement")
	if err != nil {
		fmt.Println("FAIL  temp dir:", err)
		return 1
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(preserved), 0o600); err != nil {
		fmt.Println("FAIL  write:", err)
		return 1
	}
	f, err := localenv.Load(dir)
	if err != nil {
		fmt.Println("FAIL  load:", err)
		return 1
	}
	// The form always writes back every key, including the ones nobody
	// touched, so this is what a save actually does.
	for _, e := range f.Entries() {
		v := e.Value
		if e.Key == "A_HOST" {
			v = "new.example.com"
		}
		f.Set(e.Key, v)
	}
	if err := f.Save(); err != nil {
		fmt.Println("FAIL  save:", err)
		return 1
	}
	after, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("FAIL  reread:", err)
		return 1
	}
	want := strings.Replace(preserved, "A_HOST=old.example.com", "A_HOST=new.example.com", 1)
	if string(after) != want {
		fmt.Println("FAIL  a save changed more than the one value:")
		for i, line := range strings.Split(string(after), "\n") {
			wantLines := strings.Split(want, "\n")
			if i < len(wantLines) && line != wantLines[i] {
				fmt.Printf("      line %d\n        was  %q\n        now  %q\n", i+1, wantLines[i], line)
			}
		}
		return 1
	}
	fmt.Println("save:   one value changed, every comment and blank line byte for byte")
	return 0
}

// script reads the same cases through the python implementation, which is the
// copy that ships to a customer repository.
const script = `
import json, sys
sys.path.insert(0, sys.argv[1])
from connectors import parse_value
print(json.dumps([parse_value(r) for r in json.load(sys.stdin)]))
`

func main() {
	failures := 0
	fail := func(format string, args ...any) {
		failures++
		fmt.Printf("FAIL  "+format+"\n", args...)
	}

	raws := make([]string, len(cases))
	goGot := make([]string, len(cases))
	for i, c := range cases {
		raws[i] = c.raw
		goGot[i] = localenv.ParseValue(c.raw)
		if goGot[i] != c.want {
			fail("go parse %q -> %q, want %q", c.raw, goGot[i], c.want)
		}
		// Encode then ParseValue has to be the identity, or a value the UI
		// saved is not the value the next read returns.
		if back := localenv.ParseValue(localenv.Encode(c.want)); back != c.want {
			fail("go round trip %q -> %q -> %q", c.want, localenv.Encode(c.want), back)
		}
	}
	fmt.Printf("go:     %d cases, %d failures\n", len(cases)*2, failures)

	failures += checkPreservation()

	pyGot, err := runPython(raws)
	if err != nil {
		fmt.Printf("python: SKIPPED - %v\n", err)
		fmt.Println("        this is a skip, not a pass: the shipped implementation was not read")
		os.Exit(exitCode(failures))
	}
	disagreements := 0
	for i := range cases {
		if pyGot[i] != goGot[i] {
			disagreements++
			fail("python %q vs go %q, for %q", pyGot[i], goGot[i], cases[i].raw)
		}
	}
	fmt.Printf("python: %d cases, %d disagreements with go\n", len(cases), disagreements)
	os.Exit(exitCode(failures))
}

func exitCode(failures int) int {
	if failures > 0 {
		return 1
	}
	fmt.Println("the two implementations agree")
	return 0
}

// runPython feeds the cases to parse_value in the embedded scripts.
func runPython(raws []string) ([]string, error) {
	dir := filepath.Join("internal", "scaffold", "templates",
		".agents", "skills", "db-query", "scripts")
	if _, err := os.Stat(filepath.Join(dir, "connectors.py")); err != nil {
		return nil, fmt.Errorf("run this from the repository root: %w", err)
	}
	in, err := json.Marshal(raws)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("python3", "-c", script, dir)
	cmd.Stdin = strings.NewReader(string(in))
	cmd.Stderr = os.Stderr
	// Keep the templates clean: python writes __pycache__ next to whatever it
	// imports, and go:embed all: would ship it into a customer repository.
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("python3: %w", err)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		return nil, err
	}
	return got, nil
}
