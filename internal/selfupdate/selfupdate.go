// Package selfupdate answers one question - is a newer asgard-cli published -
// without letting the answer cost anything.
//
// **Nothing here blocks a command, and nothing here updates the binary.** A CLI
// that replaces itself has to pick a moment, and every moment is somebody
// else's: a package manager's database goes out of step, a running .exe on
// Windows is locked, and /usr/local/bin is usually root's. So this reports, and
// `install.sh` is what acts.
//
// **The question is asked at most once every two hours**, recorded in the
// user's config directory - never in a customer repository, which is somebody
// else's checkout and is committed. A version check that runs on every command
// is a network call on every command, and the answer changes about as often as
// somebody publishes.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Interval is how stale an answer may be before it is asked again. Two hours is
// chosen against how often a release happens rather than how fresh an answer
// could be: asking more often changes the answer on almost no run.
const Interval = 2 * time.Hour

// IntervalText is Interval as a help screen says it.
//
// **The constant is the source and the screens read it**, because "two hours"
// typed beside a declaration is a copy: changing Interval would leave a flag's
// usage text, a Long and whatever else quoted it all saying the old number,
// and nothing would notice. `Duration.String` gives 2h0m0s, which is right and
// is not what somebody reading a help screen wants.
func IntervalText() string {
	switch {
	case Interval%time.Hour == 0 && Interval == time.Hour:
		return "an hour"
	case Interval%time.Hour == 0:
		return fmt.Sprintf("%d hours", int(Interval/time.Hour))
	case Interval%time.Minute == 0:
		return fmt.Sprintf("%d minutes", int(Interval/time.Minute))
	}
	return Interval.String()
}

// FileName is the record, beside the profiles and the credential store.
const FileName = "update-check.json"

// EnvDisable turns the check off entirely. **It exists because somebody will
// need it**: an air-gapped machine, a CI job that should make no call it did not
// ask for, or a user who simply does not want one.
const EnvDisable = "ASGARD_NO_UPDATE_CHECK"

const latestURL = "https://api.github.com/repos/asgard-ai-platform/asgard-fde-cli/releases/latest"

// Record is what the last check concluded.
type Record struct {
	// CheckedAt is when the question was last asked, successfully or not. A
	// failure is recorded too: a machine with no network would otherwise ask
	// on every command, which is the cost this file exists to avoid.
	CheckedAt time.Time `json:"checked_at"`
	// Latest is the newest tag seen, without the leading v. Empty when the
	// last check failed.
	Latest string `json:"latest,omitempty"`
}

// Result says what is known and whether anything was asked to find out.
type Result struct {
	Latest    string
	Running   string
	Newer     bool
	Asked     bool
	CheckedAt time.Time
}

// Path is where the record lives. home is the CLI's config directory.
func Path(home string) string { return filepath.Join(home, FileName) }

// Check reports whether a newer release exists, asking at most once per
// Interval and never returning an error that should stop a command.
//
// **Every failure is silent by design.** No network, a rate limit, a proxy that
// answers with HTML - none of them is a reason for the command the user
// actually ran to behave differently, and a warning about a failed version
// check is noise on every run in an environment that will never succeed.
func Check(ctx context.Context, home, running string, force bool) Result {
	res := Result{Running: running}
	if os.Getenv(EnvDisable) != "" && !force {
		return res
	}

	rec, _ := load(Path(home))
	res.Latest, res.CheckedAt = rec.Latest, rec.CheckedAt

	fresh := time.Since(rec.CheckedAt) < Interval
	if fresh && !force {
		res.Newer = newer(running, res.Latest)
		return res
	}

	res.Asked = true
	latest, err := fetch(ctx)
	rec.CheckedAt = time.Now()
	if err == nil && latest != "" {
		rec.Latest = latest
	}
	_ = save(Path(home), rec)

	res.Latest, res.CheckedAt = rec.Latest, rec.CheckedAt
	res.Newer = newer(running, res.Latest)
	return res
}

// fetch asks GitHub, on a short leash.
//
// **The repository is public, so this needs no credential** - which is what
// makes the check possible at all. It was not, and a version check that needed
// a token would have been one more thing to set up before it could tell you
// anything.
func fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", latestURL, resp.Status)
	}
	var out struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	return strings.TrimPrefix(out.TagName, "v"), nil
}

func load(path string) (Record, error) {
	var r Record
	body, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(body, &r)
}

func save(path string, r Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

// newer reports whether latest is a later release than running.
//
// **It answers false whenever it is not sure**, and that default is the whole
// of its design. A wrong "you are behind" is acted on - somebody reinstalls, or
// stops trusting the message - while a missed one costs a delay until the next
// check. So a version it cannot parse, a development build, and a running
// binary ahead of the last release all report nothing.
//
// A development build is the case that would otherwise nag every day: `go
// build` stamps a pseudo-version like 0.1.3-0.20260922031239-8e73d59+dirty,
// which is AHEAD of the 0.1.2 that is published, and comparing only the numbers
// would announce an upgrade that goes backwards.
func newer(running, latest string) bool {
	if latest == "" || running == "" {
		return false
	}
	if strings.ContainsAny(running, "-+") {
		return false // a development build; see above
	}
	r, ok := parse(running)
	if !ok {
		return false
	}
	l, ok := parse(latest)
	if !ok {
		return false
	}
	for i := range r {
		if l[i] != r[i] {
			return l[i] > r[i]
		}
	}
	return false
}

// parse reads major.minor.patch, and says so when it cannot.
func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n := 0
		if p == "" {
			return out, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return out, false
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out, true
}
