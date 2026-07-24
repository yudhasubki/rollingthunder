# Testing guide

Run tests against disposable databases. The integration and end-to-end suites create and drop
temporary objects, and manual destructive-action tests are intentionally capable of changing data.

## Fast local gate

From the repository root:

```bash
go test ./...
go vet ./...

cd frontend
npm ci
npm test
npm run lint
npm run build
```

`go test ./...` always runs the SQLite integration workflow and the headless application end-to-end
test. Live server tests skip unless their opt-in environment variable is set.

## Race and desktop builds

```bash
go test -race ./internal/db ./internal/diagnostics ./pkg/database/...
wails build -m -nocolour
```

On a current Linux distribution with WebKitGTK 4.1:

```bash
wails build -m -nocolour -tags webkit2_41
```

## Live database integration

The same workflow is exercised for each driver: connect, ping, create a temporary table, insert,
inspect metadata, execute a parameterized query, update atomically, stream CSV, delete, and drop.

### PostgreSQL

```bash
RT_INTEGRATION_POSTGRES=1 \
RT_DATABASE_HOST=127.0.0.1 \
RT_DATABASE_PORT=5432 \
RT_DATABASE_USER=rolling \
RT_DATABASE_PASSWORD=rolling \
RT_DATABASE_NAME=rolling \
go test ./integration -run TestPostgreSQLDriverWorkflow -count=1 -v
```

### MySQL

```bash
RT_INTEGRATION_MYSQL=1 \
RT_DATABASE_DRIVER=mysql \
RT_DATABASE_HOST=127.0.0.1 \
RT_DATABASE_PORT=3306 \
RT_DATABASE_USER=root \
RT_DATABASE_PASSWORD=rolling \
RT_DATABASE_NAME=rolling \
go test ./integration -run TestMySQLCompatibleDriverWorkflow -count=1 -v
```

Use `RT_DATABASE_DRIVER=mariadb` against MariaDB. CI runs the version matrix documented in
[RELEASING.md](RELEASING.md).

## Credential and migration checks

Use a disposable saved profile:

1. Save a password and accept the OS keychain prompt.
2. Inspect `connections.json`; the password must not occur anywhere and `hasPassword` must be true.
3. Edit another field while leaving Password blank. Reconnect successfully with the preserved
   credential.
4. Enter a replacement password, save, and reconnect.
5. Use the two-step Remove stored password action. Reopen the profile and confirm it requests a
   password.
6. Test an old version-1 profile fixture on a disposable OS account/keychain. The migration must
   rewrite metadata only after the keychain accepts the password.

Automated tests cover restrictive file modes, plaintext absence, fail-safe legacy migration,
backend-only secret resolution, and credential removal.

## Manual application matrix

Repeat critical workflows for PostgreSQL, MySQL/MariaDB, and SQLite where applicable:

- Create, edit, connect, cancel a slow connection attempt, switch, disconnect, and reopen profiles.
- Stop the database server, run an explicit health check, verify the degraded label, restart the
  server, and use controlled reconnect. A failed replacement must leave the old driver registered.
- Browse every supported object kind, inspect DDL/dependencies, and refresh without losing tabs.
- Verify primary/foreign keys in Structure and follow a foreign-key link to its referenced table.
- Scroll Structure, Indexes, wide data grids, and long tab strips. Sticky headers/actions and the
  active tab must remain visible.
- Insert, inline-edit, and delete rows; review staged changes; cancel and confirm the apply flow.
- Trigger truncate, drop, and unfiltered update/delete confirmations. No destructive action may run
  on the first acknowledgement.
- Filter, multi-sort, paginate, inspect a row drawer, and export page/selected/all-filtered data in
  every supported format.
- Run batches with multiple result sets, query variables, formatter/linter, Explain, cancellation,
  and explicit commit/rollback.
- Import CSV and JSON into existing and new tables, including cancellation and invalid-row errors.
- Close and reopen the application. Saved query tabs and named queries must restore only under their
  owning connection.
- Enable optional diagnostics, trigger a controlled frontend error in development, export the ZIP,
  inspect redaction, clear reports, and verify disabling stops future frontend reports.
- Complete [ACCESSIBILITY.md](ACCESSIBILITY.md)'s keyboard and assistive-technology checklist.

## Release gate

A release is blocked by:

- any unit, integration, E2E, race, vet, lint, or production-build failure;
- a plaintext credential or secret returned to the frontend;
- a failed migration that modifies the original source;
- an unsafe reconnect that drops the existing driver before its replacement is connected;
- a destructive workflow without explicit review/confirmation;
- a keyboard trap, missing blocking error announcement, or inaccessible required action;
- an unsigned tagged package or failed checksum/provenance verification.
