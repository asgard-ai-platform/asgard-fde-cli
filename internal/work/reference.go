package work

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReferenceIndex lists every document filed in references/, with where it came
// from.
//
// It is `_`-prefixed so that FiledReferences does not count it as customer
// material - the same rule that already skipped `_` and `README.md` there,
// which is the convention this file makes use of rather than invents.
var ReferenceIndex = filepath.Join(ReferenceDir, "_index.md")

// Reference is one filed source: a document, or a reading off a running system.
type Reference struct {
	// Path is relative to references/, so a document filed under a system
	// directory keeps that directory in its name.
	Path string
	// What it is, in the filer's words. Not the file name: "the RMA status
	// codes, as their support team uses them" is worth more than "rma.xlsx".
	What string
	// From is who supplied it. A role rather than a person where possible -
	// "their ERP vendor" outlives "their ERP vendor's integration lead". For a
	// reading it is the system and the query or command that produced it.
	From string
	// Dated is the document's own date, or the day a reading was taken, which
	// is the one that decides whether it is stale. Empty when the document does not carry one, and that is
	// itself worth recording.
	Dated string
	// Filed is the day it entered the repository.
	Filed string
	// Verified says whether anything in it was checked against the running
	// system. It starts as no and is meant to be edited.
	Verified string
}

// referenceHeader is written when the index does not exist yet. The index is
// created on demand rather than scaffolded, because a repository that never
// files a customer document should not carry an empty table telling it to.
const referenceHeader = `# What is filed here, and where it came from

One row per source in ` + "`references/`" + `: a document the customer supplied, or a
reading off a running system. Each is kept **byte-identical to what was
supplied or read**, so that a re-supplied version or a later reading can be
diffed against the filed one - which is why the provenance lives here rather
than in a header pasted into the file.

| document | what it is | from | dated | filed | verified |
|---|---|---|---|---|---|
`

const referenceRules = `
## The columns that earn their place

**dated** is the document's own date, not the day you received it; for a
reading, the day it was read. It is the one
that decides whether the material is stale, and a document with no date on it is
worth recording as such: material a customer wrote for their own staff describes
the system they believe they have, and a stale page reads exactly like a current
one.

**verified** starts as ` + "`no`" + ` on every row and is meant to be edited. A row
marked no is worth more than a plausible one, because the reader knows which to
trust. Change it when you have held a claim against the running system, and say
which claim and which filed reading - "status codes checked against
erp/rma-status-codes.txt, field lengths not" beats "yes". A reading's own row
is the running system as of its date.

**from** outlives people. A role - "their ERP vendor", "the support team" -
still means something when the person has moved on. For a reading it names the
system and the query or command, so the reading can be taken again.
`

// AddReference files a document and records where it came from.
//
// Filing a customer's document is a step every engagement takes and none has
// done the same way: each one invented its own provenance table, and one
// invented a `customer-source/` directory that then read like a convention. The
// record's shape was never the open question it looked like - `references/README.md`
// has said what it must carry for as long as it has existed. What was missing
// was making it mechanical, so it happens on the way in rather than being
// reconstructed later by somebody who was not there.
// bookkeeping is what `references/` holds that nobody filed: the directory's
// own documentation and the provenance table the command writes.
var bookkeeping = map[string]bool{"readme.md": true, "_index.md": true}

func AddReference(root string, src string, ref Reference, out io.Writer) (Reference, error) {
	info, err := os.Stat(src)
	if err != nil {
		return ref, fmt.Errorf("read %s: %w", src, err)
	}
	if info.IsDir() {
		return ref, fmt.Errorf("%s is a directory. File the documents individually, or copy the directory in by hand and add one row per document - a directory of customer material needs an index of its own, and `references/README.md` gives its shape", src)
	}

	if ref.Path == "" {
		ref.Path = filepath.Base(src)
	}
	// **The directory's own bookkeeping is not a customer's first version**, and
	// saying so matters: a customer handing over a `README.md` - a repository
	// export, a system's own readme - collided with the scaffold's directory
	// documentation and was told it had already filed that document.
	if bookkeeping[strings.ToLower(filepath.ToSlash(ref.Path))] {
		return ref, fmt.Errorf("%s is this directory's own, not a document somebody filed: "+
			"`README.md` says what the directory is for and `_index.md` is the provenance table. "+
			"Give the customer's document a name of its own - `--as <name>` - because the two "+
			"cannot both live at that path", filepath.ToSlash(ref.Path))
	}

	dest := filepath.Join(root, ReferenceDir, filepath.FromSlash(ref.Path))
	if _, err := os.Stat(dest); err == nil {
		return ref, fmt.Errorf("%s already exists. A customer's second version of a document is a different document: file it under its own name, with its own date, and leave the first one where it is - the two together are how anyone sees what changed", filepath.ToSlash(filepath.Join(ReferenceDir, ref.Path)))
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return ref, err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return ref, fmt.Errorf("read %s: %w", src, err)
	}
	// Copied rather than moved, and not rewritten. The customer's bytes are
	// evidence; a header pasted into them makes the next version undiffable.
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return ref, fmt.Errorf("write %s: %w", dest, err)
	}

	if ref.Verified == "" {
		ref.Verified = "no"
	}
	row := fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s |",
		filepath.ToSlash(ref.Path), orDash(ref.What), orDash(ref.From),
		orDash(ref.Dated), ref.Filed, ref.Verified)

	index := filepath.Join(root, ReferenceIndex)
	if _, err := os.Stat(index); os.IsNotExist(err) {
		if err := os.WriteFile(index, []byte(referenceHeader+row+"\n"+referenceRules), 0o644); err != nil {
			return ref, fmt.Errorf("write %s: %w", index, err)
		}
		return ref, nil
	}
	if err := appendRow(index, "", row); err != nil {
		return ref, err
	}
	return ref, nil
}

// ReferenceGaps returns the rows whose provenance is incomplete, so that
// `check` can say which documents nobody can date.
func ReferenceGaps(root string) ([]string, error) {
	text, err := readOptional(filepath.Join(root, ReferenceIndex))
	if err != nil || strings.TrimSpace(text) == "" {
		return nil, err
	}
	var gaps []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 6 {
			continue
		}
		name := strings.Trim(strings.TrimSpace(cells[0]), "`")
		var missing []string
		for i, col := range []string{"what it is", "from", "dated"} {
			if strings.TrimSpace(cells[i+1]) == "-" {
				missing = append(missing, col)
			}
		}
		if len(missing) > 0 {
			gaps = append(gaps, fmt.Sprintf("%s (no %s)", name, strings.Join(missing, ", no ")))
		}
	}
	return gaps, nil
}
