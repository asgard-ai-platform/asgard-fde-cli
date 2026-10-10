package platform

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
)

// Volume is the files of a SourceSet, or of the SourceSet a SkillSet mounts.
// The platform serves both through the same operations under two prefixes,
// and the SkillSet's is the only way to reach a SkillSet's files: its
// SourceSet is hidden from the drive routes.
type Volume struct {
	base string
}

// SourceSetVolume is a SourceSet's files. The id is the CR's name.
func SourceSetVolume(sourceSet string) Volume {
	return Volume{base: "/v1/source-set/" + url.PathEscape(sourceSet) + "/volume"}
}

// SkillSetVolume is the files of the SourceSet a SkillSet mounts.
func SkillSetVolume(skillSet string) Volume {
	return Volume{base: "/v1/skill-set/" + url.PathEscape(skillSet) + "/volume"}
}

// A path in a volume is relative to its root: no leading slash, no "." or
// ".." segment, no empty segment. The root itself is the empty path, which
// only a listing takes. The platform refuses anything else with a 400.

// VolumeEntry is one file or folder in a listing.
type VolumeEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"isDir"`
	SizeBytes int64  `json:"sizeBytes"`
	MtimeUnix int64  `json:"mtimeUnix"`
	// Mode is the Unix permission bits, 420 for 0644.
	Mode uint32 `json:"mode"`
}

// VolumeStat is what a path is. A path that does not exist is answered with
// Exists false rather than a 404.
type VolumeStat struct {
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"isDir"`
	SizeBytes int64  `json:"sizeBytes"`
	MtimeUnix int64  `json:"mtimeUnix"`
	Etag      string `json:"etag"`
	Mode      uint32 `json:"mode"`
}

// VolumePageSize is the most entries one listing call returns.
const VolumePageSize = 1000

// ListVolume returns every entry of a folder, reading as many pages as it
// takes. The empty path is the root.
func (c *Client) ListVolume(ctx context.Context, project string, v Volume, path string) ([]VolumeEntry, error) {
	var all []VolumeEntry
	for page := int64(0); ; page++ {
		var out struct {
			Entries []VolumeEntry `json:"entries"`
			Paging  *Paging       `json:"paging"`
		}
		q := url.Values{}
		q.Set("path", path)
		q.Set("page", strconv.FormatInt(page, 10))
		q.Set("page_size", strconv.Itoa(VolumePageSize))
		if err := c.do(ctx, request{method: http.MethodGet, path: v.base + "/list", query: q, project: project, out: &out}); err != nil {
			return nil, err
		}
		all = append(all, out.Entries...)
		if len(out.Entries) < VolumePageSize || out.Paging == nil || int64(len(all)) >= out.Paging.Total {
			return all, nil
		}
	}
}

// StatVolume says what a path is.
func (c *Client) StatVolume(ctx context.Context, project string, v Volume, path string) (*VolumeStat, error) {
	var out VolumeStat
	err := c.do(ctx, request{method: http.MethodGet, path: v.base + "/stat", query: url.Values{"path": {path}}, project: project, out: &out})
	return &out, err
}

// ReadVolumeFile copies a file's bytes to w. offset and limit are in bytes,
// and a limit of 0 reads to the end. It reports the file's whole size and
// whether bytes remain past what was read.
func (c *Client) ReadVolumeFile(ctx context.Context, project string, v Volume, path string, offset, limit int64, w io.Writer) (total int64, truncated bool, err error) {
	q := url.Values{"path": {path}}
	if offset > 0 {
		q.Set("offset_bytes", strconv.FormatInt(offset, 10))
	}
	if limit > 0 {
		q.Set("limit_bytes", strconv.FormatInt(limit, 10))
	}
	resp, err := c.doRaw(ctx, rawRequest{
		method: http.MethodGet, path: v.base + "/file", query: q, project: project,
		accept: "application/octet-stream", noTimeout: true,
	})
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		return 0, false, fmt.Errorf("read %s: %w", path, err)
	}
	total, _ = strconv.ParseInt(resp.Header.Get("X-Total-Bytes"), 10, 64)
	truncated, _ = strconv.ParseBool(resp.Header.Get("X-Truncated"))
	return total, truncated, nil
}

// VolumeWrite is how a file is written. Mode 0 leaves the platform's default;
// CreateOnly refuses a path that already exists, with a 409.
type VolumeWrite struct {
	Mode       uint32
	CreateOnly bool
}

// WriteVolumeFile writes r to path, replacing what is there unless
// CreateOnly, and creating the folders above it. It returns the bytes written.
func (c *Client) WriteVolumeFile(ctx context.Context, project string, v Volume, path, filename string, r io.Reader, opt VolumeWrite) (int64, error) {
	q := url.Values{"path": {path}}
	if opt.Mode != 0 {
		q.Set("mode", strconv.FormatUint(uint64(opt.Mode), 10))
	}
	if opt.CreateOnly {
		q.Set("create_only", "true")
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		err := func() error {
			part, err := mw.CreateFormFile("file", filename)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, r); err != nil {
				return err
			}
			return mw.Close()
		}()
		pw.CloseWithError(err)
	}()
	resp, err := c.doRaw(ctx, rawRequest{
		method: http.MethodPut, path: v.base + "/file", query: q, project: project,
		body: pr, contentType: mw.FormDataContentType(), sideEffect: true,
		// An upload can outlast the per-call timeout on a slow link.
		noTimeout: true,
	})
	if err != nil {
		_ = pr.CloseWithError(err)
		return 0, err
	}
	var out struct {
		BytesWritten int64 `json:"bytesWritten"`
	}
	if err := decodeEnvelope(resp, &out); err != nil {
		return 0, err
	}
	return out.BytesWritten, nil
}

// MakeVolumeDir creates a folder, and the folders above it.
func (c *Client) MakeVolumeDir(ctx context.Context, project string, v Volume, path string) error {
	return c.do(ctx, request{method: http.MethodPost, path: v.base + "/mkdir", query: url.Values{"path": {path}}, project: project, sideEffect: true})
}

// RemoveVolumePath deletes a file or an empty folder, or with recursive a
// folder and everything in it. The root cannot be removed.
func (c *Client) RemoveVolumePath(ctx context.Context, project string, v Volume, path string, recursive bool) error {
	route := "/item"
	if recursive {
		route = "/all"
	}
	return c.do(ctx, request{method: http.MethodDelete, path: v.base + route, query: url.Values{"path": {path}}, project: project, sideEffect: true})
}

// CopyVolumePath copies a file or a folder within the volume and returns the
// bytes copied. Without overwrite an existing destination is a 409.
func (c *Client) CopyVolumePath(ctx context.Context, project string, v Volume, src, dst string, overwrite bool) (int64, error) {
	var out struct {
		BytesCopied int64 `json:"bytesCopied"`
	}
	err := c.do(ctx, request{method: http.MethodPost, path: v.base + "/copy", query: srcDst(src, dst, overwrite), project: project, out: &out, sideEffect: true})
	return out.BytesCopied, err
}

// MoveVolumePath moves or renames a file or a folder within the volume.
func (c *Client) MoveVolumePath(ctx context.Context, project string, v Volume, src, dst string, overwrite bool) error {
	return c.do(ctx, request{method: http.MethodPost, path: v.base + "/move", query: srcDst(src, dst, overwrite), project: project, sideEffect: true})
}

func srcDst(src, dst string, overwrite bool) url.Values {
	q := url.Values{"src": {src}, "dst": {dst}}
	if overwrite {
		q.Set("overwrite", "true")
	}
	return q
}
