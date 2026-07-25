package database

import (
	"context"
	"fmt"
	"strings"
)

type PrincipalKind string

const (
	PrincipalRole PrincipalKind = "role"
	PrincipalUser PrincipalKind = "user"
)

type DatabasePrincipal struct {
	Name        string        `json:"name"`
	Host        string        `json:"host,omitempty"`
	Kind        PrincipalKind `json:"kind"`
	CanLogin    bool          `json:"canLogin"`
	Superuser   bool          `json:"superuser"`
	CreateDB    bool          `json:"createDb"`
	CreateRole  bool          `json:"createRole"`
	Inherit     bool          `json:"inherit"`
	Replication bool          `json:"replication"`
	BypassRLS   bool          `json:"bypassRls"`
	Locked      bool          `json:"locked"`
	AuthMethod  string        `json:"authMethod,omitempty"`
}

type DatabaseGrant struct {
	Grantee    string `json:"grantee"`
	Role       string `json:"role,omitempty"`
	ObjectType string `json:"objectType"`
	Schema     string `json:"schema,omitempty"`
	Object     string `json:"object,omitempty"`
	Privilege  string `json:"privilege"`
	Grantable  bool   `json:"grantable"`
	Statement  string `json:"statement,omitempty"`
}

type SecurityOverview struct {
	Supported   bool                `json:"supported"`
	Engine      string              `json:"engine"`
	CurrentUser string              `json:"currentUser"`
	Principals  []DatabasePrincipal `json:"principals"`
	Grants      []DatabaseGrant     `json:"grants"`
	Message     string              `json:"message,omitempty"`
}

type SecurityChangeAction string

const (
	SecurityCreatePrincipal SecurityChangeAction = "create_principal"
	SecurityAlterPrincipal  SecurityChangeAction = "alter_principal"
	SecurityDropPrincipal   SecurityChangeAction = "drop_principal"
	SecurityGrantPrivilege  SecurityChangeAction = "grant_privilege"
	SecurityRevokePrivilege SecurityChangeAction = "revoke_privilege"
	SecurityGrantRole       SecurityChangeAction = "grant_role"
	SecurityRevokeRole      SecurityChangeAction = "revoke_role"
)

func (action SecurityChangeAction) Valid() bool {
	switch action {
	case SecurityCreatePrincipal,
		SecurityAlterPrincipal,
		SecurityDropPrincipal,
		SecurityGrantPrivilege,
		SecurityRevokePrivilege,
		SecurityGrantRole,
		SecurityRevokeRole:
		return true
	default:
		return false
	}
}

type PrincipalOptions struct {
	Name        string        `json:"name"`
	Host        string        `json:"host,omitempty"`
	Kind        PrincipalKind `json:"kind"`
	Password    string        `json:"password,omitempty"`
	CanLogin    bool          `json:"canLogin"`
	Superuser   bool          `json:"superuser"`
	CreateDB    bool          `json:"createDb"`
	CreateRole  bool          `json:"createRole"`
	Inherit     bool          `json:"inherit"`
	Replication bool          `json:"replication"`
	BypassRLS   bool          `json:"bypassRls"`
	Locked      bool          `json:"locked"`
}

type GrantOptions struct {
	Grantee     string `json:"grantee"`
	GranteeHost string `json:"granteeHost,omitempty"`
	Role        string `json:"role,omitempty"`
	RoleHost    string `json:"roleHost,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	Schema      string `json:"schema,omitempty"`
	Object      string `json:"object,omitempty"`
	Privilege   string `json:"privilege,omitempty"`
	Grantable   bool   `json:"grantable"`
}

type SecurityChangeRequest struct {
	Action    SecurityChangeAction `json:"action"`
	Principal PrincipalOptions     `json:"principal"`
	Grant     GrantOptions         `json:"grant"`
}

func (request SecurityChangeRequest) Validate() error {
	if !request.Action.Valid() {
		return fmt.Errorf("unsupported security action %q", request.Action)
	}
	switch request.Action {
	case SecurityCreatePrincipal,
		SecurityAlterPrincipal,
		SecurityDropPrincipal:
		if strings.TrimSpace(request.Principal.Name) == "" {
			return fmt.Errorf("principal name is required")
		}
		if request.Principal.Kind != PrincipalRole &&
			request.Principal.Kind != PrincipalUser {
			return fmt.Errorf("principal kind must be role or user")
		}
	case SecurityGrantRole, SecurityRevokeRole:
		if strings.TrimSpace(request.Grant.Grantee) == "" ||
			strings.TrimSpace(request.Grant.Role) == "" {
			return fmt.Errorf("role and grantee are required")
		}
	case SecurityGrantPrivilege, SecurityRevokePrivilege:
		if strings.TrimSpace(request.Grant.Grantee) == "" {
			return fmt.Errorf("grant grantee is required")
		}
		if strings.TrimSpace(request.Grant.ObjectType) == "" ||
			strings.TrimSpace(request.Grant.Privilege) == "" {
			return fmt.Errorf("grant object type and privilege are required")
		}
	}
	return nil
}

type SecurityChangePlan struct {
	Summary           string
	Statements        []string
	PreviewStatements []string
	Destructive       bool
	Transactional     bool
	Warnings          []string
}

func (plan SecurityChangePlan) Validate() error {
	objectPlan := ObjectChangePlan{
		Summary:       plan.Summary,
		Statements:    plan.Statements,
		Destructive:   plan.Destructive,
		Transactional: plan.Transactional,
		Warnings:      plan.Warnings,
	}
	if err := objectPlan.Validate(); err != nil {
		return err
	}
	if len(plan.PreviewStatements) != len(plan.Statements) {
		return fmt.Errorf("security preview statement count does not match execution plan")
	}
	return nil
}

func (plan SecurityChangePlan) Fingerprint(engine string) string {
	return (ObjectChangePlan{
		Summary:       plan.Summary,
		Statements:    plan.Statements,
		Destructive:   plan.Destructive,
		Transactional: plan.Transactional,
		Warnings:      plan.Warnings,
	}).Fingerprint(engine)
}

type SecurityChangePreview struct {
	Summary        string   `json:"summary"`
	SQL            string   `json:"sql"`
	StatementCount int      `json:"statementCount"`
	Destructive    bool     `json:"destructive"`
	Transactional  bool     `json:"transactional"`
	Warnings       []string `json:"warnings"`
	Fingerprint    string   `json:"fingerprint"`
}

type ApplySecurityChangeRequest struct {
	Change      SecurityChangeRequest `json:"change"`
	Fingerprint string                `json:"fingerprint"`
}

type SecurityChangeResult struct {
	Applied        bool   `json:"applied"`
	StatementCount int    `json:"statementCount"`
	Fingerprint    string `json:"fingerprint"`
}

type SecurityDriver interface {
	GetSecurityOverview(
		ctx context.Context,
		principal string,
		host string,
	) (SecurityOverview, error)
	BuildSecurityChange(
		ctx context.Context,
		request SecurityChangeRequest,
	) (SecurityChangePlan, error)
	ApplySecurityChange(
		ctx context.Context,
		plan SecurityChangePlan,
	) error
}
