package cli

import (
	"path/filepath"
	"testing"
)

func TestRecordedPath(t *testing.T) {
	root := filepath.FromSlash("/work/acme-test-asgard")
	cases := map[string]string{
		"/work/asgard-ext-acme":              "../asgard-ext-acme",
		"/work/acme-test-asgard/vendor/ext":  "vendor/ext",
		"/Users/someone/src/asgard-ext-acme": "/Users/someone/src/asgard-ext-acme",
	}
	for src, want := range cases {
		if got := recordedPath(root, filepath.FromSlash(src)); got != filepath.ToSlash(filepath.FromSlash(want)) && got != filepath.FromSlash(want) {
			t.Errorf("recordedPath(%s) = %s, want %s", src, got, want)
		}
	}
}
