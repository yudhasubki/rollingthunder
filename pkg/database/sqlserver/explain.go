package sqlserver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"rollingthunder/pkg/database"
)

type sqlServerPlanNode struct {
	node     database.ExplainPlanNode
	children []*sqlServerPlanNode
}

func (s *SQLServer) ExplainQuery(
	ctx context.Context,
	query string,
) (database.ExplainPlan, error) {
	return s.ExplainQueryWithArgs(ctx, query, nil)
}

func (s *SQLServer) ExplainQueryWithArgs(
	ctx context.Context,
	query string,
	args []interface{},
) (
	plan database.ExplainPlan,
	err error,
) {
	if err := s.ensureConnected(); err != nil {
		return database.ExplainPlan{}, err
	}
	connection, err := s.conn.Conn(ctx)
	if err != nil {
		return database.ExplainPlan{}, err
	}
	defer connection.Close()
	showplanEnabled := false
	defer func() {
		if !showplanEnabled {
			return
		}
		resetContext, cancel := context.WithTimeout(
			context.Background(),
			3*time.Second,
		)
		defer cancel()
		if _, resetErr := connection.ExecContext(
			resetContext,
			"SET SHOWPLAN_XML OFF",
		); resetErr != nil {
			discardSQLServerConnection(connection)
			if err == nil {
				err = fmt.Errorf(
					"reset SQL Server SHOWPLAN_XML session: %w",
					resetErr,
				)
			}
		}
	}()
	if _, err := connection.ExecContext(
		ctx,
		"SET SHOWPLAN_XML ON",
	); err != nil {
		return database.ExplainPlan{}, err
	}
	showplanEnabled = true
	rows, err := connection.QueryContext(ctx, query, args...)
	if err != nil {
		return database.ExplainPlan{}, err
	}
	documents := make([]string, 0, 1)
	for {
		for rows.Next() {
			var document string
			if err := rows.Scan(&document); err != nil {
				_ = rows.Close()
				return database.ExplainPlan{}, err
			}
			if strings.TrimSpace(document) != "" {
				documents = append(documents, document)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return database.ExplainPlan{}, err
		}
		if !rows.NextResultSet() {
			break
		}
	}
	if err := rows.Close(); err != nil {
		return database.ExplainPlan{}, err
	}
	if len(documents) == 0 {
		return database.ExplainPlan{}, fmt.Errorf(
			"SQL Server returned an empty SHOWPLAN_XML document",
		)
	}
	roots := make([]database.ExplainPlanNode, 0)
	summaries := make([]string, 0)
	for _, document := range documents {
		documentRoots, summary, parseErr := parseSQLServerShowplan(
			document,
		)
		if parseErr != nil {
			return database.ExplainPlan{}, parseErr
		}
		roots = append(roots, documentRoots...)
		if summary != "" {
			summaries = append(summaries, summary)
		}
	}
	if len(roots) == 0 {
		return database.ExplainPlan{}, fmt.Errorf(
			"SQL Server SHOWPLAN_XML has no relational operators",
		)
	}
	summary := roots[0].Summary
	if len(summaries) > 0 {
		summary = summaries[0]
	}
	return database.ExplainPlan{
		Engine:  "Microsoft SQL Server",
		Summary: summary,
		Roots:   roots,
		Raw:     strings.Join(documents, "\n\n"),
	}, nil
}

func discardSQLServerConnection(connection *sql.Conn) {
	_ = connection.Raw(func(any) error {
		return sqldriver.ErrBadConn
	})
}

func xmlAttribute(
	element xml.StartElement,
	name string,
) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func sqlServerPlanFloat(
	element xml.StartElement,
	name string,
) float64 {
	value, _ := strconv.ParseFloat(xmlAttribute(element, name), 64)
	return value
}

func trimSQLServerObjectPart(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		value = strings.ReplaceAll(value, "]]", "]")
	}
	return value
}

func parseSQLServerShowplan(
	document string,
) ([]database.ExplainPlanNode, string, error) {
	decoder := xml.NewDecoder(strings.NewReader(document))
	stack := make([]*sqlServerPlanNode, 0)
	roots := make([]*sqlServerPlanNode, 0)
	statementSummary := ""
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", fmt.Errorf(
				"decode SQL Server SHOWPLAN_XML: %w",
				err,
			)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "StmtSimple", "StmtCond", "StmtCursor":
				if statementSummary == "" {
					statementSummary = strings.TrimSpace(
						xmlAttribute(typed, "StatementText"),
					)
				}
			case "RelOp":
				nodeID := xmlAttribute(typed, "NodeId")
				nodeType := xmlAttribute(typed, "PhysicalOp")
				if nodeType == "" {
					nodeType = xmlAttribute(typed, "LogicalOp")
				}
				details := make(map[string]string)
				logical := xmlAttribute(typed, "LogicalOp")
				if logical != "" && logical != nodeType {
					details["Logical operation"] = logical
				}
				for _, detail := range []struct {
					attribute string
					label     string
				}{
					{"EstimateCPU", "Estimated CPU"},
					{"EstimateIO", "Estimated I/O"},
					{"AvgRowSize", "Average row size"},
				} {
					if value := xmlAttribute(
						typed,
						detail.attribute,
					); value != "" {
						details[detail.label] = value
					}
				}
				if len(details) == 0 {
					details = nil
				}
				item := &sqlServerPlanNode{
					node: database.ExplainPlanNode{
						ID:            "sqlserver-" + nodeID,
						NodeType:      nodeType,
						Summary:       nodeType,
						TotalCost:     sqlServerPlanFloat(typed, "EstimatedTotalSubtreeCost"),
						EstimatedRows: sqlServerPlanFloat(typed, "EstimateRows"),
						Details:       details,
					},
					children: make([]*sqlServerPlanNode, 0),
				}
				if len(stack) == 0 {
					roots = append(roots, item)
				} else {
					stack[len(stack)-1].children = append(
						stack[len(stack)-1].children,
						item,
					)
				}
				stack = append(stack, item)
			case "Object":
				if len(stack) == 0 {
					continue
				}
				schema := trimSQLServerObjectPart(
					xmlAttribute(typed, "Schema"),
				)
				table := trimSQLServerObjectPart(
					xmlAttribute(typed, "Table"),
				)
				relation := table
				if schema != "" && table != "" {
					relation = schema + "." + table
				}
				current := stack[len(stack)-1]
				if relation != "" {
					current.node.Relation = relation
					current.node.Summary = current.node.NodeType +
						" on " + relation
				}
				if index := trimSQLServerObjectPart(
					xmlAttribute(typed, "Index"),
				); index != "" {
					if current.node.Details == nil {
						current.node.Details = make(map[string]string)
					}
					current.node.Details["Index"] = index
				}
			case "ScalarOperator":
				if len(stack) == 0 {
					continue
				}
				predicate := strings.TrimSpace(
					xmlAttribute(typed, "ScalarString"),
				)
				current := stack[len(stack)-1]
				if predicate != "" {
					if current.node.Details == nil {
						current.node.Details = make(map[string]string)
					}
					if _, exists := current.node.Details["Expression"]; !exists {
						current.node.Details["Expression"] = predicate
					}
				}
			}
		case xml.EndElement:
			if typed.Name.Local == "RelOp" && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	result := make([]database.ExplainPlanNode, 0, len(roots))
	for _, root := range roots {
		result = append(result, finalizeSQLServerPlanNode(root, ""))
	}
	return result, statementSummary, nil
}

func finalizeSQLServerPlanNode(
	source *sqlServerPlanNode,
	parentID string,
) database.ExplainPlanNode {
	node := source.node
	node.ParentID = parentID
	node.Children = make(
		[]database.ExplainPlanNode,
		0,
		len(source.children),
	)
	for _, child := range source.children {
		node.Children = append(
			node.Children,
			finalizeSQLServerPlanNode(child, node.ID),
		)
	}
	return node
}

var _ database.ExplainPlanDriver = (*SQLServer)(nil)
