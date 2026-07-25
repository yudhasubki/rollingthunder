package mysql

import (
	"context"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type mysqlPrincipalRow struct {
	Name       string `db:"rt_user"`
	Host       string `db:"rt_host"`
	AuthMethod string `db:"rt_plugin"`
	Locked     string `db:"rt_locked"`
	Superuser  string `db:"rt_super"`
	CreateRole string `db:"rt_create_user"`
}

func quoteMySQLLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func mysqlAccount(name string, host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "%"
	}
	return quoteMySQLLiteral(strings.TrimSpace(name)) + "@" + quoteMySQLLiteral(host)
}

func (m *MySQL) mysqlRoleReference(name string, host string) string {
	if strings.EqualFold(m.engine, "MariaDB") {
		return quoteMySQLIdentifier(strings.TrimSpace(name))
	}
	return mysqlAccount(name, host)
}

func (m *MySQL) GetSecurityOverview(
	ctx context.Context,
	principal string,
	host string,
) (database.SecurityOverview, error) {
	if err := m.ensureConnected(); err != nil {
		return database.SecurityOverview{}, err
	}
	var currentUser string
	if err := m.conn.GetContext(ctx, &currentUser, "SELECT CURRENT_USER()"); err != nil {
		return database.SecurityOverview{}, err
	}
	var rows []mysqlPrincipalRow
	const principalsQuery = `
		SELECT
			User AS rt_user,
			Host AS rt_host,
			COALESCE(plugin, '') AS rt_plugin,
			COALESCE(account_locked, 'N') AS rt_locked,
			COALESCE(Super_priv, 'N') AS rt_super,
			COALESCE(Create_user_priv, 'N') AS rt_create_user
		FROM mysql.user
		ORDER BY User, Host`
	if err := m.conn.SelectContext(ctx, &rows, principalsQuery); err != nil {
		const fallback = `
			SELECT
				User AS rt_user,
				Host AS rt_host,
				COALESCE(plugin, '') AS rt_plugin,
				'N' AS rt_locked,
				COALESCE(Super_priv, 'N') AS rt_super,
				COALESCE(Create_user_priv, 'N') AS rt_create_user
			FROM mysql.user
			ORDER BY User, Host`
		if fallbackErr := m.conn.SelectContext(ctx, &rows, fallback); fallbackErr != nil {
			return database.SecurityOverview{}, fallbackErr
		}
	}
	principals := make([]database.DatabasePrincipal, 0, len(rows))
	for _, row := range rows {
		locked := strings.EqualFold(row.Locked, "Y")
		kind := database.PrincipalUser
		if locked && strings.TrimSpace(row.AuthMethod) == "" {
			kind = database.PrincipalRole
		}
		principals = append(principals, database.DatabasePrincipal{
			Name:       row.Name,
			Host:       row.Host,
			Kind:       kind,
			CanLogin:   kind == database.PrincipalUser && !locked,
			Superuser:  strings.EqualFold(row.Superuser, "Y"),
			CreateRole: strings.EqualFold(row.CreateRole, "Y"),
			Inherit:    true,
			Locked:     locked,
			AuthMethod: row.AuthMethod,
		})
	}

	grants := make([]database.DatabaseGrant, 0)
	principal = strings.TrimSpace(principal)
	if principal != "" {
		account := mysqlAccount(principal, host)
		rows, err := m.conn.QueryxContext(ctx, "SHOW GRANTS FOR "+account)
		if err != nil {
			return database.SecurityOverview{}, err
		}
		defer rows.Close()
		for rows.Next() {
			values, err := rows.SliceScan()
			if err != nil {
				return database.SecurityOverview{}, err
			}
			if len(values) == 0 {
				continue
			}
			statement := fmt.Sprint(normalizeMySQLValue(values[0]))
			grants = append(grants, database.DatabaseGrant{
				Grantee:    principal,
				ObjectType: "statement",
				Privilege:  "GRANT",
				Statement:  statement,
			})
		}
		if err := rows.Err(); err != nil {
			return database.SecurityOverview{}, err
		}
	}
	return database.SecurityOverview{
		Supported:   true,
		Engine:      "mysql",
		CurrentUser: currentUser,
		Principals:  principals,
		Grants:      grants,
	}, nil
}

func mysqlPrivilege(value string) (string, error) {
	privilege := strings.ToUpper(
		strings.Join(strings.Fields(strings.TrimSpace(value)), " "),
	)
	allowed := map[string]struct{}{
		"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {},
		"CREATE": {}, "DROP": {}, "ALTER": {}, "INDEX": {},
		"REFERENCES": {}, "EXECUTE": {}, "CREATE VIEW": {},
		"SHOW VIEW": {}, "TRIGGER": {}, "EVENT": {},
		"CREATE ROUTINE": {}, "ALTER ROUTINE": {}, "CREATE USER": {},
		"PROCESS": {}, "RELOAD": {}, "REPLICATION CLIENT": {},
		"REPLICATION SLAVE": {}, "ALL": {}, "ALL PRIVILEGES": {},
	}
	if _, exists := allowed[privilege]; !exists {
		return "", fmt.Errorf("unsupported MySQL privilege %q", value)
	}
	if privilege == "ALL" {
		privilege = "ALL PRIVILEGES"
	}
	return privilege, nil
}

func mysqlGrantTarget(options database.GrantOptions) (string, error) {
	switch strings.ToLower(strings.TrimSpace(options.ObjectType)) {
	case "global":
		return "*.*", nil
	case "database":
		schema := options.Schema
		if strings.TrimSpace(schema) == "" {
			schema = options.Object
		}
		if strings.TrimSpace(schema) == "" {
			return "", fmt.Errorf("database name is required")
		}
		return quoteMySQLIdentifier(schema) + ".*", nil
	case "table":
		if strings.TrimSpace(options.Schema) == "" ||
			strings.TrimSpace(options.Object) == "" {
			return "", fmt.Errorf("database and table names are required")
		}
		return quoteMySQLQualifiedIdentifier(options.Schema, options.Object), nil
	default:
		return "", fmt.Errorf(
			"unsupported MySQL grant object type %q",
			options.ObjectType,
		)
	}
}

func (m *MySQL) BuildSecurityChange(
	_ context.Context,
	request database.SecurityChangeRequest,
) (database.SecurityChangePlan, error) {
	if err := request.Validate(); err != nil {
		return database.SecurityChangePlan{}, err
	}
	switch request.Action {
	case database.SecurityCreatePrincipal:
		options := request.Principal
		if options.Kind == database.PrincipalRole {
			reference := m.mysqlRoleReference(options.Name, options.Host)
			statement := "CREATE ROLE " + reference + ";"
			return database.SecurityChangePlan{
				Summary:           "Create database role " + options.Name,
				Statements:        []string{statement},
				PreviewStatements: []string{statement},
				Transactional:     false,
				Warnings: []string{
					"MySQL and MariaDB account DDL auto-commits.",
				},
			}, nil
		}
		account := mysqlAccount(options.Name, options.Host)
		statement := "CREATE USER " + account
		preview := statement
		if options.Password != "" {
			statement += " IDENTIFIED BY " + quoteMySQLLiteral(options.Password)
			preview += " IDENTIFIED BY '••••••'"
		}
		if options.Locked {
			statement += " ACCOUNT LOCK"
			preview += " ACCOUNT LOCK"
		}
		statement += ";"
		preview += ";"
		return database.SecurityChangePlan{
			Summary:           "Create database user " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{preview},
			Transactional:     false,
			Warnings: []string{
				"Account DDL auto-commits. Grant only the minimum required privileges after creation.",
				"Database audit or general logs may record account-management statements. Review server logging policy before setting a password.",
			},
		}, nil

	case database.SecurityAlterPrincipal:
		options := request.Principal
		if options.Kind == database.PrincipalRole {
			return database.SecurityChangePlan{}, fmt.Errorf(
				"MySQL and MariaDB roles are changed through grants, not ALTER ROLE attributes",
			)
		}
		account := mysqlAccount(options.Name, options.Host)
		statement := "ALTER USER " + account
		preview := statement
		if options.Password != "" {
			statement += " IDENTIFIED BY " + quoteMySQLLiteral(options.Password)
			preview += " IDENTIFIED BY '••••••'"
		}
		if options.Locked {
			statement += " ACCOUNT LOCK"
			preview += " ACCOUNT LOCK"
		} else {
			statement += " ACCOUNT UNLOCK"
			preview += " ACCOUNT UNLOCK"
		}
		statement += ";"
		preview += ";"
		return database.SecurityChangePlan{
			Summary:           "Alter database user " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{preview},
			Transactional:     false,
			Warnings: []string{
				"Account changes auto-commit and can immediately affect active clients.",
				"Database audit or general logs may record account-management statements. Review server logging policy before setting a password.",
			},
		}, nil

	case database.SecurityDropPrincipal:
		options := request.Principal
		kind := "USER"
		reference := mysqlAccount(options.Name, options.Host)
		if options.Kind == database.PrincipalRole {
			kind = "ROLE"
			reference = m.mysqlRoleReference(options.Name, options.Host)
		}
		statement := "DROP " + kind + " " + reference + ";"
		return database.SecurityChangePlan{
			Summary:           "Drop database " + strings.ToLower(kind) + " " + options.Name,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       true,
			Transactional:     false,
			Warnings: []string{
				"Dropping an account removes its grants and cannot be rolled back.",
			},
		}, nil

	case database.SecurityGrantRole, database.SecurityRevokeRole:
		grant := request.Grant
		role := m.mysqlRoleReference(grant.Role, grant.RoleHost)
		grantee := mysqlAccount(grant.Grantee, grant.GranteeHost)
		verb := "GRANT"
		middle := " TO "
		summaryVerb := "Grant"
		destructive := false
		if request.Action == database.SecurityRevokeRole {
			verb = "REVOKE"
			middle = " FROM "
			summaryVerb = "Revoke"
			destructive = true
		}
		statement := verb + " " + role + middle + grantee + ";"
		return database.SecurityChangePlan{
			Summary:           summaryVerb + " role " + grant.Role + " for " + grant.Grantee,
			Statements:        []string{statement},
			PreviewStatements: []string{statement},
			Destructive:       destructive,
			Transactional:     false,
		}, nil

	case database.SecurityGrantPrivilege, database.SecurityRevokePrivilege:
		grant := request.Grant
		privilege, err := mysqlPrivilege(grant.Privilege)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		target, err := mysqlGrantTarget(grant)
		if err != nil {
			return database.SecurityChangePlan{}, err
		}
		account := mysqlAccount(grant.Grantee, grant.GranteeHost)
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
			"%s %s ON %s%s%s%s;",
			verb,
			privilege,
			target,
			middle,
			account,
			suffix,
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
			Transactional:     false,
			Warnings: []string{
				"MySQL and MariaDB grant changes auto-commit.",
			},
		}, nil
	default:
		return database.SecurityChangePlan{}, fmt.Errorf(
			"unsupported MySQL security action %q",
			request.Action,
		)
	}
}

func (m *MySQL) ApplySecurityChange(
	ctx context.Context,
	plan database.SecurityChangePlan,
) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	for index, statement := range plan.Statements {
		if _, err := m.conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf(
				"execute security statement %d of %d: %w",
				index+1,
				len(plan.Statements),
				err,
			)
		}
	}
	return nil
}

var _ database.SecurityDriver = (*MySQL)(nil)
