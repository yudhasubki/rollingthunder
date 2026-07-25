package postgres

import (
	"context"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type postgresPrincipalRow struct {
	Name        string `db:"name"`
	CanLogin    bool   `db:"can_login"`
	Superuser   bool   `db:"superuser"`
	CreateDB    bool   `db:"create_db"`
	CreateRole  bool   `db:"create_role"`
	Inherit     bool   `db:"inherit_role"`
	Replication bool   `db:"replication"`
	BypassRLS   bool   `db:"bypass_rls"`
}

type postgresGrantRow struct {
	Grantee    string `db:"grantee"`
	Role       string `db:"role_name"`
	ObjectType string `db:"object_type"`
	Schema     string `db:"object_schema"`
	Object     string `db:"object_name"`
	Privilege  string `db:"privilege"`
	Grantable  bool   `db:"grantable"`
}

func (p *Postgres) GetSecurityOverview(
	ctx context.Context,
	principal string,
	_ string,
) (database.SecurityOverview, error) {
	if p.conn == nil {
		return database.SecurityOverview{}, fmt.Errorf("PostgreSQL connection is not open")
	}
	var currentUser string
	if err := p.conn.GetContext(ctx, &currentUser, "SELECT current_user"); err != nil {
		return database.SecurityOverview{}, err
	}
	var rows []postgresPrincipalRow
	const principalsQuery = `
		SELECT
			rolname AS name,
			rolcanlogin AS can_login,
			rolsuper AS superuser,
			rolcreatedb AS create_db,
			rolcreaterole AS create_role,
			rolinherit AS inherit_role,
			rolreplication AS replication,
			rolbypassrls AS bypass_rls
		FROM pg_catalog.pg_roles
		WHERE rolname !~ '^pg_'
		ORDER BY rolname`
	if err := p.conn.SelectContext(ctx, &rows, principalsQuery); err != nil {
		return database.SecurityOverview{}, err
	}
	principals := make([]database.DatabasePrincipal, 0, len(rows))
	for _, row := range rows {
		kind := database.PrincipalRole
		if row.CanLogin {
			kind = database.PrincipalUser
		}
		principals = append(principals, database.DatabasePrincipal{
			Name:        row.Name,
			Kind:        kind,
			CanLogin:    row.CanLogin,
			Superuser:   row.Superuser,
			CreateDB:    row.CreateDB,
			CreateRole:  row.CreateRole,
			Inherit:     row.Inherit,
			Replication: row.Replication,
			BypassRLS:   row.BypassRLS,
		})
	}

	grants := make([]database.DatabaseGrant, 0)
	principal = strings.TrimSpace(principal)
	if principal != "" {
		var grantRows []postgresGrantRow
		const grantsQuery = `
			SELECT
				grantee,
				''::text AS role_name,
				'TABLE'::text AS object_type,
				table_schema AS object_schema,
				table_name AS object_name,
				privilege_type AS privilege,
				is_grantable = 'YES' AS grantable
			FROM information_schema.role_table_grants
			WHERE grantee = $1
			UNION ALL
			SELECT
				member.rolname AS grantee,
				role.rolname AS role_name,
				'ROLE'::text AS object_type,
				''::text AS object_schema,
				role.rolname AS object_name,
				'MEMBER'::text AS privilege,
				membership.admin_option AS grantable
			FROM pg_catalog.pg_auth_members membership
			JOIN pg_catalog.pg_roles role ON role.oid = membership.roleid
			JOIN pg_catalog.pg_roles member ON member.oid = membership.member
			WHERE member.rolname = $1
			ORDER BY object_type, object_schema, object_name, privilege`
		if err := p.conn.SelectContext(
			ctx,
			&grantRows,
			grantsQuery,
			principal,
		); err != nil {
			return database.SecurityOverview{}, err
		}
		for _, row := range grantRows {
			grants = append(grants, database.DatabaseGrant{
				Grantee:    row.Grantee,
				Role:       row.Role,
				ObjectType: strings.ToLower(row.ObjectType),
				Schema:     row.Schema,
				Object:     row.Object,
				Privilege:  row.Privilege,
				Grantable:  row.Grantable,
			})
		}
	}
	return database.SecurityOverview{
		Supported:   true,
		Engine:      "postgres",
		CurrentUser: currentUser,
		Principals:  principals,
		Grants:      grants,
	}, nil
}

func postgresPrincipalAttributes(options database.PrincipalOptions) []string {
	login := options.CanLogin || options.Kind == database.PrincipalUser
	attributes := []string{
		map[bool]string{true: "LOGIN", false: "NOLOGIN"}[login],
		map[bool]string{true: "SUPERUSER", false: "NOSUPERUSER"}[options.Superuser],
		map[bool]string{true: "CREATEDB", false: "NOCREATEDB"}[options.CreateDB],
		map[bool]string{true: "CREATEROLE", false: "NOCREATEROLE"}[options.CreateRole],
		map[bool]string{true: "INHERIT", false: "NOINHERIT"}[options.Inherit],
		map[bool]string{true: "REPLICATION", false: "NOREPLICATION"}[options.Replication],
		map[bool]string{true: "BYPASSRLS", false: "NOBYPASSRLS"}[options.BypassRLS],
	}
	return attributes
}

func postgresPrivilege(value string, objectType string) (string, error) {
	privilege := strings.ToUpper(strings.TrimSpace(value))
	allowed := map[string]map[string]struct{}{
		"database": {
			"CONNECT": {}, "CREATE": {}, "TEMPORARY": {}, "TEMP": {},
		},
		"schema": {
			"USAGE": {}, "CREATE": {},
		},
		"table": {
			"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {},
			"TRUNCATE": {}, "REFERENCES": {}, "TRIGGER": {}, "ALL": {},
		},
		"all_tables_in_schema": {
			"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {},
			"TRUNCATE": {}, "REFERENCES": {}, "TRIGGER": {}, "ALL": {},
		},
		"sequence": {
			"USAGE": {}, "SELECT": {}, "UPDATE": {}, "ALL": {},
		},
	}
	byType, exists := allowed[objectType]
	if !exists {
		return "", fmt.Errorf("unsupported PostgreSQL grant object type %q", objectType)
	}
	if _, exists := byType[privilege]; !exists {
		return "", fmt.Errorf(
			"privilege %q is not valid for PostgreSQL %s grants",
			value,
			objectType,
		)
	}
	if privilege == "TEMP" {
		privilege = "TEMPORARY"
	}
	return privilege, nil
}

func postgresGrantTarget(options database.GrantOptions) (string, error) {
	objectType := strings.ToLower(strings.TrimSpace(options.ObjectType))
	switch objectType {
	case "database":
		if strings.TrimSpace(options.Object) == "" {
			return "", fmt.Errorf("database name is required")
		}
		return "DATABASE " + quotePostgresIdentifier(options.Object), nil
	case "schema":
		schema := options.Schema
		if strings.TrimSpace(schema) == "" {
			schema = options.Object
		}
		if strings.TrimSpace(schema) == "" {
			return "", fmt.Errorf("schema name is required")
		}
		return "SCHEMA " + quotePostgresIdentifier(schema), nil
	case "table":
		if strings.TrimSpace(options.Schema) == "" ||
			strings.TrimSpace(options.Object) == "" {
			return "", fmt.Errorf("table schema and name are required")
		}
		return "TABLE " + quotePostgresQualifiedIdentifier(
			options.Schema,
			options.Object,
		), nil
	case "all_tables_in_schema":
		if strings.TrimSpace(options.Schema) == "" {
			return "", fmt.Errorf("schema name is required")
		}
		return "ALL TABLES IN SCHEMA " +
			quotePostgresIdentifier(options.Schema), nil
	case "sequence":
		if strings.TrimSpace(options.Schema) == "" ||
			strings.TrimSpace(options.Object) == "" {
			return "", fmt.Errorf("sequence schema and name are required")
		}
		return "SEQUENCE " + quotePostgresQualifiedIdentifier(
			options.Schema,
			options.Object,
		), nil
	default:
		return "", fmt.Errorf("unsupported PostgreSQL grant object type %q", objectType)
	}
}

func (p *Postgres) BuildSecurityChange(
	_ context.Context,
	request database.SecurityChangeRequest,
) (database.SecurityChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.SecurityChangePlan{}, err
	}
	switch request.Action {
	case database.SecurityCreatePrincipal, database.SecurityAlterPrincipal:
		options := request.Principal
		verb := "CREATE ROLE"
		summaryVerb := "Create"
		if request.Action == database.SecurityAlterPrincipal {
			verb = "ALTER ROLE"
			summaryVerb = "Alter"
		}
		statement := verb + " " + quotePostgresIdentifier(options.Name) +
			" WITH " + strings.Join(postgresPrincipalAttributes(options), " ")
		preview := statement
		if options.Password != "" {
			statement += " PASSWORD " + quotePostgresLiteral(options.Password)
			preview += " PASSWORD '••••••'"
		}
		statement += ";"
		preview += ";"
		return database.SecurityChangePlan{
			Summary:           summaryVerb + " PostgreSQL role " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{preview},
			Transactional:     true,
			Warnings: []string{
				"Role attributes can grant server-wide access. Review SUPERUSER, CREATEROLE, and CREATEDB carefully.",
				"Database audit logs may record role-management statements. Review server logging policy before setting a password.",
			},
		}, nil

	case database.SecurityDropPrincipal:
		name := request.Principal.Name
		statement := "DROP ROLE " + quotePostgresIdentifier(name) + ";"
		return database.SecurityChangePlan{
			Summary:           "Drop PostgreSQL role " + name,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       true,
			Transactional:     true,
			Warnings: []string{
				"PostgreSQL refuses to drop a role that still owns objects or has dependent grants.",
			},
		}, nil

	case database.SecurityGrantRole, database.SecurityRevokeRole:
		grant := request.Grant
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
		statement := verb + " " + quotePostgresIdentifier(grant.Role) +
			middle + quotePostgresIdentifier(grant.Grantee) + suffix + ";"
		return database.SecurityChangePlan{
			Summary: summaryVerb + " role " + grant.Role +
				strings.ToLower(middle) + grant.Grantee,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       destructive,
			Transactional:     true,
		}, nil

	case database.SecurityGrantPrivilege, database.SecurityRevokePrivilege:
		grant := request.Grant
		objectType := strings.ToLower(strings.TrimSpace(grant.ObjectType))
		privilege, err := postgresPrivilege(grant.Privilege, objectType)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		target, err := postgresGrantTarget(grant)
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
		statement := fmt.Sprintf(
			"%s %s ON %s%s%s;",
			verb,
			privilege,
			target,
			middle,
			quotePostgresIdentifier(grant.Grantee)+suffix,
		)
		return database.SecurityChangePlan{
			Summary: fmt.Sprintf(
				"%s %s on %s for %s",
				summaryVerb,
				privilege,
				target,
				grant.Grantee,
			),
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       destructive,
			Transactional:     true,
		}, nil
	default:
		return database.SecurityChangePlan{}, fmt.Errorf(
			"unsupported PostgreSQL security action %q",
			request.Action,
		)
	}
}

func (p *Postgres) ApplySecurityChange(
	ctx context.Context,
	plan database.SecurityChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	transaction, err := p.conn.BeginTxx(ctx, nil)
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
				"execute security statement %d of %d: %w",
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

var _ database.SecurityDriver = (*Postgres)(nil)
