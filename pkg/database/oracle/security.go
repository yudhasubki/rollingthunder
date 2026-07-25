package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

const oraclePrincipalsQuery = `
	SELECT
		principal_name,
		principal_kind,
		can_login,
		is_superuser,
		can_create_role,
		is_locked,
		authentication_method
	FROM (
		SELECT
			user_object.username AS principal_name,
			'user' AS principal_kind,
			CASE WHEN user_object.account_status = 'OPEN' THEN 1 ELSE 0 END AS can_login,
			CASE WHEN EXISTS (
				SELECT 1
				FROM dba_role_privs role_grant
				WHERE role_grant.grantee = user_object.username
					AND role_grant.granted_role = 'DBA'
			) THEN 1 ELSE 0 END AS is_superuser,
			CASE WHEN EXISTS (
				SELECT 1
				FROM dba_sys_privs system_grant
				WHERE system_grant.grantee = user_object.username
					AND system_grant.privilege IN (
						'CREATE ROLE', 'GRANT ANY ROLE'
					)
			) THEN 1 ELSE 0 END AS can_create_role,
			CASE WHEN user_object.account_status LIKE '%LOCKED%' THEN 1 ELSE 0 END AS is_locked,
			NVL(user_object.authentication_type, '') AS authentication_method
		FROM dba_users user_object
		WHERE user_object.oracle_maintained = 'N'
			OR user_object.username = SYS_CONTEXT('USERENV', 'SESSION_USER')

		UNION ALL

		SELECT
			role_object.role AS principal_name,
			'role' AS principal_kind,
			0 AS can_login,
			CASE WHEN role_object.role = 'DBA' THEN 1 ELSE 0 END AS is_superuser,
			0 AS can_create_role,
			0 AS is_locked,
			NVL(role_object.authentication_type, '') AS authentication_method
		FROM dba_roles role_object
		WHERE role_object.oracle_maintained = 'N'
			OR role_object.role IN ('DBA', 'CONNECT', 'RESOURCE', 'PDB_DBA')
	)
	ORDER BY principal_kind, principal_name`

const oracleGrantsQuery = `
	SELECT
		grantee,
		role_name,
		object_type,
		object_schema,
		object_name,
		privilege_name,
		is_grantable
	FROM (
		SELECT
			role_grant.grantee AS grantee,
			role_grant.granted_role AS role_name,
			'role' AS object_type,
			'' AS object_schema,
			role_grant.granted_role AS object_name,
			'MEMBER' AS privilege_name,
			CASE WHEN role_grant.admin_option = 'YES' THEN 1 ELSE 0 END AS is_grantable
		FROM dba_role_privs role_grant
		WHERE role_grant.grantee = :principal

		UNION ALL

		SELECT
			system_grant.grantee AS grantee,
			'' AS role_name,
			'system' AS object_type,
			'' AS object_schema,
			'' AS object_name,
			system_grant.privilege AS privilege_name,
			CASE WHEN system_grant.admin_option = 'YES' THEN 1 ELSE 0 END AS is_grantable
		FROM dba_sys_privs system_grant
		WHERE system_grant.grantee = :principal

		UNION ALL

		SELECT
			object_grant.grantee AS grantee,
			'' AS role_name,
			LOWER(NVL(object_grant.type, 'TABLE')) AS object_type,
			object_grant.owner AS object_schema,
			object_grant.table_name AS object_name,
			object_grant.privilege AS privilege_name,
			CASE WHEN object_grant.grantable = 'YES' THEN 1 ELSE 0 END AS is_grantable
		FROM dba_tab_privs object_grant
		WHERE object_grant.grantee = :principal
	)
	ORDER BY object_type, object_schema, object_name, privilege_name`

type oraclePrincipalRow struct {
	Name       string
	Kind       string
	CanLogin   int
	Superuser  int
	CreateRole int
	Locked     int
	AuthMethod string
}

type oracleGrantRow struct {
	Grantee    string
	Role       string
	ObjectType string
	Schema     string
	Object     string
	Privilege  string
	Grantable  int
}

func (o *Oracle) GetSecurityOverview(
	ctx context.Context,
	principal string,
	_ string,
) (database.SecurityOverview, error) {
	if err := o.ensureConnected(); err != nil {
		return database.SecurityOverview{}, err
	}
	var currentUser string
	if err := o.conn.QueryRowContext(
		ctx,
		"SELECT SYS_CONTEXT('USERENV', 'SESSION_USER') FROM dual",
	).Scan(&currentUser); err != nil {
		return database.SecurityOverview{}, err
	}
	rows, err := o.conn.QueryContext(ctx, oraclePrincipalsQuery)
	if err != nil {
		return database.SecurityOverview{}, err
	}
	principals := make([]database.DatabasePrincipal, 0)
	for rows.Next() {
		var row oraclePrincipalRow
		if err := rows.Scan(
			&row.Name,
			&row.Kind,
			&row.CanLogin,
			&row.Superuser,
			&row.CreateRole,
			&row.Locked,
			&row.AuthMethod,
		); err != nil {
			_ = rows.Close()
			return database.SecurityOverview{}, err
		}
		kind := database.PrincipalUser
		if strings.EqualFold(row.Kind, string(database.PrincipalRole)) {
			kind = database.PrincipalRole
		}
		principals = append(principals, database.DatabasePrincipal{
			Name:       row.Name,
			Kind:       kind,
			CanLogin:   row.CanLogin != 0,
			Superuser:  row.Superuser != 0,
			CreateRole: row.CreateRole != 0,
			Inherit:    true,
			Locked:     row.Locked != 0,
			AuthMethod: row.AuthMethod,
		})
	}
	if err := rows.Close(); err != nil {
		return database.SecurityOverview{}, err
	}
	if err := rows.Err(); err != nil {
		return database.SecurityOverview{}, err
	}

	grants := make([]database.DatabaseGrant, 0)
	principal = strings.ToUpper(strings.TrimSpace(principal))
	if principal != "" {
		grantRows, err := o.conn.QueryContext(
			ctx,
			oracleGrantsQuery,
			sql.Named("principal", principal),
		)
		if err != nil {
			return database.SecurityOverview{}, err
		}
		for grantRows.Next() {
			var row oracleGrantRow
			if err := grantRows.Scan(
				&row.Grantee,
				&row.Role,
				&row.ObjectType,
				&row.Schema,
				&row.Object,
				&row.Privilege,
				&row.Grantable,
			); err != nil {
				_ = grantRows.Close()
				return database.SecurityOverview{}, err
			}
			grants = append(grants, database.DatabaseGrant{
				Grantee:    row.Grantee,
				Role:       row.Role,
				ObjectType: row.ObjectType,
				Schema:     row.Schema,
				Object:     row.Object,
				Privilege:  row.Privilege,
				Grantable:  row.Grantable != 0,
			})
		}
		if err := grantRows.Close(); err != nil {
			return database.SecurityOverview{}, err
		}
		if err := grantRows.Err(); err != nil {
			return database.SecurityOverview{}, err
		}
	}
	return database.SecurityOverview{
		Supported:   true,
		Engine:      database.DriverOracle,
		CurrentUser: currentUser,
		Principals:  principals,
		Grants:      grants,
	}, nil
}

func oracleSecurityIdentifier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("Oracle principal name is required")
	}
	if len(value) > 128 {
		return "", fmt.Errorf("Oracle principal name exceeds 128 bytes")
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return character < 0x20 || character == 0x7f
	}) >= 0 {
		return "", fmt.Errorf("Oracle principal name contains control characters")
	}
	return quoteIdentifier(strings.ToUpper(value)), nil
}

func oraclePassword(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("Oracle password cannot contain line breaks or NUL bytes")
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`, nil
}

func oraclePrivilege(value string, objectType string) (string, error) {
	privilege := strings.ToUpper(
		strings.Join(strings.Fields(strings.TrimSpace(value)), " "),
	)
	allowed := map[string]map[string]struct{}{
		"system": {
			"ALTER SYSTEM": {}, "ALTER USER": {}, "CREATE ANY INDEX": {},
			"CREATE PROCEDURE": {}, "CREATE ROLE": {}, "CREATE SEQUENCE": {},
			"CREATE SESSION": {}, "CREATE TABLE": {}, "CREATE TRIGGER": {},
			"CREATE TYPE": {}, "CREATE USER": {}, "CREATE VIEW": {},
			"DELETE ANY TABLE": {}, "DROP USER": {}, "EXECUTE ANY PROCEDURE": {},
			"GRANT ANY ROLE": {}, "INSERT ANY TABLE": {},
			"SELECT ANY DICTIONARY": {}, "SELECT ANY TABLE": {},
			"UPDATE ANY TABLE": {},
		},
		"table": {
			"ALTER": {}, "DELETE": {}, "FLASHBACK": {}, "INDEX": {},
			"INSERT": {}, "READ": {}, "REFERENCES": {}, "SELECT": {},
			"UPDATE": {},
		},
		"view": {
			"DELETE": {}, "INSERT": {}, "READ": {}, "SELECT": {}, "UPDATE": {},
		},
		"materialized view": {
			"SELECT": {}, "READ": {},
		},
		"sequence": {
			"ALTER": {}, "SELECT": {},
		},
		"procedure": {
			"DEBUG": {}, "EXECUTE": {},
		},
		"function": {
			"DEBUG": {}, "EXECUTE": {},
		},
		"package": {
			"DEBUG": {}, "EXECUTE": {},
		},
		"type": {
			"DEBUG": {}, "EXECUTE": {}, "UNDER": {},
		},
		"directory": {
			"EXECUTE": {}, "READ": {}, "WRITE": {},
		},
	}
	byType, exists := allowed[objectType]
	if !exists {
		return "", fmt.Errorf(
			"unsupported Oracle grant object type %q",
			objectType,
		)
	}
	if _, exists := byType[privilege]; !exists {
		return "", fmt.Errorf(
			"privilege %q is not valid for Oracle %s grants",
			value,
			objectType,
		)
	}
	return privilege, nil
}

func oracleGrantTarget(options database.GrantOptions) (string, string, error) {
	objectType := strings.ToLower(strings.TrimSpace(options.ObjectType))
	privilege, err := oraclePrivilege(options.Privilege, objectType)
	if err != nil {
		return "", "", err
	}
	if objectType == "system" {
		return privilege, "", nil
	}
	if strings.TrimSpace(options.Object) == "" {
		return "", "", fmt.Errorf("Oracle grant object name is required")
	}
	if objectType == "directory" {
		return privilege, quoteIdentifier(options.Object), nil
	}
	if strings.TrimSpace(options.Schema) == "" {
		return "", "", fmt.Errorf("Oracle grant schema is required")
	}
	return privilege, quoteQualified(options.Schema, options.Object), nil
}

func (o *Oracle) BuildSecurityChange(
	_ context.Context,
	request database.SecurityChangeRequest,
) (database.SecurityChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.SecurityChangePlan{}, err
	}
	switch request.Action {
	case database.SecurityCreatePrincipal:
		options := request.Principal
		principal, err := oracleSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		if options.Kind == database.PrincipalRole {
			statement := "CREATE ROLE " + principal + ";"
			return database.SecurityChangePlan{
				Summary:           "Create Oracle role " + options.Name,
				Statements:        []string{statement},
				PreviewStatements: []string{statement},
				Transactional:     false,
				Warnings: []string{
					"Oracle role DDL auto-commits.",
				},
			}, nil
		}
		if options.Password == "" {
			return database.SecurityChangePlan{}, fmt.Errorf(
				"Oracle users require an initial password",
			)
		}
		password, err := oraclePassword(options.Password)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		lock := options.Locked || !options.CanLogin
		statement := "CREATE USER " + principal + " IDENTIFIED BY " + password
		preview := "CREATE USER " + principal + ` IDENTIFIED BY "••••••"`
		if lock {
			statement += " ACCOUNT LOCK"
			preview += " ACCOUNT LOCK"
		}
		statement += ";"
		preview += ";"
		return database.SecurityChangePlan{
			Summary:           "Create Oracle user " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{preview},
			Transactional:     false,
			Warnings: []string{
				"Oracle user DDL auto-commits. Grant CREATE SESSION only when this account should log in.",
				"Database audit logs may record account-management statements. The password is redacted only in Rolling Thunder's preview.",
			},
		}, nil

	case database.SecurityAlterPrincipal:
		options := request.Principal
		if options.Kind == database.PrincipalRole {
			return database.SecurityChangePlan{}, fmt.Errorf(
				"Oracle role membership and privileges are changed through GRANT and REVOKE",
			)
		}
		principal, err := oracleSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		statement := "ALTER USER " + principal
		preview := statement
		if options.Password != "" {
			password, err := oraclePassword(options.Password)
			if err != nil {
				return database.SecurityChangePlan{}, err
			}
			statement += " IDENTIFIED BY " + password
			preview += ` IDENTIFIED BY "••••••"`
		}
		if options.Locked || !options.CanLogin {
			statement += " ACCOUNT LOCK"
			preview += " ACCOUNT LOCK"
		} else {
			statement += " ACCOUNT UNLOCK"
			preview += " ACCOUNT UNLOCK"
		}
		statement += ";"
		preview += ";"
		return database.SecurityChangePlan{
			Summary:           "Alter Oracle user " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{preview},
			Transactional:     false,
			Warnings: []string{
				"Oracle account changes auto-commit and can immediately affect active sessions.",
			},
		}, nil

	case database.SecurityDropPrincipal:
		options := request.Principal
		principal, err := oracleSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		kind := "USER"
		if options.Kind == database.PrincipalRole {
			kind = "ROLE"
		}
		statement := "DROP " + kind + " " + principal + ";"
		return database.SecurityChangePlan{
			Summary:           "Drop Oracle " + strings.ToLower(kind) + " " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       true,
			Transactional:     false,
			Warnings: []string{
				"Oracle security DDL auto-commits and cannot be rolled back.",
				"Dropping a user without CASCADE is intentionally refused when it still owns database objects.",
			},
		}, nil

	case database.SecurityGrantRole, database.SecurityRevokeRole:
		grant := request.Grant
		role, err := oracleSecurityIdentifier(grant.Role)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		grantee, err := oracleSecurityIdentifier(grant.Grantee)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		verb := "GRANT"
		middle := " TO "
		summaryVerb := "Grant"
		destructive := false
		suffix := ""
		if request.Action == database.SecurityRevokeRole {
			verb = "REVOKE"
			middle = " FROM "
			summaryVerb = "Revoke"
			destructive = true
		} else if grant.Grantable {
			suffix = " WITH ADMIN OPTION"
		}
		statement := verb + " " + role + middle + grantee + suffix + ";"
		return database.SecurityChangePlan{
			Summary:           summaryVerb + " Oracle role " + grant.Role + " for " + grant.Grantee,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       destructive,
			Transactional:     false,
			Warnings: []string{
				"Oracle role grants auto-commit.",
			},
		}, nil

	case database.SecurityGrantPrivilege, database.SecurityRevokePrivilege:
		grant := request.Grant
		objectType := strings.ToLower(strings.TrimSpace(grant.ObjectType))
		privilege, target, err := oracleGrantTarget(grant)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		grantee, err := oracleSecurityIdentifier(grant.Grantee)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		verb := "GRANT"
		middle := " TO "
		summaryVerb := "Grant"
		destructive := false
		suffix := ""
		if request.Action == database.SecurityRevokePrivilege {
			verb = "REVOKE"
			middle = " FROM "
			summaryVerb = "Revoke"
			destructive = true
		} else if grant.Grantable {
			if objectType == "system" {
				suffix = " WITH ADMIN OPTION"
			} else {
				suffix = " WITH GRANT OPTION"
			}
		}
		onTarget := ""
		if target != "" {
			onTarget = " ON " + target
		}
		statement := verb + " " + privilege + onTarget + middle +
			grantee + suffix + ";"
		return database.SecurityChangePlan{
			Summary: fmt.Sprintf(
				"%s Oracle %s for %s",
				summaryVerb,
				privilege,
				grant.Grantee,
			),
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       destructive,
			Transactional:     false,
			Warnings: []string{
				"Oracle privilege changes auto-commit. System privileges can affect the complete database.",
			},
		}, nil
	default:
		return database.SecurityChangePlan{}, fmt.Errorf(
			"unsupported Oracle security action %q",
			request.Action,
		)
	}
}

func (o *Oracle) ApplySecurityChange(
	ctx context.Context,
	plan database.SecurityChangePlan,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	for index, statement := range plan.Statements {
		if _, err := o.conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf(
				"execute Oracle security statement %d of %d: %w",
				index+1,
				len(plan.Statements),
				err,
			)
		}
	}
	return nil
}

var _ database.SecurityDriver = (*Oracle)(nil)
