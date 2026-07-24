package sqlite

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"rollingthunder/pkg/database"
)

type sqlitePlanRow struct {
	ID     int    `db:"id"`
	Parent int    `db:"parent"`
	Unused int    `db:"notused"`
	Detail string `db:"detail"`
}

var sqlitePlanRelation = regexp.MustCompile(
	`(?i)^(?:SCAN|SEARCH)(?: TABLE)? ([^ ]+)`,
)

func (s *SQLite) ExplainQuery(
	ctx context.Context,
	query string,
) (database.ExplainPlan, error) {
	return s.ExplainQueryWithArgs(ctx, query, nil)
}

func (s *SQLite) ExplainQueryWithArgs(
	ctx context.Context,
	query string,
	args []interface{},
) (database.ExplainPlan, error) {
	var rows []sqlitePlanRow
	if err := s.conn.SelectContext(
		ctx,
		&rows,
		"EXPLAIN QUERY PLAN "+query,
		args...,
	); err != nil {
		return database.ExplainPlan{}, err
	}
	if len(rows) == 0 {
		return database.ExplainPlan{}, fmt.Errorf("SQLite returned an empty explain plan")
	}

	nodes := make(map[int]database.ExplainPlanNode, len(rows))
	children := make(map[int][]int, len(rows))
	order := make([]int, 0, len(rows))
	for _, row := range rows {
		nodeType := strings.Fields(row.Detail)
		label := "Operation"
		if len(nodeType) > 0 {
			label = strings.ToUpper(nodeType[0])
			if label == "USE" && len(nodeType) > 1 {
				label += " " + strings.ToUpper(nodeType[1])
			}
		}
		relation := ""
		if matches := sqlitePlanRelation.FindStringSubmatch(row.Detail); len(matches) == 2 {
			relation = strings.Trim(matches[1], `"'`+"`")
		}
		node := database.ExplainPlanNode{
			ID:       fmt.Sprintf("sqlite-%d", row.ID),
			NodeType: label,
			Relation: relation,
			Summary:  row.Detail,
			Details: map[string]string{
				"detail": row.Detail,
			},
			Children: make([]database.ExplainPlanNode, 0),
		}
		nodes[row.ID] = node
		order = append(order, row.ID)
		if row.Parent != row.ID {
			children[row.Parent] = append(children[row.Parent], row.ID)
		}
	}

	roots := make([]database.ExplainPlanNode, 0)
	for _, id := range order {
		row := findSQLitePlanRow(rows, id)
		if _, parentExists := nodes[row.Parent]; !parentExists || row.Parent == row.ID {
			roots = append(roots, buildSQLitePlanNode(id, "", nodes, children))
		}
	}
	return database.ExplainPlan{
		Engine:  "SQLite",
		Summary: roots[0].Summary,
		Roots:   roots,
		Raw:     formatSQLitePlanRows(rows),
	}, nil
}

func buildSQLitePlanNode(
	id int,
	parentID string,
	nodes map[int]database.ExplainPlanNode,
	children map[int][]int,
) database.ExplainPlanNode {
	node := nodes[id]
	node.ParentID = parentID
	node.Children = make([]database.ExplainPlanNode, 0, len(children[id]))
	for _, childID := range children[id] {
		node.Children = append(
			node.Children,
			buildSQLitePlanNode(childID, node.ID, nodes, children),
		)
	}
	return node
}

func findSQLitePlanRow(rows []sqlitePlanRow, id int) sqlitePlanRow {
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	return sqlitePlanRow{ID: id, Parent: id}
}

func formatSQLitePlanRows(rows []sqlitePlanRow) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf(
			"%d | parent %d | %s",
			row.ID,
			row.Parent,
			row.Detail,
		))
	}
	return strings.Join(lines, "\n")
}
