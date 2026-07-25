<div align="center">
  <a href="https://github.com/yudhasubki/rollingthunder/">
    <img src="build/appicon.png" width="112" alt="Rolling Thunder logo" />
  </a>
</div>

<h1 align="center">Rolling Thunder</h1>

<p align="center">
  A focused, cross-platform database studio for exploring schemas, editing data, and running SQL
  without leaving the desktop.
</p>

<p align="center">
  Built with Go, Wails, Svelte 5, and Monaco Editor.
</p>

<p align="center">
  <img
    src="docs/assets/screenshots/workspace-structure.png"
    width="100%"
    alt="Rolling Thunder workspace showing a PostgreSQL table structure, columns, keys, relations, and indexes"
  />
</p>

<p align="center">
  <sub>Inspect tables, keys, native types, relations, and indexes without losing database context.</sub>
</p>

> [!WARNING]
> Rolling Thunder is under active development. Keep independent backups before using structural or
> destructive workflows against production-critical databases.

## What works today

### Connections

- Save and edit reusable connection profiles.
- Keep passwords in macOS Keychain, Windows Credential Manager, or Linux Secret Service instead of
  profile JSON.
- Keep multiple database sessions open and switch between them.
- Classify profiles as Unclassified, Development, Staging, or Production. Colors are derived from
  that operational meaning instead of being arbitrary profile decoration.
- Configure PostgreSQL or MySQL/MariaDB TLS modes and client certificates.
- Route PostgreSQL and MySQL/MariaDB through an SSH tunnel using an SSH agent, private key, or
  password. Host keys must match a known-hosts entry or an explicitly pinned SHA256 fingerprint.
- Keep SSH passwords and private-key passphrases in the operating-system credential store alongside
  database passwords; profile JSON contains only non-secret SSH settings.
- Open or create SQLite database files through the native file picker.
- Open connection management as a modal without leaving the workspace.
- Select the database provider before filling in provider-specific settings.
- Cancel connection attempts, enforce a 15-second timeout, inspect live health/latency, and replace a
  degraded driver with a controlled reconnect that keeps the previous driver on failure.

### Database workspace

- Browse PostgreSQL schemas, MySQL/MariaDB databases, and SQLite attached databases.
- Explore and search tables, views, materialized views, routines, triggers, sequences, types,
  constraints, indexes, and PostgreSQL extensions according to each driver capability.
- Inspect columns, constraints, relationships, indexes, and table DDL.
- Identify primary and foreign keys explicitly and open referenced tables directly from Structure.
- Open an interactive schema diagram.
- Browse paginated table data with typed, parameterized filters and server-side sorting.
- Use single-column sorting or Shift-click headers for prioritized multi-column sorting.
- Read typed cell previews with explicit `NULL`, boolean, JSON, date/time, and binary states.
- Keep row numbers, actions, and headers visible while scrolling, and resize columns when needed.
- Inspect table rows and query results in a searchable right-side detail drawer.
- Copy individual field values or the complete row as formatted JSON.
- Stage row inserts, updates, and deletes before applying them.
- Create, truncate, and drop tables with confirmation.
- Add, alter, rename, and drop table columns; manage indexes and constraints through reviewed SQL
  previews according to each engine's capabilities.
- Keep table, query, diagram, and create-table tabs open together.
- Restore each saved profile's own query workspace without mixing tabs between active connections.

<p align="center">
  <img
    src="docs/assets/screenshots/schema-diagram.png"
    width="100%"
    alt="Rolling Thunder interactive schema diagram showing tables and foreign-key relationships"
  />
</p>

<p align="center">
  <sub>Map tables and follow foreign-key relationships in the interactive schema diagram.</sub>
</p>

### Database tools

- Compare two active connections of the same engine, review table/column/index drift, and apply a
  fingerprinted schema-sync plan. Destructive changes remain opt-in and unsupported constraint
  drift is called out for manual review.
- Create and restore SQLite online backups without an external process.
- Create PostgreSQL custom-format backups with `pg_dump` and restore them with `pg_restore`.
- Create and restore MySQL/MariaDB SQL backups with their native client tools. Database and SSH
  secrets are never placed in process arguments; PostgreSQL maintenance uses a temporary `0600`
  password file instead of exposing the password in the child-process environment.
- Preview every restore, verify the selected file has not changed, explicitly confirm the target,
  and cancel long-running external maintenance jobs.
- Inspect PostgreSQL roles or MySQL/MariaDB users, review grants, and preview create/alter/drop,
  grant/revoke, and role-membership changes before applying them.
- Monitor active PostgreSQL and MySQL/MariaDB sessions, waits, blockers, duration, and current SQL;
  cancel a statement or terminate a session through an explicit confirmation flow.

### Data export

- Export the current table page, checked rows, or every filtered row as CSV, JSON, or driver-owned
  `INSERT` statements.
- Preserve the active table filters and server-side sort order in exported data.
- Stream all-filtered table exports instead of loading the complete dataset in memory.
- Export all loaded query results or only checked rows without rerunning arbitrary SQL.
- Configure the CSV delimiter, column header, `NULL` representation, and UTF-8, UTF-8 BOM, or
  UTF-16 LE encoding.
- Choose pretty-printed or compact JSON; dates use ISO 8601 and binary values carry a `base64:`
  prefix.
- Batch SQL rows into configurable multi-value `INSERT` statements and optionally wrap the file in
  `BEGIN` / `COMMIT`.
- Let the active driver quote native SQL values and omit generated columns. PostgreSQL preserves
  identity values with `OVERRIDING SYSTEM VALUE`; MySQL/MariaDB can add
  `ON DUPLICATE KEY UPDATE`.
- Select the destination through a format-aware native file picker.
- Follow live row, byte, elapsed-time, and percentage progress for running exports.
- Cancel a running export without replacing an existing destination file.
- Keep an existing destination file unchanged if an export is cancelled or fails before
  completion.

### SQL editor

- Monaco-powered SQL editing and syntax highlighting.
- Execute the selected SQL or the statement under the cursor.
- Persist query tabs, save/tag named queries, and keep query history.
- Execute SQL batches and inspect multiple result sets independently.
- Supply typed query variables without string-concatenating values into SQL.
- Format SQL and apply configurable safety/style lint rules.
- Explain a query and inspect cost/row estimates without executing the statement.
- Jump from identifiers and aliases to matching database objects.
- Import CSV or JSON into an existing table or a reviewed new table through native file access.
- Open a command palette and customize keyboard shortcuts.
- Inspect execution status, result grids, and activity-console feedback.
- Cancel a running query with live elapsed-time feedback.
- Start an explicit transaction per query tab, then commit or roll back from the editor toolbar.
- Block raw transaction-control statements from pooled auto-commit queries so transaction state
  cannot silently move to another connection.
- Review and explicitly confirm an `UPDATE` or `DELETE` without a top-level `WHERE` clause.
- See stable query error codes and recovery hints for syntax, constraint, permission, cancellation,
  and transaction failures.
- Paginate query results in 100-row client pages and cap interactive results at 1,000 rows with a
  visible truncation warning.
- Schema-aware completion for schemas, tables, columns, and aliases.
- Context-aware suggestions after `FROM`, `JOIN`, `WHERE`, `SET`, and qualified names such as
  `customer.`.
- Function, keyword, and snippet catalogs for PostgreSQL, MySQL, and SQLite dialects.
- Lazy column metadata loading, including tables that were not part of the initial metadata warm-up.
- Independent editor models for every query tab, without duplicate completion providers.

### Release readiness

- Versioned, atomic, permission-restricted saved-profile and diagnostics settings.
- Centralized runtime defaults and semantic UI tokens, with regression tests that reject literal
  component colors and raw framework palette classes.
- Backend validation for ports, TLS/SSH modes, control characters, and oversized connection
  metadata before values reach a driver or maintenance command.
- Fail-safe migration of legacy plaintext profile passwords into the OS credential store.
- Local-only, opt-in frontend diagnostics with redaction, rotation, explicit export, and deletion.
- Minimal local crash reports without automatic upload.
- Non-blocking stable-release checks with a snoozable in-app update dialog and an explicit,
  browser-based download action.
- Keyboard focus traps, skip navigation, live status announcements, and reduced-motion support.
- Headless end-to-end coverage for connect, edit, export, truncate, and drop workflows.
- Live integration matrices for PostgreSQL 14–18, MySQL 8.4/9.7 LTS with legacy 8.0
  compatibility, and MariaDB 10.11/11.4/11.8/12.3 LTS.
- Automated native macOS, Windows, and Linux builds with checksums and GitHub/Sigstore provenance
  attestations. macOS packaging does not require an Apple Developer account.

## Database support

| Engine          | Connect/query | Object explorer                             | Maintenance / admin                     | CI integration                    |
| --------------- | ------------- | ------------------------------------------- | --------------------------------------- | --------------------------------- |
| PostgreSQL      | Available     | Full capability-based explorer              | Sync, backup, roles/grants, activity    | 14-18                             |
| MySQL / MariaDB | Available     | Databases, tables, views, routines/triggers | Sync, backup, users/grants, activity    | 8.4/9.7 + legacy 8.0; 10.11-12.3 |
| SQLite          | Available     | Attached DBs, tables, views, triggers       | Sync and built-in online backup/restore | Bundled engine                    |

## Known gaps

CSV and JSON export are available for table data and loaded query results. Driver-owned `INSERT`
export is available for table data, but PostgreSQL export intentionally does not reset sequence
state. Arbitrary query results do not offer SQL `INSERT` export because they do not provide a
reliable target table.

Other important limitations include:

- PostgreSQL backup/restore requires compatible `pg_dump` and `pg_restore` executables in `PATH`.
  MySQL/MariaDB backup/restore similarly requires `mysqldump`/`mariadb-dump` and
  `mysql`/`mariadb`.
- Role/user management and the activity monitor are server features and are not exposed for
  SQLite.
- Schema sync currently automates table, column, and index changes. Complex constraint drift is
  reported for manual review instead of generating unsafe SQL.
- SSH host verification is deliberately strict. Rolling Thunder does not silently trust a host on
  first use; add it to `known_hosts` or pin the SHA256 host-key fingerprint.
- Linux desktop packages currently target WebKitGTK 4.1 and require compatible GTK/WebKit runtime
  libraries from the distribution.
- macOS release archives are not code-signed or notarized, so Gatekeeper can require approval on the
  first launch.
- Windows and Linux tagged-release signing depends on repository signing certificates/keys and
  intentionally fails closed when they are absent.
- Automated accessibility contracts cover common regressions, but every stable release still
  requires the manual assistive-technology matrix.

Maintainer guardrails for runtime policy, trust boundaries, and semantic colors live in
[AGENTS.md](AGENTS.md).

## Tech stack

- **Desktop runtime:** Wails 2
- **Backend:** Go 1.23, pgx, and sqlx
- **Frontend:** Svelte 5 and TypeScript
- **UI:** Tailwind CSS, Melt UI, and Lucide
- **SQL editor:** Monaco Editor
- **Database drivers:** PostgreSQL, MySQL/MariaDB, and pure-Go SQLite

## Getting started

### Download

Prebuilt packages are available from
[GitHub Releases](https://github.com/yudhasubki/rollingthunder/releases):

- macOS Intel: `rollingthunder_<version>_darwin_amd64.zip`
- macOS Apple Silicon: `rollingthunder_<version>_darwin_arm64.zip`
- Windows: `rollingthunder_<version>_windows_amd64_installer.exe`
- Linux: `rollingthunder_<version>_linux_amd64.tar.gz`

Every release includes `SHA256SUMS` and GitHub provenance attestations. macOS may show a Gatekeeper
prompt on first launch because the app is currently distributed without code signing or
notarization. See [release packaging and verification](docs/RELEASING.md) for verification and
first-launch guidance.

### Prerequisites

- Go 1.23 or newer
- Node.js and npm
- Wails 2 CLI
- A supported database server, or a local SQLite database file
- Optional native database client tools for PostgreSQL/MySQL/MariaDB backup and restore

### Development

```bash
git clone https://github.com/yudhasubki/rollingthunder.git
cd rollingthunder

cd frontend
npm install
cd ..

wails dev
```

### Production build

```bash
wails build
```

### Local checks

```bash
go test ./...
go test -race ./internal/db ./internal/diagnostics ./pkg/database/...
go vet ./...

cd frontend
npm test
npm run lint
npm run build

cd ..
wails build -m -nocolour
```

See [docs/TESTING.md](docs/TESTING.md) for live PostgreSQL/MySQL/MariaDB integration variables and
the full manual release matrix.

## Security, privacy, and releases

- [Security policy](SECURITY.md)
- [Privacy and diagnostics](docs/PRIVACY.md)
- [Storage and migration policy](docs/MIGRATIONS.md)
- [Accessibility audit](docs/ACCESSIBILITY.md)
- [Release packaging and verification](docs/RELEASING.md)

## Roadmap

Milestones 1-6 cover reliable data workflows, the complete object explorer, multi-engine drivers,
power-user query tooling, release readiness, and reviewed administration/portability workflows.
See [ROADMAP.md](ROADMAP.md) for acceptance criteria and shipped scope.

## Contributing

Issues and focused pull requests are welcome while the architecture is still evolving. Please
include the target database engine, expected behavior, and reproduction steps for database-specific
bugs.

## License

Rolling Thunder is available under the [MIT License](LICENSE).
