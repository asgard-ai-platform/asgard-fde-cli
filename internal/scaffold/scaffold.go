// Package scaffold writes the parts of a customer repository that are the same
// for every engagement: the four-layer docs model, the SDD rules, and the
// design-time skills that hold for any Asgard.
//
// It deliberately does not write two other things. The customer's own knowledge
// - which systems exist, how the projects split, what the CRs look like - is
// what the onboarding is for, and templates cannot produce it. And anything
// that describes a particular Asgard server comes from `asgard-cli skill
// update`, which asks the platform; see the embed comment below for why that
// line is where it is.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// The tree is embedded with all: so that dot-prefixed paths (.agents, .github,
// .gitignore, .env.example) are included; the default pattern skips them.
//
// NO SKILL THAT DESCRIBES A PARTICULAR ASGARD SERVER MAY BE EMBEDDED HERE, and
// the reason is not tidiness. Asgard is a SaaS platform today and an on-prem
// product next: a customer's server can be several versions behind ours, or
// ahead of it, and a skill saying what a CRD field is called is only true of one
// of them. A skill compiled into this binary is pinned to whatever release the
// customer happened to install the CLI from, which is unrelated to the server
// they deploy against - so it would be wrong for every on-prem installation not
// on our version, and there would be no way to fix it without shipping them a
// binary.
//
// **asgard-cr-verification was exactly that, and it proved the point before it
// left.** Written into a repository by scaffold and never overwritten - scaffold
// does not replace a file that exists, and no version number covered it - it
// spent a month telling readers to look in `deploy.yaml`, to overlay a
// per-environment values file and to run a python CRD-fidelity script, none of
// which had existed since the Pipeline cut-over. It is now served from
// `/v1/docs/skills` and rewritten on every `asgard-cli skill update`.
//
// **The line is authority, not subject.** What stays here is what is true of any
// Asgard, whoever is running it: the repository skeleton, the docs layers, the
// declaration template, how to write plain Chinese, how to run a local gate,
// how to model a semantic layer from a customer's own database. What leaves is
// every claim about what a server accepts, rejects or calls things - the CRD
// shapes, the processor catalogue, and the document that says what happens when
// you get one of them wrong.
//
// See asgard-odin-pm tracking/studio/tasks, TASK-035, phase C. **Moving any of
// it back in here would look like a simplification and would break every
// customer whose server is not on our version.**
//
//go:embed all:templates
var templates embed.FS

const (
	templateRoot = "templates"

	// tmplSuffix marks a file that is rendered rather than copied. Templates use
	// << >> because the files they produce contain Helm's {{ }}.
	tmplSuffix = ".tmpl"
	leftDelim  = "<<"
	rightDelim = ">>"

	// These path segments are placeholders expanded at render time. A path
	// containing projectDir is written once per project.
	specSlugDir = "__SPEC_SLUG__"
	projectDir  = "__PROJECT__"

	// pycacheDir is never part of the skeleton; see plan.
	pycacheDir = "__pycache__"
)

// Project is one project's chart, as the templates see it.
type Project struct {
	Slug string
}

// Data is what every template is rendered with. Project is set only while
// rendering a file that lives under a __PROJECT__ path.
type Data struct {
	Projects []Project
	// RepoName is the repository's own directory name - a fact on disk, not a
	// recorded one, so renaming the checkout needs no correction anywhere.
	RepoName string
	SpecSlug string

	Project *Project
}

// NewData derives the render data from what the caller found.
//
// **Nothing here is read from a config file, and nothing is asked of the
// platform.** The projects are the repository's own directories and
// declaration; the repository name is the directory it is in. Both are facts on
// disk, which is why the skeleton can be written before there is an account.
func NewData(root string, projects []string) Data {
	out := make([]Project, len(projects))
	for i, slug := range projects {
		out[i] = Project{Slug: slug}
	}
	return Data{
		Projects: out,
		RepoName: filepath.Base(root),
		SpecSlug: repo.SpecSlugIn(root),
	}
}

// Status is what happened to one file.
type Status int

const (
	// Created means the file was written.
	Created Status = iota
	// Skipped means a file was already there and force was not set.
	Skipped
	// Overwritten means an existing file was replaced because force was set.
	Overwritten
	// Updated means the file was refreshed without discarding anything the
	// engagement wrote: either only its managed region was replaced, or the
	// whole of it was provably still what this CLI last wrote and the render
	// moved underneath it - a renamed checkout, an added project.
	Updated
	// Preserved means force was set and the file was left alone anyway,
	// because it is one the CLI's own commands write into and it no longer
	// matches the template it started as.
	Preserved
	// Stale means the file is shipped material that differs from what this
	// binary carries, and which way round is not knowable - the record does
	// not cover it, or the two CLI versions cannot be ordered. It is left
	// alone - the point is to say so, because "already present" reads as "up
	// to date" and an agent acted on that reading.
	Stale
	// Behind means the file is shipped material this CLI has since changed,
	// nobody here has touched it, and the CLI that wrote it was older. This is
	// the case `--force` is for.
	Behind
	// Ahead means the same comparison the other way round: the CLI that wrote
	// this repository was NEWER than the one running. Taking this binary's
	// copy would be a downgrade, so `--force` does not.
	Ahead
	// Edited means a shipped file differs from what this CLI last wrote to it.
	// Somebody here changed it, and `--force` would discard that - which is
	// worth knowing before running it rather than afterwards.
	Edited
	// Retired means the record says this CLI wrote the file and this binary no
	// longer ships it. It is reported and never removed: deleting a file from
	// a customer's repository is not something a scaffold does on its own.
	//
	// The exported corpus is the single exception, and it is a directory rather
	// than a file - see Replaced and `replaceCorpus`.
	Retired
	// Replaced means the exported platform corpus was written by a different
	// version of this CLI and the whole directory was removed before being
	// written again. It is the one delete this tool performs, because that
	// material is generated outright and a page renamed upstream would
	// otherwise leave both names on disk.
	Replaced
	// Missing means a shipped file is not there at all. Only InspectShipped
	// returns it - Write would have written it - and it means an agent working
	// here is reading no copy of something this CLI ships.
	Missing
)

func (s Status) String() string {
	switch s {
	case Created:
		return "created"
	case Overwritten:
		return "overwritten"
	case Updated:
		return "updated"
	case Replaced:
		return "replaced"
	case Preserved:
		return "preserved"
	case Stale:
		return "stale"
	case Behind:
		return "behind"
	case Ahead:
		return "ahead"
	case Edited:
		return "edited"
	case Retired:
		return "retired"
	case Missing:
		return "missing"
	default:
		return "skipped"
	}
}

// Result reports one file's outcome, with paths relative to the repo root.
type Result struct {
	Path   string
	Status Status
}

// Write renders the skeleton into root. It never removes anything, and without
// force it leaves existing files alone, so it can be run again after a project
// is added or when a file was deleted by hand.
//
// **The shipped files are the ones it has an opinion about**, and it needs the
// record beside them to have it: see StampName for what a byte comparison
// against this binary cannot answer and why each of those three questions has
// been got wrong here.
func Write(root string, projects []string, force bool) ([]Result, error) {
	jobs, err := plan(NewData(root, projects))
	if err != nil {
		return nil, err
	}

	// The record is read once, before anything is written: every comparison
	// below is against what this CLI last wrote, not against what it is about
	// to write.
	stamp, err := ReadStamp(root)
	if err != nil {
		return nil, err
	}
	recorded := map[string]Entry{}
	if stamp != nil {
		recorded = stamp.Files
	}
	running := version.Get().Version

	// Before anything is written: the exported corpus is replaced wholesale
	// when this repository's copy came from another version of this CLI, so
	// the loop below writes it fresh rather than merging into it. See
	// replaceCorpus for why this one subtree is deleted and nothing else is.
	replacedCorpus, err := replaceCorpus(root, recorded, running)
	if err != nil {
		return nil, err
	}

	// wrote becomes the new record. An entry is carried forward when a run
	// leaves its file alone, because the record says what this CLI last wrote
	// to a path and a run that wrote nothing there did not change that.
	// Recording the digest of whatever is on disk instead would launder a hand
	// edit into the record, and the next run could no longer see it.
	wrote := map[string]Entry{}

	results := make([]Result, 0, len(jobs)+1)
	if replacedCorpus {
		results = append(results, Result{Path: corpusSkillDir, Status: Replaced})
	}
	for _, j := range jobs {
		target := filepath.Join(root, j.target)
		key := stampKey(j.target)
		isShipped := shipped(j.target)
		keep := func() {
			if was, ok := recorded[key]; ok && isShipped {
				wrote[key] = was
			}
		}
		note := func(content []byte) {
			if isShipped {
				wrote[key] = Entry{Digest: digest(content), CLIVersion: running}
			}
		}

		content, err := j.content()
		if err != nil {
			return nil, err
		}
		current, exists, err := readIfExists(target)
		if err != nil {
			return nil, err
		}

		if !exists {
			if err := writeFile(target, content, executable(j.target)); err != nil {
				return nil, err
			}
			note(content)
			results = append(results, Result{Path: j.target, Status: Created})
			continue
		}

		// An accumulator is a file the CLI's other commands write into after
		// scaffold has run - an index, the open-questions table. --force means
		// "discard local edits to the skeleton", and these stopped being
		// skeleton the first time `request add` or `question add` touched them.
		// Overwriting one silently destroys an engagement's interview, and in a
		// repo with no commits there is nothing to recover from. Untouched ones
		// still match their template, so leaving those to the normal path costs
		// nothing.
		if force && accumulator(j.target) && !bytes.Equal(current, content) {
			keep()
			results = append(results, Result{Path: j.target, Status: Preserved})
			continue
		}

		// A shipped file's state is worked out before --force is consulted,
		// because it decides what --force may do. A repository written by a
		// NEWER CLI is not behind, and handing it this binary's older copy is
		// the one thing --force must never do - which matters from the moment
		// the binary can update itself, since then the two versions move
		// without anybody choosing.
		state := Skipped
		if isShipped {
			state = classify(current, content, recorded[key], running)
		}
		if state == Ahead {
			keep()
			results = append(results, Result{Path: j.target, Status: Ahead})
			continue
		}

		// A managed region is derived from the config, so leaving it stale
		// would put the file out of step with the repo - the project table in
		// README.md is the case that matters, because the gate compares it
		// against the directories on disk. It is tried before the states
		// below: replacing one marked region is the narrower change, and it is
		// the only one that can refresh a file somebody has also edited.
		//
		// **It is tried before --force rather than instead of it.** --force
		// takes the newer shipped material, and in a file with a region the
		// shipped material IS the region: the scaffolded AGENTS.md tells its
		// reader in as many words that the half above the marker is theirs and
		// is never overwritten, and --force taking the file whole made that
		// sentence false in the very file the sentence is in.
		if merged, updated := mergeManaged(current, content); updated {
			if err := writeFile(target, merged, executable(j.target)); err != nil {
				return nil, err
			}
			note(merged)
			results = append(results, Result{Path: j.target, Status: Updated})
			continue
		}
		if managedRegion.Find(current) != nil {
			// The region is already in step, and everything around it is the
			// engagement's whatever --force says.
			keep()
			results = append(results, Result{Path: j.target, Status: Skipped})
			continue
		}

		// **`--force` reaches only what this CLI owns outright**, which is what
		// `shipped` decides and what the sentence above it has always said:
		// everything else in the skeleton is meant to be edited, so a
		// difference there is the engagement's work rather than drift.
		//
		// Until this asked, the default was to overwrite everything and each
		// exception was added one file at a time. `accumulators` below is what
		// is left of that approach, and it did not scale: the second time this
		// happened it took six files, five of which the record never claimed,
		// and none of which were on the list. **A whitelist of what not to
		// destroy is the wrong polarity** - it is only ever as complete as the
		// last incident.
		if force && isShipped {
			if err := writeFile(target, content, executable(j.target)); err != nil {
				return nil, err
			}
			note(content)
			results = append(results, Result{Path: j.target, Status: Overwritten})
			continue
		}

		if state == Updated {
			// Provably still what this CLI wrote, from a binary of this
			// version, so what has moved is what the render reads: the
			// repository's directory name, or its project list. Taking the new
			// one discards nothing, so it does not wait for --force - and
			// until this could be told apart from an edit, a renamed checkout
			// reported AGENTS.md stale for the rest of the engagement.
			if err := writeFile(target, content, executable(j.target)); err != nil {
				return nil, err
			}
			note(content)
			results = append(results, Result{Path: j.target, Status: Updated})
			continue
		}

		if state == Skipped && isShipped && bytes.Equal(current, content) {
			// Already what this binary carries. Recording it is how a
			// repository scaffolded before the record existed joins the
			// mechanism without --force: from the next run on, an edit to it
			// can be told from a repository that is behind.
			//
			// **The equality is load-bearing.** A file with a managed region
			// is Skipped once its region is in step, and what is on disk is
			// then the merge - the engagement's half plus our region - not the
			// render. Recording the render's digest for it would put a claim
			// in the record that this CLI wrote bytes it did not, and the next
			// run would read that claim, find the versions equal and take the
			// file whole. Which is how the answers above the marker in
			// AGENTS.md got written away once already.
			note(content)
		} else {
			keep()
		}
		results = append(results, Result{Path: j.target, Status: state})
	}

	retired, err := retiredFiles(root, recorded, jobs)
	if err != nil {
		return nil, err
	}
	for _, key := range retired {
		wrote[key] = recorded[key]
		results = append(results, Result{Path: filepath.FromSlash(key), Status: Retired})
	}

	// A record that gains a timestamp on every re-run is a committed file that
	// churns for no reason, and a diff a reviewer learns to skip.
	if stamp != nil && maps.Equal(stamp.Files, wrote) {
		return results, nil
	}
	if err := writeStamp(root, Stamp{
		WrittenAt: time.Now().UTC().Format(time.RFC3339),
		Files:     wrote,
	}); err != nil {
		return nil, err
	}

	return results, nil
}

// InspectShipped reports the state of the material this CLI ships into a
// repository - AGENTS.md and the design-time skills - and writes nothing.
//
// **A check that changes what it checks is not a check**, which is why this is
// not Write with a flag. Write merges managed regions, applies --force and
// rewrites the record, and each of those is a write that whoever asked the
// question did not ask for.
//
// It covers the shipped files only, and that is what makes it cheap enough to
// run on the end of any command: fifteen renders and fifteen reads of an
// embedded filesystem, no network, no session, no repository binding. **The
// cheapness is the point.** The reference material's other half needs a
// platform to compare against and skips when there is none; this half is
// answerable everywhere, and once a binary can replace itself it is also the
// half that moves without anybody asking - so the check that catches it has to
// be the one nothing can turn off.
func InspectShipped(root string, projects []string) ([]Result, error) {
	jobs, err := plan(NewData(root, projects))
	if err != nil {
		return nil, err
	}
	stamp, err := ReadStamp(root)
	if err != nil {
		return nil, err
	}
	recorded := map[string]Entry{}
	if stamp != nil {
		recorded = stamp.Files
	}
	running := version.Get().Version

	var out []Result
	for _, j := range jobs {
		if !shipped(j.target) {
			continue
		}
		content, err := j.content()
		if err != nil {
			return nil, err
		}
		current, exists, err := readIfExists(filepath.Join(root, j.target))
		if err != nil {
			return nil, err
		}
		if !exists {
			out = append(out, Result{Path: j.target, Status: Missing})
			continue
		}
		out = append(out, Result{
			Path:   j.target,
			Status: classify(current, content, recorded[stampKey(j.target)], running),
		})
	}

	retired, err := retiredFiles(root, recorded, jobs)
	if err != nil {
		return nil, err
	}
	for _, key := range retired {
		out = append(out, Result{Path: filepath.FromSlash(key), Status: Retired})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Writers lists the CLI versions the record at root says wrote the material
// here. It is empty when there is no record, which is not the same as none.
func Writers(root string) ([]string, error) {
	stamp, err := ReadStamp(root)
	if err != nil {
		return nil, err
	}
	return stamp.Writers(), nil
}

// retiredFiles are the paths the record says this CLI wrote and this binary no
// longer ships, sorted, and still present.
//
// **Nothing else can see them.** `plan` produces a job for each file the binary
// carries, so a skill dropped from the embed is compared against nothing: it
// stays in the repository saying whatever it said when it was written, and
// "already present" is the only thing any report ever says about the directory
// it sits in. `asgard-cr-verification` spent a month telling readers to look in
// a file that had not existed since the Pipeline cut-over, and what ended it
// was somebody re-reading a provenance line rather than any check here.
//
// A recorded file that is gone from disk is not retired, it is finished: the
// entry goes with it and nothing is reported.
func retiredFiles(root string, recorded map[string]Entry, jobs []job) ([]string, error) {
	shipping := make(map[string]bool, len(jobs))
	for _, j := range jobs {
		shipping[stampKey(j.target)] = true
	}

	var out []string
	for key := range recorded {
		if shipping[key] {
			continue
		}
		exists, err := fileExists(filepath.Join(root, filepath.FromSlash(key)))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}

// job is one template rendered to one path with one set of data.
type job struct {
	source string
	target string
	data   Data

	// body is set when the contents come from outside the embedded tree. Such
	// a job is copied verbatim - there is no template to render, and nothing in
	// a wiki page varies by repository. See corpus.go.
	body []byte
}

// content returns what to write, from the body when the job carries one and
// from the template tree otherwise.
func (j job) content() ([]byte, error) {
	if j.body != nil {
		return j.body, nil
	}
	return render(j.source, j.data)
}

// accumulators are the files this CLI's own commands append to. The spec
// module index is matched by suffix because its directory carries the spec slug.
var accumulators = map[string]bool{
	// **The deployment declaration, and the one that costs most.** It decides
	// which releases exist, what triggers each and which keys each takes -
	// none of it knowable from a scaffold, all of it written by hand. `--force`
	// replaced a twelve-release declaration with the twenty-four it generates
	// from the directories under `projects/`, taking every `chartValues` and
	// `appSecret` list with it, in a repository that had asked for the newer
	// skills and nothing else.
	//
	// It is the clearest case of the rule below rather than an exception to
	// it: a file the scaffold writes once and the engagement owns from then on.
	// The record agrees - `.asgard-scaffold.json` claims `AGENTS.md` and no
	// other top-level file, so `--force` was replacing something nothing said
	// it had written.
	//
	// **This list is no longer the protection, and adding to it is no longer
	// the fix.** `--force` reaches only what `shipped` claims, so every file
	// here is already covered by being absent from that. What is left is depth:
	// if a path is ever added to `shipped` by mistake, a name here still stops
	// the overwrite. Adding a file here and nothing else is what failed twice.
	pipelineconfig.FileName: true,

	filepath.Join("docs", "open-questions.md"):             true,
	filepath.Join("requirements", "requests", "_index.md"): true,
	filepath.Join("requirements", "tasks", "_index.md"):    true,
	filepath.Join("docs", "decisions", "README.md"):        true,
}

// OwnedByCLI reports whether `--force` may replace a path.
//
// **Exported so a test can ask the question rather than list paths.** The
// previous version of this protection was a list of names, and a list is only
// ever as complete as the last incident - it missed six files the second time.
// A test that walks the skeleton and asks this covers a file added to the
// scaffold without being edited.
func OwnedByCLI(target string) bool { return shipped(target) }

// shipped reports whether a file is material this CLI owns outright - written
// once and never edited by the engagement, the way a wiki page is never edited
// by a reader. Only these are worth reporting as stale: everything else in the
// skeleton is meant to be edited, so a difference there is the engagement's
// work, not drift.
func shipped(target string) bool {
	t := filepath.ToSlash(target)
	switch {
	case strings.HasPrefix(t, ".agents/skills/"):
		return true
	case t == "AGENTS.md":
		return true
	// **Somebody else's layout, so nobody here edits it.** The Claude Code
	// plugin and its marketplace manifest have a schema this repository does
	// not own, and `CLAUDE.md` is one line pointing at AGENTS.md. Naming them
	// is what keeps `--force` able to update them now that it reaches nothing
	// else - and a file left out of this list stops being updated silently,
	// because `gate` reports staleness only for what is shipped.
	case strings.HasPrefix(t, "plugins/asgard-fde/"):
		return true
	case t == filepath.ToSlash(filepath.Join(".claude-plugin", "marketplace.json")):
		return true
	case t == "CLAUDE.md":
		return true
	}
	return false
}

func accumulator(target string) bool {
	if accumulators[target] {
		return true
	}
	dir, file := filepath.Split(target)
	slash := filepath.ToSlash(dir)

	// docs/spec/<slug>/README.md carries the living spec's module index and its
	// traceability table, both written a row at a time as the work happens.
	if file == "README.md" && strings.HasPrefix(slash, "docs/spec/") {
		return true
	}

	// **A chart's values.yaml, which every `add` appends to.** `appendValues`
	// puts the keys each new CR reads there, because values.yaml has to default
	// every `.Values.*` a template reads - so after one `add` the file is the
	// engagement's and the templates beside it depend on what is in it.
	//
	// Regenerating it left a chart that **does not render**: the CR still reads
	// `.Values.dbDB.host` and the block is gone, so `helm template` fails on a
	// nil pointer. `check` said `ok` either way, because the structure is
	// intact - the failure is one command further on, which is what made this
	// worth a test that renders rather than one that asserts this list.
	if file == "values.yaml" && strings.HasPrefix(slash, "projects/") && strings.HasSuffix(slash, "/chart/app/") {
		return true
	}

	return false
}

// plan walks the embedded tree and expands the placeholder path segments. A
// template under __PROJECT__ produces one file per project.
//
// There is no per-environment expansion any more. Where a chart deploys is a
// release binding it to a platform project, declared in .asgard-pipeline.yaml
// and resolved by the platform, so there is nothing here that varies by
// environment to write a file for.
func plan(data Data) ([]job, error) {
	var jobs []job

	err := fs.WalkDir(templates, templateRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Python bytecode is written next to the source the moment anybody
			// runs a db-query script in place, and `go:embed all:` has no
			// exclude pattern - so without this a maintainer's local test run
			// ships .pyc files into every customer repository built from that
			// binary. It happened on the first build after the skill landed.
			if d.Name() == pycacheDir {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".pyc") {
			return nil
		}

		rel, err := filepath.Rel(templateRoot, path)
		if err != nil {
			return fmt.Errorf("resolve template path %s: %w", path, err)
		}
		rel = strings.ReplaceAll(rel, specSlugDir, data.SpecSlug)

		if !strings.Contains(rel, projectDir) {
			jobs = append(jobs, job{source: path, target: trimSuffix(rel), data: data})
			return nil
		}

		for _, project := range data.Projects {
			projectData := data
			projectData.Project = &project
			projectRel := strings.ReplaceAll(rel, projectDir, project.Slug)

			jobs = append(jobs, job{source: path, target: trimSuffix(projectRel), data: projectData})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The platform corpus is not in this package's embedded tree - go:embed
	// cannot reach across a package - so it arrives as jobs carrying their
	// bytes rather than a template path. See corpus.go.
	corpus, err := corpusJobs()
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, corpus...)

	return jobs, nil
}

// trimSuffix drops the marker that says a file is rendered rather than copied.
func trimSuffix(rel string) string {
	return strings.TrimSuffix(rel, tmplSuffix)
}

// render returns the file's contents, running it through text/template only
// when it carries the template suffix.
func render(path string, data Data) ([]byte, error) {
	content, err := templates.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", path, err)
	}
	if !strings.HasSuffix(path, tmplSuffix) {
		return content, nil
	}

	tmpl, err := template.New(filepath.Base(path)).
		Delims(leftDelim, rightDelim).
		Option("missingkey=error").
		Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", path, err)
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render template %s: %w", path, err)
	}
	return []byte(out.String()), nil
}

// executable reports whether the written file needs the execute bit: the gate
// scripts carry a shebang and are run directly.
//
// .sh is still here although no template is one any more. common/render.sh was,
// until it became `asgard-cli render` - it was bash calling yq, so the whole
// acceptance gate was unavailable on Windows while helm itself has a native
// Windows build. Keep the case: the next shell script somebody adds should not
// arrive without its execute bit.
func executable(rel string) bool {
	switch filepath.Ext(rel) {
	case ".sh", ".py":
		return true
	default:
		return false
	}
}

// readIfExists reads a file, and reports a missing one as absent rather than as
// an error. Write needs the bytes of everything that is there - the comparisons
// are all against them - so it reads each file once instead of stat-ing it and
// then reading it again in whichever branch it lands in.
func readIfExists(path string) ([]byte, bool, error) {
	body, err := os.ReadFile(path)
	switch {
	case err == nil:
		return body, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
}

func writeFile(path string, content []byte, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	// WriteFile does not change the mode of a file that already exists.
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// managedRegion matches a block the CLI keeps in step with the config even in a
// file that has otherwise been edited by hand.
var managedRegion = regexp.MustCompile(`(?s)<!-- asgard-cli:managed:start -->.*?<!-- asgard-cli:managed:end -->`)

// mergeManaged replaces the managed region of the file at path with the one from
// freshly rendered content. It reports false when either side has no managed
// region, or when the region is already identical.
func mergeManaged(current, rendered []byte) ([]byte, bool) {
	want := managedRegion.Find(rendered)
	if want == nil {
		return nil, false
	}
	if managedRegion.Find(current) == nil {
		// The marker was removed deliberately; leave the file alone.
		return nil, false
	}

	merged := managedRegion.ReplaceAllFunc(current, func([]byte) []byte { return want })
	if bytes.Equal(merged, current) {
		return nil, false
	}
	return merged, true
}

// TemplateBodies returns every embedded scaffold template, keyed by its path
// under templates/. See generate.TemplateBodies for why: a sweep for a renamed
// field has to reach the files a repository is built from, not only the prose
// that describes them.
func TemplateBodies() (map[string]string, error) {
	out := map[string]string{}
	err := fs.WalkDir(templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(path, "templates/")] = string(raw)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The corpus skill is a Go constant rather than a file in the tree, so the
	// walk misses it while it names several commands - which is exactly what
	// the command audit exists to catch. The pages and extracts it writes are
	// not added: they are already audited as themselves.
	out[filepath.ToSlash(filepath.Join(corpusSkillDir, "SKILL.md"))] = corpusSkill
	out[filepath.ToSlash(filepath.Join(corpusSkillDir, "index.md"))] = corpusIndexHead + corpusIndexTail

	return out, nil
}
