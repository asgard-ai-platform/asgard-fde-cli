package extension

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Meta is the index an Extension declares in the YAML frontmatter of its
// README.md. The registry, `extension search` and the gallery all read it.
//
// There is no version field: the version is the git tag, and a second copy in
// the file would be one that can disagree with it.
type Meta struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Summary  string   `yaml:"summary" json:"summary"`
	Kind     string   `yaml:"kind" json:"kind"`
	System   System   `yaml:"system" json:"system"`
	Logo     string   `yaml:"logo,omitempty" json:"logo,omitempty"`
	Routes   []Route  `yaml:"routes" json:"routes"`
	OAuth    *OAuth   `yaml:"oauth,omitempty" json:"oauth,omitempty"`
	Requires Requires `yaml:"requires" json:"requires"`
	Listing  string   `yaml:"listing" json:"listing"`
	Source   string   `yaml:"source" json:"source"`
	Owners   []string `yaml:"owners" json:"owners"`
	Tags     []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// System is the external system an Extension is about.
type System struct {
	Vendor   string `yaml:"vendor" json:"vendor"`
	Product  string `yaml:"product" json:"product"`
	Category string `yaml:"category" json:"category"`
	Docs     string `yaml:"docs,omitempty" json:"docs,omitempty"`
}

// Route is one usage crossed with one way of authenticating, written
// `<usage>.<auth>`. Each has a document under routes/ named after its id.
type Route struct {
	ID       string   `yaml:"id" json:"id"`
	Status   string   `yaml:"status" json:"status"`
	Produces []string `yaml:"produces,omitempty" json:"produces,omitempty"`
	Brakes   []string `yaml:"brakes,omitempty" json:"brakes,omitempty"`
	Note     string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// OAuth describes the customer's own OAuth client, which every
// `*.oauth-customer` route needs.
type OAuth struct {
	CustomerClient *CustomerClient `yaml:"customer_client,omitempty" json:"customer_client,omitempty"`
}

// CustomerClient is where the customer creates the client and what to register.
type CustomerClient struct {
	Where        string   `yaml:"where" json:"where"`
	ClientType   string   `yaml:"client_type,omitempty" json:"client_type,omitempty"`
	Setup        string   `yaml:"setup,omitempty" json:"setup,omitempty"`
	Review       string   `yaml:"review,omitempty" json:"review,omitempty"`
	RedirectURIs []string `yaml:"redirect_uris,omitempty" json:"redirect_uris,omitempty"`
}

// Requires is what an Extension depends on. A mismatch only warns: the FDE
// Agent decides what to do about it.
type Requires struct {
	AsgardCLI  string   `yaml:"asgard_cli,omitempty" json:"asgard_cli,omitempty"`
	Skills     []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty"`
	Platform   []string `yaml:"platform,omitempty" json:"platform,omitempty"`
}

// errNoFrontmatter is returned for a file that does not open with `---`.
var errNoFrontmatter = errors.New("no YAML frontmatter: the file must open with a line that is exactly ---")

// frontmatter splits a Markdown file into its YAML frontmatter and the body.
func frontmatter(data []byte) (yamlPart, body []byte, err error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) == 0 || string(bytes.TrimRight(lines[0], "\r\n")) != "---" {
		return nil, nil, errNoFrontmatter
	}
	for i := 1; i < len(lines); i++ {
		if string(bytes.TrimRight(lines[i], "\r\n")) == "---" {
			return bytes.Join(lines[1:i], nil), bytes.Join(lines[i+1:], nil), nil
		}
	}
	return nil, nil, errors.New("YAML frontmatter is not closed: no second line that is exactly ---")
}

// ReadMeta parses README.md's frontmatter. It also returns the top-level keys
// as written, so a caller can report one the schema does not know.
func ReadMeta(path string) (Meta, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, nil, err
	}
	return ParseMeta(data)
}

// ParseMeta is ReadMeta for a README.md already in memory, such as one read out
// of an archive.
func ParseMeta(data []byte) (Meta, []string, error) {
	y, _, err := frontmatter(data)
	if err != nil {
		return Meta{}, nil, err
	}
	if bytes.Contains(y, []byte("{{")) {
		return Meta{}, nil, errors.New("the frontmatter still has template placeholders ({{...}}); replace them before anything can read it")
	}
	var m Meta
	if err := yaml.Unmarshal(y, &m); err != nil {
		return Meta{}, nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(y, &node); err != nil {
		return Meta{}, nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	var keys []string
	if len(node.Content) == 1 && node.Content[0].Kind == yaml.MappingNode {
		mapping := node.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			keys = append(keys, mapping.Content[i].Value)
		}
	}
	return m, keys, nil
}

// skillFrontmatter is the part of SKILL.md an agent's skill loader reads.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func readSkill(path string) (skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillFrontmatter{}, err
	}
	y, _, err := frontmatter(data)
	if err != nil {
		return skillFrontmatter{}, err
	}
	var s skillFrontmatter
	if err := yaml.Unmarshal(y, &s); err != nil {
		return skillFrontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	return s, nil
}
