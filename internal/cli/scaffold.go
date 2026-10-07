package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// scaffoldRoot is where the skeleton goes.
//
// **The skeleton is written where you are.** Every other command finds the
// repository by its declaration, and this is the command that writes one - so
// it cannot require one to already exist.
func scaffoldRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	if found := repo.Root(dir); found != "" {
		return found, nil
	}
	return dir, nil
}

// runScaffold writes the skeleton and reports it.
//
// There used to be a `scaffold` command that was this and nothing else, beside
// an `init` that was this plus a binding plus a skills fetch. Once init stopped
// needing a session the two did the same thing, and two commands doing the same
// thing is a question - "which of these do I run?" - that gets asked, and was,
// twice. This is what is left: one command, one internal writer.
func runScaffold(cmd *cobra.Command, root string, force bool) error {
	projects, err := repo.Projects(root)
	if err != nil {
		return err
	}

	results, err := scaffold.Write(root, projects, force)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var created, overwritten, updated, skipped int
	var preserved, stale, behind, ahead, edited, retired []string
	var replaced string
	for _, r := range results {
		switch r.Status {
		case scaffold.Created:
			created++
			fmt.Fprintf(out, "  created      %s\n", r.Path)
		case scaffold.Overwritten:
			overwritten++
			fmt.Fprintf(out, "  overwritten  %s\n", r.Path)
		case scaffold.Updated:
			// Printed rather than counted among the untouched. It used to be
			// only a managed region being replaced, which is invisible by
			// design; it now also covers a whole file this CLI wrote being
			// re-rendered because the repository moved under it, and a command
			// that changes a file says which.
			updated++
			fmt.Fprintf(out, "  updated      %s\n", r.Path)
		case scaffold.Preserved:
			preserved = append(preserved, r.Path)
			fmt.Fprintf(out, "  preserved    %s\n", r.Path)
		case scaffold.Stale:
			stale = append(stale, r.Path)
			fmt.Fprintf(out, "  stale        %s\n", r.Path)
		case scaffold.Behind:
			behind = append(behind, r.Path)
			fmt.Fprintf(out, "  behind       %s\n", r.Path)
		case scaffold.Ahead:
			ahead = append(ahead, r.Path)
			fmt.Fprintf(out, "  ahead        %s\n", r.Path)
		case scaffold.Edited:
			edited = append(edited, r.Path)
			fmt.Fprintf(out, "  edited       %s\n", r.Path)
		case scaffold.Retired:
			retired = append(retired, r.Path)
			fmt.Fprintf(out, "  retired      %s\n", r.Path)
		case scaffold.Replaced:
			replaced = r.Path
			fmt.Fprintf(out, "  replaced     %s\n", r.Path)
		default:
			skipped++
		}
	}

	fmt.Fprintf(out, "\n%d created", created)
	if overwritten > 0 {
		fmt.Fprintf(out, ", %d overwritten", overwritten)
	}
	if updated > 0 {
		fmt.Fprintf(out, ", %d updated", updated)
	}
	if skipped > 0 {
		fmt.Fprintf(out, ", %d already present", skipped)
	}
	for _, c := range []struct {
		label string
		paths []string
	}{
		{"behind", behind}, {"edited", edited}, {"ahead", ahead},
		{"stale", stale}, {"retired", retired},
	} {
		if len(c.paths) > 0 {
			fmt.Fprintf(out, ", %d %s", len(c.paths), c.label)
		}
	}
	fmt.Fprintf(out, " in %s\n", root)

	// The one delete this tool performs, so it says so rather than letting the
	// files underneath it read as ordinary creations. Nothing there is the
	// engagement's: the directory is generated outright, and a page renamed
	// upstream would otherwise leave both names on disk for grep to find.
	if replaced != "" {
		fmt.Fprintf(out, "\n%s came from another version of this CLI, so it was\n"+
			"removed and written again. That directory is generated, and it is the\n"+
			"only one this command deletes.\n", replaced)
	}

	// "already present" reads as "up to date", and that reading has been acted
	// on: an agent re-ran scaffold after an upgrade, saw nothing to do, told
	// the user the repo was current, and went on to work from a skill three
	// versions old.
	//
	// **The conditions below are separated because each is fixed
	// somewhere else**, and one of them must not be fixed with --force at all.
	// They used to be one line saying "yours are older", which was a guess: it
	// was printed over an engagement's own answers as readily as over material
	// that really was behind, and the remedy it offered would have deleted
	// them. `scaffold.StampName` is the record that tells those apart.
	if len(behind) > 0 {
		reportShipped(out, behind, "are shipped material this CLI has since changed. Nobody here has\n"+
			"touched them, so yours are simply older:")
		fmt.Fprintf(out, "\nTake the newer ones with `asgard-cli init --force`. Nothing an\n"+
			"`asgard-cli` command writes into is touched by that - indexes, the\n"+
			"open-questions table and the living spec are preserved either way.\n")
	}

	if len(edited) > 0 {
		reportShipped(out, edited, "differ from what this CLI last wrote to them, so somebody here\n"+
			"changed them:")
		fmt.Fprintf(out, "\nThey were left alone. `--force` would discard those changes. For example,\n"+
			"the scaffolded AGENTS.md ships a project list whose rows say\n"+
			"`TODO: what it does`, and filling one in puts the file in this state.\n"+
			"A correction to shipped material belongs upstream, in the CLI, rather\n"+
			"than in a file the next `--force` replaces.\n")
	}

	if len(ahead) > 0 {
		reportShipped(out, ahead, "were written here by a NEWER asgard-cli than the one running:")
		fmt.Fprintf(out, "\nThey were left alone, and `--force` will not take this binary's copy of\n"+
			"them either - that would be a downgrade, and \"take the newer shipped\n"+
			"material\" is what the flag is for. Upgrade the CLI. To reset one to this\n"+
			"binary's version deliberately, delete it and run this again.\n")
	}

	if len(stale) > 0 {
		reportShipped(out, stale, "are shipped material that differs from what this CLI carries, and\n"+
			"which way round is not knowable:")
		fmt.Fprintf(out, "\nEither %s does not record who wrote them - a repository\n"+
			"scaffolded before it existed - or the two versions cannot be ordered,\n"+
			"which is what a `go build` binary reports. `asgard-cli init --force`\n"+
			"takes this binary's copy; read the diff first.\n", scaffold.StampName)
	}

	if len(retired) > 0 {
		reportShipped(out, retired, "were written here by an asgard-cli that shipped them, and this one\n"+
			"does not:")
		fmt.Fprintf(out, "\nNothing checks them any more, so they will not be updated. Read them and\n"+
			"delete them; a scaffold does not remove files from a customer's repository\n"+
			"on its own.\n")
	}

	if len(preserved) > 0 {
		reportShipped(out, preserved, "preserved despite --force, because `asgard-cli` writes into them\n"+
			"and they no longer match the template they started as:")
		fmt.Fprintf(out, "\n--force discards local edits to the skeleton, and each of these stopped\n"+
			"being skeleton the first time an `asgard-cli` command wrote to it. A file with\n"+
			"a managed region is the other way round: --force replaces the region and leaves\n"+
			"what the engagement wrote around it. To reset one deliberately, delete it and\n"+
			"run scaffold again.\n")
	}

	return nil
}

// reportShipped prints one condition: how many files are in it, what the
// condition is, and which files. The sentence is the caller's because each of
// them is a different thing to have happened.
func reportShipped(out io.Writer, paths []string, condition string) {
	fmt.Fprintf(out, "\n%d file(s) %s\n\n", len(paths), condition)
	for _, p := range paths {
		fmt.Fprintf(out, "  %s\n", p)
	}
}

// warnIfShippedStale says so when the material this CLI ships into a
// repository is not what this binary carries.
//
// **It is warnIfBehind's sibling, and it is the one that always fires.** That
// one rides on a platform response, so it is silent on every command that
// reached no platform - which is most of them, and all of them on a plane.
// This one compares against the binary it is part of, so the only thing it
// needs is a repository to be standing in.
//
// **A binary that can replace itself is why this cannot live in `init`.**
// Nothing re-runs `init`: it is documented as the one command written for a
// person, and a person runs it once, in an empty directory. With a self-update
// the material moves between two commands, without anybody having run
// anything - so the only thing that can report it is something already being
// run, which is the argument warnIfBehind makes for the platform's half and it
// holds harder here.
//
// **It says nothing about an edited file.** Somebody here changed shipped
// material on purpose, and repeating that after every command is nagging about
// a decision already taken. `gate` lists it, once, when asked.
func warnIfShippedStale(cmd *cobra.Command) {
	// `init`, `gate` and `skill` have each said it already, in more detail and
	// with the remedy in context. Saying it again underneath them reads as a
	// second, different problem.
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "init", "gate", "skill":
			return
		}
	}
	// `repo.Root` and not `locateRepo`: that one finds the checkout by its
	// `.git`, and whether the material this CLI shipped here is current has
	// nothing to do with whether anybody has run `git init` yet - `asgard-cli
	// init` deliberately writes the skeleton before there is a repository. A
	// non-empty answer here is also the "is this the kind of repository that
	// needs the material" test, because what it locates IS the declaration.
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	repoRoot := repo.Root(dir)
	if repoRoot == "" {
		return
	}
	projects, err := repo.Projects(repoRoot)
	if err != nil {
		return
	}
	results, err := scaffold.InspectShipped(repoRoot, projects)
	if err != nil {
		return
	}

	counts := map[scaffold.Status]int{}
	for _, r := range results {
		counts[r.Status]++
	}

	errOut := cmd.ErrOrStderr()
	if n := counts[scaffold.Ahead]; n > 0 {
		// The command rather than the word "upgrade": `installCommand` is the
		// one place that spells it, so this line, the gate's remedy and the
		// README cannot come to disagree about what to run.
		fmt.Fprintf(errOut, "\n%d file(s) here were written by a newer asgard-cli than this one (%s)\n"+
			"this repository is ahead of the binary, not behind it:\n    %s\n",
			n, version.Get().Version, upgradeCommand())
	}

	var parts []string
	total := 0
	for _, st := range []scaffold.Status{scaffold.Missing, scaffold.Retired, scaffold.Behind, scaffold.Stale} {
		if n := counts[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, st))
			total += n
		}
	}
	if total == 0 {
		return
	}
	// Which files, and what to do about each, is the gate's to say: these four
	// states are fixed in three different ways and a one-line remedy would be
	// right about one of them.
	fmt.Fprintf(errOut, "\n%d file(s) of what this CLI ships here are not what it carries now (%s)\n"+
		"    asgard-cli gate\n", total, strings.Join(parts, ", "))
}
