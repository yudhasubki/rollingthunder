package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

func (p *Postgres) ExplainQuery(
	ctx context.Context,
	query string,
) (database.ExplainPlan, error) {
	return p.ExplainQueryWithArgs(ctx, query, nil)
}

func (p *Postgres) ExplainQueryWithArgs(
	ctx context.Context,
	query string,
	args []interface{},
) (database.ExplainPlan, error) {
	var raw string
	if err := p.conn.GetContext(
		ctx,
		&raw,
		"EXPLAIN (FORMAT JSON) "+query,
		args...,
	); err != nil {
		return database.ExplainPlan{}, err
	}

	var documents []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &documents); err != nil {
		return database.ExplainPlan{}, fmt.Errorf("decode PostgreSQL explain plan: %w", err)
	}
	if len(documents) == 0 {
		return database.ExplainPlan{}, fmt.Errorf("PostgreSQL returned an empty explain plan")
	}
	rootMap, ok := documents[0]["Plan"].(map[string]interface{})
	if !ok {
		return database.ExplainPlan{}, fmt.Errorf("PostgreSQL explain plan has no root node")
	}
	root := postgresPlanNode(rootMap, "", "pg-0")
	pretty, _ := json.MarshalIndent(documents, "", "  ")
	return database.ExplainPlan{
		Engine:  p.engine,
		Summary: root.Summary,
		Roots:   []database.ExplainPlanNode{root},
		Raw:     string(pretty),
	}, nil
}

func postgresPlanNode(
	source map[string]interface{},
	parentID string,
	id string,
) database.ExplainPlanNode {
	nodeType := stringValue(source["Node Type"])
	relation := stringValue(source["Relation Name"])
	if schema := stringValue(source["Schema"]); schema != "" && relation != "" {
		relation = schema + "." + relation
	}
	summary := nodeType
	if relation != "" {
		summary += " on " + relation
	}
	node := database.ExplainPlanNode{
		ID:            id,
		ParentID:      parentID,
		NodeType:      nodeType,
		Relation:      relation,
		Summary:       summary,
		StartupCost:   floatValue(source["Startup Cost"]),
		TotalCost:     floatValue(source["Total Cost"]),
		EstimatedRows: floatValue(source["Plan Rows"]),
		ActualRows:    floatValue(source["Actual Rows"]),
		Details:       make(map[string]string),
		Children:      make([]database.ExplainPlanNode, 0),
	}
	for _, key := range []string{
		"Join Type",
		"Index Name",
		"Index Cond",
		"Filter",
		"Hash Cond",
		"Merge Cond",
		"Sort Key",
		"Group Key",
		"Strategy",
		"Parallel Aware",
	} {
		if value, exists := source[key]; exists {
			node.Details[key] = formatPlanValue(value)
		}
	}
	if len(node.Details) == 0 {
		node.Details = nil
	}
	if plans, ok := source["Plans"].([]interface{}); ok {
		for index, childValue := range plans {
			child, ok := childValue.(map[string]interface{})
			if !ok {
				continue
			}
			childID := fmt.Sprintf("%s-%d", id, index)
			node.Children = append(
				node.Children,
				postgresPlanNode(child, id, childID),
			)
		}
	}
	return node
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func floatValue(value interface{}) float64 {
	number, _ := value.(float64)
	return number
}

func formatPlanValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(value)
	}
}
