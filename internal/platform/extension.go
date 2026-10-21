package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The Extension routes serve a Stack's index of FDE Extensions, which the
// platform relays from its registry. An Extension belongs to the whole Stack
// rather than to a workspace, so none of these routes takes the workspace
// header.
//
// {extension} is an Extension's full path, slashes and all
// ("github.com/asgard-ai-platform/asgard-ext-google-drive"), or its short id.
// The paths have the shape of a Go module proxy.

// extensionInfoTimeout bounds a call that can make the registry fetch a
// repository first: a version it has not seen yet, or an Extension it has never
// fetched. The platform waits up to a minute for the registry, so this waits
// longer than the platform does.
const extensionInfoTimeout = 2 * time.Minute

// ExtensionQuery filters a search. Empty fields match everything.
type ExtensionQuery struct {
	Query    string
	Usage    string
	Auth     string
	Category string
}

// ExtensionEntry is one Extension in a search: its identity and its latest
// version.
type ExtensionEntry struct {
	Path     string             `json:"path"`
	Source   string             `json:"source"`
	Latest   *ExtensionVersion  `json:"latest"`
	Metadata ExtensionMetadata  `json:"metadata"`
	Logo     *ExtensionLogoInfo `json:"logo,omitempty"`
}

// ExtensionVersion is one version the registry offers.
type ExtensionVersion struct {
	Path        string     `json:"path"`
	Version     string     `json:"version"`
	Commit      string     `json:"commit"`
	Prerelease  bool       `json:"prerelease"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
}

// ExtensionMetadata is the README frontmatter of one version, as the registry
// read it. The registry does not validate content, so any field may be empty.
type ExtensionMetadata struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Summary  string            `json:"summary"`
	Kind     string            `json:"kind"`
	System   ExtensionSystem   `json:"system"`
	Logo     string            `json:"logo"`
	Routes   []ExtensionRoute  `json:"routes"`
	Requires ExtensionRequires `json:"requires"`
	Listing  string            `json:"listing"`
	Source   string            `json:"source"`
	Owners   []string          `json:"owners"`
	Tags     []string          `json:"tags"`
}

type ExtensionSystem struct {
	Vendor   string `json:"vendor"`
	Product  string `json:"product"`
	Category string `json:"category"`
	Docs     string `json:"docs"`
}

type ExtensionRoute struct {
	ID       string   `json:"id"`
	Usage    string   `json:"usage"`
	Auth     string   `json:"auth"`
	Status   string   `json:"status"`
	Produces []string `json:"produces"`
	Brakes   []string `json:"brakes"`
	Note     string   `json:"note"`
}

type ExtensionRequires struct {
	AsgardCLI  string   `json:"asgard_cli"`
	Skills     []string `json:"skills"`
	Extensions []string `json:"extensions"`
	Platform   []string `json:"platform"`
}

type ExtensionLogoInfo struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Sha256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

// ExtensionRefusedVersion is a semver tag the registry does not offer, and
// why, in words for the author.
type ExtensionRefusedVersion struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Reason  string `json:"reason"`
}

// ExtensionMovedTag is a tag moved to another commit after the registry first
// saw it. The first commit keeps being served.
type ExtensionMovedTag struct {
	Version         string     `json:"version"`
	FirstSeenCommit string     `json:"first_seen_commit"`
	CurrentCommit   string     `json:"current_commit"`
	DetectedAt      *time.Time `json:"detected_at,omitempty"`
}

// ExtensionVersionList is every version of one Extension: offered (oldest
// first), refused, and moved.
type ExtensionVersionList struct {
	Path            string                    `json:"path"`
	Versions        []ExtensionVersion        `json:"versions"`
	RefusedVersions []ExtensionRefusedVersion `json:"refused_versions"`
	MovedTags       []ExtensionMovedTag       `json:"moved_tags"`
}

// ExtensionVersionInfo is one version in full.
type ExtensionVersionInfo struct {
	Version       ExtensionVersion   `json:"version"`
	Metadata      ExtensionMetadata  `json:"metadata"`
	Frontmatter   map[string]any     `json:"frontmatter"`
	Readme        string             `json:"readme"`
	Logo          *ExtensionLogoInfo `json:"logo,omitempty"`
	ArchiveSha256 string             `json:"archive_sha256"`
	ArchiveSize   int64              `json:"archive_size"`
	Moved         *ExtensionMovedTag `json:"moved,omitempty"`
	LogoProblem   string             `json:"logo_problem,omitempty"`
}

// ExtensionArchive describes a downloaded archive, from the headers the
// platform sent with it and the bytes that arrived.
type ExtensionArchive struct {
	Path    string
	Version string
	Commit  string
	// Sha256 is the hex digest of the bytes received, which matched the one
	// the platform sent.
	Sha256 string
	Size   int64
}

// extensionPath is the route of one Extension: each segment of its path is
// escaped and the slashes between them are kept.
func extensionPath(extension string) string {
	segments := strings.Split(strings.Trim(extension, "/"), "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return "/v1/extensions/" + strings.Join(segments, "/")
}

// SearchExtensions searches the index. Unlisted Extensions are included.
func (c *Client) SearchExtensions(ctx context.Context, q ExtensionQuery) ([]ExtensionEntry, error) {
	query := url.Values{}
	for k, v := range map[string]string{"q": q.Query, "usage": q.Usage, "auth": q.Auth, "category": q.Category} {
		if v != "" {
			query.Set(k, v)
		}
	}
	var out []ExtensionEntry
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/extensions", query: query, out: &out, noWorkspace: true, timeout: extensionInfoTimeout})
	return out, err
}

// ExtensionVersions lists an Extension's versions, with the ones refused and
// why.
func (c *Client) ExtensionVersions(ctx context.Context, extension string) (*ExtensionVersionList, error) {
	var out ExtensionVersionList
	err := c.do(ctx, request{method: http.MethodGet, path: extensionPath(extension) + "/@v/list", out: &out, noWorkspace: true, timeout: extensionInfoTimeout})
	return &out, err
}

// LatestExtensionVersion is the highest release, or the highest pre-release
// when there is no release.
func (c *Client) LatestExtensionVersion(ctx context.Context, extension string) (*ExtensionVersion, error) {
	var out ExtensionVersion
	err := c.do(ctx, request{method: http.MethodGet, path: extensionPath(extension) + "/@latest", out: &out, noWorkspace: true, timeout: extensionInfoTimeout})
	return &out, err
}

// ExtensionVersionInfo reads one version. A version the registry has not seen
// yet is fetched on the spot, so this is also how an author confirms a new tag
// was taken in.
func (c *Client) ExtensionVersionInfo(ctx context.Context, extension, version string) (*ExtensionVersionInfo, error) {
	var out ExtensionVersionInfo
	err := c.do(ctx, request{method: http.MethodGet, path: extensionPath(extension) + "/@v/" + url.PathEscape(version) + ".info", out: &out, noWorkspace: true, timeout: extensionInfoTimeout})
	return &out, err
}

// DownloadExtension writes one version's archive (a tar.gz) to w. It fails
// with ErrChecksumMismatch when the bytes do not hash to what the platform sent
// in X-Extension-Sha256 - in which case w has already received them, and the
// caller is the one to discard them. A download cut off midway is one of
// those.
func (c *Client) DownloadExtension(ctx context.Context, extension, version string, w io.Writer) (*ExtensionArchive, error) {
	path := extensionPath(extension) + "/@v/" + url.PathEscape(version) + ".tar.gz"
	// The registry may fetch the repository before the first byte, and a slow
	// link can outlast any fixed bound after it; the bytes are bounded by what
	// an Extension ships.
	resp, err := c.doRaw(ctx, rawRequest{method: http.MethodGet, path: path, accept: "application/gzip", noTimeout: true, noWorkspace: true})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	got := hex.EncodeToString(h.Sum(nil))
	want := resp.Header.Get("X-Extension-Sha256")
	if err != nil {
		return nil, fmt.Errorf("%w: the download of %s %s stopped after %d bytes: %v", ErrChecksumMismatch, extension, version, n, err)
	}
	if want == "" {
		return nil, fmt.Errorf("the platform sent %s %s without X-Extension-Sha256, so the download cannot be checked", extension, version)
	}
	if !strings.EqualFold(want, got) {
		return nil, fmt.Errorf("%w: %s %s hashes to %s and the platform sent %s", ErrChecksumMismatch, extension, version, got, want)
	}
	return &ExtensionArchive{
		Path:    resp.Header.Get("X-Extension-Path"),
		Version: resp.Header.Get("X-Extension-Version"),
		Commit:  resp.Header.Get("X-Extension-Commit"),
		Sha256:  got,
		Size:    n,
	}, nil
}

// extensionErrorText words a refusal from the Extension routes. Their 404 and
// 400 carry the registry's own sentence - "no Extension asgard-ext-foo on this
// Stack", or why a version is refused - which says more than the route does.
func extensionErrorText(e *APIError, msg string) (string, bool) {
	if !strings.HasPrefix(e.Path, "/v1/extensions") {
		return "", false
	}
	switch e.Status {
	case http.StatusNotFound, http.StatusBadRequest:
		return fmt.Sprintf("%s (%d)", msg, e.Status), true
	}
	return "", false
}
