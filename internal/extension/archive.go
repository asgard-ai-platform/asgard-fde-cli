package extension

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// maxArchiveBytes bounds what an archive may expand to. An Extension is text
// and a few small assets; the bound only stops an archive that would fill the
// disk.
const maxArchiveBytes = 256 << 20

// FromArchive reads an Extension's archive - the tar.gz the platform serves -
// into the files a copy writes, keyed like Collect's. It takes only the
// Shipped entries and reports the top-level names it left out, so a registry
// whose list has grown does not bring anything this build does not know into
// the repository.
//
// It refuses an archive holding anything but directories and regular files, or
// a name that would land outside the Extension's directory: either would point
// somewhere other than where the copy is written.
func FromArchive(r io.Reader) (files map[string]File, skipped []string, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("the archive is not gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files = map[string]File{}
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read the archive: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || path.IsAbs(name) || slices.Contains(strings.Split(name, "/"), "..") {
			return nil, nil, fmt.Errorf("the archive holds %q, which would land outside the Extension's directory", hdr.Name)
		}
		name = path.Clean(name)
		top, _, _ := strings.Cut(name, "/")
		if !slices.Contains(Shipped, top) {
			if !slices.Contains(skipped, top) {
				skipped = append(skipped, top)
			}
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, nil, fmt.Errorf("the archive holds %s, which is not a regular file; only regular files are copied", name)
		}
		total += hdr.Size
		if total > maxArchiveBytes {
			return nil, nil, fmt.Errorf("the archive expands to more than %d MiB, which no Extension needs", maxArchiveBytes>>20)
		}
		data, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
		if err != nil {
			return nil, nil, fmt.Errorf("read %s from the archive: %w", name, err)
		}
		files[name] = File{Data: data, Exec: hdr.Mode&0o111 != 0}
	}
	return files, skipped, nil
}
