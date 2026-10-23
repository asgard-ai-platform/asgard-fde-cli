// Package localenv reads and writes the repository's .env - the design-time
// credentials, on this laptop, for the coding agent working in this repo.
//
// **It is one of three credential mechanisms and the only one that lives in the
// repository.** Pipeline variables are the platform's, set with `asgard-cli
// pipeline variables set`; a runtime secret is a Kubernetes Secret the platform
// provisions for a deployed CR. The value is often the same string - it is the
// same database - but they are set in three different places, on three
// lifecycles, and reading one to obtain another is how a production credential
// ends up somewhere nobody can withdraw it from.
//
// The file is edited by hand as well, so **its comments, blank lines and order
// survive a write**. Only the values on the lines this changes are touched.
//
// The format is the one the db-query scripts read, documented in
// .agents/skills/db-query/references/connectors.md. Both implementations have to
// agree on what a comment is, or the UI shows one value and python reads another.
package localenv

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileName is the file, at the repository root.
const FileName = ".env"

// Entry is one key as the UI sees it.
type Entry struct {
	Key string
	// Value is the current value, empty when the key is a placeholder.
	Value string
	// Note is the comment lines immediately above the key, which is where
	// `--keys` writes what the key is for. A note after the `=` would be read
	// as the value, so there is never one there.
	Note string
	// Secret marks a key whose value should be masked in the UI until asked
	// for. It is a guess from the name and deliberately errs towards masking.
	Secret bool
}

// File is a parsed .env that can be written back without losing anything.
type File struct {
	Path string
	// trailingNewline records whether the file ended with one, so writing it
	// back does not produce a one-character diff.
	trailingNewline bool
	lines           []line
}

// line is one physical line. An assignment carries its key and decoded value;
// everything else - comments, blanks, anything unparseable - is kept verbatim
// and written back untouched.
type line struct {
	raw   string
	key   string
	value string
	// comment is anything after the value on the same line, kept verbatim
	// including the whitespace before it. `USER=reader   # read-only account`
	// is a sentence somebody wrote for the next reader, and rewriting the line
	// without it deletes that sentence to change a value it does not describe.
	comment string
}

// assignment matches `KEY=value`, with the optional `export ` some .env files
// carry so they can be sourced by a shell.
var assignment = regexp.MustCompile(`^[ \t]*(?:export[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)[ \t]*=(.*)$`)

// secretish decides which keys are masked by default.
//
// The list is the vocabulary the CRDs use for credentials, plus the words that
// mean the same thing elsewhere. It is a display default, not a security
// boundary: masking one field too many costs a click, and masking one too few
// puts a credential on a screen somebody is sharing.
var secretish = []string{
	"password", "secret", "token", "key", "credential", "passwd", "pwd", "auth",
}

// Load reads the .env at root. A file that does not exist is not an error: it
// is a repository where nothing has been filled in yet, and Save creates it.
func Load(root string) (*File, error) {
	path := filepath.Join(root, FileName)
	f := &File{Path: path, trailingNewline: true}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	body := string(data)
	f.trailingNewline = body == "" || strings.HasSuffix(body, "\n")
	body = strings.TrimSuffix(body, "\n")
	if body == "" {
		f.lines = nil
		return f, nil
	}
	for _, raw := range strings.Split(body, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		if m := assignment.FindStringSubmatch(raw); m != nil {
			_, comment := splitRHS(m[2])
			f.lines = append(f.lines, line{raw: raw, key: m[1], value: ParseValue(m[2]), comment: comment})
			continue
		}
		f.lines = append(f.lines, line{raw: raw})
	}
	return f, nil
}

// Entries lists the keys in file order, each with the comment above it.
func (f *File) Entries() []Entry {
	var out []Entry
	var note []string
	for _, l := range f.lines {
		if l.key == "" {
			trimmed := strings.TrimSpace(l.raw)
			switch {
			case trimmed == "":
				// A blank line ends a note: what is above it belongs to
				// whatever group it separated, not to the next key.
				note = nil
			case strings.HasPrefix(trimmed, "#"):
				text := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
				// A group banner - `# --- UOF_DB (postgres) ---` - describes
				// the block, not the first key in it.
				if !isBanner(text) {
					note = append(note, text)
				}
			default:
				note = nil
			}
			continue
		}
		// A trailing comment is a note about this key too, and the person
		// filling the form in should see what it says.
		if trailing := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l.comment), "#")); trailing != "" {
			note = append(note, trailing)
		}
		out = append(out, Entry{
			Key:    l.key,
			Value:  l.value,
			Note:   strings.Join(note, " "),
			Secret: IsSecret(l.key),
		})
		note = nil
	}
	return out
}

// isBanner reports whether a comment is a group heading rather than a note
// about the key below it.
func isBanner(text string) bool {
	return strings.HasPrefix(text, "---") || strings.HasSuffix(text, "---")
}

// IsSecret guesses whether a key holds a credential, for masking.
func IsSecret(key string) bool {
	lower := strings.ToLower(key)
	for _, word := range secretish {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// Set writes a value for a key, in place when the key is already there and
// appended when it is not.
//
// In place matters: the key's note, its position and the group it belongs to
// are how somebody reading this file by hand finds their way around, and
// rewriting the file would sort all of that away.
//
// **A value that did not change does not get rewritten**, which is not an
// optimisation. Encode is allowed to spell a value differently from the way
// somebody typed it - dropping quotes it does not need, say - and doing that to
// a line nobody touched turns a save into a diff across the whole file. It also
// changes meaning where it matters: `'literal $x'` re-spelled bare is a
// different string to any shell that ever sources this file.
func (f *File) Set(key, value string) {
	for i := range f.lines {
		if f.lines[i].key != key {
			continue
		}
		if f.lines[i].value == value {
			return
		}
		f.lines[i].value = value
		f.lines[i].raw = key + "=" + Encode(value) + f.lines[i].comment
		return
	}
	f.lines = append(f.lines, line{raw: key + "=" + Encode(value), key: key, value: value})
}

// keyName is a key a shell can export: the names assignment parses.
var keyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// AddPlaceholders appends an empty `KEY=` for each key that is not in the file
// yet, and returns the ones it added. Nothing reaches the disk until Save.
//
// The person filling the form in is often not an engineer. Adding a key by hand
// - finding the button, spelling the name exactly as the agent will read it -
// is where they give up, so the keys the agent asked for are always there.
//
// A name that is not a valid key is an error rather than a placeholder: a typo
// in --focus would otherwise become a field nobody reads.
func (f *File) AddPlaceholders(keys []string) ([]string, error) {
	var added []string
	for _, k := range keys {
		if !keyName.MatchString(k) {
			return nil, fmt.Errorf("%q is not a key name: letters, digits and _, not starting with a digit", k)
		}
		if f.Has(k) {
			continue
		}
		f.lines = append(f.lines, line{raw: k + "=", key: k})
		added = append(added, k)
	}
	return added, nil
}

// Has reports whether the key is in the file at all, filled in or not.
func (f *File) Has(key string) bool {
	for _, l := range f.lines {
		if l.key == key {
			return true
		}
	}
	return false
}

// Save writes the file back, creating it if it was not there.
//
// 0600: this file holds the customer's credentials, and the default 0644 puts
// them within reach of anything else running as another user on the machine.
func (f *File) Save() error {
	var b strings.Builder
	for i, l := range f.lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(l.raw)
	}
	if f.trailingNewline && b.Len() > 0 {
		b.WriteString("\n")
	}
	if err := os.WriteFile(f.Path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", f.Path, err)
	}
	return nil
}

// ParseValue decodes the right-hand side of an assignment.
//
// It is the Go half of a rule with two implementations - the other is
// parse_value in the db-query scripts - so the two are written to the same
// description and any change belongs in both:
//
//   - A `#` opens a comment only when whitespace comes before it, so
//     `PASSWORD=abc#123` is a whole password.
//   - Quotes are how a value keeps its leading or trailing spaces. Inside
//     single quotes everything is literal; inside double quotes `\\`, `\"`,
//     `\n`, `\r` and `\t` are escapes, which is what lets a PEM live on one
//     line.
func ParseValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch raw[0] {
	case '\'':
		if end := strings.IndexByte(raw[1:], '\''); end >= 0 {
			return raw[1 : 1+end]
		}
		return raw[1:]
	case '"':
		return unescape(raw[1:])
	case '#':
		// `KEY=   # what this is for` has to read as empty, or a documented
		// placeholder looks filled in. The cost is that a bare value cannot
		// start with a #; Encode quotes one that does.
		return ""
	}
	if i := commentStart(raw); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}

// splitRHS cuts the right-hand side into the value token and whatever trailing
// comment follows it, keeping the whitespace between them.
//
// It follows ParseValue exactly - the same quoting, the same rule for when a
// `#` opens a comment - because the two halves have to agree on where the value
// ends. If they disagree, a save either eats part of a comment or writes part of
// one into the value.
func splitRHS(raw string) (value, comment string) {
	lead := len(raw) - len(strings.TrimLeft(raw, " \t"))
	body := raw[lead:]
	if body == "" {
		return raw, ""
	}
	switch body[0] {
	case '\'':
		if end := strings.IndexByte(body[1:], '\''); end >= 0 {
			return raw[:lead+end+2], raw[lead+end+2:]
		}
		return raw, ""
	case '"':
		for i := 1; i < len(body); i++ {
			if body[i] == '\\' {
				i++
				continue
			}
			if body[i] == '"' {
				return raw[:lead+i+1], raw[lead+i+1:]
			}
		}
		return raw, ""
	case '#':
		return raw[:lead], raw[lead:]
	}
	if i := commentStart(body); i >= 0 {
		// Back up over the whitespace that introduced the comment, so it stays
		// with the comment rather than with the value.
		j := i
		for j > 0 && (body[j-1] == ' ' || body[j-1] == '\t') {
			j--
		}
		return raw[:lead+j], raw[lead+j:]
	}
	return raw, ""
}

// commentStart finds a `#` that has whitespace before it, or -1.
func commentStart(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

// unescape reads a double-quoted body up to its closing quote.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			break
		}
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		default:
			// An unknown escape keeps both characters: a Windows path in a
			// value is not a mistake, and silently eating its backslashes
			// would be.
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// Encode renders a value for the right-hand side, quoting only when a bare
// value would not read back as itself.
//
// Exported as the other half of ParseValue: the pair is the format, and
// hack/dotenv-agreement checks that Encode then ParseValue is the identity and
// that the python implementation reads what this one writes.
func Encode(v string) string {
	if v == "" {
		return ""
	}
	bare := v == strings.TrimSpace(v) &&
		commentStart(v) < 0 &&
		!strings.ContainsAny(v, "\n\r\t") &&
		v[0] != '"' && v[0] != '\'' && v[0] != '#'
	if bare {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(v) + `"`
}
