package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("verified", check{
		Needs: "this repository",
		What:  "run the checks whose inputs have moved and skip the ones whose have not - a pass that does not start from zero. `--all` ignores the record, `--forget` deletes it",
		Run:   runVerified,
	})
}

// **A pass re-reads everything every time, and that is the cost that makes a
// reading backlog look endless.** What a check answers is a function of its
// inputs, so a check whose inputs have not moved has already been answered;
// running it again produces the same exit code more slowly, and doing that
// eight times in one sitting is most of why the same parts get read again and
// again.
//
// **The record is in `.out/` and is not committed.** It says what passed
// against which inputs on this machine. A shared record would be a claim about
// somebody else's tree, and the first time it was wrong nobody would trust any
// of it.
const verifiedRecord = ".out/verified.json"

// inputClass is a body of bytes a check reads. A check names the classes it
// reads, and its key is those classes' digests - so a prose change does not
// invalidate a check that only reads the CRDs, and pulling a clone does not
// invalidate one that only reads this repository.
//
// **The classes are coarse on purpose.** Naming the exact files a check reads
// is a second copy of what the check does, kept by hand, and it goes stale in
// the direction that matters: a check reads a new file, the list does not say
// so, and the record reports a pass for an answer that moved underneath it.
// **A skip that lies is worse than re-running everything**, so the unit is a
// whole class, and a check whose inputs are not classifiable declares none and
// always runs.
type inputClass string

const (
	classCorpus inputClass = "corpus" // internal/corpus, internal/stage/prompts, internal/needs, internal/brief
	classGo     inputClass = "go"     // every .go file, which is the binary and the checks alike
	classDocs   inputClass = "docs"   // the root documents and the maintenance skills
	classKube   inputClass = "kube"   // the asgard-kube clone, at its commit
	classCore   inputClass = "core"
	classDocsUp inputClass = "docs-upstream"
)

// The gate is not only `hack/`. This is the rest of it: the audit flags that
// fail.
//
// **A pass that skips half the gate and says nothing is the lie this whole
// mechanism exists to avoid.** The audits are the expensive half over the
// corpus and were outside the record entirely, so "it did not start from zero"
// was true of the fast half and false of the slow one.
//
// `--urls` is absent for its own reason, the same one that keeps it out of CI:
// a third party's outage is not this repository's failure, and its answer is
// not a function of anything in this list.
var gateAudits = []string{"links", "commands", "bare", "paths", "unverified"}

// reads says which classes each check's answer depends on.
//
// **Absent means "always run".** That is the safe default and the one a new
// check gets for free: a check nobody has classified is a check that runs,
// which costs time and never lies.
var reads = map[string][]inputClass{
	"index":        {classCorpus, classGo},
	"doc-paths":    {classDocs, classGo},
	"pass-list":    {classDocs, classGo},
	"goal":         {classCorpus, classGo, classDocs},
	"aliases":      {classCorpus, classGo},
	"write-path":   {classCorpus, classGo},
	"coverage":     {classCorpus, classGo, classDocsUp},
	"counts":       {classCorpus, classGo, classDocsUp},
	"tables":       {classCorpus, classGo, classDocs, classKube},
	"processors":   {classCorpus, classGo, classCore, classDocsUp},
	"validate-crs": {classCorpus, classGo, classKube},
	"extract-crs":  {classCorpus, classGo},

	// The audits read what the binary embeds, which is the corpus compiled by
	// the Go source.
	"--links":      {classCorpus, classGo},
	"--commands":   {classCorpus, classGo, classDocs},
	"--bare":       {classCorpus, classGo},
	"--paths":      {classCorpus, classGo},
	"--unverified": {classCorpus, classGo},
}

func runVerified(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	all, forget := false, false
	for _, a := range args {
		switch a {
		case "--all":
			all = true
		case "--forget":
			forget = true
		}
	}
	recordPath := filepath.Join(root, verifiedRecord)
	if forget {
		if err := os.Remove(recordPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Println("forgot every recorded pass; the next run checks everything.")
		return nil
	}

	digests, err := classDigests(root)
	if err != nil {
		return err
	}
	record := map[string]string{}
	if !all {
		if data, err := os.ReadFile(recordPath); err == nil {
			_ = json.Unmarshal(data, &record)
		}
	}

	names := goCheckNames()
	ran, skipped, failed := 0, 0, 0
	for _, n := range names {
		if checks[n].Listing || n == "verified" || n == "pass" || n == "list" || n == "sources" {
			continue
		}
		key := checkKey(n, digests)
		if record[n] == key {
			fmt.Printf("skip  %-14s inputs unchanged since it last passed\n", n)
			skipped++
			continue
		}
		start := time.Now()
		if err := checks[n].Run(nil); err != nil {
			fmt.Printf("FAIL  %-14s %v\n", n, err)
			delete(record, n)
			failed++
			continue
		}
		fmt.Printf("ok    %-14s %s\n", n, time.Since(start).Round(time.Millisecond))
		record[n] = key
		ran++
	}

	// **The audits read the embedded corpus through the built binary**, so
	// their inputs are the corpus and the Go source that embeds it - the same
	// two classes, because the binary is a function of both.
	binary := filepath.Join(root, ".out/asgard-cli")
	for _, flag := range gateAudits {
		name := "--" + flag
		key := checkKey(name, digests)
		if record[name] == key {
			fmt.Printf("skip  %-14s inputs unchanged since it last passed\n", name)
			skipped++
			continue
		}
		start := time.Now()
		out, err := exec.Command(binary, "audit-material", name).CombinedOutput()
		if err != nil {
			fmt.Printf("FAIL  %-14s %v\n%s\n", name, err, out)
			delete(record, name)
			failed++
			continue
		}
		fmt.Printf("ok    %-14s %s\n", name, time.Since(start).Round(time.Millisecond))
		record[name] = key
		ran++
	}

	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(recordPath, append(data, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("\n%d ran, %d skipped, %d failed.\n", ran, skipped, failed)
	fmt.Println("\n**A skip is not a pass.** It says this check answered these exact inputs")
	fmt.Println("before, on this machine. It says nothing about the surfaces no check")
	fmt.Println("reaches, which is where every defect found by reading has come from -")
	fmt.Println("`go run ./hack related` names the documents a change owes a re-read.")
	if failed > 0 {
		return errFailed
	}
	return nil
}

// checkKey is what a recorded pass is keyed by: the check's name and the
// digests of every class it reads. A check that declares no class gets a key
// that never matches, so it always runs.
func checkKey(name string, digests map[inputClass]string) string {
	classes, known := reads[name]
	if !known {
		// **A key that never matches has to be unique per call, and "" is the
		// opposite.** The record is a map, so a check that has never passed
		// reads back as "" too - an unclassified check matched its own absence
		// and was reported as skipped on the very first run, having never run.
		// Nor can the key be any fixed value: the run that records one makes
		// every later run match it. So it is bytes nothing else will produce.
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Sprintf("unclassified-%d", time.Now().UnixNano())
		}
		return "unclassified-" + hex.EncodeToString(b[:])
	}
	// **Sorted, so reordering `reads` costs nothing.** That list is
	// hand-written and somebody will tidy it; a re-run is the safe direction to
	// be wrong in, but it is still a cost with no answer behind it.
	sorted := append([]inputClass(nil), classes...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", name)
	for _, c := range sorted {
		fmt.Fprintf(h, "%s=%s\n", c, digests[c])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func classDigests(root string) (map[inputClass]string, error) {
	out := map[inputClass]string{}
	var err error
	if out[classCorpus], err = treeDigest(root, []string{
		"internal/corpus", "internal/stage/prompts", "internal/needs", "internal/brief",
	}); err != nil {
		return nil, err
	}
	if out[classGo], err = treeDigest(root, []string{"internal", "hack", "cmd"}); err != nil {
		return nil, err
	}
	if out[classDocs], err = treeDigest(root, []string{
		"Goal.md", "README.md", "README.zh-TW.md", "AGENTS.md", "STRUCTURE.md",
		"APPROACH.md", "TASK.md", ".agents/skills",
	}); err != nil {
		return nil, err
	}
	for c, env := range map[inputClass]string{
		classKube: "kube", classCore: "core", classDocsUp: "docs",
	} {
		// **A clone is keyed by its commit, not by hashing it.** These are
		// other people's repositories, some of them large, and the commit is
		// exactly the thing a reading is held against - which is what
		// `go run ./hack sources` already reports.
		dir, err := src.Resolve(env)
		if err != nil {
			out[c] = "unresolved"
			continue
		}
		b, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err != nil {
			out[c] = "unknown"
			continue
		}
		out[c] = strings.TrimSpace(string(b))
	}
	return out, nil
}

// hasOutSegment reports whether a repository-relative path has a `.out`
// directory anywhere in it.
//
// **A segment, and relative to the root.** A substring test on the absolute
// path was both too loose and catastrophically so: a checkout living under any
// directory named `.out` excluded every file, every class digested to nothing,
// every key became the same, and every check skipped forever - a lying skip
// reached by where somebody happened to clone. Relative fixes that; by segment
// rather than by prefix keeps the exclusion at any depth, which is what a
// reader who puts scratch output beside the thing it came from expects.
func hasOutSegment(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".out" {
			return true
		}
	}
	return false
}

// treeDigest hashes every file under the given paths, by name and content.
func treeDigest(root string, paths []string) (string, error) {
	h := sha256.New()
	var files []string
	for _, p := range paths {
		full := filepath.Join(root, p)
		err := filepath.Walk(full, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			// **Tests are excluded, and only tests.** A check's answer depends
			// on its own code, so `hack/*.go` is in this class - but a test
			// file changes what is proved about a check, never what it
			// answers, and including them means writing one invalidates every
			// recorded pass in the repository.
			//
			// **The output directory is excluded relative to the root, not by
			// substring.** Matching `/.out/` anywhere in an absolute path
			// means a checkout living under a directory of that name digests
			// every class to nothing - and a digest of nothing is the same for
			// every class, so every check matches its record forever. That is
			// the lying skip this whole file exists to avoid, reached by where
			// somebody happened to clone.
			rel, relErr := filepath.Rel(root, path)
			if fi.IsDir() || strings.HasSuffix(path, "_test.go") || relErr != nil || hasOutSegment(rel) {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(files)
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s\n", rel)
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
