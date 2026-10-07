// Package check verifies the structural invariants of a customer repository -
// the ones a chart render cannot see, and that only surface at deploy time or
// when the next person tries to pick the repo up.
//
// It replaces the check_repo_consistency.py the layout used to carry, so that
// the repo step of `asgard-cli gate` needs no Python environment and every repo gets the same
// version of the rules.
package check

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/localenv"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

// Level separates a problem that fails the gate from one that is only worth
// mentioning.
type Level int

const (
	// Error fails the check.
	Error Level = iota
	// Warning is reported but does not fail.
	Warning
)

// Finding is one problem.
type Finding struct {
	Level   Level
	Message string
}

// Report is everything one run found.
type Report struct {
	Scope    []string
	Findings []Finding
}

// Errors returns the findings that fail the gate.
func (r Report) Errors() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == Error {
			out = append(out, f)
		}
	}
	return out
}

// Warnings returns the findings that do not fail the gate.
func (r Report) Warnings() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == Warning {
			out = append(out, f)
		}
	}
	return out
}

// OK reports whether the repository passed.
func (r Report) OK() bool {
	return len(r.Errors()) == 0
}

type checker struct {
	root     string
	findings []Finding
}

func (c *checker) errf(format string, args ...any) {
	c.findings = append(c.findings, Finding{Error, fmt.Sprintf(format, args...)})
}

func (c *checker) warnf(format string, args ...any) {
	c.findings = append(c.findings, Finding{Warning, fmt.Sprintf(format, args...)})
}

// Run checks the repository at root. Passing project slugs limits the
// project-scoped checks to those; repo-wide checks always run.
func Run(root string, only ...string) (Report, error) {
	c := &checker{root: root}

	projects, err := c.discoverProjects()
	if err != nil {
		return Report{}, err
	}

	scope := projects
	if len(only) > 0 {
		known := make(map[string]bool, len(projects))
		for _, p := range projects {
			known[p] = true
		}
		scope = nil
		for _, name := range only {
			if !known[name] {
				c.errf("unknown project %q; projects/ has %s", name, strings.Join(projects, ", "))
				continue
			}
			scope = append(scope, name)
		}
	}

	// The declaration is repository-wide rather than per project: one file
	// names every release, and a release is what binds a chart to a place.
	if err := c.checkDeclaration(); err != nil {
		return Report{}, err
	}
	if err := c.checkRegistry(projects); err != nil {
		return Report{}, err
	}
	if err := c.checkAssetSkills(projects); err != nil {
		return Report{}, err
	}
	c.checkRequirementIndexes()
	if err := c.checkEnvIgnored(); err != nil {
		return Report{}, err
	}
	if err := c.checkInterviewRecorded(); err != nil {
		return Report{}, err
	}
	if err := c.checkQuestionsFollowTheDeck(); err != nil {
		return Report{}, err
	}
	if err := c.checkQuestionNumbersUnique(); err != nil {
		return Report{}, err
	}
	if err := c.checkProjectsFollowRequests(projects); err != nil {
		return Report{}, err
	}
	if err := c.checkReferenceProvenance(); err != nil {
		return Report{}, err
	}
	if err := c.checkDocs(); err != nil {
		return Report{}, err
	}
	if err := c.checkOrphans(); err != nil {
		return Report{}, err
	}
	if err := c.checkCommands(); err != nil {
		return Report{}, err
	}

	return Report{Scope: scope, Findings: c.findings}, nil
}

// discoverProjects finds the project directories. A project is a directory
// under projects/ that has a chart - the same rule the layout documents.
func (c *checker) discoverProjects() ([]string, error) {
	dir := filepath.Join(c.root, "projects")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		c.errf("projects/ does not exist; every project is projects/<name>/")
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var projects []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chart := filepath.Join(dir, e.Name(), "chart", "app", "Chart.yaml")
		if _, err := os.Stat(chart); err == nil {
			projects = append(projects, e.Name())
		}
	}
	sort.Strings(projects)

	// The declaration and the disk are the two facts, and each direction is
	// worth reporting: a chart the declaration does not name deploys nowhere,
	// and a chart path the declaration names that has no Chart.yaml fails the
	// plan rather than this gate. Neither is compared against a third record
	// any more - `.asgard-config.json` was that record, and it drifted from
	// both.
	if decl, err := pipelineconfig.LoadFromRepo(c.root, ""); err == nil {
		named := map[string]bool{}
		for _, chart := range decl.Charts() {
			named[chart] = true
			if _, err := os.Stat(filepath.Join(c.root, filepath.FromSlash(chart), "Chart.yaml")); err != nil {
				c.errf("%s declares chart %s, which has no Chart.yaml; the plan fails on this with config/chart-missing",
					pipelineconfig.FileName, chart)
			}
		}
		for _, p := range projects {
			if !named[filepath.ToSlash(repo.ChartDir(p))] {
				c.warnf("projects/%s has a chart that %s does not deploy; declare a release for it, or delete the directory",
					p, pipelineconfig.FileName)
			}
		}
	}

	// A repository scaffolded before the config was deleted still has the file,
	// and it records four things that are now derived, asked for, or gone -
	// so leaving it means two answers to every question it used to answer.
	if repo.HasLegacyConfig(c.root) {
		c.errf("%s is still here. It is not read any more: the project list comes from %s and the directories on disk, "+
			"the customer's name comes from the platform, and the deployment shape and olapOnlyLayers are gone. Delete it",
			repo.LegacyConfigName, pipelineconfig.FileName)
	}
	return projects, nil
}

// checkDeclaration verifies `.asgard-pipeline.yaml`, which replaced the
// per-project deploy.yaml, the per-environment values files and the tag-driven
// workflow all at once.
//
// It checks the shape this CLI needs to do its own work - a release with a
// chart directory that exists - and nothing else. Whether the declaration is
// valid is the platform's answer: the same parse that decides a run computes
// required-missing and matches trigger patterns, and a second opinion here
// would disagree with it the first time either changed.
func (c *checker) checkDeclaration() error {
	path := filepath.Join(c.root, pipelineconfig.FileName)
	cfg, err := pipelineconfig.Load(path)
	if errors.Is(err, pipelineconfig.ErrNotFound) {
		c.errf("no %s; it declares what this repository deploys and the platform reads it on every run", pipelineconfig.FileName)
		return nil
	}
	if err != nil {
		c.errf("%v", err)
		return nil
	}

	if len(cfg.Releases) == 0 {
		// The fact alone strands a reader who has just run `init`: they are
		// told nothing deploys and not what makes it deploy. A repository this
		// early is SUPPOSED to look like this, so the warning says both.
		c.warnf("%s declares no releases, so no tag or branch deploys anything from here. "+
			"A repository this early is expected to look like this: `asgard-cli project add <slug>` "+
			"writes a chart, and a release for it is declared here", pipelineconfig.FileName)
		return nil
	}

	seen := map[string]bool{}
	for _, r := range cfg.Releases {
		switch {
		case r.Name == "":
			c.errf("%s declares a release with no name", pipelineconfig.FileName)
			continue
		case seen[r.Name]:
			c.errf("%s declares release %q twice", pipelineconfig.FileName, r.Name)
			continue
		}
		seen[r.Name] = true

		if r.Chart == "" {
			c.errf("%s: release %q names no chart", pipelineconfig.FileName, r.Name)
			continue
		}
		chart := filepath.Join(c.root, filepath.FromSlash(r.Chart))
		if _, err := os.Stat(filepath.Join(chart, "Chart.yaml")); err != nil {
			c.errf("%s: release %q names chart %s, which has no Chart.yaml", pipelineconfig.FileName, r.Name, r.Chart)
		}
		// A release with no trigger can still be run by hand, so this is a
		// warning: it is a repository nobody can deploy by pushing, which is
		// usually a mistake and occasionally the point.
		if r.On == nil || r.On.Pattern == "" {
			c.warnf("%s: release %q declares no trigger, so only a manual run deploys it", pipelineconfig.FileName, r.Name)
		}
	}
	c.checkOneReleasePerEnvironment(cfg)
	return nil
}

// environmentWords are the names an environment goes by in a release name or a
// trigger pattern. Matched as whole tokens, never as substrings: a project
// called "developer-portal" is not a dev release, and finding one would train
// everybody to ignore the warning.
var environmentWords = map[string]bool{
	"dev": true, "develop": true, "development": true,
	"stg": true, "stage": true, "staging": true,
	"prod": true, "production": true,
	"uat": true, "sit": true, "qa": true, "test": true,
}

// tokens splits an identifier or a trigger pattern into the words in it, so a
// match is on a word rather than on a substring.
var tokens = regexp.MustCompile(`[a-zA-Z]+`)

// namesAnEnvironment reports whether any whole word in s is an environment
// name.
func namesAnEnvironment(s string) bool {
	for _, w := range tokens.FindAllString(s, -1) {
		if environmentWords[strings.ToLower(w)] {
			return true
		}
	}
	return false
}

// checkOneReleasePerEnvironment warns about a chart that one release names, when
// that release says which environment it is.
//
// **A project normally needs a release per environment**, sharing one chart
// directory and differing by trigger pattern, each created against a different
// platform project — which is what gives them different namespaces. One release
// is right for a POC nobody will maintain, and for nothing else.
//
// Nothing else says so. Every prompt around a release reads as singular, which
// reads as a 1:1:1 chart-to-release-to-platform-project mapping, and an agent
// onboarding a repository takes the prompts literally: the reported case wrote
// a lone release with the pattern `dev-\d+\.\d+\.\d+` — naming a dev
// environment and never asking what the other one was.
//
// That is the shape this catches, and it is why the environment word is
// required rather than warning on every single-release chart. A release that
// does not say which environment it is has not made the mistake this is about;
// a release that does has named one of a set and stopped at one.
func (c *checker) checkOneReleasePerEnvironment(cfg *pipelineconfig.Config) {
	byChart := map[string][]pipelineconfig.Release{}
	order := []string{}
	for _, r := range cfg.Releases {
		if r.Chart == "" {
			continue
		}
		if _, seen := byChart[r.Chart]; !seen {
			order = append(order, r.Chart)
		}
		byChart[r.Chart] = append(byChart[r.Chart], r)
	}
	for _, chart := range order {
		rs := byChart[chart]
		if len(rs) != 1 {
			continue
		}
		r := rs[0]
		pattern := ""
		if r.On != nil {
			pattern = r.On.Pattern
		}
		if !namesAnEnvironment(r.Name) && !namesAnEnvironment(pattern) {
			continue
		}
		c.warnf("%s: chart %s is named by one release, %q, and that release says which "+
			"environment it is. A project normally needs one per environment - the same chart, "+
			"a different name and on.pattern, each created against a DIFFERENT platform project, "+
			"which is what gives them different namespaces. One release is the POC shape; if that "+
			"is what this is, nothing here is wrong",
			pipelineconfig.FileName, chart, r.Name)
	}
}

// projectRow matches the second column of the root README's project table,
// with or without the projects/ prefix.
var projectRow = regexp.MustCompile("(?m)^\\|[^|]*\\|\\s*`(?:projects/)?([a-z0-9-]+)/?`\\s*\\|")

// checkRegistry keeps the root README's project table and the directories in
// step. The table is how a reader learns what exists, so a stale one is worse
// than none.
func (c *checker) checkRegistry(projects []string) error {
	path := filepath.Join(c.root, "README.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		c.errf("README.md does not exist")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	listed := map[string]bool{}
	for _, m := range projectRow.FindAllSubmatch(data, -1) {
		listed[string(m[1])] = true
	}
	actual := map[string]bool{}
	for _, p := range projects {
		actual[p] = true
	}

	for _, p := range projects {
		if !listed[p] {
			c.errf("README.md project table is missing %q, which exists under projects/", p)
		}
	}
	for name := range listed {
		if !actual[name] {
			c.errf("README.md project table lists %q, which has no directory under projects/", name)
		}
	}
	return nil
}

var frontmatterBlock = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)

// frontmatter returns the top-level scalar fields of a markdown file's
// frontmatter, or nil when there is none.
func frontmatter(data []byte) map[string]string {
	m := frontmatterBlock.FindSubmatch(data)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(m[1]), "\n") {
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return out
}

// checkAssetSkills validates the runtime skills. The platform resolves one
// directory as one skill, so the directory name and the declared name have to
// agree or the SkillSet's searchPath silently resolves to nothing.
//
// The "none yet" warning is held back until there is a project. A freshly
// scaffolded repo has no agent to carry a skill, so warning there fires on
// every run of every new repo and teaches the reader that a warn from this
// command means nothing - which matters, because the interview check below
// reports something worth acting on through the same channel.
// chartsDeclareSkillSet reports whether any project's chart declares a SkillSet.
// One does when its skills come from a git repository the Syncer clones, and
// then this repository's own `assets/skills/` is empty by design.
func (c *checker) chartsDeclareSkillSet(projects []string) bool {
	for _, p := range projects {
		dir := filepath.Join(c.root, filepath.FromSlash(repo.ChartDir(p)), "templates")
		found := false
		_ = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || found {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if skillSetKind.Match(data) {
				found = true
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}

var skillSetKind = regexp.MustCompile(`(?m)^kind:\s*SkillSet\s*$`)

func (c *checker) checkAssetSkills(projects []string) error {
	c.checkRenamedCommonDir()

	dir := filepath.Join(c.root, assetsDir, "skills")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		if len(projects) > 0 && !c.chartsDeclareSkillSet(projects) {
			c.warnf("assets/skills/ does not exist; it is where runtime skills live, and there are none yet")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	// **A chart that declares a SkillSet has answered this.** Those skills live
	// in the git repository its Syncer clones, which is the ordinary shape -
	// `assets/skills/` is for skills THIS repository ships, and staying empty
	// is what the scaffold's own README says it does. Warning anyway meant a
	// finished repo carried a warning it could never clear, which is how a
	// reader learns that a warn from this command means nothing.
	if len(projects) > 0 && c.chartsDeclareSkillSet(projects) {
		return nil
	}

	found := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found = true
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			c.errf("assets/skills/%s/ has no SKILL.md; one directory is one skill", e.Name())
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		fm := frontmatter(data)
		if fm == nil {
			c.errf("assets/skills/%s/SKILL.md has no frontmatter; it needs name and description", e.Name())
			continue
		}
		for _, key := range []string{"name", "description"} {
			if fm[key] == "" {
				c.errf("assets/skills/%s/SKILL.md frontmatter is missing %s", e.Name(), key)
			}
		}
		if name := fm["name"]; name != "" && name != e.Name() {
			c.errf("assets/skills/%s/SKILL.md declares name %q, which does not match its directory", e.Name(), name)
		}
	}
	if !found && len(projects) > 0 {
		c.warnf("assets/skills/ has no skill directories yet")
	}
	return nil
}

// checkInterviewRecorded reports customer material that nothing has been done
// with - read, and left in the repository with no trace of the reading.
//
// Every other check here asks whether a file is well formed. This one asks
// whether work left anything behind, because that failure is not a malformed
// record - it is no record at all. Material is read, the analysis is done well,
// it is delivered in conversation, and the repository ends the day looking
// exactly as it did before.
//
// **Filed material is not evidence that an interview happened.** The first
// version of this check assumed it was, and warned through several hours of a
// perfectly normal state: a customer sent their own document ahead of the
// meeting, which is the usual order here because their internal approval comes
// before they will book one. Nothing was being lost - the material had been
// read and sixteen open questions written from it - and the tool said otherwise
// on every run. A warning that fires while somebody is doing the right thing
// teaches them to stop reading warnings.
//
// So open questions count as a record. They are the trace the reading leaves
// before there is anything to request.
func (c *checker) checkInterviewRecorded() error {
	filed, err := work.FiledReferences(c.root)
	if err != nil {
		return err
	}
	if filed == 0 {
		return nil
	}

	requests, err := work.ReadRequests(c.root)
	if err != nil {
		return err
	}
	if len(requests) > 0 {
		return nil
	}

	questions, err := work.ReadQuestions(c.root)
	if err != nil {
		return err
	}
	if len(questions) > 0 {
		return nil
	}

	scheduled, err := meetingScheduled(c.root)
	if err != nil {
		return err
	}
	if scheduled {
		return nil
	}

	c.warnf("references/ holds %d file(s) of customer material, and nothing records that it "+
		"was read - no question in %s, no meeting filed, no request. Read it into questions "+
		"first: `asgard-cli question add \"<what blocks it>\" --ask \"<who can answer>\"`. "+
		"**A request comes after the interview, not before it** - six of its seven sections "+
		"are what the interview decides",
		filed, filepath.ToSlash(work.QuestionFile))
	return nil
}

// checkReferenceProvenance reports a filed document whose row does not say what
// it is, who gave it to you, or when it was written.
//
// All three are recoverable only from the person who handed it over, and that
// person stops being available at roughly the point somebody needs to know. The
// date is the one that decides whether the material is stale, and a customer's
// own documentation reads exactly the same whether it is current or four years
// old.
//
// A warning, and only for documents filed through `reference add` - a row exists
// because somebody used the command, and complaining about documents copied in
// by hand would mean complaining about every repository that predates it.
func (c *checker) checkReferenceProvenance() error {
	gaps, err := work.ReferenceGaps(c.root)
	if err != nil || len(gaps) == 0 {
		return err
	}
	c.warnf("%s has %d row(s) with provenance missing: %s. Ask whoever supplied the document, while you still can - the date is the one that matters, because a stale page reads exactly like a current one",
		filepath.ToSlash(work.ReferenceIndex), len(gaps), strings.Join(gaps, "; "))
	return nil
}

// checkProjectsFollowRequests reports a project no recorded requirement asks
// for.
//
// The split is supposed to follow the audience: a project exists because
// somebody on the other end needs something, and that somebody is written down
// as a request. `next` states the rule and `--stage projects` explains it at length, and
// `project add` has always accepted a split with nothing on file - so the rule
// was advice, and the repository could not tell a considered split from a
// project somebody made because the chart was getting long.
//
// It is a warning and it is conditional on there being any requests at all. A
// repository that has not started filing them is at an earlier stage and
// `checkInterviewRecorded` covers that; complaining here as well would give one
// situation two voices. Once one request exists, the convention is in use, and a
// project outside it is worth a sentence.
func (c *checker) checkProjectsFollowRequests(projects []string) error {
	if len(projects) == 0 {
		return nil
	}

	requests, err := work.ReadRequests(c.root)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}

	asked := map[string]bool{}
	for _, r := range requests {
		if r.Project != "" {
			asked[r.Project] = true
		}
	}

	var orphans []string
	for _, p := range projects {
		if !asked[p] {
			orphans = append(orphans, p)
		}
	}
	if len(orphans) == 0 {
		return nil
	}
	sort.Strings(orphans)

	noun, verb := "project", "names"
	if len(orphans) > 1 {
		noun, verb = "projects", "name"
	}
	c.warnf("%s %s: no request in %s %s %s as its target. The split follows the audience, so a project usually exists because a recorded requirement asked for one - "+
		"either point an existing request at it (`--project <slug>` when you open it, or edit the row) or write down what it is for. "+
		"If it deliberately has no requirement behind it, say so in the decision record for the split; the next person reading the chart cannot tell that from an omission",
		noun, strings.Join(orphans, ", "), filepath.ToSlash(work.RequestIndex), verb, pluralOne(len(orphans)))
	return nil
}

// pluralOne keeps the sentence above readable for one project and for several.
func pluralOne(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// checkQuestionNumbersUnique catches two questions wearing the same number.
//
// This is a merge artefact and cannot be caught anywhere else. Numbers are
// assigned from the highest already in the file, so two branches that each add
// a question both produce the same number, and git merges the two rows cleanly
// because they are different lines. The repository is then one where "question
// 7" is ambiguous in every request and meeting note that cites it, and where
// `asgard-cli question answered 7` moves whichever row it reaches first.
//
// It is an error rather than a warning because the fix costs a minute now and
// gets more expensive with every document that cites the number - and because
// unlike the deploy-gate warnings, there is no phase of an onboarding during
// which this condition is correct.
func (c *checker) checkQuestionNumbersUnique() error {
	all, err := work.AllQuestions(c.root)
	if err != nil {
		return err
	}

	byNumber := map[string][]work.NumberedQuestion{}
	var order []string
	for _, q := range all {
		if _, seen := byNumber[q.Number]; !seen {
			order = append(order, q.Number)
		}
		byNumber[q.Number] = append(byNumber[q.Number], q)
	}

	for _, n := range order {
		rows := byNumber[n]
		if len(rows) < 2 {
			continue
		}
		// The same number in both tables with the same text is one question
		// that was answered, which is the history the file is meant to keep.
		same := true
		for _, r := range rows[1:] {
			if r.Text != rows[0].Text {
				same = false
			}
		}
		if same && len(rows) == 2 && rows[0].Text == rows[1].Text {
			continue
		}

		var texts []string
		for _, r := range rows {
			where := "Open"
			if r.Answered {
				where = "Answered"
			}
			// Questions are written in Chinese, so this counts runes: a
			// byte slice at 60 lands mid-character and prints a replacement.
			text := r.Text
			if rs := []rune(text); len(rs) > 40 {
				text = string(rs[:40]) + "..."
			}
			texts = append(texts, fmt.Sprintf("%s: %q", where, text))
		}
		c.errf("%s has question %s %d times - %s. Two branches numbered a question from the same highest number and the merge kept both. "+
			"Renumber the later one and fix anything that cites it: `grep -rn 'question %s' docs/ requests/`",
			filepath.ToSlash(work.QuestionFile), n, len(rows), strings.Join(texts, "; "), n)
	}
	return nil
}

// checkQuestionsFollowTheDeck reports a deck edited more recently than the
// questions it came from.
//
// The deck is not a rendering of `docs/open-questions.md` - it is that file's
// second editor. Working through one with a customer rewrites the questions,
// retires some and finds others, and one engagement's twenty rounds of revision
// changed about half of them, dropped three and added six. None of it went
// back, because nothing said it should.
//
// That matters more than an ordinary staleness because the questions file is
// what the next person picking the repository up reads first. They read the
// superseded version, and walk into a meeting with questions already
// abandoned. Worse, a judgement that was overturned survives there
// looking considered - in that engagement, a security reasoning the deck had
// corrected was still sitting in the file, argued well.
//
// Modification time is a weak signal and the right one here: it is exactly the
// question being asked, it needs no parsing, and a false positive costs one
// glance.
func (c *checker) checkQuestionsFollowTheDeck() error {
	questions := filepath.Join(c.root, work.QuestionFile)
	if _, err := os.Stat(questions); err != nil {
		return nil
	}

	// The newest date the questions themselves record - a row's raised date, or
	// the date an answer was written next to it.
	//
	// **Not the file's modification time**, which any edit silences - including
	// one that touches only the prose. A date inside a row moves only when
	// somebody works the questions.
	body, err := os.ReadFile(questions)
	if err != nil {
		return err
	}
	worked := ""
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		for _, d := range dateIn.FindAllString(line, -1) {
			if d > worked {
				worked = d
			}
		}
	}

	dir := filepath.Join(c.root, "docs", "meeting-notes")
	var newer []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == dir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch ext := strings.ToLower(filepath.Ext(d.Name())); ext {
		case ".html", ".md", ".json", ".pdf":
		default:
			return nil
		}
		if d.Name() == "README.md" || strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		// A meeting's date is in its directory name, which is what the layout
		// asks for; a file dropped straight into meeting-notes/ falls back to
		// its own name, and then to when it was written.
		rel, _ := filepath.Rel(c.root, path)
		when := ""
		if m := dateIn.FindString(filepath.ToSlash(rel)); m != "" {
			when = m
		} else if info, err := d.Info(); err == nil {
			when = info.ModTime().Format("2006-01-02")
		}
		if when != "" && when > worked {
			newer = append(newer, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	if len(newer) == 0 {
		return nil
	}

	sort.Strings(newer)
	since := worked
	if since == "" {
		since = "no date at all"
	}
	c.warnf("%d meeting file(s) are dated after anything in %s, the newest being %s, and that file's most recent "+
		"row is %s. **Working a deck rewrites the questions**, so if any was answered, reworded, retired or discovered "+
		"while building it, that belongs back in the rows. `asgard-cli question` prints them, and "+
		"`asgard-cli question answered <n> \"<answer>\"` moves one with the date. This compares the dates **inside** the "+
		"rows rather than the file's timestamp: an edit to the prose used to silence it, and the edit that did was a "+
		"command rename nobody connected to a warning",
		len(newer), filepath.ToSlash(work.QuestionFile), newer[len(newer)-1], since)
	return nil
}

// **What this still does not catch, measured rather than assumed: a meeting on
// the same day as the newest row.** Dates tie and nothing orders them, which is
// exactly the case that produced this fix - a discovery deck dated 2026-09-03
// with a question raised 2026-09-03 and an answer from that meeting never
// written back. A timestamp of any granularity has the same hole.
//
// The shape that would catch it is a warning that does not clear itself: it
// stands until somebody records that the questions were worked, rather than
// until a comparison happens to pass. That needs `check` to write state, and
// `check` writes nothing today - which is a contract worth more than this
// check. Recorded rather than papered over.
//
// dateIn finds an ISO date, in a table row or in a path.
var dateIn = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func meetingScheduled(root string) (bool, error) {
	dir := filepath.Join(root, "docs", "meeting-notes")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "_") || e.Name() == "README.md" {
			continue
		}
		if dateNamed.MatchString(e.Name()) || (e.IsDir() && len(e.Name()) > 10) {
			return true, nil
		}
	}
	return false, nil
}

// checkRequirementIndexes verifies the SDD entry points exist.
func (c *checker) checkRequirementIndexes() {
	for _, rel := range []string{
		filepath.Join("requirements", "_index.md"),
		filepath.Join("requirements", "requests", "_index.md"),
		filepath.Join("requirements", "tasks", "_index.md"),
	} {
		if _, err := os.Stat(filepath.Join(c.root, rel)); err != nil {
			c.errf("missing %s; it is an SDD entry point, see docs/spec-driven-development.md", filepath.ToSlash(rel))
		}
	}
}

// checkEnvIgnored refuses to let the design-time credentials be committable.
//
// **An error rather than a warning.** Everything else here is about a document
// being wrong, which costs a reader some time; this is one `git add -A` away
// from a customer's database password in a repository's history, and history is
// not something a delete removes. `asgard-cli local-env` adds the line itself,
// so reaching this means somebody wrote the file another way.
func (c *checker) checkEnvIgnored() error {
	if _, err := os.Stat(filepath.Join(c.root, localenv.FileName)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data, err := os.ReadFile(filepath.Join(c.root, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case localenv.FileName, "/" + localenv.FileName, "*.env", ".env*":
			return nil
		}
	}
	c.errf("%s exists and .gitignore does not cover it; it holds design-time credentials "+
		"and must never be committable. `asgard-cli local-env` adds the line", localenv.FileName)
	return nil
}

// assetsDir holds what the running system uses, most often by way of a Syncer
// pulling this repository into a SourceSet volume.
//
// It was `common/` until 2026-09-07, named after the fact that the projects in
// one engagement shared it. Sharing was that engagement's circumstance; what
// the directory is for is the runtime.
const assetsDir = "assets"

// legacyAssetsDir is what assetsDir used to be called.
const legacyAssetsDir = "common"

// checkRenamedCommonDir tells a repository written before the rename what to do
// about it, once.
//
// **A warning, not an error.** The rename is not finished by moving the
// directory: a SkillSet's searchPaths name a path inside the synced volume, so
// they carry this repository's directory name - `git/common/skills/<skill>` has
// to become `git/assets/skills/<skill>` - and that string lives in a Helm
// template, where nothing here can see it. So this cannot verify the fix, only
// name the half somebody is about to forget.
//
// Getting it wrong is quiet: a searchPath that resolves to no skill directory
// resolves to **zero skills**, and the platform does not call that an error.
func (c *checker) checkRenamedCommonDir() {
	if _, err := os.Stat(filepath.Join(c.root, legacyAssetsDir)); err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(c.root, assetsDir)); err == nil {
		// Both present: somebody is mid-migration and knows it.
		return
	}
	c.warnf("%s/ is what %s/ is called now - it holds what the runtime uses, and "+
		"whether the projects share it was never the point. Move it, and then move "+
		"the other half: a SkillSet's searchPaths carry this directory's name, so "+
		"`git/%s/skills/<skill>` has to become `git/%s/skills/<skill>` in the chart. "+
		"A searchPath that resolves to no directory resolves to zero skills, and the "+
		"platform does not report that",
		assetsDir, legacyAssetsDir, legacyAssetsDir, assetsDir)
}

// dateNamed matches the filename convention for dated records.
var dateNamed = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-[a-z0-9][a-z0-9-]*\.md$`)

// moduleLink matches a link to a module file from a spec index.
var moduleLink = regexp.MustCompile(`\]\(([a-z0-9][a-z0-9-]*\.md)\)`)

// markdownLink matches a relative markdown link, ignoring any anchor.
var markdownLink = regexp.MustCompile(`\]\(([^)\s#]+\.md)(?:#[^)\s]*)?\)`)

// checkDocs validates the spec layer: the files the four-layer model needs, the
// living spec's module index matching what is on disk, dated filenames, and
// that every relative link inside docs/ resolves. Rotten links are how this
// layer decays.
func (c *checker) checkDocs() error {
	docs := filepath.Join(c.root, "docs")
	if _, err := os.Stat(docs); os.IsNotExist(err) {
		c.errf("docs/ does not exist; it is the spec and decision layer, see docs/README.md")
		return nil
	}

	for _, rel := range []string{
		"README.md",
		"spec-driven-development.md",
		filepath.Join("spec", "README.md"),
		filepath.Join("decisions", "README.md"),
		filepath.Join("decisions", "_decision-template.md"),
		filepath.Join("meeting-notes", "README.md"),
		filepath.Join("meeting-notes", "_template.md"),
	} {
		if _, err := os.Stat(filepath.Join(docs, rel)); err != nil {
			c.errf("missing docs/%s; see the converge loop in docs/README.md", filepath.ToSlash(rel))
		}
	}

	if err := c.checkLivingSpec(filepath.Join(docs, "spec")); err != nil {
		return err
	}
	if err := c.checkDatedNames(docs); err != nil {
		return err
	}
	return c.checkDocLinks(docs)
}

// checkLivingSpec compares each spec slug's module index against the files
// present. A module written but never indexed is invisible to the next reader.
func (c *checker) checkLivingSpec(specDir string) error {
	entries, err := os.ReadDir(specDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", specDir, err)
	}

	slugs := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slugs++
		slug := e.Name()
		index := filepath.Join(specDir, slug, "README.md")

		data, err := os.ReadFile(index)
		if os.IsNotExist(err) {
			c.errf("missing docs/spec/%s/README.md, the living spec's module index", slug)
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", index, err)
		}

		listed := map[string]bool{}
		for _, m := range moduleLink.FindAllSubmatch(data, -1) {
			listed[string(m[1])] = true
		}

		modules, err := os.ReadDir(filepath.Join(specDir, slug))
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Join(specDir, slug), err)
		}
		actual := map[string]bool{}
		for _, m := range modules {
			if m.IsDir() || m.Name() == "README.md" || !strings.HasSuffix(m.Name(), ".md") {
				continue
			}
			actual[m.Name()] = true
			if !listed[m.Name()] {
				c.errf("docs/spec/%s/README.md does not index %s, which exists", slug, m.Name())
			}
		}
		for name := range listed {
			if !actual[name] {
				c.errf("docs/spec/%s/README.md indexes %s, which does not exist", slug, name)
			}
		}
	}
	if slugs == 0 {
		c.warnf("docs/spec/ has no living spec yet; it should be docs/spec/<slug>/")
	}
	// **Two living-spec roots is the failure the four-layer split exists to
	// prevent**, and the next reader cannot tell which is current. It is a
	// warning rather than an error because `docs/spec/README.md` allows a
	// second slug for a genuinely separate system - and it says that is rare,
	// so the common cause is a rename left half-done.
	if slugs > 1 {
		roots := make([]string, 0, slugs)
		for _, e := range entries {
			if e.IsDir() {
				roots = append(roots, "docs/spec/"+e.Name()+"/")
			}
		}
		c.warnf("%d living specs: %s. `docs/spec/README.md` names the one in use, and says a second "+
			"slug is only for a system with its own audience and lifecycle - so two is usually a rename "+
			"that stopped halfway. Two indexes that disagree is what the layers exist to prevent, and "+
			"nothing here can tell you which is current", slugs, strings.Join(roots, ", "))
	}
	return nil
}

// checkDatedNames enforces YYYY-MM-DD-<topic>.md on the immutable layers. The
// date is what makes a record findable later.
func (c *checker) checkDatedNames(docs string) error {
	for _, sub := range []string{"decisions", "meeting-notes"} {
		dir := filepath.Join(docs, sub)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || name == "README.md" || strings.HasPrefix(name, "_") || !strings.HasSuffix(name, ".md") {
				continue
			}
			if !dateNamed.MatchString(name) {
				c.errf("docs/%s/%s is not named YYYY-MM-DD-<topic>.md with a lowercase kebab-case topic", sub, name)
			}
		}
	}
	return nil
}

// checkDocLinks resolves every relative markdown link inside docs/.
func (c *checker) checkDocLinks(docs string) error {
	return filepath.Walk(docs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(c.root, path)
		for _, m := range markdownLink.FindAllSubmatch(data, -1) {
			target := string(m[1])
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "/") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				c.errf("%s links to %s, which does not resolve", filepath.ToSlash(rel), target)
			}
		}
		return nil
	})
}

// wikiPage reports whether a file under docs/ or requirements/ is a page that
// somebody is expected to reach by following a link. READMEs are entry points
// and are reached by opening a directory; a leading underscore marks an index
// or a template, which is linked TO rather than linked FROM.
func wikiPage(rel string) bool {
	dir := filepath.ToSlash(filepath.Dir(rel))
	if !strings.HasPrefix(dir, "docs") && !strings.HasPrefix(dir, "requirements") {
		return false
	}
	// A living spec module is owned by checkLivingSpec, which compares the slug
	// README against the files on disk and says exactly which is missing. That
	// is the same defect stated more precisely, so reporting it here as well
	// would print two findings for one problem.
	if strings.HasPrefix(dir, "docs/spec/") {
		return false
	}
	name := filepath.Base(rel)
	return strings.HasSuffix(name, ".md") &&
		name != "README.md" &&
		!strings.HasPrefix(name, "_")
}

// checkOrphans reports pages nothing links to.
//
// It is the one part of a knowledge base's decay that is mechanical. A page
// with no inbound link is not read, and the person who wrote it never finds
// out, because the file is still sitting there: a decision nobody applied, a
// module missing from its index, a task spec that fell out of the queue.
//
// A warning, not an error. A decision recorded today and not yet applied to the
// living spec is an orphan for as long as that takes, and that is a normal
// state to pass through - failing the gate on it would leave the gate red in
// the middle of ordinary work. The three kinds of rot that are NOT mechanical -
// two pages that contradict each other, a claim a newer source superseded, and
// a concept discussed everywhere but owned by no page - need a reader, and the
// knowledge-base skill under .agents/skills/ is how that pass is run.
func (c *checker) checkOrphans() error {
	linked := map[string]bool{}
	var pages []string

	err := filepath.Walk(c.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".out", "node_modules", ".venv":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Every markdown file in the repo counts as a source of links, not just
		// the ones under docs/: AGENTS.md is where an agent starts, and a page
		// reachable only from a skill is still reachable.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for _, m := range markdownLink.FindAllSubmatch(data, -1) {
			target := string(m[1])
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "/") {
				continue
			}
			linked[filepath.Clean(filepath.Join(filepath.Dir(path), target))] = true
		}

		if rel, err := filepath.Rel(c.root, path); err == nil && wikiPage(rel) {
			pages = append(pages, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Strings(pages)
	for _, path := range pages {
		if linked[filepath.Clean(path)] {
			continue
		}
		rel, _ := filepath.Rel(c.root, path)
		c.warnf("%s has no inbound link, so nothing leads a reader to it: %s",
			filepath.ToSlash(rel), orphanHint(rel))
	}
	return nil
}

// orphanHint names the index that was supposed to carry the link, because the
// fix differs by layer and "add a link" does not say where.
func orphanHint(rel string) string {
	switch dir := filepath.ToSlash(filepath.Dir(rel)); {
	case strings.HasPrefix(dir, "docs/decisions"):
		return "link it from the living spec module it changed, and from docs/decisions/README.md"
	case strings.HasPrefix(dir, "docs/meeting-notes"):
		return "link it from the decision it converged into, or from docs/meeting-notes/README.md"
	case strings.HasPrefix(dir, "docs/spec"):
		return "add it to that slug's README module index"
	case strings.HasPrefix(dir, "requirements/requests"):
		return "register it in requirements/requests/_index.md"
	case strings.HasPrefix(dir, "requirements/tasks"):
		return "register it in requirements/tasks/_index.md"
	default:
		return "link it from the index of its layer"
	}
}
