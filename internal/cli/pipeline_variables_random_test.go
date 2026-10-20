package cli

import (
	"encoding/hex"
	"testing"

	"github.com/spf13/cobra"
)

// **--random is the recipe that works on every operating system.** The value
// is 64 hex characters with no newline - what `openssl rand -hex 32 | tr -d
// '\n'` was for, without either tool - and two calls never agree.
func TestRandomVariableValue(t *testing.T) {
	cmd := &cobra.Command{}

	a, err := readVariableValue(cmd, []string{"asgard_resource_api_key"}, kindSecret, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 {
		t.Errorf("len = %d, want 64 hex characters", len(a))
	}
	if _, err := hex.DecodeString(a); err != nil {
		t.Errorf("not hex: %q", a)
	}
	b, _ := readVariableValue(cmd, []string{"asgard_resource_api_key"}, kindSecret, "", "", true)
	if a == b {
		t.Error("two generated values are the same")
	}

	// A generated value nobody knows is only a credential, and a second
	// source of the value means one of them is being ignored.
	for name, tc := range map[string]struct {
		args     []string
		kind     string
		fromFile string
	}{
		"not a secret":    {[]string{"k"}, kindConfig, ""},
		"and an argument": {[]string{"k", "v"}, kindSecret, ""},
		"and --from-file": {[]string{"k"}, kindSecret, "-"},
		"chart value":     {[]string{"k"}, kindChartValue, ""},
	} {
		if _, err := readVariableValue(cmd, tc.args, tc.kind, tc.fromFile, "", true); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
