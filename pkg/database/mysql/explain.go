package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (m *MySQL) ExplainQuery(
	ctx context.Context,
	query string,
) (database.ExplainPlan, error) {
	return m.ExplainQueryWithArgs(ctx, query, nil)
}

func (m *MySQL) ExplainQueryWithArgs(
	ctx context.Context,
	query string,
	args []interface{},
) (database.ExplainPlan, error) {
	var raw string
	if err := m.conn.GetContext(
		ctx,
		&raw,
		"EXPLAIN FORMAT=JSON "+query,
		args...,
	); err != nil {
		return database.ExplainPlan{}, err
	}

	var document map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return database.ExplainPlan{}, fmt.Errorf("decode MySQL explain plan: %w", err)
	}
	roots := mysqlPlanChildren(document, "", "mysql")
	if len(roots) == 0 {
		return database.ExplainPlan{}, fmt.Errorf("MySQL returned an empty explain plan")
	}
	pretty, _ := json.MarshalIndent(document, "", "  ")
	return database.ExplainPlan{
		Engine:  m.engine,
		Summary: roots[0].Summary,
		Roots:   roots,
		Raw:     string(pretty),
	}, nil
}

func mysqlPlanChildren(
	source map[string]interface{},
	parentID string,
	idPrefix string,
) []database.ExplainPlanNode {
	nodes := make([]database.ExplainPlanNode, 0)
	if table, ok := source["table"].(map[string]interface{}); ok {
		nodes = append(nodes, mysqlTablePlanNode(table, parentID, idPrefix+"-table"))
	}

	for _, key := range []string{
		"query_block",
		"grouping_operation",
		"ordering_operation",
		"duplicates_removal",
		"materialized_from_subquery",
		"query_specifications",
	} {
		child, ok := source[key].(map[string]interface{})
		if !ok {
			continue
		}
		childNodes := mysqlPlanChildren(child, parentID, idPrefix+"-"+key)
		if len(childNodes) == 0 {
			continue
		}
		wrapper := mysqlWrapperNode(key, child, parentID, idPrefix+"-"+key+"-node")
		for index := range childNodes {
			childNodes[index].ParentID = wrapper.ID
		}
		wrapper.Children = childNodes
		nodes = append(nodes, wrapper)
	}

	if nested, ok := source["nested_loop"].([]interface{}); ok {
		wrapper := database.ExplainPlanNode{
			ID:       idPrefix + "-nested-loop",
			ParentID: parentID,
			NodeType: "Nested loop",
			Summary:  "Nested loop",
			Children: make([]database.ExplainPlanNode, 0),
		}
		for index, value := range nested {
			child, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			wrapper.Children = append(
				wrapper.Children,
				mysqlPlanChildren(
					child,
					wrapper.ID,
					fmt.Sprintf("%s-nested-%d", idPrefix, index),
				)...,
			)
		}
		nodes = append(nodes, wrapper)
	}
	return nodes
}

func mysqlWrapperNode(
	key string,
	source map[string]interface{},
	parentID string,
	id string,
) database.ExplainPlanNode {
	label := strings.ReplaceAll(key, "_", " ")
	label = strings.ToUpper(label[:1]) + label[1:]
	node := database.ExplainPlanNode{
		ID:       id,
		ParentID: parentID,
		NodeType: label,
		Summary:  label,
	}
	if cost, ok := source["cost_info"].(map[string]interface{}); ok {
		node.TotalCost = numericPlanValue(cost["query_cost"])
	}
	return node
}

func mysqlTablePlanNode(
	table map[string]interface{},
	parentID string,
	id string,
) database.ExplainPlanNode {
	relation := fmt.Sprint(table["table_name"])
	if relation == "<nil>" {
		relation = ""
	}
	access := fmt.Sprint(table["access_type"])
	if access == "<nil>" {
		access = "table access"
	}
	node := database.ExplainPlanNode{
		ID:            id,
		ParentID:      parentID,
		NodeType:      strings.ToUpper(access),
		Relation:      relation,
		Summary:       strings.ToUpper(access) + " on " + relation,
		EstimatedRows: numericPlanValue(table["rows_examined_per_scan"]),
		Details:       make(map[string]string),
	}
	if cost, ok := table["cost_info"].(map[string]interface{}); ok {
		node.TotalCost = numericPlanValue(cost["prefix_cost"])
		if node.TotalCost == 0 {
			node.TotalCost = numericPlanValue(cost["read_cost"])
		}
	}
	for _, key := range []string{
		"key",
		"possible_keys",
		"used_key_parts",
		"attached_condition",
		"using_index",
	} {
		if value, exists := table[key]; exists {
			node.Details[strings.ReplaceAll(key, "_", " ")] = fmt.Sprint(value)
		}
	}
	if len(node.Details) == 0 {
		node.Details = nil
	}
	return node
}

func numericPlanValue(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case json.Number:
		number, _ := typed.Float64()
		return number
	case string:
		var number float64
		_, _ = fmt.Sscan(typed, &number)
		return number
	default:
		return 0
	}
}
