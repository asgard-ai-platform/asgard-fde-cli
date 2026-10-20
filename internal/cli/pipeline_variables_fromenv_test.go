package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --from-env is the recipe that works on every operating system: the value
// comes out of .env exactly as local-env wrote it, with no shell in between to
// re-encode it or add a line ending.
func TestFromEnvVariableValue(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".asgard-pipeline.yaml", "releases: []\n")
	write(".env", "# Intercom\r\nINTERCOM_ACCESS_TOKEN=dG9rOjE2=\r\nEMPTY_ONE=\r\nQUOTED=\"a b\"\r\n")
	t.Chdir(root)
	cmd := &cobra.Command{}

	got, err := readVariableValue(cmd, []string{"intercom_access_token"}, kindSecret, "", "INTERCOM_ACCESS_TOKEN", false)
	if err != nil || got != "dG9rOjE2=" {
		t.Fatalf("value = %q, %v; want the token with no CR", got, err)
	}
	if got, _ := readVariableValue(cmd, []string{"k"}, kindChartValue, "", "QUOTED", false); got != "a b" {
		t.Errorf("quoted value = %q, want a b", got)
	}

	for name, tc := range map[string]struct {
		args     []string
		fromFile string
		key      string
		random   bool
		want     string
	}{
		"a placeholder":   {[]string{"k"}, "", "EMPTY_ONE", false, "has no value yet"},
		"a missing key":   {[]string{"k"}, "", "NOPE", false, "is not in"},
		"and an argument": {[]string{"k", "v"}, "", "INTERCOM_ACCESS_TOKEN", false, "more than one way"},
		"and --from-file": {[]string{"k"}, "-", "INTERCOM_ACCESS_TOKEN", false, "more than one way"},
		"and --random":    {[]string{"k"}, "", "INTERCOM_ACCESS_TOKEN", true, "more than one way"},
	} {
		_, err := readVariableValue(cmd, tc.args, kindSecret, tc.fromFile, tc.key, tc.random)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to say %q", name, err, tc.want)
		}
	}
}
