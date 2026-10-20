package gate

import (
	"strings"
	"testing"
)

// agentDoc is an Agent with one semanticLayers list and nothing else, which is
// all these cases turn on.
func agentDoc(name string, layers ...string) Doc {
	var list []any
	for _, l := range layers {
		list = append(list, map[string]any{"name": l, "allowQuery": true})
	}
	return Doc{Kind: "Agent", Name: name, Spec: map[string]any{
		"managed": map[string]any{"semanticLayers": list},
	}}
}

// R13 is what is left of R1, and the line it draws is the one the old rule did
// not: a layer bound by two Agents is a shape the platform deploys and a chart
// set can mean, and the same layer listed twice by one Agent is a copied line
// nobody edited.
//
// The middle case is the one that regressed a whole chart set. It failed 121
// times across 11 of 12 charts, all deliberate, which is what took the rule out.
func TestLayerSharingAndDuplicates(t *testing.T) {
	for _, c := range []struct {
		name   string
		docs   []Doc
		fails  bool
		expect string
	}{
		{
			name:  "one agent, one layer",
			docs:  []Doc{agentDoc("ag-procurement", "sl-erp")},
			fails: false,
		},
		{
			name: "two agents reading the same system",
			docs: []Doc{
				agentDoc("ag-procurement", "sl-erp"),
				agentDoc("ag-finance", "sl-erp"),
			},
			fails: false,
		},
		{
			name:  "one agent across several systems",
			docs:  []Doc{agentDoc("ag-mgmt-insight", "sl-erp", "sl-wms", "sl-crm")},
			fails: false,
		},
		{
			name:   "one agent listing one layer twice",
			docs:   []Doc{agentDoc("ag-procurement", "sl-erp", "sl-erp")},
			fails:  true,
			expect: "R13 ag-procurement: lists sl-erp twice",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := AgentSplit(c.docs, Options{})
			if res.OK() == c.fails {
				t.Fatalf("OK()=%v, want %v; problems: %q", res.OK(), !c.fails, res.Problems)
			}
			if c.expect == "" {
				return
			}
			found := false
			for _, p := range res.Problems {
				if strings.Contains(p, c.expect) {
					found = true
				}
			}
			if !found {
				t.Fatalf("no problem containing %q; got %q", c.expect, res.Problems)
			}
		})
	}
}

// The summary is read to see how wide a chart's read path is, so a layer four
// role agents share is one layer there, not four.
func TestSummaryNamesEachLayerOnce(t *testing.T) {
	res := AgentSplit([]Doc{
		agentDoc("ag-procurement", "sl-erp"),
		agentDoc("ag-finance", "sl-erp", "sl-crm"),
		agentDoc("ag-planning", "sl-erp"),
	}, Options{})
	if got, want := res.Summary, "3 agent(s) (0 published), layers: sl-crm, sl-erp"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

// R1b counts every capability source the Agent CRD has. Mounted files are one:
// an Agent that reads a Syncer's copy of a drive has sourceSetMounts and
// nothing else, which is the brake the Google Drive Extension recommends.
func TestNoCapabilityCountsMountedFiles(t *testing.T) {
	mounted := Doc{Kind: "Agent", Name: "ag-purchasing", Spec: map[string]any{
		"managed": map[string]any{"sourceSetMounts": []any{
			map[string]any{"sourceSetName": "ss-drive", "mountPath": "/mnt/drive", "readOnly": true},
		}},
	}}
	if res := AgentSplit([]Doc{mounted}, Options{}); !res.OK() {
		t.Fatalf("an Agent with only sourceSetMounts was refused: %q", res.Problems)
	}

	empty := Doc{Kind: "Agent", Name: "ag-empty", Spec: map[string]any{"managed": map[string]any{}}}
	res := AgentSplit([]Doc{empty}, Options{})
	found := false
	for _, p := range res.Problems {
		if strings.Contains(p, "R1b ag-empty") && strings.Contains(p, "no sourceSetMounts") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an Agent with nothing was not refused by R1b: %q", res.Problems)
	}
}
