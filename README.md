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

> [!WARNING]
> Rolling Thunder is under active development and is not ready for production-critical database
> work. PostgreSQL is currently the only functional database backend. MySQL and SQLite are planned,
> but their connection providers are intentionally disabled until their drivers reach feature
> parity.

## What works today

### Connections

- Save and edit reusable connection profiles.
- Keep multiple database sessions open and switch between them.
- Identify environments with profile colors.
- Configure PostgreSQL SSL modes and client certificates.
- Open connection management as a modal without leaving the workspace.
- Select the database provider before filling in provider-specific settings.

### Database workspace

- Browse PostgreSQL schemas and tables.
- Search tables and switch schemas.
- Inspect columns, constraints, relationships, indexes, and table DDL.
- Identify primary and foreign keys explicitly and open referenced tables directly from Structure.
- Open an interactive schema diagram.
- Browse paginated table data with filters and server-side sorting.
- Use single-column sorting or Shift-click headers for prioritized multi-column sorting.
- Read typed cell previews with explicit `NULL`, boolean, JSON, date/time, and binary states.
- Keep row numbers, actions, and headers visible while scrolling, and resize columns when needed.
- Inspect table rows and query results in a searchable right-side detail drawer.
- Copy individual field values or the complete row as formatted JSON.
- Stage row inserts, updates, and deletes before applying them.
- Create, truncate, and drop tables with confirmation.
- Keep table, query, diagram, and create-table tabs open together.

### Data export

- Export the current table page or every filtered row as UTF-8 CSV or JSON.
- Preserve the active table filters and server-side sort order in exported data.
- Stream all-filtered CSV and JSON exports from PostgreSQL instead of loading the complete dataset in
  memory.
- Export the rows already loaded in a query result without rerunning arbitrary SQL.
- Configure the CSV delimiter, column header, and `NULL` representation.
- Choose pretty-printed or compact JSON; dates use ISO 8601 and binary values carry a `base64:`
  prefix.
- Select the destination through a format-aware native file picker.
- Keep an existing destination file unchanged if an export fails before completion.

### SQL editor

- Monaco-powered SQL editing and syntax highlighting.
- Execute the selected SQL or the statement under the cursor.
- Query history, execution status, result grids, and activity-console feedback.
- Paginate query results in 100-row client pages and cap interactive results at 1,000 rows with a
  visible truncation warning.
- Schema-aware completion for schemas, tables, columns, and aliases.
- Context-aware suggestions after `FROM`, `JOIN`, `WHERE`, `SET`, and qualified names such as
  `customer.`.
- Function, keyword, and snippet catalogs for PostgreSQL, MySQL, and SQLite dialects.
- Lazy column metadata loading, including tables that were not part of the initial metadata warm-up.
- Independent editor models for every query tab, without duplicate completion providers.

The MySQL and SQLite completion catalogs are ready in the editor architecture, but they will become
user-facing only after the corresponding database drivers ship.

## Database support

| Engine          | Connect and query | Object metadata    | Dialect completion       |
| --------------- | ----------------- | ------------------ | ------------------------ |
| PostgreSQL      | Available         | Schemas and tables | Available                |
| MySQL / MariaDB | Planned           | Planned            | Completion catalog ready |
| SQLite          | Planned           | Planned            | Completion catalog ready |

## Known gaps

Rolling Thunder currently has a table-first object explorer. It does not yet expose database views,
materialized views, functions, procedures, triggers, sequences, or custom types.

CSV and JSON export are available for table data and loaded query results. SQL `INSERT`,
selected-row export, configurable encodings, and granular progress/cancellation remain on the
[project roadmap](ROADMAP.md).

Other important limitations include:

- PostgreSQL is the only real database driver.
- Indexes can be inspected but not created or edited visually.
- Existing tables cannot yet be altered from the structure editor.
- Query cancellation, multiple result sets, and visual query plans are not implemented.
- Saved credentials are not yet integrated with the operating-system keychain.

## Tech stack

- **Desktop runtime:** Wails 2
- **Backend:** Go 1.23, pgx, and sqlx
- **Frontend:** Svelte 5 and TypeScript
- **UI:** Tailwind CSS, Melt UI, and Lucide
- **SQL editor:** Monaco Editor
- **Current database driver:** PostgreSQL

## Getting started

### Prerequisites

- Go 1.23 or newer
- Node.js and npm
- Wails 2 CLI
- A PostgreSQL database for the current build

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

cd frontend
npm test
npm run build
```

## Roadmap

The next milestones extend export formats and controls, add a complete database-object explorer,
ship real MySQL and SQLite drivers, and harden the app for production. See
[ROADMAP.md](ROADMAP.md) for priorities and acceptance criteria.

## Contributing

Issues and focused pull requests are welcome while the architecture is still evolving. Please
include the target database engine, expected behavior, and reproduction steps for database-specific
bugs.

## License

Rolling Thunder is available under the [MIT License](LICENSE).
