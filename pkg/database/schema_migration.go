package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type SchemaMigrationRequest struct {
	SourceConnectionID string `json:"sourceConnectionId"`
	SourceSchema       string `json:"sourceSchema"`
	TargetConnectionID string `json:"targetConnectionId"`
	TargetSchema       string `json:"targetSchema"`
	IncludeDestructive bool   `json:"includeDestructive"`
}

func (request SchemaMigrationRequest) Validate() error {
	if strings.TrimSpace(request.SourceConnectionID) == "" {
		return fmt.Errorf("source connection is required")
	}
	if strings.TrimSpace(request.TargetConnectionID) == "" {
		return fmt.Errorf("target connection is required")
	}
	if strings.TrimSpace(request.SourceSchema) == "" {
		return fmt.Errorf("source schema is required")
	}
	if strings.TrimSpace(request.TargetSchema) == "" {
		return fmt.Errorf("target schema is required")
	}
	if request.SourceConnectionID == request.TargetConnectionID &&
		request.SourceSchema == request.TargetSchema {
		return fmt.Errorf("source and target must be different")
	}
	return nil
}

type SchemaMigrationChange struct {
	ID          string   `json:"id"`
	Action      string   `json:"action"`
	Object      string   `json:"object"`
	Summary     string   `json:"summary"`
	Statements  []string `json:"statements"`
	Destructive bool     `json:"destructive"`
	Selected    bool     `json:"selected"`
	Supported   bool     `json:"supported"`
	Reason      string   `json:"reason,omitempty"`
	Warnings    []string `json:"warnings"`
}

type SchemaMigrationPreview struct {
	SourceEngine    string                  `json:"sourceEngine"`
	TargetEngine    string                  `json:"targetEngine"`
	SourceSchema    string                  `json:"sourceSchema"`
	TargetSchema    string                  `json:"targetSchema"`
	Changes         []SchemaMigrationChange `json:"changes"`
	SQL             string                  `json:"sql"`
	StatementCount  int                     `json:"statementCount"`
	SelectedChanges int                     `json:"selectedChanges"`
	ManualChanges   int                     `json:"manualChanges"`
	Destructive     bool                    `json:"destructive"`
	Transactional   bool                    `json:"transactional"`
	Warnings        []string                `json:"warnings"`
	Fingerprint     string                  `json:"fingerprint"`
}

type ApplySchemaMigrationRequest struct {
	Migration   SchemaMigrationRequest `json:"migration"`
	Fingerprint string                 `json:"fingerprint"`
}

type SchemaMigrationResult struct {
	Applied        bool   `json:"applied"`
	StatementCount int    `json:"statementCount"`
	Fingerprint    string `json:"fingerprint"`
}

type SchemaTableSnapshot struct {
	Name       string     `json:"name"`
	Definition string     `json:"definition"`
	Columns    Structures `json:"columns"`
	Indexes    Indices    `json:"indexes"`
}

type SchemaSnapshot struct {
	Engine string                `json:"engine"`
	Schema string                `json:"schema"`
	Tables []SchemaTableSnapshot `json:"tables"`
}

func SchemaMigrationFingerprint(
	request SchemaMigrationRequest,
	engine string,
	changes []SchemaMigrationChange,
) string {
	hash := sha256.New()
	values := []string{
		strings.TrimSpace(engine),
		strings.TrimSpace(request.SourceConnectionID),
		strings.TrimSpace(request.SourceSchema),
		strings.TrimSpace(request.TargetConnectionID),
		strings.TrimSpace(request.TargetSchema),
		fmt.Sprintf("%t", request.IncludeDestructive),
	}
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	for _, change := range changes {
		_, _ = hash.Write([]byte(change.ID))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(fmt.Sprintf(
			"%t:%t:%t",
			change.Destructive,
			change.Selected,
			change.Supported,
		)))
		_, _ = hash.Write([]byte{0})
		for _, statement := range change.Statements {
			_, _ = hash.Write([]byte(strings.TrimSpace(statement)))
			_, _ = hash.Write([]byte{0})
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
