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
- [x] Context-aware SQL completion for schemas, tables, aliases, and columns
- [x] PostgreSQL, MySQL, and SQLite keyword/function completion catalogs
- [x] Regression tests for SQL context parsing and dialect catalogs
- [x] Streaming CSV export for table pages, all filtered rows, and loaded query results
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
- [ ] Export as JSON.
- [ ] Export rows as SQL `INSERT` statements.
- [x] Choose between the current table page or all filtered rows.
- [ ] Export selected rows.
- [x] Stream all-filtered table exports instead of holding the complete dataset in memory.
- [x] Use the native destination picker and show useful completion, cancellation, and error
      messages.
- [ ] Add granular progress and cancellation for a running export.
- [x] Add delimiter, header, and null-value options; write CSV as UTF-8.
- [ ] Add configurable CSV encodings.

### Query and data safety

- [ ] Cancel a running query.
- [ ] Add an explicit transaction mode with commit and rollback.
- [ ] Replace free-form filter assembly with a typed, driver-owned filter expression.
- [ ] Warn before an unfiltered update or delete.
- [ ] Add stable error codes and actionable connection/query messages.

## Milestone 2 — Complete database-object explorer

Tables are only one kind of database object. The explorer will move to a capability-based tree so
each engine can expose the objects it genuinely supports.

### Read and inspect

- [ ] Views
- [ ] Materialized views where supported
- [ ] Functions and stored procedures
- [ ] Triggers
- [ ] Sequences and identity generators
- [ ] Custom types, enums, and domains
- [ ] Constraints and foreign-key dependencies
- [ ] Extensions for PostgreSQL

Every supported object should provide:

- [ ] Search and grouping in the explorer
- [ ] Definition / DDL view
- [ ] Dependency and dependent-object information
- [ ] Copy-name and copy-qualified-name actions
- [ ] Refresh without losing the current workspace

### Manage objects

- [ ] Create, alter, rename, and drop views.
- [ ] Create and edit functions/procedures with dialect-aware templates.
- [ ] Enable, disable, create, and drop triggers where supported.
- [ ] Create and drop indexes from the structure view.
- [ ] Alter existing table columns and constraints.
- [ ] Preview generated SQL before applying structural changes.

## Milestone 3 — Real multi-engine support

The connection-provider flow and SQL completion catalogs are already engine-aware. This milestone
adds production-quality drivers and exposes only capabilities implemented by each engine.

### Driver capability contract

- [ ] Declare support for schemas, databases, views, routines, triggers, DDL, explain plans, and
      transactional DDL.
- [ ] Centralize identifier quoting, placeholders, pagination, filtering, and sorting per dialect.
- [ ] Normalize metadata without erasing engine-specific details.
- [ ] Add a shared conformance test suite for every driver.

### MySQL / MariaDB

- [ ] Connect, reconnect, and configure TLS.
- [ ] Browse databases, tables, views, routines, and triggers.
- [ ] Map MySQL types, generated columns, indexes, and auto-increment behavior.
- [ ] Support MySQL pagination, explain plans, and `ON DUPLICATE KEY` workflows.

### SQLite

- [ ] Open and create local database files.
- [ ] Support `main`, attached databases, tables, views, triggers, and indexes.
- [ ] Map rowid, affinity, generated columns, and SQLite pragmas.
- [ ] Handle file locking, busy timeouts, and WAL mode clearly.

## Milestone 4 — Power-user query tooling

- [ ] Persist query tabs and restore them on launch.
- [ ] Saved snippets and named queries.
- [ ] Multiple result sets.
- [ ] Explain-plan viewer with cost and row-estimate visualization.
- [ ] SQL formatting and configurable linting.
- [ ] Find references and jump from an identifier to its object.
- [ ] Parameterized query variables.
- [ ] Import CSV/JSON into an existing or new table.
- [ ] Command palette and customizable keyboard shortcuts.

## Milestone 5 — Release readiness

- [ ] Store credentials in macOS Keychain, Windows Credential Manager, or Linux Secret Service.
- [ ] Add connection timeouts, keepalive, health checks, and controlled reconnects.
- [ ] Integration tests against supported database versions.
- [ ] End-to-end tests for connection, editing, export, and destructive actions.
- [ ] Accessibility and keyboard-navigation audit.
- [ ] Signed packages and automated releases for macOS, Windows, and Linux.
- [ ] Migration policy for saved profiles and workspace state.
- [ ] Crash reporting and opt-in diagnostics with documented privacy behavior.

## Prioritization principles

1. Protect data before adding convenience.
2. Keep pagination, filtering, sorting, and export in the driver layer so behavior stays correct
   across engines.
3. Expose engine-specific features through capabilities instead of pretending every database works
   the same way.
4. Keep common workflows simple while allowing advanced details to remain discoverable.
5. Do not mark an engine supported until it passes the shared driver conformance suite.
