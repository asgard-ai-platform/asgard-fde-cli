package scaffold

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// Skill is one design-time skill the scaffold writes into a customer repo.
//
// They are searchable for the same reason the wiki is. An agent asked to build
// a deck greps for slides, and until this existed the answer
// was that nothing matched anywhere - while the skill that owns the whole
// subject sat in the repository it was standing in. The material was there and
// the tool's own way in did not reach it.
type Skill struct {
	Name        string
	Description string
	Path        string

	// Links is what this skill points a reader at, read when it was parsed.
	// A skill sends a reader into the wiki and the extracts like anything
	// else here, so it is part of the same graph.
	Links []kb.Link
}

const skillRoot = "templates/.agents/skills"

var frontmatterField = regexp.MustCompile(`(?m)^(name|description):\s*(.*)$`)

// Skills lists the design-time skills, sorted by name.
func Skills() ([]Skill, error) {
	docs, err := corpus.List()
	if err != nil {
		return nil, err
	}
	out := make([]Skill, 0, len(docs))
	for _, d := range docs {
		out = append(out, Skill{Name: d.Name, Description: d.Summary, Path: Path(d.Name), Links: d.Links})
	}
	return out, nil
}

// Body returns one skill's SKILL.md as it ships. Exported so the audit command
// can walk the same bytes an engagement reads.
func Body(name string) (string, error) { return skillBody(name) }

// skillBody reads one skill's SKILL.md, rendered or not. The templated ones are
// searched as written, placeholders and all: a search hits prose, and the only
// placeholder left in any of them is the spec slug, which is a constant.
func skillBody(name string) (string, error) { return corpus.Read(name) }

// corpus is the skills as a body of material, on the same terms as the wiki and
// the extracts.
//
// Two things differ and both are supplied rather than worked around. A skill is
// `<name>/SKILL.md`, not `<name>.md`, so Docs resolves the names. And its
// metadata is YAML frontmatter - `name` and `description` - which is the Agent
// Skills contract the runtime discovers it by, and is not this repo's to
// change, so ParseDoc reads that instead of a "# " heading. What comes out is a
// kb.Doc like any other.
var corpus = kb.Corpus{
	FS:       templates,
	Dir:      skillRoot,
	Docs:     skillRefs,
	ParseDoc: parseSkill,
	Noun:     "skill",
	Command:  "ls .agents/skills/",
}

// skillRefs lists each skill directory and the SKILL.md inside it. The
// templated ones are searched as written, placeholders and all: a search hits
// prose, and the only placeholder left in any of them is the spec slug, which
// is a constant.
func skillRefs() ([]kb.Ref, error) {
	entries, err := fs.ReadDir(templates, skillRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", skillRoot, err)
	}

	var out []kb.Ref
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, suffix := range []string{"/SKILL.md.tmpl", "/SKILL.md"} {
			path := skillRoot + "/" + e.Name() + suffix
			if _, err := fs.Stat(templates, path); err == nil {
				out = append(out, kb.Ref{Name: e.Name(), Path: path})
				break
			}
		}
	}
	return out, nil
}

// parseSkill takes the title and summary from the frontmatter rather than from
// a heading.
//
// The heading used to carry the workspace name as an unrendered placeholder,
// which read as broken text in a listing. That reason is gone with the name,
// and the behaviour stays for a better one: the frontmatter `description` is
// what an agent's own runtime reads to decide whether a skill is relevant, so a
// listing that shows anything else is showing a second answer to the same
// question.
func parseSkill(name string, data []byte) kb.Doc {
	d := kb.Parse(name, data)
	d.Title = ""
	d.Summary = ""
	for _, m := range frontmatterField.FindAllStringSubmatch(string(data), -1) {
		if m[1] == "description" {
			d.Summary = strings.TrimSpace(m[2])
		}
	}
	// The frontmatter description is the title line as well as the summary: a
	// skill has no separate one-line name, and the "# " heading below the
	// frontmatter says what the file is with the customer's name interpolated
	// into it, which reads as broken text unrendered.
	d.Title = d.Summary
	return d
}

// Path is where one skill lands in a customer repository.
func Path(name string) string { return ".agents/skills/" + name + "/SKILL.md" }

// List returns every skill, sorted by name.
func List() ([]kb.Doc, error) { return corpus.List() }

// skipFrontmatter drops the leading --- block, so a search quotes the skill's
// prose rather than the description it is already being shown beside.
//
// **`kb.Body` is the one that decides where a block ends.** This was a second
// copy of it, and the two would have disagreed the first time either was
// corrected - which is not hypothetical: the copy in `kb` was fixed the day
// this was written, for an unterminated block that swallowed a document.
func skipFrontmatter(body string) string { return kb.Body(body) }
