package database

import "context"

type ExplainPlanNode struct {
	ID            string            `json:"id"`
	ParentID      string            `json:"parentId,omitempty"`
	NodeType      string            `json:"nodeType"`
	Relation      string            `json:"relation,omitempty"`
	Summary       string            `json:"summary"`
	StartupCost   float64           `json:"startupCost,omitempty"`
	TotalCost     float64           `json:"totalCost,omitempty"`
	EstimatedRows float64           `json:"estimatedRows,omitempty"`
	ActualRows    float64           `json:"actualRows,omitempty"`
	Details       map[string]string `json:"details,omitempty"`
	Children      []ExplainPlanNode `json:"children,omitempty"`
}

type ExplainPlan struct {
	Engine  string            `json:"engine"`
	Summary string            `json:"summary"`
	Roots   []ExplainPlanNode `json:"roots"`
	Raw     string            `json:"raw"`
}

type ExplainPlanDriver interface {
	ExplainQuery(ctx context.Context, query string) (ExplainPlan, error)
}
