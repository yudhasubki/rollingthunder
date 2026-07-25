package database

import (
	"fmt"
	"strings"
)

const (
	DefaultDataSyncRowLimit = 5000
	MaxDataSyncRowLimit     = 10000
)

type DataSyncRequest struct {
	SourceConnectionID string   `json:"sourceConnectionId"`
	SourceSchema       string   `json:"sourceSchema"`
	SourceTable        string   `json:"sourceTable"`
	TargetConnectionID string   `json:"targetConnectionId"`
	TargetSchema       string   `json:"targetSchema"`
	TargetTable        string   `json:"targetTable"`
	KeyColumns         []string `json:"keyColumns,omitempty"`
	CompareColumns     []string `json:"compareColumns,omitempty"`
	MaxRows            int      `json:"maxRows,omitempty"`
}

func (request DataSyncRequest) Validate() error {
	if strings.TrimSpace(request.SourceConnectionID) == "" ||
		strings.TrimSpace(request.TargetConnectionID) == "" {
		return fmt.Errorf("source and target connections are required")
	}
	if strings.TrimSpace(request.SourceTable) == "" ||
		strings.TrimSpace(request.TargetTable) == "" {
		return fmt.Errorf("source and target tables are required")
	}
	if request.MaxRows < 0 || request.MaxRows > MaxDataSyncRowLimit {
		return fmt.Errorf(
			"row limit must be between 1 and %d",
			MaxDataSyncRowLimit,
		)
	}
	return nil
}

type DataSyncChange struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	Key            map[string]interface{} `json:"key"`
	Source         map[string]interface{} `json:"source,omitempty"`
	Target         map[string]interface{} `json:"target,omitempty"`
	ChangedColumns []string               `json:"changedColumns,omitempty"`
}

type DataSyncPreview struct {
	SourceEngine   string           `json:"sourceEngine"`
	TargetEngine   string           `json:"targetEngine"`
	KeyColumns     []string         `json:"keyColumns"`
	CompareColumns []string         `json:"compareColumns"`
	Changes        []DataSyncChange `json:"changes"`
	Added          int              `json:"added"`
	Updated        int              `json:"updated"`
	Deleted        int              `json:"deleted"`
	SourceRows     int              `json:"sourceRows"`
	TargetRows     int              `json:"targetRows"`
	Truncated      bool             `json:"truncated"`
	SafeToApply    bool             `json:"safeToApply"`
	Warnings       []string         `json:"warnings"`
	Fingerprint    string           `json:"fingerprint"`
}

type ApplyDataSyncRequest struct {
	Sync              DataSyncRequest `json:"sync"`
	Fingerprint       string          `json:"fingerprint"`
	SelectedChangeIDs []string        `json:"selectedChangeIds,omitempty"`
}

type DataSyncResult struct {
	Applied     bool   `json:"applied"`
	Inserted    int    `json:"inserted"`
	Updated     int    `json:"updated"`
	Deleted     int    `json:"deleted"`
	Fingerprint string `json:"fingerprint"`
}
