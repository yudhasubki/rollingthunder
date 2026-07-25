package mysql

import (
	"encoding/json"
	"testing"
)

func TestMySQLPlanChildrenSupportsJSONV2QueryPlanEnvelope(t *testing.T) {
	const raw = `{
		"query": "select Name from country where Code like 'A%'",
		"query_plan": {
			"inputs": [
				{
					"operation": "Index range scan on country using PRIMARY",
					"index_name": "PRIMARY",
					"table_name": "country",
					"access_type": "index",
					"schema_name": "world",
					"estimated_rows": 17.0,
					"estimated_total_cost": 3.25
				}
			],
			"condition": "(country.Code like 'A%')",
			"operation": "Filter: (country.Code like 'A%')",
			"access_type": "filter",
			"estimated_rows": 17.0,
			"estimated_total_cost": 3.668778400708174
		},
		"query_type": "select",
		"json_schema_version": "2.0"
	}`
	var document map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}

	roots := mysqlPlanChildren(document, "", "mysql")
	if len(roots) != 1 {
		t.Fatalf("roots = %+v, want one JSON v2 root", roots)
	}
	root := roots[0]
	if root.NodeType != "FILTER" ||
		root.Summary != "Filter: (country.Code like 'A%')" ||
		root.EstimatedRows != 17 ||
		root.TotalCost != 3.668778400708174 {
		t.Fatalf("root = %+v", root)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root children = %+v, want one input", root.Children)
	}
	child := root.Children[0]
	if child.ParentID != root.ID ||
		child.NodeType != "INDEX" ||
		child.Relation != "world.country" ||
		child.EstimatedRows != 17 ||
		child.TotalCost != 3.25 {
		t.Fatalf("child = %+v", child)
	}
	if child.Details["index name"] != "PRIMARY" {
		t.Fatalf("child details = %+v", child.Details)
	}
}

func TestMySQLPlanChildrenSupportsDirectJSONV2Root(t *testing.T) {
	document := map[string]interface{}{
		"operation":            "Rows fetched before execution",
		"access_type":          "rows_fetched_before_execution",
		"estimated_rows":       float64(1),
		"estimated_total_cost": float64(0),
	}

	roots := mysqlPlanChildren(document, "", "mysql")
	if len(roots) != 1 {
		t.Fatalf("roots = %+v, want one JSON v2 root", roots)
	}
	if roots[0].Summary != "Rows fetched before execution" ||
		roots[0].NodeType != "ROWS_FETCHED_BEFORE_EXECUTION" ||
		roots[0].EstimatedRows != 1 {
		t.Fatalf("root = %+v", roots[0])
	}
}

func TestMySQLPlanChildrenStillSupportsJSONV1(t *testing.T) {
	document := map[string]interface{}{
		"query_block": map[string]interface{}{
			"select_id": float64(1),
			"table": map[string]interface{}{
				"table_name":             "orders",
				"access_type":            "ALL",
				"rows_examined_per_scan": float64(12),
			},
		},
	}

	roots := mysqlPlanChildren(document, "", "mysql")
	if len(roots) != 1 || len(roots[0].Children) != 1 {
		t.Fatalf("roots = %+v, want legacy query-block and table nodes", roots)
	}
	table := roots[0].Children[0]
	if table.Relation != "orders" ||
		table.NodeType != "ALL" ||
		table.EstimatedRows != 12 {
		t.Fatalf("table = %+v", table)
	}
}
