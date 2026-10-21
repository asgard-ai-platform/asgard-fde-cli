package extension

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// Level says whether a finding fails the lint.
type Level string

const (
	// Error fails the lint. The registry does not check content, so a version
	// tagged with one is offered anyway; this is the only place it is caught.
	Error Level = "error"
	// Warning does not.
	Warning Level = "warning"
)

// Finding is one thing Lint has to say about an Extension.
type Finding struct {
	Level   Level  `json:"level"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

var (
	usages = map[string]bool{"build-time": true, "shared-account": true, "personal-account": true}
	auths  = map[string]bool{"oauth-customer": true, "oauth-asgard": true, "static-secret": true, "none": true}

	kinds      = map[string]bool{"integration": true, "knowledge": true}
	categories = map[string]bool{
		"documents": true, "communication": true, "crm": true, "erp": true, "project": true,
		"finance": true, "developer": true, "calendar": true, "data": true,
	}
	statuses = map[string]bool{"verified": true, "experimental": true}
	listings = map[string]bool{"public": true, "unlisted": true}
	sources  = map[string]bool{"closed": true, "open": true}

	// knownKeys are the frontmatter keys Meta reads, plus the one that is
	// refused on purpose. Anything else is most likely a typo, which would
	// otherwise be ignored without a word.
	knownKeys = map[string]bool{
		"id": true, "name": true, "summary": true, "kind": true, "system": true, "logo": true,
		"routes": true, "oauth": true, "requires": true, "listing": true, "source": true,
		"owners": true, "tags": true, "version": true,
	}
)

// summaryLimit is how long a gallery card's line can be before it is cut.
const summaryLimit = 80

// Lint checks an extension repository at dir against what the FDE Agent relies
// on. It is the only check of an Extension's content: the registry takes a
// version in on its identity alone. It does not render the examples: that
// needs helm and the gate, and the caller runs them.
//
// It returns the parsed index when README.md could be read, so a caller can
// report what it was about.
func Lint(dir string) ([]Finding, *Meta) {
	l := &linter{dir: dir}
	meta := l.readme()
	if meta != nil {
		l.skill(meta.ID)
		l.routes(meta)
		l.oauth(meta)
		l.logo(meta)
		l.files()
	}
	l.examples(meta)
	sort.SliceStable(l.out, func(i, j int) bool {
		if l.out[i].Level != l.out[j].Level {
			return l.out[i].Level == Error
		}
		return l.out[i].Path < l.out[j].Path
	})
	return l.out, meta
}

// Failed reports whether any finding is an error.
func Failed(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == Error {
			return true
		}
	}
	return false
}

type linter struct {
	dir string
	out []Finding
}

func (l *linter) errorf(path, format string, args ...any) {
	l.out = append(l.out, Finding{Level: Error, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (l *linter) warnf(path, format string, args ...any) {
	l.out = append(l.out, Finding{Level: Warning, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (l *linter) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(l.dir, filepath.FromSlash(rel)))
	return err == nil
}

func (l *linter) readme() *Meta {
	const p = "README.md"
	meta, keys, err := ReadMeta(filepath.Join(l.dir, p))
	if err != nil {
		if os.IsNotExist(err) {
			l.errorf(p, "missing: README.md carries the index in its YAML frontmatter")
		} else {
			l.errorf(p, "%v", err)
		}
		return nil
	}

	for _, k := range keys {
		switch {
		case k == "version":
			l.errorf(p, "version: remove it; the version is the git tag, and a second copy can disagree with it")
		case !knownKeys[k]:
			l.warnf(p, "%s: not a field of the index; it is ignored", k)
		}
	}

	switch {
	case meta.ID == "":
		l.errorf(p, "id: required")
	case !ValidID(meta.ID):
		l.errorf(p, "id: %q must be %s followed by kebab-case, e.g. %sgoogle-drive", meta.ID, Prefix, Prefix)
	default:
		if base := filepath.Base(l.absDir()); base != meta.ID {
			l.warnf(p, "id: %q differs from the directory name %q; the id is the repository name, so one of them is wrong", meta.ID, base)
		}
	}
	if meta.Name == "" {
		l.errorf(p, "name: required")
	}
	switch n := utf8.RuneCountInString(strings.TrimSpace(meta.Summary)); {
	case n == 0:
		l.errorf(p, "summary: required")
	case n > summaryLimit:
		l.warnf(p, "summary: %d characters; a gallery card shows about %d", n, summaryLimit)
	}
	if !kinds[meta.Kind] {
		l.errorf(p, "kind: %q must be one of %s", meta.Kind, keysOf(kinds))
	}
	if meta.System.Vendor == "" || meta.System.Product == "" {
		l.errorf(p, "system: vendor and product are required")
	}
	if !categories[meta.System.Category] {
		l.errorf(p, "system.category: %q must be one of %s", meta.System.Category, keysOf(categories))
	}
	if len(meta.Routes) == 0 {
		l.errorf(p, "routes: at least one route is required")
	}
	if !listings[meta.Listing] {
		l.errorf(p, "listing: %q must be one of %s", meta.Listing, keysOf(listings))
	}
	if !sources[meta.Source] {
		l.errorf(p, "source: %q must be one of %s", meta.Source, keysOf(sources))
	}
	if len(meta.Owners) == 0 {
		l.errorf(p, "owners: at least one contact email is required")
	}
	for _, o := range meta.Owners {
		if !strings.Contains(o, "@") {
			l.errorf(p, "owners: %q is not an email address", o)
		}
	}
	if meta.Logo != "" && !l.exists(meta.Logo) {
		l.warnf(p, "logo: %s does not exist", meta.Logo)
	}
	return &meta
}

func (l *linter) absDir() string {
	if abs, err := filepath.Abs(l.dir); err == nil {
		return abs
	}
	return l.dir
}

func (l *linter) skill(id string) {
	const p = "SKILL.md"
	s, err := readSkill(filepath.Join(l.dir, p))
	if err != nil {
		if os.IsNotExist(err) {
			l.errorf(p, "missing: SKILL.md is how the FDE Agent finds this Extension once it lands in .agents/skills/<id>/")
		} else {
			l.errorf(p, "%v", err)
		}
		return
	}
	if s.Name != id {
		l.errorf(p, "name: %q must equal the id %q, because the directory it lands in is named after the id", s.Name, id)
	}
	if strings.TrimSpace(s.Description) == "" {
		l.errorf(p, "description: required; it is what tells the FDE Agent when to read this")
	}
}

func (l *linter) routes(meta *Meta) {
	const p = "README.md"
	declared := map[string]bool{}
	for i, r := range meta.Routes {
		where := fmt.Sprintf("routes[%d]", i)
		usage, auth, ok := strings.Cut(r.ID, ".")
		switch {
		case r.ID == "":
			l.errorf(p, "%s.id: required", where)
			continue
		case !ok || !usages[usage] || !auths[auth]:
			l.errorf(p, "%s.id: %q must be <usage>.<auth>, usage one of %s and auth one of %s", where, r.ID, keysOf(usages), keysOf(auths))
			continue
		case auth == "oauth-asgard":
			l.errorf(p, "%s.id: %q is not supported; routes use the customer's own OAuth client (oauth-customer)", where, r.ID)
		case r.ID == "personal-account.static-secret":
			l.errorf(p, "%s.id: %q is not supported; a personal account goes through OAuth", where, r.ID)
		}
		if declared[r.ID] {
			l.errorf(p, "%s.id: %q is declared twice", where, r.ID)
		}
		declared[r.ID] = true
		if !statuses[r.Status] {
			l.errorf(p, "%s.status: %q must be one of %s", where, r.Status, keysOf(statuses))
		}
		for _, b := range r.Brakes {
			if _, ok := Brakes[b]; !ok {
				l.errorf(p, "%s.brakes: %q is not a brake; one of %s", where, b, keysOf(Brakes))
			}
		}
		if !l.exists("routes/" + r.ID + ".md") {
			l.errorf("routes/"+r.ID+".md", "missing: every declared route has its own document")
		}
	}

	entries, err := os.ReadDir(filepath.Join(l.dir, "routes"))
	if err != nil {
		return
	}
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".md")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || !declared[id] {
			l.errorf("routes/"+e.Name(), "not a declared route; routes/ holds one document per entry in routes: and nothing else")
		}
	}
}

func (l *linter) oauth(meta *Meta) {
	const p = "README.md"
	needs := false
	for _, r := range meta.Routes {
		if strings.HasSuffix(r.ID, ".oauth-customer") {
			needs = true
		}
	}
	if !needs {
		return
	}
	if meta.OAuth == nil || meta.OAuth.CustomerClient == nil {
		l.errorf(p, "oauth.customer_client: required when a route is *.oauth-customer")
		return
	}
	cc := meta.OAuth.CustomerClient
	if cc.Where == "" {
		l.errorf(p, "oauth.customer_client.where: required; it says where the customer creates the client")
	}
	if cc.Setup != "" && !l.exists(cc.Setup) {
		l.errorf(p, "oauth.customer_client.setup: %s does not exist", cc.Setup)
	}
}

// files checks the rules about what is where in the repository.
func (l *linter) files() {
	// A runtime skill template outside examples/assets/skills/ is read by the
	// FDE Agent as one of its own design-time skills once the Extension lands.
	_ = filepath.WalkDir(l.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(l.dir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "SKILL.md" && rel != "SKILL.md" && !runtimeSkillPath(rel) {
			l.warnf(rel, "a SKILL.md outside examples/<set>/assets/skills/; a runtime skill template belongs there, where it is not loaded as a design-time skill")
		}
		if d.Name() == ".env" || (strings.HasPrefix(d.Name(), ".env.") && d.Name() != ".env.example") {
			l.errorf(rel, "an environment file in an Extension; it is committed into every customer repository that takes this Extension")
		}
		if strings.HasPrefix(rel, "scripts/") && shellScript(d.Name()) {
			l.errorf(rel, "a shell script in scripts/; it runs on the FDE person's own machine, whatever its operating system. Write it in Python")
		}
		return nil
	})
}

// logoLimit is the largest logo the registry takes.
const logoLimit = 256 << 10

// logo checks the file `logo` names. The registry takes a version whose logo it
// cannot use and simply shows none, so each problem only warns.
func (l *linter) logo(meta *Meta) {
	if meta.Logo == "" {
		return
	}
	const p = "README.md"
	rel := filepath.ToSlash(filepath.Clean(meta.Logo))
	top, _, _ := strings.Cut(rel, "/")
	ext := strings.ToLower(filepath.Ext(rel))
	switch {
	case filepath.IsAbs(meta.Logo) || strings.HasPrefix(rel, "../") || !slices.Contains(Shipped, top):
		l.warnf(p, "logo %s is not among what the Extension ships (assets/ is where it goes); the gallery will show none", meta.Logo)
		return
	case ext != ".svg" && ext != ".png":
		l.warnf(p, "logo %s is neither .svg nor .png; the gallery will show none", meta.Logo)
		return
	}
	info, err := os.Stat(filepath.Join(l.dir, filepath.FromSlash(rel)))
	switch {
	case err != nil:
		l.warnf(p, "logo %s does not exist; the gallery will show none", meta.Logo)
	case !info.Mode().IsRegular():
		l.warnf(p, "logo %s is not a regular file; the gallery will show none", meta.Logo)
	case info.Size() > logoLimit:
		l.warnf(p, "logo %s is %d KiB, over the %d KiB the registry takes; the gallery will show none", meta.Logo, info.Size()>>10, logoLimit>>10)
	}
}

// shellScript reports whether name is a script for one operating system's
// shell. Only scripts/ is held to it: a runtime skill's scripts run in the
// platform's Linux sandbox, where a shell script is fine.
func shellScript(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sh", ".bash", ".zsh", ".ps1", ".bat", ".cmd":
		return true
	}
	return false
}

// examples checks the shape of examples/: pseudo charts, one folder per set
// of CRs, named after the route it shows and referenced from that route's
// document. They are not rendered - placeholders stand for what only the FDE
// repository knows - so the shape is what can be checked here, and each file
// has to say in its first line that it is an example.
func (l *linter) examples(meta *Meta) {
	root := filepath.Join(l.dir, "examples")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	if !l.exists("examples/README.md") {
		l.warnf("examples", "no README.md; it lists each set, the route it belongs to, and what each <placeholder> stands for")
	}

	routeDocs := l.routeDocs()
	routes := map[string]bool{}
	if meta != nil {
		for _, r := range meta.Routes {
			routes[r.ID] = true
		}
	}
	for _, e := range entries {
		rel := "examples/" + e.Name()
		if !e.IsDir() {
			if e.Name() != "README.md" {
				l.warnf(rel, "a file directly in examples/; each set of CRs is a folder of its own")
			}
			continue
		}
		if !l.exists(rel + "/README.md") {
			l.warnf(rel, "no README.md; it says how the CRs connect, which asgard-cli add to run first, and where each file goes in the FDE repository")
		}
		if meta != nil && !routes[setRoute(e.Name(), routes)] {
			l.warnf(rel, "named after no declared route; a set is <route-id> or <route-id>.<purpose>")
		}
		if !regexp.MustCompile(regexp.QuoteMeta(rel) + `([^A-Za-z0-9._-]|$)`).MatchString(routeDocs) {
			l.warnf(rel, "no document under routes/ refers to it, so the FDE Agent is never sent there")
		}
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(l.dir, path)
		rel = filepath.ToSlash(rel)
		if !textExample(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !hasExampleBanner(data) {
			l.warnf(rel, "first line does not say EXAMPLE; the FDE Agent treats a file that looks finished as finished")
		}
		return nil
	})
}

// setRoute is the route a set's folder name names: the whole name, or the
// part before the last dot-separated suffix.
func setRoute(name string, routes map[string]bool) string {
	if routes[name] {
		return name
	}
	for i := len(name) - 1; i > 0; i-- {
		if name[i] == '.' && routes[name[:i]] {
			return name[:i]
		}
	}
	return name
}

// routeDocs is every route document, concatenated, for finding references.
func (l *linter) routeDocs() string {
	var b strings.Builder
	entries, _ := os.ReadDir(filepath.Join(l.dir, "routes"))
	for _, e := range entries {
		if data, err := os.ReadFile(filepath.Join(l.dir, "routes", e.Name())); err == nil {
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// runtimeSkillPath reports whether rel is under examples/<set>/assets/skills/.
func runtimeSkillPath(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) > 4 && parts[0] == "examples" && parts[2] == "assets" && parts[3] == "skills"
}

func textExample(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".md", ".tpl", ".txt", ".py", ".sh", ".json":
		return name != "README.md" && name != ".helmignore"
	}
	return false
}

// hasExampleBanner looks at the first non-empty line, or for a Markdown file
// with frontmatter, the first non-empty line after it.
func hasExampleBanner(data []byte) bool {
	if _, body, err := frontmatter(data); err == nil {
		data = body
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == "---" {
			continue
		}
		return strings.Contains(line, "EXAMPLE")
	}
	return false
}

func keysOf[V any](m map[string]V) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
