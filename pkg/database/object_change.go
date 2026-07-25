package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type ObjectChangeAction string

const (
	ObjectChangeCreate         ObjectChangeAction = "create"
	ObjectChangeReplace        ObjectChangeAction = "replace"
	ObjectChangeRename         ObjectChangeAction = "rename"
	ObjectChangeDrop           ObjectChangeAction = "drop"
	ObjectChangeEnable         ObjectChangeAction = "enable"
	ObjectChangeDisable        ObjectChangeAction = "disable"
	ObjectChangeCreateIndex    ObjectChangeAction = "create_index"
	ObjectChangeAddColumn      ObjectChangeAction = "add_column"
	ObjectChangeAlterColumn    ObjectChangeAction = "alter_column"
	ObjectChangeDropColumn     ObjectChangeAction = "drop_column"
	ObjectChangeAddConstraint  ObjectChangeAction = "add_constraint"
	ObjectChangeDropConstraint ObjectChangeAction = "drop_constraint"
)

func (action ObjectChangeAction) Valid() bool {
	switch action {
	case ObjectChangeCreate,
		ObjectChangeReplace,
		ObjectChangeRename,
		ObjectChangeDrop,
		ObjectChangeEnable,
		ObjectChangeDisable,
		ObjectChangeCreateIndex,
		ObjectChangeAddColumn,
		ObjectChangeAlterColumn,
		ObjectChangeDropColumn,
		ObjectChangeAddConstraint,
		ObjectChangeDropConstraint:
		return true
	default:
		return false
	}
}

type IndexChange struct {
	Table   Table    `json:"table"`
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Method  string   `json:"method,omitempty"`
	Where   string   `json:"where,omitempty"`
}

type ColumnChange struct {
	Table       Table   `json:"table"`
	Name        string  `json:"name"`
	NewName     string  `json:"newName,omitempty"`
	DataType    string  `json:"dataType,omitempty"`
	Using       string  `json:"using,omitempty"`
	Nullable    *bool   `json:"nullable,omitempty"`
	Default     *string `json:"default,omitempty"`
	DropDefault bool    `json:"dropDefault,omitempty"`
}

type AddColumnChange struct {
	Table  Table            `json:"table"`
	Column ColumnDefinition `json:"column"`
	First  bool             `json:"first,omitempty"`
	After  string           `json:"after,omitempty"`
}

type DropColumnChange struct {
	Table Table  `json:"table"`
	Name  string `json:"name"`
}

type ConstraintChange struct {
	Table      Table  `json:"table"`
	Name       string `json:"name"`
	Definition string `json:"definition,omitempty"`
}

type ObjectChangeRequest struct {
	Action     ObjectChangeAction `json:"action"`
	Reference  ObjectReference    `json:"reference"`
	NewName    string             `json:"newName,omitempty"`
	Definition string             `json:"definition,omitempty"`
	Cascade    bool               `json:"cascade,omitempty"`
	Index      *IndexChange       `json:"index,omitempty"`
	AddColumn  *AddColumnChange   `json:"addColumn,omitempty"`
	Column     *ColumnChange      `json:"column,omitempty"`
	DropColumn *DropColumnChange  `json:"dropColumn,omitempty"`
	Constraint *ConstraintChange  `json:"constraint,omitempty"`
}

func (request ObjectChangeRequest) Validate() error {
	if !request.Action.Valid() {
		return fmt.Errorf("unsupported object change action %q", request.Action)
	}

	switch request.Action {
	case ObjectChangeCreateIndex:
		if request.Index == nil {
			return fmt.Errorf("index details are required")
		}
		if strings.TrimSpace(request.Index.Name) == "" {
			return fmt.Errorf("index name is required")
		}
		if strings.TrimSpace(request.Index.Table.Name) == "" {
			return fmt.Errorf("index table is required")
		}
		if len(request.Index.Columns) == 0 {
			return fmt.Errorf("at least one index column is required")
		}
		return nil

	case ObjectChangeAlterColumn:
		if request.Column == nil {
			return fmt.Errorf("column change details are required")
		}
		if strings.TrimSpace(request.Column.Table.Name) == "" ||
			strings.TrimSpace(request.Column.Name) == "" {
			return fmt.Errorf("table and column names are required")
		}
		if strings.TrimSpace(request.Column.NewName) == "" &&
			strings.TrimSpace(request.Column.DataType) == "" &&
			request.Column.Nullable == nil &&
			request.Column.Default == nil &&
			!request.Column.DropDefault {
			return fmt.Errorf("the column change does not contain any alterations")
		}
		if request.Column.Default != nil && request.Column.DropDefault {
			return fmt.Errorf("set default and drop default cannot be combined")
		}
		return nil

	case ObjectChangeAddColumn:
		if request.AddColumn == nil {
			return fmt.Errorf("new column details are required")
		}
		if strings.TrimSpace(request.AddColumn.Table.Name) == "" {
			return fmt.Errorf("column table is required")
		}
		if strings.TrimSpace(request.AddColumn.Column.Name) == "" {
			return fmt.Errorf("column name is required")
		}
		if strings.TrimSpace(request.AddColumn.Column.Type) == "" {
			return fmt.Errorf("column data type is required")
		}
		if request.AddColumn.First &&
			strings.TrimSpace(request.AddColumn.After) != "" {
			return fmt.Errorf("a column cannot be positioned both first and after another column")
		}
		return nil

	case ObjectChangeDropColumn:
		if request.DropColumn == nil {
			return fmt.Errorf("column removal details are required")
		}
		if strings.TrimSpace(request.DropColumn.Table.Name) == "" ||
			strings.TrimSpace(request.DropColumn.Name) == "" {
			return fmt.Errorf("table and column names are required")
		}
		return nil

	case ObjectChangeAddConstraint, ObjectChangeDropConstraint:
		if request.Constraint == nil {
			return fmt.Errorf("constraint details are required")
		}
		if strings.TrimSpace(request.Constraint.Table.Name) == "" ||
			strings.TrimSpace(request.Constraint.Name) == "" {
			return fmt.Errorf("table and constraint names are required")
		}
		if request.Action == ObjectChangeAddConstraint &&
			strings.TrimSpace(request.Constraint.Definition) == "" {
			return fmt.Errorf("constraint definition is required")
		}
		return nil
	}

	if err := request.Reference.Validate(); err != nil {
		return err
	}
	switch request.Action {
	case ObjectChangeCreate, ObjectChangeReplace:
		if strings.TrimSpace(request.Definition) == "" {
			return fmt.Errorf("object definition is required")
		}
	case ObjectChangeRename:
		if strings.TrimSpace(request.NewName) == "" {
			return fmt.Errorf("new object name is required")
		}
	case ObjectChangeEnable, ObjectChangeDisable:
		if request.Reference.Kind != ObjectKindTrigger {
			return fmt.Errorf("only triggers can be enabled or disabled")
		}
	}
	return nil
}

type ObjectChangePlan struct {
	Summary       string
	Statements    []string
	Destructive   bool
	Transactional bool
	Warnings      []string
	Refresh       []ObjectReference
}

func (plan ObjectChangePlan) Validate() error {
	if strings.TrimSpace(plan.Summary) == "" {
		return fmt.Errorf("object change summary is required")
	}
	if len(plan.Statements) == 0 {
		return fmt.Errorf("object change contains no SQL statements")
	}
	for index, statement := range plan.Statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			return fmt.Errorf("object change statement %d is empty", index+1)
		}
		if strings.ContainsRune(statement, '\x00') {
			return fmt.Errorf("object change statement %d contains a NUL byte", index+1)
		}
	}
	return nil
}

func (plan ObjectChangePlan) Fingerprint(engine string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(engine)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.TrimSpace(plan.Summary)))
	_, _ = hash.Write([]byte{0})
	for _, statement := range plan.Statements {
		_, _ = hash.Write([]byte(strings.TrimSpace(statement)))
		_, _ = hash.Write([]byte{0})
	}
	if plan.Destructive {
		_, _ = hash.Write([]byte("destructive"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type ObjectChangePreview struct {
	Summary        string            `json:"summary"`
	SQL            string            `json:"sql"`
	StatementCount int               `json:"statementCount"`
	Destructive    bool              `json:"destructive"`
	Transactional  bool              `json:"transactional"`
	Warnings       []string          `json:"warnings"`
	Fingerprint    string            `json:"fingerprint"`
	Refresh        []ObjectReference `json:"refresh"`
}

type ApplyObjectChangeRequest struct {
	Change      ObjectChangeRequest `json:"change"`
	Fingerprint string              `json:"fingerprint"`
}

type ObjectChangeResult struct {
	Applied        bool              `json:"applied"`
	StatementCount int               `json:"statementCount"`
	Fingerprint    string            `json:"fingerprint"`
	Refresh        []ObjectReference `json:"refresh"`
}

type ObjectChangeDriver interface {
	BuildObjectChange(
		ctx context.Context,
		request ObjectChangeRequest,
	) (ObjectChangePlan, error)
	ApplyObjectChange(
		ctx context.Context,
		plan ObjectChangePlan,
	) error
}

func PreviewObjectChange(
	engine string,
	plan ObjectChangePlan,
) (ObjectChangePreview, error) {
	if err := plan.Validate(); err != nil {
		return ObjectChangePreview{}, err
	}
	warnings := plan.Warnings
	if warnings == nil {
		warnings = make([]string, 0)
	}
	refresh := plan.Refresh
	if refresh == nil {
		refresh = make([]ObjectReference, 0)
	}
	return ObjectChangePreview{
		Summary:        plan.Summary,
		SQL:            strings.Join(plan.Statements, "\n\n"),
		StatementCount: len(plan.Statements),
		Destructive:    plan.Destructive,
		Transactional:  plan.Transactional,
		Warnings:       warnings,
		Fingerprint:    plan.Fingerprint(engine),
		Refresh:        refresh,
	}, nil
}
