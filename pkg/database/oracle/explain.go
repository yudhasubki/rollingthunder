package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"rollingthunder/pkg/database"

	"github.com/google/uuid"
)

type oracleExplainRow struct {
	id          int64
	parentID    sql.NullInt64
	operation   string
	options     sql.NullString
	objectOwner sql.NullString
	objectName  sql.NullString
	objectType  sql.NullString
	cost        sql.NullFloat64
	cardinality sql.NullFloat64
	bytes       sql.NullFloat64
	access      sql.NullString
	filter      sql.NullString
}

func (o *Oracle) ExplainQuery(
	ctx context.Context,
	query string,
) (database.ExplainPlan, error) {
	return o.ExplainQueryWithArgs(ctx, query, nil)
}

func (o *Oracle) ExplainQueryWithArgs(
	ctx context.Context,
	query string,
	args []interface{},
) (database.ExplainPlan, error) {
	if err := o.ensureConnected(); err != nil {
		return database.ExplainPlan{}, err
	}
	query = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
	if query == "" {
		return database.ExplainPlan{}, fmt.Errorf(
			"Oracle explain query is empty",
		)
	}
	transaction, err := o.conn.BeginTx(ctx, nil)
	if err != nil {
		return database.ExplainPlan{}, err
	}
	defer func() { _ = transaction.Rollback() }()

	statementID := strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
	explainSQL := "EXPLAIN PLAN SET STATEMENT_ID = '" +
		statementID + "' FOR " + query
	if _, err := transaction.ExecContext(
		ctx,
		explainSQL,
		args...,
	); err != nil {
		return database.ExplainPlan{}, err
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			id,
			parent_id,
			operation,
			options,
			object_owner,
			object_name,
			object_type,
			cost,
			cardinality,
			bytes,
			access_predicates,
			filter_predicates
		FROM plan_table
		WHERE statement_id = :1
		ORDER BY id`,
		statementID,
	)
	if err != nil {
		return database.ExplainPlan{}, fmt.Errorf(
			"read Oracle PLAN_TABLE: %w",
			err,
		)
	}
	defer rows.Close()
	values := make([]oracleExplainRow, 0)
	for rows.Next() {
		var row oracleExplainRow
		if err := rows.Scan(
			&row.id,
			&row.parentID,
			&row.operation,
			&row.options,
			&row.objectOwner,
			&row.objectName,
			&row.objectType,
			&row.cost,
			&row.cardinality,
			&row.bytes,
			&row.access,
			&row.filter,
		); err != nil {
			return database.ExplainPlan{}, err
		}
		values = append(values, row)
	}
	if err := rows.Err(); err != nil {
		return database.ExplainPlan{}, err
	}
	return buildOracleExplainPlan(values)
}

func buildOracleExplainPlan(
	rows []oracleExplainRow,
) (database.ExplainPlan, error) {
	if len(rows) == 0 {
		return database.ExplainPlan{}, fmt.Errorf(
			"Oracle returned an empty explain plan",
		)
	}
	nodes := make(map[int64]database.ExplainPlanNode, len(rows))
	children := make(map[int64][]int64, len(rows))
	order := make([]int64, 0, len(rows))
	raw := make([]string, 0, len(rows))
	for _, row := range rows {
		nodeType := strings.TrimSpace(row.operation)
		if row.options.Valid && strings.TrimSpace(row.options.String) != "" {
			nodeType += " " + strings.TrimSpace(row.options.String)
		}
		relation := ""
		if row.objectName.Valid {
			relation = row.objectName.String
			if row.objectOwner.Valid &&
				strings.TrimSpace(row.objectOwner.String) != "" {
				relation = row.objectOwner.String + "." + relation
			}
		}
		summary := nodeType
		if relation != "" {
			summary += " on " + relation
		}
		details := make(map[string]string)
		if row.objectType.Valid && row.objectType.String != "" {
			details["Object type"] = row.objectType.String
		}
		if row.bytes.Valid {
			details["Estimated bytes"] = strconv.FormatFloat(
				row.bytes.Float64,
				'f',
				-1,
				64,
			)
		}
		if row.access.Valid && strings.TrimSpace(row.access.String) != "" {
			details["Access predicates"] = row.access.String
		}
		if row.filter.Valid && strings.TrimSpace(row.filter.String) != "" {
			details["Filter predicates"] = row.filter.String
		}
		if len(details) == 0 {
			details = nil
		}
		node := database.ExplainPlanNode{
			ID:            fmt.Sprintf("oracle-%d", row.id),
			NodeType:      nodeType,
			Relation:      relation,
			Summary:       summary,
			TotalCost:     row.cost.Float64,
			EstimatedRows: row.cardinality.Float64,
			Details:       details,
			Children:      make([]database.ExplainPlanNode, 0),
		}
		nodes[row.id] = node
		order = append(order, row.id)
		if row.parentID.Valid && row.parentID.Int64 != row.id {
			children[row.parentID.Int64] = append(
				children[row.parentID.Int64],
				row.id,
			)
		}
		raw = append(raw, fmt.Sprintf(
			"%d | parent %s | %s | rows %.0f | cost %.2f",
			row.id,
			nullableOraclePlanID(row.parentID),
			summary,
			row.cardinality.Float64,
			row.cost.Float64,
		))
	}
	roots := make([]database.ExplainPlanNode, 0)
	for _, id := range order {
		row := findOracleExplainRow(rows, id)
		if !row.parentID.Valid {
			roots = append(
				roots,
				buildOracleExplainNode(id, "", nodes, children),
			)
			continue
		}
		if _, exists := nodes[row.parentID.Int64]; !exists {
			roots = append(
				roots,
				buildOracleExplainNode(id, "", nodes, children),
			)
		}
	}
	if len(roots) == 0 {
		return database.ExplainPlan{}, fmt.Errorf(
			"Oracle explain plan has no root node",
		)
	}
	return database.ExplainPlan{
		Engine:  "Oracle Database",
		Summary: roots[0].Summary,
		Roots:   roots,
		Raw:     strings.Join(raw, "\n"),
	}, nil
}

func nullableOraclePlanID(value sql.NullInt64) string {
	if !value.Valid {
		return "-"
	}
	return strconv.FormatInt(value.Int64, 10)
}

func findOracleExplainRow(
	rows []oracleExplainRow,
	id int64,
) oracleExplainRow {
	for _, row := range rows {
		if row.id == id {
			return row
		}
	}
	return oracleExplainRow{id: id}
}

func buildOracleExplainNode(
	id int64,
	parentID string,
	nodes map[int64]database.ExplainPlanNode,
	children map[int64][]int64,
) database.ExplainPlanNode {
	node := nodes[id]
	node.ParentID = parentID
	node.Children = make(
		[]database.ExplainPlanNode,
		0,
		len(children[id]),
	)
	for _, childID := range children[id] {
		node.Children = append(
			node.Children,
			buildOracleExplainNode(
				childID,
				node.ID,
				nodes,
				children,
			),
		)
	}
	return node
}

var _ database.ExplainPlanDriver = (*Oracle)(nil)
