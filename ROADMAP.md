# Rolling Thunder Roadmap

Rolling Thunder is being built as a capable database studio, not only a table viewer. This roadmap
is ordered by dependency and user impact. It intentionally does not promise release dates.

## Shipped foundation

- [x] PostgreSQL connections with saved profiles and SSL configuration
- [x] Multiple active connections and an in-workspace connection switcher
- [x] Schema and table explorer
- [x] Structure, data, indexes, and DDL views
- [x] Schema-qualified primary/foreign-key metadata with navigable relation links
- [x] Schema diagram
- [x] Staged row insert, update, and delete workflows
- [x] Table creation, truncation, and deletion
- [x] Stable PostgreSQL server-side sorting with multi-column priority
- [x] Typed data-grid previews with sticky headers, sticky edge columns, and resizable columns
- [x] Searchable row-detail drawer for table data and query results
- [x] Multi-tab workspace with query history
- [x] Bounded query results with a 1,000-row safety cap and client-side pagination
- [x] Cancellable queries with elapsed-time feedback
- [x] Per-query-tab explicit transactions with commit and rollback
- [x] Typed, parameterized, driver-owned table filters
- [x] Confirmation guard for unfiltered `UPDATE` and `DELETE` statements
- [x] Stable connection/query error codes with actionable recovery hints
- [x] Context-aware SQL completion for schemas, tables, aliases, and columns
- [x] PostgreSQL, MySQL, and SQLite keyword/function completion catalogs
- [x] Regression tests for SQL context parsing and dialect catalogs
- [x] Streaming CSV and JSON export for table pages, all filtered rows, and loaded query results
- [x] Streaming PostgreSQL `INSERT` export for table pages and all filtered rows
- [x] Checked-row export for table pages and query-result pages
- [x] Live export progress, safe cancellation, and configurable CSV encodings
- [x] Dark and light appearance modes
- [x] Activity console and informative loading states

## Milestone 1 — Reliable data workflows

This milestone closes the biggest gaps in daily table and query-result work.

### Server-side sorting

- [x] Add a typed sort model to the database-driver interface.
- [x] Support single and multi-column sorting.
- [x] Preserve sorting while filtering and paginating.
- [x] Handle `NULLS FIRST` / `NULLS LAST` where an engine supports it.
- [x] Quote identifiers through the active driver instead of concatenating raw SQL.
- [x] Show active sort order and priority in the data-grid header.

Sorting is complete when changing pages cannot reorder rows unpredictably and every driver has
explicit, tested ordering behavior.

### Export

- [x] Export table rows and loaded query results as CSV.
- [x] Export table rows and loaded query results as JSON.
- [x] Export PostgreSQL table rows as SQL `INSERT` statements.
- [x] Choose between the current table page or all filtered rows.
- [x] Export selected rows.
- [x] Stream all-filtered table exports instead of holding the complete dataset in memory.
- [x] Use the native destination picker and show useful completion, cancellation, and error
      messages.
- [x] Add granular progress and cancellation for a running export.
- [x] Add delimiter, header, and null-value options.
- [x] Add UTF-8, UTF-8 BOM, and UTF-16 LE CSV encodings.

### Query and data safety

- [x] Cancel a running query.
- [x] Add an explicit transaction mode with commit and rollback.
- [x] Replace free-form filter assembly with a typed, driver-owned filter expression.
- [x] Warn before an unfiltered update or delete.
- [x] Add stable error codes and actionable connection/query messages.

## Milestone 2 — Complete database-object explorer

Tables are only one kind of database object. The explorer will move to a capability-based tree so
each engine can expose the objects it genuinely supports.

### Read and inspect

- [x] Views
- [x] Materialized views where supported
- [x] Functions and stored procedures
- [x] Triggers
- [x] Sequences and identity generators
- [x] Custom types, enums, and domains
- [x] Constraints and foreign-key dependencies
- [x] Extensions for PostgreSQL

Every supported object should provide:

- [x] Search and grouping in the explorer
- [x] Definition / DDL view
- [x] Dependency and dependent-object information
- [x] Copy-name and copy-qualified-name actions
- [x] Refresh without losing the current workspace

### Manage objects

- [x] Create, alter, rename, and drop views.
- [x] Create and edit functions/procedures with dialect-aware templates.
- [x] Enable, disable, create, and drop triggers where supported.
- [x] Create and drop indexes from the structure view.
- [x] Alter existing table columns and constraints.
- [x] Preview generated SQL before applying structural changes.

## Milestone 3 — Real multi-engine support

The connection-provider flow and SQL completion catalogs are already engine-aware. This milestone
adds production-quality drivers and exposes only capabilities implemented by each engine.

### Driver capability contract

- [x] Declare support for schemas, databases, views, routines, triggers, DDL, explain plans, and
      transactional DDL.
- [x] Centralize identifier quoting, placeholders, pagination, filtering, and sorting per dialect.
- [x] Normalize metadata without erasing engine-specific details.
- [x] Add a shared conformance test suite for every driver.

### MySQL / MariaDB

- [x] Connect, reconnect, and configure TLS.
- [x] Browse databases, tables, views, routines, and triggers.
- [x] Map MySQL types, generated columns, indexes, and auto-increment behavior.
- [x] Support MySQL pagination, explain plans, and `ON DUPLICATE KEY` workflows.

### SQLite

- [x] Open and create local database files.
- [x] Support `main`, attached databases, tables, views, triggers, and indexes.
- [x] Map rowid, affinity, generated columns, and SQLite pragmas.
- [x] Handle file locking, busy timeouts, and WAL mode clearly.

## Milestone 4 — Power-user query tooling

- [x] Persist query tabs and restore them on launch.
- [x] Saved snippets and named queries.
- [x] Multiple result sets.
- [x] Explain-plan viewer with cost and row-estimate visualization.
- [x] SQL formatting and configurable linting.
- [x] Find references and jump from an identifier to its object.
- [x] Parameterized query variables.
- [x] Import CSV/JSON into an existing or new table.
- [x] Command palette and customizable keyboard shortcuts.

## Milestone 5 — Release readiness

- [x] Store credentials in macOS Keychain, Windows Credential Manager, or Linux Secret Service.
- [x] Add bounded connection attempts with elapsed-time feedback and cancellation.
- [x] Add keepalive, health checks, and controlled reconnects.
- [x] Integration tests against supported database versions.
- [x] End-to-end tests for connection, editing, export, and destructive actions.
- [x] Accessibility and keyboard-navigation audit.
- [x] Automated release artifacts for macOS, Windows, and Linux with checksums and provenance;
      macOS artifacts are explicitly labelled unsigned.
- [x] Migration policy for saved profiles and workspace state.
- [x] Crash reporting and opt-in diagnostics with documented privacy behavior.

Release verification, supported database versions, the artifact trust model, privacy behavior,
storage migrations, and the manual accessibility matrix are documented under [`docs/`](docs/).

## Prioritization principles

1. Protect data before adding convenience.
2. Keep pagination, filtering, sorting, and export in the driver layer so behavior stays correct
   across engines.
3. Expose engine-specific features through capabilities instead of pretending every database works
   the same way.
4. Keep common workflows simple while allowing advanced details to remain discoverable.
5. Do not mark an engine supported until it passes the shared driver conformance suite.
