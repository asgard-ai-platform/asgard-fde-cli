package selfupdate

// Replacing the running binary with the newest release.
//
// **The order is what makes this safe, not the download.** The new binary is
// written beside the old one, run there, and only renamed over it once it has
// answered - so a build that macOS kills, a truncated download or an archive
// with nothing in it leaves the working binary exactly where it was. A scheme
// that replaces first and discovers the problem afterwards has already taken
// the tool away from whoever was about to use it.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// downloadBase is the release whose assets carry no version in their names, so
// one URL keeps working across releases. `.github/workflows/release.yml`
// publishes those copies and `install.sh` takes the same ones.
const downloadBase = "https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download"

// firstRunLeash is how long the new binary gets to answer before this gives up
// on it.
//
// **It is minutes rather than seconds because of Gatekeeper.** The releases are
// ad-hoc signed and not notarized, so macOS scans the first execution of a
// newly written binary: the same asset has been seen to run at once, to stall
// for minutes, and to be killed outright. Waiting is the whole point of running
// it here - the alternative is that the stall happens to whoever runs the tool
// next, in front of a customer.
const firstRunLeash = 3 * time.Minute

// AssetName is the archive this platform takes, with no version in it.
//
// **One asset serves both Macs.** The release carries a universal binary, so
// there is nothing for the person updating to know about Intel against Apple
// silicon - and getting that choice wrong gives an exec format error rather
// than a message.
func AssetName(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "asgard-cli_darwin_all.tar.gz"
	case "windows":
		return fmt.Sprintf("asgard-cli_windows_%s.zip", goarch)
	default:
		return fmt.Sprintf("asgard-cli_%s_%s.tar.gz", goos, goarch)
	}
}

// Apply replaces the binary at target with the published release named by
// latest, and reports each step to out.
//
// It returns an error having changed nothing, or nil having replaced the file.
// There is deliberately no third outcome: a half-applied update is the state
// worth engineering away.
func Apply(ctx context.Context, latest, target string, out io.Writer) error {
	dir := filepath.Dir(target)
	asset := AssetName(runtime.GOOS, runtime.GOARCH)

	// **The staging directory is beside the target, not in the system temp.**
	// The last step is a rename, a rename is only atomic within one
	// filesystem, and /tmp is a different one often enough that copying would
	// become the fallback - which is the non-atomic write this avoids.
	stage, err := os.MkdirTemp(dir, ".asgard-cli-update-")
	if err != nil {
		return fmt.Errorf("nothing was changed: %s is not writable (%w)", dir, err)
	}
	defer os.RemoveAll(stage)

	fmt.Fprintf(out, "Downloading %s...\n", asset)
	archive := filepath.Join(stage, asset)
	sum, err := download(ctx, downloadBase+"/"+asset, archive)
	if err != nil {
		return err
	}

	// **Matched by hash rather than by name.** The release carries the same
	// bytes twice - once with a version in the filename and once without, so
	// that the URL above needs no version - and only the versioned name is in
	// checksums.txt. `install.sh` verifies the same way for the same reason.
	sums := filepath.Join(stage, "checksums.txt")
	if _, err := download(ctx, downloadBase+"/checksums.txt", sums); err != nil {
		return err
	}
	if err := verify(sums, sum); err != nil {
		return err
	}
	fmt.Fprintf(out, "  checksum ok\n")

	staged := filepath.Join(stage, "asgard-cli"+exeSuffix())
	if err := extract(archive, staged); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return err
	}

	// **Run it before anything is replaced.** This is the step that makes the
	// rest safe, and on macOS it is also where the Gatekeeper scan is spent:
	// whatever it costs, it costs here, to somebody who is waiting for an
	// update rather than to whoever needs the tool next.
	fmt.Fprintf(out, "Checking it runs (macOS scans a new binary once, which can take a minute)...\n")
	got, err := ran(ctx, staged)
	if err != nil {
		return fmt.Errorf("the downloaded binary did not run, so %s was left alone: %w", target, err)
	}
	if latest != "" && strings.TrimPrefix(got, "v") != strings.TrimPrefix(latest, "v") {
		return fmt.Errorf("the downloaded binary reports %s and the release is %s, so %s was left alone",
			got, latest, target)
	}
	fmt.Fprintf(out, "  it reports %s\n", got)

	if err := replace(staged, target); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nReplaced %s\n", target)
	return nil
}

// download writes one URL to path and returns the SHA-256 of what arrived.
//
// The digest comes from the bytes as they are written rather than from reading
// the file back, so what is verified is what landed.
func download(ctx context.Context, url, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get %s: %s", url, resp.Status)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return "", fmt.Errorf("get %s: %w", url, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verify requires the digest to appear in a goreleaser checksums file.
func verify(sumsPath, want string) error {
	body, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	for line := range strings.Lines(string(body)) {
		digest, _, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && digest == want {
			return nil
		}
	}
	return fmt.Errorf("checksum mismatch: %s is not in this release's checksums.txt, so nothing was changed.\n"+
		"Report this - it should not happen", want)
}

// extract pulls the asgard-cli binary out of the downloaded archive.
func extract(archive, dest string) error {
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(archive, dest)
	}
	return extractTarGz(archive, dest)
}

func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(archive), err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Base(archive), err)
		}
		if filepath.Base(hdr.Name) != "asgard-cli"+exeSuffix() {
			continue
		}
		return write(dest, tr)
	}
	return fmt.Errorf("%s carries no asgard-cli, so nothing was changed", filepath.Base(archive))
}

func extractZip(archive, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(archive), err)
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) != "asgard-cli"+exeSuffix() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return write(dest, rc)
	}
	return fmt.Errorf("%s carries no asgard-cli, so nothing was changed", filepath.Base(archive))
}

func write(dest string, from io.Reader) error {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, from); err != nil {
		return err
	}
	return f.Close()
}

// ran executes the staged binary and returns the version it reports.
//
// `version --json` rather than the plain form: a parsed field is the answer,
// where a printed line would have to be recovered from formatting this program
// also owns and could change.
func ran(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, firstRunLeash)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, "version", "--json")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// The new binary must not ask about its own updates while it is being
	// installed, and must not stamp the record on this run.
	cmd.Env = append(os.Environ(), EnvDisable+"=1")

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return "", fmt.Errorf("its `version --json` was not JSON: %w", err)
	}
	if info.Version == "" {
		return "", fmt.Errorf("its `version --json` carried no version")
	}
	return info.Version, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
