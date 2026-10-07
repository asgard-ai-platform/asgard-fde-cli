package cli

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// TestWriteObjectStatus covers the distinction the flag exists for: an object
// that reports something, and one whose kind has nothing to report. Printing
// both as blank is what sends somebody looking for a problem that cannot exist.
func TestWriteObjectStatus(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string
		not  []string
	}{{
		// A Syncer declares a status schema.
		name: "reports",
		yaml: `apiVersion: asgard-ai.com/v1alpha1
kind: Syncer
metadata:
  name: sy-skills
spec:
  suspend: false
status:
  syncState: Succeeded
  lastSuccessfulTimestamp: "2026-09-08T10:11:12Z"
  conditions:
    - type: Ready
      status: "True"
`,
		want: []string{"syncState: Succeeded", "lastSuccessfulTimestamp", "type: Ready"},
		not:  []string{"none reported", "unreadable"},
	}, {
		// A DataConnector declares no status properties at all, and it is
		// half of the mimir-dashboard shape, which is the case the issue was
		// filed from. A SemanticLayer was the example here and does declare
		// one.
		name: "nothing to report",
		yaml: `apiVersion: asgard-ai.com/v1alpha1
kind: DataConnector
metadata:
  name: dc-uof
spec:
  dataConnectorClass: postgres
`,
		want: []string{"status: none reported"},
		not:  []string{"unreadable"},
	}, {
		// An empty status block is the same answer as no block: the object has
		// told us nothing, and saying so beats printing two blank lines.
		name: "empty block",
		yaml: "kind: Agent\nstatus: {}\n",
		want: []string{"status: none reported"},
	}, {
		name: "unparseable",
		yaml: "kind: Agent\n\tstatus: broken\n",
		want: []string{"status: unreadable"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			writeObjectStatus(&b, &platform.LiveObject{Yaml: c.yaml})
			got := b.String()
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in:\n%s", n, got)
				}
			}
		})
	}
}
