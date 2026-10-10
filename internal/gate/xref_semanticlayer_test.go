package gate

import (
	"strings"
	"testing"
)

// layer builds a SemanticLayer whose cubes are given as name -> dimension names.
func layer(cubes map[string][]string, pk map[string][]string, joins ...map[string]any) Doc {
	var cs []any
	for name, dims := range cubes {
		var ds []any
		for _, d := range dims {
			ds = append(ds, map[string]any{"name": d})
		}
		c := map[string]any{"name": name, "sqlTable": name, "dimensions": ds}
		if keys, ok := pk[name]; ok {
			var ks []any
			for _, k := range keys {
				ks = append(ks, k)
			}
			c["primaryKeyDimensions"] = ks
		}
		cs = append(cs, c)
	}
	var js []any
	for _, j := range joins {
		js = append(js, j)
	}
	return Doc{Kind: "SemanticLayer", Name: "sl-x", Spec: map[string]any{"cubes": cs, "joins": js}}
}

func join(name, fromCube, fromDim, toCube, toDim string) map[string]any {
	return map[string]any{
		"name":         name,
		"relationship": "many_to_one",
		"from":         map[string]any{"cube": fromCube, "dimensions": []any{fromDim}},
		"to":           map[string]any{"cube": toCube, "dimensions": []any{toDim}},
	}
}

// A join names a cube by cubes[].name, and the CRD does not check that the
// name resolves. Neither does a dry run, so this is the only place it is seen.
func TestSemanticLayerJoinToUndeclaredCube(t *testing.T) {
	sl := layer(map[string][]string{"orders": {"id", "customer_id"}}, nil,
		join("orders_customers", "orders", "customer_id", "customers", "id"))

	got := joined(Xref([]Doc{sl}, Options{}))
	if !strings.Contains(got, "joins[orders_customers].to.cube") || !strings.Contains(got, `"customers"`) {
		t.Errorf("a join to a cube the layer does not declare went unreported:\n%s", got)
	}
	if strings.Contains(got, "joins[orders_customers].from") {
		t.Errorf("the side of the join that resolves was reported:\n%s", got)
	}
}

// Both cubes exist and the dimension does not: a cube-level check passes this,
// so it needs each cube's own dimension list.
func TestSemanticLayerJoinToUndeclaredDimension(t *testing.T) {
	sl := layer(map[string][]string{
		"orders":    {"id", "customer_id"},
		"customers": {"id"},
	}, nil, join("orders_customers", "orders", "cust_id", "customers", "id"))

	got := joined(Xref([]Doc{sl}, Options{}))
	if !strings.Contains(got, "joins[orders_customers].from.dimensions") || !strings.Contains(got, `"cust_id"`) {
		t.Errorf("a join naming a dimension its cube does not declare went unreported:\n%s", got)
	}
}

func TestSemanticLayerPrimaryKeyDimensionIsResolved(t *testing.T) {
	sl := layer(map[string][]string{"orders": {"order_no"}}, map[string][]string{"orders": {"id"}})

	got := joined(Xref([]Doc{sl}, Options{}))
	if !strings.Contains(got, "cubes[orders].primaryKeyDimensions") || !strings.Contains(got, `"id"`) {
		t.Errorf("a primaryKeyDimensions entry naming no dimension went unreported:\n%s", got)
	}
}

// A layer whose names all resolve must produce nothing from this check, or it
// gets turned off.
func TestSemanticLayerResolvedNamesAreNotReported(t *testing.T) {
	sl := layer(map[string][]string{
		"orders":    {"id", "customer_id"},
		"customers": {"id"},
	}, map[string][]string{"orders": {"id"}, "customers": {"id"}},
		join("orders_customers", "orders", "customer_id", "customers", "id"))

	got := joined(Xref([]Doc{sl}, Options{}))
	for _, field := range []string{"joins[", "primaryKeyDimensions"} {
		if strings.Contains(got, field) {
			t.Errorf("a layer whose names all resolve was reported:\n%s", got)
		}
	}
}
