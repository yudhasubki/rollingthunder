package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type sqlServerPrincipalRow struct {
	name       string
	kind       database.PrincipalKind
	canLogin   bool
	superuser  bool
	createDB   bool
	createRole bool
	locked     bool
	authMethod string
	scope      string
	login      string
}

func sqlServerNullableText(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func (s *SQLServer) sqlServerPrincipals(
	ctx context.Context,
) ([]database.DatabasePrincipal, error) {
	serverRows, err := s.conn.QueryContext(ctx, `
		SELECT
			principal_value.name,
			principal_value.type,
			principal_value.type_desc,
			principal_value.is_disabled,
			CONVERT(bit, COALESCE(
				IS_SRVROLEMEMBER(N'sysadmin', principal_value.name), 0
			)),
			CONVERT(bit, COALESCE(
				IS_SRVROLEMEMBER(N'dbcreator', principal_value.name), 0
			)),
			CONVERT(bit, COALESCE(
				IS_SRVROLEMEMBER(N'securityadmin', principal_value.name), 0
			))
		FROM sys.server_principals principal_value
		WHERE principal_value.type IN ('S', 'U', 'G', 'E', 'X', 'R')
			AND principal_value.name NOT LIKE N'##%##'
			AND principal_value.name <> N'sa'
		ORDER BY principal_value.name`)
	if err != nil {
		return nil, fmt.Errorf("read SQL Server logins: %w", err)
	}
	principals := make([]database.DatabasePrincipal, 0)
	for serverRows.Next() {
		var row sqlServerPrincipalRow
		var principalType string
		var disabled, superuser, createDB, createRole bool
		if err := serverRows.Scan(
			&row.name,
			&principalType,
			&row.authMethod,
			&disabled,
			&superuser,
			&createDB,
			&createRole,
		); err != nil {
			_ = serverRows.Close()
			return nil, err
		}
		kind := database.PrincipalLogin
		canLogin := !disabled
		if principalType == "R" {
			kind = database.PrincipalRole
			canLogin = false
		}
		principals = append(principals, database.DatabasePrincipal{
			Name:       row.name,
			Kind:       kind,
			CanLogin:   canLogin,
			Superuser:  superuser,
			CreateDB:   createDB,
			CreateRole: createRole,
			Inherit:    true,
			Locked:     disabled,
			AuthMethod: row.authMethod,
			Scope:      "server",
		})
	}
	if err := serverRows.Close(); err != nil {
		return nil, err
	}
	if err := serverRows.Err(); err != nil {
		return nil, err
	}

	databaseRows, err := s.conn.QueryContext(ctx, `
		SELECT
			principal_value.name,
			principal_value.type,
			principal_value.type_desc,
			COALESCE(principal_value.authentication_type_desc, N''),
			COALESCE(SUSER_SNAME(principal_value.sid), N''),
			CONVERT(bit, CASE
				WHEN principal_value.type = 'R' THEN 0
				WHEN principal_value.authentication_type = 0 THEN 0
				ELSE 1
			END),
			CONVERT(bit, CASE WHEN EXISTS (
				SELECT 1
				FROM sys.database_permissions permission_value
				WHERE permission_value.grantee_principal_id =
					principal_value.principal_id
					AND permission_value.class = 0
					AND permission_value.permission_name = N'CONNECT'
					AND permission_value.state = 'D'
			) THEN 1 ELSE 0 END),
			CONVERT(bit, COALESCE(
				IS_ROLEMEMBER(N'db_owner', principal_value.name), 0
			)),
			CONVERT(bit, COALESCE(
				IS_ROLEMEMBER(N'db_securityadmin', principal_value.name), 0
			))
		FROM sys.database_principals principal_value
		WHERE principal_value.principal_id > 4
			AND principal_value.type IN ('S', 'U', 'G', 'E', 'X', 'R')
			AND principal_value.name NOT IN (
				N'dbo', N'guest', N'INFORMATION_SCHEMA', N'sys'
			)
		ORDER BY principal_value.type, principal_value.name`)
	if err != nil {
		return nil, fmt.Errorf("read SQL Server database users and roles: %w", err)
	}
	defer databaseRows.Close()
	for databaseRows.Next() {
		var (
			name, principalType, typeDescription, authMethod string
			login                                            sql.NullString
			canLogin, locked, owner, securityAdmin           bool
		)
		if err := databaseRows.Scan(
			&name,
			&principalType,
			&typeDescription,
			&authMethod,
			&login,
			&canLogin,
			&locked,
			&owner,
			&securityAdmin,
		); err != nil {
			return nil, err
		}
		kind := database.PrincipalUser
		if principalType == "R" {
			kind = database.PrincipalRole
			authMethod = typeDescription
			login = sql.NullString{}
		}
		principals = append(principals, database.DatabasePrincipal{
			Name:       name,
			Kind:       kind,
			CanLogin:   canLogin && !locked,
			Superuser:  owner,
			CreateRole: securityAdmin,
			Inherit:    true,
			Locked:     locked,
			AuthMethod: authMethod,
			Scope:      "database",
			Login:      sqlServerNullableText(login),
		})
	}
	if err := databaseRows.Err(); err != nil {
		return nil, err
	}
	return principals, nil
}

func sqlServerGrantObjectType(
	classDescription string,
	objectType string,
) string {
	switch strings.ToUpper(strings.TrimSpace(classDescription)) {
	case "SERVER":
		return "server"
	case "DATABASE":
		return "database"
	case "SCHEMA":
		return "schema"
	case "DATABASE_PRINCIPAL":
		return "principal"
	case "OBJECT_OR_COLUMN":
		switch strings.ToUpper(strings.TrimSpace(objectType)) {
		case "V":
			return "view"
		case "P", "PC":
			return "procedure"
		case "FN", "FS", "FT", "IF", "TF":
			return "function"
		default:
			return "table"
		}
	default:
		return strings.ToLower(strings.TrimSpace(classDescription))
	}
}

func sqlServerGrantState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "D":
		return "deny"
	case "W":
		return "grant_with_option"
	case "G":
		return "grant"
	case "R":
		return "revoke"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

func sqlServerGrantScopes(scope string) (bool, bool, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "":
		return true, true, nil
	case "server":
		return true, false, nil
	case "database":
		return false, true, nil
	default:
		return false, false, fmt.Errorf(
			"SQL Server principal scope must be server or database",
		)
	}
}

func (s *SQLServer) sqlServerGrants(
	ctx context.Context,
	principal string,
	scope string,
) ([]database.DatabaseGrant, error) {
	grants := make([]database.DatabaseGrant, 0)
	includeServer, includeDatabase, err := sqlServerGrantScopes(scope)
	if err != nil {
		return nil, err
	}

	if includeServer {
		serverRows, err := s.conn.QueryContext(ctx, `
		SELECT
			member_value.name,
			role_value.name,
			N'role' AS object_type,
			N'' AS object_schema,
			role_value.name AS object_name,
			N'MEMBER' AS permission_name,
			N'G' AS permission_state,
			N'' AS object_type_code
		FROM sys.server_role_members membership
		JOIN sys.server_principals role_value
			ON role_value.principal_id = membership.role_principal_id
		JOIN sys.server_principals member_value
			ON member_value.principal_id = membership.member_principal_id
		WHERE member_value.name = @p1

		UNION ALL

		SELECT
			principal_value.name,
			N'' AS role_name,
			permission_value.class_desc,
			N'' AS object_schema,
			N'' AS object_name,
			permission_value.permission_name,
			permission_value.state,
			N'' AS object_type_code
		FROM sys.server_permissions permission_value
		JOIN sys.server_principals principal_value
			ON principal_value.principal_id =
				permission_value.grantee_principal_id
		WHERE principal_value.name = @p1
		ORDER BY object_type, object_name, permission_name`,
			principal,
		)
		if err != nil {
			return nil, fmt.Errorf("read SQL Server login grants: %w", err)
		}
		for serverRows.Next() {
			var (
				grantee, role, classDescription, schemaName string
				objectName, privilege, state, objectType    string
			)
			if err := serverRows.Scan(
				&grantee,
				&role,
				&classDescription,
				&schemaName,
				&objectName,
				&privilege,
				&state,
				&objectType,
			); err != nil {
				_ = serverRows.Close()
				return nil, err
			}
			kind := sqlServerGrantObjectType(classDescription, objectType)
			if strings.EqualFold(classDescription, "role") {
				kind = "role"
			}
			grants = append(grants, database.DatabaseGrant{
				Grantee:    grantee,
				Role:       role,
				ObjectType: kind,
				Schema:     schemaName,
				Object:     objectName,
				Privilege:  privilege,
				Grantable:  strings.EqualFold(state, "W"),
				State:      sqlServerGrantState(state),
			})
		}
		if err := serverRows.Close(); err != nil {
			return nil, err
		}
		if err := serverRows.Err(); err != nil {
			return nil, err
		}
	}
	if !includeDatabase {
		return grants, nil
	}

	databaseRows, err := s.conn.QueryContext(ctx, `
		SELECT
			member_value.name,
			role_value.name,
			N'role' AS class_desc,
			N'' AS schema_name,
			role_value.name AS object_name,
			N'MEMBER' AS permission_name,
			N'G' AS permission_state,
			N'' AS object_type_code
		FROM sys.database_role_members membership
		JOIN sys.database_principals role_value
			ON role_value.principal_id = membership.role_principal_id
		JOIN sys.database_principals member_value
			ON member_value.principal_id = membership.member_principal_id
		WHERE member_value.name = @p1

		UNION ALL

		SELECT
			principal_value.name,
			N'' AS role_name,
			permission_value.class_desc,
			CASE
				WHEN permission_value.class = 3 THEN
					SCHEMA_NAME(permission_value.major_id)
				WHEN permission_value.class = 1 THEN
					OBJECT_SCHEMA_NAME(permission_value.major_id)
				ELSE N''
			END AS schema_name,
			CASE
				WHEN permission_value.class = 3 THEN
					SCHEMA_NAME(permission_value.major_id)
				WHEN permission_value.class = 1 THEN
					OBJECT_NAME(permission_value.major_id)
				ELSE N''
			END AS object_name,
			permission_value.permission_name,
			permission_value.state,
			COALESCE(object_value.type, N'') AS object_type_code
		FROM sys.database_permissions permission_value
		JOIN sys.database_principals principal_value
			ON principal_value.principal_id =
				permission_value.grantee_principal_id
		LEFT JOIN sys.objects object_value
			ON permission_value.class = 1
			AND object_value.object_id = permission_value.major_id
		WHERE principal_value.name = @p1
		ORDER BY class_desc, schema_name, object_name, permission_name`,
		principal,
	)
	if err != nil {
		return nil, fmt.Errorf("read SQL Server database grants: %w", err)
	}
	defer databaseRows.Close()
	for databaseRows.Next() {
		var (
			grantee, role, classDescription string
			schemaName, objectName          sql.NullString
			privilege, state, objectType    string
		)
		if err := databaseRows.Scan(
			&grantee,
			&role,
			&classDescription,
			&schemaName,
			&objectName,
			&privilege,
			&state,
			&objectType,
		); err != nil {
			return nil, err
		}
		kind := sqlServerGrantObjectType(classDescription, objectType)
		if strings.EqualFold(classDescription, "role") {
			kind = "role"
		}
		grants = append(grants, database.DatabaseGrant{
			Grantee:    grantee,
			Role:       role,
			ObjectType: kind,
			Schema:     sqlServerNullableText(schemaName),
			Object:     sqlServerNullableText(objectName),
			Privilege:  privilege,
			Grantable:  strings.EqualFold(state, "W"),
			State:      sqlServerGrantState(state),
		})
	}
	if err := databaseRows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

func (s *SQLServer) GetSecurityOverview(
	ctx context.Context,
	principal string,
	scope string,
) (database.SecurityOverview, error) {
	if err := s.ensureConnected(); err != nil {
		return database.SecurityOverview{}, err
	}
	var currentUser string
	if err := s.conn.QueryRowContext(
		ctx,
		`SELECT CONCAT(ORIGINAL_LOGIN(), N' / ', USER_NAME())`,
	).Scan(&currentUser); err != nil {
		return database.SecurityOverview{}, err
	}
	principals, err := s.sqlServerPrincipals(ctx)
	if err != nil {
		return database.SecurityOverview{}, err
	}
	grants := make([]database.DatabaseGrant, 0)
	if principal = strings.TrimSpace(principal); principal != "" {
		grants, err = s.sqlServerGrants(ctx, principal, scope)
		if err != nil {
			return database.SecurityOverview{}, err
		}
	}
	return database.SecurityOverview{
		Supported:   true,
		Engine:      database.DriverSQLServer,
		CurrentUser: currentUser,
		Principals:  principals,
		Grants:      grants,
	}, nil
}

func sqlServerSecurityIdentifier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("SQL Server principal name is required")
	}
	if len(value) > 128 {
		return "", fmt.Errorf("SQL Server principal name exceeds 128 bytes")
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return character < 0x20 || character == 0x7f
	}) >= 0 {
		return "", fmt.Errorf("SQL Server principal name contains control characters")
	}
	return quoteIdentifier(value), nil
}

func sqlServerPassword(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("SQL Server password is required")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("SQL Server password cannot contain line breaks or NUL bytes")
	}
	return "N'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

func sqlServerPrivilege(value string, objectType string) (string, error) {
	privilege := strings.ToUpper(
		strings.Join(strings.Fields(strings.TrimSpace(value)), " "),
	)
	allowed := map[string]map[string]struct{}{
		"server": {
			"ALTER ANY LOGIN": {}, "ALTER ANY SERVER ROLE": {},
			"CONNECT SQL": {}, "CONTROL SERVER": {},
			"VIEW ANY DATABASE": {}, "VIEW SERVER STATE": {},
			"VIEW SERVER PERFORMANCE STATE": {},
		},
		"database": {
			"ALTER ANY ROLE": {}, "ALTER ANY SCHEMA": {},
			"ALTER ANY USER": {}, "BACKUP DATABASE": {}, "BACKUP LOG": {},
			"CONNECT": {}, "CONTROL": {}, "CREATE FUNCTION": {},
			"CREATE PROCEDURE": {}, "CREATE TABLE": {}, "CREATE VIEW": {},
			"VIEW DATABASE STATE": {}, "VIEW DEFINITION": {},
		},
		"schema": {
			"ALTER": {}, "CONTROL": {}, "DELETE": {}, "EXECUTE": {},
			"INSERT": {}, "REFERENCES": {}, "SELECT": {}, "UPDATE": {},
			"VIEW DEFINITION": {},
		},
		"table": {
			"ALTER": {}, "CONTROL": {}, "DELETE": {}, "INSERT": {},
			"REFERENCES": {}, "SELECT": {}, "UPDATE": {},
			"VIEW DEFINITION": {},
		},
		"view": {
			"ALTER": {}, "CONTROL": {}, "DELETE": {}, "INSERT": {},
			"REFERENCES": {}, "SELECT": {}, "UPDATE": {},
			"VIEW DEFINITION": {},
		},
		"procedure": {
			"CONTROL": {}, "EXECUTE": {}, "VIEW DEFINITION": {},
		},
		"function": {
			"CONTROL": {}, "EXECUTE": {}, "REFERENCES": {}, "SELECT": {},
			"VIEW DEFINITION": {},
		},
	}
	byType, exists := allowed[objectType]
	if !exists {
		return "", fmt.Errorf(
			"unsupported SQL Server grant object type %q",
			objectType,
		)
	}
	if _, exists := byType[privilege]; !exists {
		return "", fmt.Errorf(
			"privilege %q is not valid for SQL Server %s grants",
			value,
			objectType,
		)
	}
	return privilege, nil
}

func sqlServerGrantTarget(options database.GrantOptions) (string, string, error) {
	objectType := strings.ToLower(strings.TrimSpace(options.ObjectType))
	privilege, err := sqlServerPrivilege(options.Privilege, objectType)
	if err != nil {
		return "", "", err
	}
	switch objectType {
	case "server", "database":
		return privilege, "", nil
	case "schema":
		schema := strings.TrimSpace(options.Schema)
		if schema == "" {
			schema = strings.TrimSpace(options.Object)
		}
		if schema == "" {
			return "", "", fmt.Errorf("SQL Server schema name is required")
		}
		return privilege, "SCHEMA::" + quoteIdentifier(schema), nil
	case "table", "view", "procedure", "function":
		if strings.TrimSpace(options.Schema) == "" ||
			strings.TrimSpace(options.Object) == "" {
			return "", "", fmt.Errorf(
				"SQL Server %s schema and name are required",
				objectType,
			)
		}
		return privilege,
			"OBJECT::" + quoteQualified(options.Schema, options.Object),
			nil
	default:
		return "", "", fmt.Errorf(
			"unsupported SQL Server grant object type %q",
			objectType,
		)
	}
}

func sqlServerPlan(
	summary string,
	statements []string,
	preview []string,
	destructive bool,
	warnings ...string,
) database.SecurityChangePlan {
	return database.SecurityChangePlan{
		Summary:           summary,
		Statements:        statements,
		PreviewStatements: preview,
		Destructive:       destructive,
		Transactional:     true,
		Warnings:          warnings,
	}
}

func sqlServerServerPlan(
	summary string,
	statements []string,
	preview []string,
	destructive bool,
	warnings ...string,
) database.SecurityChangePlan {
	warnings = append(
		warnings,
		"SQL Server server-scoped security DDL is applied outside a user transaction. If a multi-statement change fails, review the resulting account state before retrying.",
	)
	plan := sqlServerPlan(
		summary,
		statements,
		preview,
		destructive,
		warnings...,
	)
	plan.Transactional = false
	return plan
}

func sqlServerPrincipalScope(
	options database.PrincipalOptions,
) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(options.Scope))
	switch options.Kind {
	case database.PrincipalLogin:
		if scope == "" {
			scope = "server"
		}
		if scope != "server" {
			return "", fmt.Errorf("SQL Server logins are server-scoped")
		}
	case database.PrincipalUser:
		if scope == "" {
			scope = "database"
		}
		if scope != "database" {
			return "", fmt.Errorf("SQL Server users are database-scoped")
		}
	case database.PrincipalRole:
		if scope == "" {
			scope = "database"
		}
		if scope != "server" && scope != "database" {
			return "", fmt.Errorf(
				"SQL Server role scope must be server or database",
			)
		}
	}
	return scope, nil
}

func sqlServerFixedRole(scope string, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	var roles map[string]struct{}
	if scope == "server" {
		roles = map[string]struct{}{
			"bulkadmin": {}, "dbcreator": {}, "diskadmin": {},
			"processadmin": {}, "public": {}, "securityadmin": {},
			"serveradmin": {}, "setupadmin": {}, "sysadmin": {},
		}
	} else {
		roles = map[string]struct{}{
			"db_accessadmin": {}, "db_backupoperator": {},
			"db_datareader": {}, "db_datawriter": {},
			"db_ddladmin": {}, "db_denydatareader": {},
			"db_denydatawriter": {}, "db_owner": {},
			"db_securityadmin": {}, "public": {},
		}
	}
	_, exists := roles[role]
	return exists
}

func (s *SQLServer) sqlServerRoleScope(
	ctx context.Context,
	role string,
	requestedScope string,
) (string, error) {
	var serverRole, databaseRole int
	if err := s.conn.QueryRowContext(
		ctx,
		`SELECT
			CASE WHEN EXISTS (
				SELECT 1 FROM sys.server_principals
				WHERE type = 'R' AND name = @p1
			) THEN 1 ELSE 0 END,
			CASE WHEN EXISTS (
				SELECT 1 FROM sys.database_principals
				WHERE type = 'R' AND name = @p1
			) THEN 1 ELSE 0 END`,
		role,
	).Scan(&serverRole, &databaseRole); err != nil {
		return "", err
	}
	requestedScope = strings.ToLower(strings.TrimSpace(requestedScope))
	switch requestedScope {
	case "server":
		if serverRole == 0 {
			return "", fmt.Errorf(
				"SQL Server server role %q was not found",
				role,
			)
		}
		return "server", nil
	case "database":
		if databaseRole == 0 {
			return "", fmt.Errorf(
				"SQL Server database role %q was not found",
				role,
			)
		}
		return "database", nil
	case "":
	default:
		return "", fmt.Errorf(
			"SQL Server role scope must be server or database",
		)
	}
	switch {
	case serverRole != 0 && databaseRole != 0:
		return "", fmt.Errorf(
			"SQL Server role %q exists at server and database scope; use a uniquely named role",
			role,
		)
	case serverRole != 0:
		return "server", nil
	case databaseRole != 0:
		return "database", nil
	default:
		return "", fmt.Errorf("SQL Server role %q was not found", role)
	}
}

func (s *SQLServer) BuildSecurityChange(
	ctx context.Context,
	request database.SecurityChangeRequest,
) (database.SecurityChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.SecurityChangePlan{}, err
	}
	switch request.Action {
	case database.SecurityCreatePrincipal:
		options := request.Principal
		scope, err := sqlServerPrincipalScope(options)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		principal, err := sqlServerSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		switch options.Kind {
		case database.PrincipalLogin:
			password, err := sqlServerPassword(options.Password)
			if err != nil {
				return database.SecurityChangePlan{}, err
			}
			statement := "CREATE LOGIN " + principal +
				" WITH PASSWORD = " + password + ", CHECK_POLICY = ON;"
			preview := "CREATE LOGIN " + principal +
				" WITH PASSWORD = N'••••••', CHECK_POLICY = ON;"
			statements := []string{statement}
			previews := []string{preview}
			if options.Locked || !options.CanLogin {
				disable := "ALTER LOGIN " + principal + " DISABLE;"
				statements = append(statements, disable)
				previews = append(previews, disable)
			}
			return sqlServerServerPlan(
				"Create SQL Server login "+options.Name,
				statements,
				previews,
				false,
				"Logins are server-scoped. Grant only the minimum server and database access required.",
				"SQL Server audit logs may record account-management statements. The password is redacted only in Rolling Thunder's preview.",
			), nil
		case database.PrincipalRole:
			target := "ROLE"
			if scope == "server" {
				target = "SERVER ROLE"
			}
			statement := "CREATE " + target + " " + principal + ";"
			buildPlan := sqlServerPlan
			if scope == "server" {
				buildPlan = sqlServerServerPlan
			}
			return buildPlan(
				"Create SQL Server "+scope+" role "+options.Name,
				[]string{statement},
				[]string{statement},
				false,
			), nil
		case database.PrincipalUser:
			statement := "CREATE USER " + principal
			preview := statement
			switch {
			case strings.TrimSpace(options.Login) != "":
				login, err := sqlServerSecurityIdentifier(options.Login)
				if err != nil {
					return database.SecurityChangePlan{}, err
				}
				statement += " FOR LOGIN " + login
				preview += " FOR LOGIN " + login
			case options.Password != "":
				password, err := sqlServerPassword(options.Password)
				if err != nil {
					return database.SecurityChangePlan{}, err
				}
				statement += " WITH PASSWORD = " + password
				preview += " WITH PASSWORD = N'••••••'"
			default:
				statement += " WITHOUT LOGIN"
				preview += " WITHOUT LOGIN"
			}
			statement += ";"
			preview += ";"
			statements := []string{statement}
			previews := []string{preview}
			if options.Locked || !options.CanLogin {
				deny := "DENY CONNECT TO " + principal + ";"
				statements = append(statements, deny)
				previews = append(previews, deny)
			}
			return sqlServerPlan(
				"Create SQL Server database user "+options.Name,
				statements,
				previews,
				false,
				"Contained users require database containment support. Map an existing login when containment is not enabled.",
			), nil
		}

	case database.SecurityAlterPrincipal:
		options := request.Principal
		if _, err := sqlServerPrincipalScope(options); err != nil {
			return database.SecurityChangePlan{}, err
		}
		principal, err := sqlServerSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		statements := make([]string, 0, 3)
		previews := make([]string, 0, 3)
		switch options.Kind {
		case database.PrincipalRole:
			return database.SecurityChangePlan{}, fmt.Errorf(
				"SQL Server role membership and privileges are changed through ALTER ROLE, GRANT, and REVOKE",
			)
		case database.PrincipalLogin:
			if options.Password != "" {
				password, err := sqlServerPassword(options.Password)
				if err != nil {
					return database.SecurityChangePlan{}, err
				}
				statements = append(
					statements,
					"ALTER LOGIN "+principal+" WITH PASSWORD = "+password+";",
				)
				previews = append(
					previews,
					"ALTER LOGIN "+principal+" WITH PASSWORD = N'••••••';",
				)
			}
			state := "ENABLE"
			if options.Locked || !options.CanLogin {
				state = "DISABLE"
			}
			statement := "ALTER LOGIN " + principal + " " + state + ";"
			statements = append(statements, statement)
			previews = append(previews, statement)
		case database.PrincipalUser:
			if strings.TrimSpace(options.Login) != "" {
				login, err := sqlServerSecurityIdentifier(options.Login)
				if err != nil {
					return database.SecurityChangePlan{}, err
				}
				statement := "ALTER USER " + principal + " WITH LOGIN = " +
					login + ";"
				statements = append(statements, statement)
				previews = append(previews, statement)
			}
			if options.Password != "" {
				password, err := sqlServerPassword(options.Password)
				if err != nil {
					return database.SecurityChangePlan{}, err
				}
				statements = append(
					statements,
					"ALTER USER "+principal+" WITH PASSWORD = "+password+";",
				)
				previews = append(
					previews,
					"ALTER USER "+principal+" WITH PASSWORD = N'••••••';",
				)
			}
			verb := "GRANT"
			if options.Locked || !options.CanLogin {
				verb = "DENY"
			}
			statement := verb + " CONNECT TO " + principal + ";"
			statements = append(statements, statement)
			previews = append(previews, statement)
		}
		buildPlan := sqlServerPlan
		if options.Kind == database.PrincipalLogin {
			buildPlan = sqlServerServerPlan
		}
		return buildPlan(
			"Alter SQL Server "+string(options.Kind)+" "+options.Name,
			statements,
			previews,
			false,
			"Account changes can immediately affect new connection attempts.",
		), nil

	case database.SecurityDropPrincipal:
		options := request.Principal
		scope, err := sqlServerPrincipalScope(options)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		principal, err := sqlServerSecurityIdentifier(options.Name)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		kind := "USER"
		warning := "SQL Server refuses to drop a database principal with dependent ownership."
		if options.Kind == database.PrincipalRole {
			kind = "ROLE"
			if sqlServerFixedRole(scope, options.Name) {
				return database.SecurityChangePlan{}, fmt.Errorf(
					"SQL Server fixed %s role %q cannot be dropped",
					scope,
					options.Name,
				)
			}
			if scope == "server" {
				kind = "SERVER ROLE"
			}
		} else if options.Kind == database.PrincipalLogin {
			kind = "LOGIN"
			warning = "Dropping a server login can orphan users in multiple databases. Rolling Thunder never drops mapped database users automatically."
		}
		statement := "DROP " + kind + " " + principal + ";"
		buildPlan := sqlServerPlan
		if options.Kind == database.PrincipalLogin ||
			(options.Kind == database.PrincipalRole && scope == "server") {
			buildPlan = sqlServerServerPlan
		}
		return buildPlan(
			"Drop SQL Server "+strings.ToLower(kind)+" "+options.Name,
			[]string{statement},
			[]string{statement},
			true,
			warning,
		), nil

	case database.SecurityGrantRole, database.SecurityRevokeRole:
		if request.Grant.Grantable {
			return database.SecurityChangePlan{}, fmt.Errorf(
				"SQL Server role membership does not expose a per-membership admin option",
			)
		}
		role, err := sqlServerSecurityIdentifier(request.Grant.Role)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		grantee, err := sqlServerSecurityIdentifier(request.Grant.Grantee)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		scope, err := s.sqlServerRoleScope(
			ctx,
			request.Grant.Role,
			request.Grant.ObjectType,
		)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		target := "ROLE"
		if scope == "server" {
			target = "SERVER ROLE"
		}
		action := "ADD"
		summaryVerb := "Grant"
		destructive := false
		if request.Action == database.SecurityRevokeRole {
			action = "DROP"
			summaryVerb = "Revoke"
			destructive = true
		}
		statement := "ALTER " + target + " " + role + " " + action +
			" MEMBER " + grantee + ";"
		buildPlan := sqlServerPlan
		if scope == "server" {
			buildPlan = sqlServerServerPlan
		}
		return buildPlan(
			summaryVerb+" SQL Server "+scope+" role "+
				request.Grant.Role+" for "+request.Grant.Grantee,
			[]string{statement},
			[]string{statement},
			destructive,
		), nil

	case database.SecurityGrantPrivilege, database.SecurityRevokePrivilege:
		grant := request.Grant
		privilege, target, err := sqlServerGrantTarget(grant)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		grantee, err := sqlServerSecurityIdentifier(grant.Grantee)
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
			suffix = " WITH GRANT OPTION"
		}
		onTarget := ""
		if target != "" {
			onTarget = " ON " + target
		}
		statement := verb + " " + privilege + onTarget + middle +
			grantee + suffix + ";"
		buildPlan := sqlServerPlan
		if strings.EqualFold(grant.ObjectType, "server") {
			buildPlan = sqlServerServerPlan
		}
		return buildPlan(
			fmt.Sprintf(
				"%s SQL Server %s for %s",
				summaryVerb,
				privilege,
				grant.Grantee,
			),
			[]string{statement},
			[]string{statement},
			destructive,
			"Server- and database-level permissions can affect objects outside the currently visible schema.",
		), nil
	}
	return database.SecurityChangePlan{}, fmt.Errorf(
		"unsupported SQL Server security action %q",
		request.Action,
	)
}

func (s *SQLServer) ApplySecurityChange(
	ctx context.Context,
	plan database.SecurityChangePlan,
) error {
	if err := s.ensureConnected(); err != nil {
		return err
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	if !plan.Transactional {
		for index, statement := range plan.Statements {
			if _, err := s.conn.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf(
					"execute SQL Server security statement %d of %d: %w",
					index+1,
					len(plan.Statements),
					err,
				)
			}
		}
		return nil
	}
	transaction, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()
	for index, statement := range plan.Statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf(
				"execute SQL Server security statement %d of %d: %w",
				index+1,
				len(plan.Statements),
				err,
			)
		}
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

var _ database.SecurityDriver = (*SQLServer)(nil)
