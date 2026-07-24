package database

import (
	"context"
	"fmt"
	"strings"
)

type ObjectKind string

const (
	ObjectKindUnknown          ObjectKind = "unknown"
	ObjectKindTable            ObjectKind = "table"
	ObjectKindView             ObjectKind = "view"
	ObjectKindMaterializedView ObjectKind = "materialized_view"
	ObjectKindFunction         ObjectKind = "function"
	ObjectKindProcedure        ObjectKind = "procedure"
	ObjectKindTrigger          ObjectKind = "trigger"
	ObjectKindSequence         ObjectKind = "sequence"
	ObjectKindType             ObjectKind = "type"
	ObjectKindEnum             ObjectKind = "enum"
	ObjectKindDomain           ObjectKind = "domain"
	ObjectKindConstraint       ObjectKind = "constraint"
	ObjectKindExtension        ObjectKind = "extension"
	ObjectKindIndex            ObjectKind = "index"
)

func (kind ObjectKind) Valid() bool {
	switch kind {
	case ObjectKindUnknown,
		ObjectKindTable,
		ObjectKindView,
		ObjectKindMaterializedView,
		ObjectKindFunction,
		ObjectKindProcedure,
		ObjectKindTrigger,
		ObjectKindSequence,
		ObjectKindType,
		ObjectKindEnum,
		ObjectKindDomain,
		ObjectKindConstraint,
		ObjectKindExtension,
		ObjectKindIndex:
		return true
	default:
		return false
	}
}

type ObjectReference struct {
	ID           string     `json:"id,omitempty"`
	Kind         ObjectKind `json:"kind"`
	Schema       string     `json:"schema,omitempty"`
	Name         string     `json:"name"`
	Signature    string     `json:"signature,omitempty"`
	ParentSchema string     `json:"parentSchema,omitempty"`
	ParentName   string     `json:"parentName,omitempty"`
}

func (reference ObjectReference) Validate() error {
	if !reference.Kind.Valid() || reference.Kind == ObjectKindUnknown {
		return fmt.Errorf("a supported database object kind is required")
	}
	if strings.TrimSpace(reference.Name) == "" &&
		strings.TrimSpace(reference.ID) == "" {
		return fmt.Errorf("database object name or ID is required")
	}
	return nil
}

func (reference ObjectReference) QualifiedName() string {
	if reference.Schema == "" {
		return reference.Name
	}
	return reference.Schema + "." + reference.Name
}

type ObjectProperty struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Category string `json:"category,omitempty"`
}

type DatabaseObject struct {
	Reference      ObjectReference      `json:"reference"`
	DisplayName    string               `json:"displayName"`
	Description    string               `json:"description,omitempty"`
	CanOpenData    bool                 `json:"canOpenData"`
	CanManage      bool                 `json:"canManage"`
	AllowedActions []ObjectChangeAction `json:"allowedActions,omitempty"`
	Properties     []ObjectProperty     `json:"properties,omitempty"`
}

type ObjectFilter struct {
	Schema string       `json:"schema,omitempty"`
	Kinds  []ObjectKind `json:"kinds,omitempty"`
	Search string       `json:"search,omitempty"`
}

type ObjectDependency struct {
	Reference   ObjectReference `json:"reference"`
	Description string          `json:"description,omitempty"`
}

type ObjectDetail struct {
	Object       DatabaseObject     `json:"object"`
	Definition   string             `json:"definition,omitempty"`
	Comment      string             `json:"comment,omitempty"`
	Properties   []ObjectProperty   `json:"properties,omitempty"`
	Columns      Structures         `json:"columns,omitempty"`
	Dependencies []ObjectDependency `json:"dependencies,omitempty"`
	Dependents   []ObjectDependency `json:"dependents,omitempty"`
}

type ObjectDriver interface {
	ListObjects(
		ctx context.Context,
		filter ObjectFilter,
	) ([]DatabaseObject, error)
	GetObjectDetail(
		ctx context.Context,
		reference ObjectReference,
	) (ObjectDetail, error)
}
