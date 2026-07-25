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

Each server driver has two live layers:

- the integration smoke workflow connects, creates a temporary table, runs a parameterized query,
  applies an atomic update, streams CSV, deletes, and drops;
- the package conformance workflow exercises every required `database.Driver` method, verifies that
  advertised capabilities implement their interfaces, and runs safe representative optional
  workflows. It covers schema/database info, data types,
  table DDL, primary/foreign-key metadata, filters and sorting, direct and staged CRUD, bounded query
  results, commit and rollback, Explain, reviewed column/index/view changes, object details and
  dependencies, CSV/JSON/SQL export, truncate, drop, and close.

PostgreSQL and MySQL/MariaDB conformance also reads activity and security metadata. In disposable CI
containers, `ROLLINGTHUNDER_TEST_PRIVILEGED=1` additionally creates and drops a uniquely named test
role so `ApplySecurityChange` is covered. Do not enable that flag against a shared or production
server.

SQLite conformance is always available locally:

```bash
go test ./pkg/database/sqlite -run TestSQLiteLiveConformance -count=1 -v
```

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

The full PostgreSQL conformance suite uses:

```bash
ROLLINGTHUNDER_POSTGRES_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_POSTGRES_TEST_PORT=5432 \
ROLLINGTHUNDER_POSTGRES_TEST_USER=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_PASSWORD=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_DATABASE=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_SSL_MODE=disable \
go test ./pkg/database/postgres -run TestPostgresLiveConformance -count=1 -v
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

The full MySQL/MariaDB conformance suite uses:

```bash
ROLLINGTHUNDER_MYSQL_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_MYSQL_TEST_PORT=3306 \
ROLLINGTHUNDER_MYSQL_TEST_USER=root \
ROLLINGTHUNDER_MYSQL_TEST_PASSWORD=rolling \
ROLLINGTHUNDER_MYSQL_TEST_DATABASE=rolling \
ROLLINGTHUNDER_MYSQL_TEST_SSL_MODE=disable \
go test ./pkg/database/mysql -run TestMySQLLiveConformance -count=1 -v
```

### Oracle Database

```bash
RT_INTEGRATION_ORACLE=1 \
RT_DATABASE_HOST=127.0.0.1 \
RT_DATABASE_PORT=1521 \
RT_DATABASE_USER=system \
RT_DATABASE_PASSWORD=RollingThunder_2026 \
RT_DATABASE_NAME=FREEPDB1 \
RT_DATABASE_SCHEMA=SYSTEM \
RT_DATABASE_SSL_MODE=disable \
go test ./integration -run TestOracleDriverWorkflow -count=1 -v
```

The package-level shared conformance suite uses the equivalent connection variables. The privileged
and Data Pump flags below create and remove a disposable role and application schema, then
round-trip that schema through `DATA_PUMP_DIR`; enable them only against a disposable Oracle
instance:

```bash
ROLLINGTHUNDER_ORACLE_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_ORACLE_TEST_PORT=1521 \
ROLLINGTHUNDER_ORACLE_TEST_USER=system \
ROLLINGTHUNDER_ORACLE_TEST_PASSWORD=RollingThunder_2026 \
ROLLINGTHUNDER_ORACLE_TEST_SERVICE=FREEPDB1 \
ROLLINGTHUNDER_ORACLE_TEST_SSL_MODE=disable \
ROLLINGTHUNDER_ORACLE_TEST_PRIVILEGED=1 \
ROLLINGTHUNDER_ORACLE_TEST_DATAPUMP=1 \
go test ./pkg/database/oracle -run TestOracleLiveConformance -count=1 -v
```

The standard activity and security read paths are exercised whenever the Oracle driver advertises
those capabilities.

### SQL Server

```bash
RT_INTEGRATION_SQLSERVER=1 \
RT_DATABASE_HOST=127.0.0.1 \
RT_DATABASE_PORT=1433 \
RT_DATABASE_USER=sa \
RT_DATABASE_PASSWORD='RollingThunder_2026!' \
RT_DATABASE_NAME=master \
RT_DATABASE_SCHEMA=dbo \
RT_DATABASE_SSL_MODE=disable \
go test ./integration -run TestSQLServerDriverWorkflow -count=1 -v
```

The package-level shared conformance suite uses the equivalent
`ROLLINGTHUNDER_SQLSERVER_TEST_HOST`, `_PORT`, `_USER`, `_PASSWORD`, `_DATABASE`, and `_SSL_MODE`
variables:

```bash
go test ./pkg/database/sqlserver -run TestSQLServerLiveConformance -count=1 -v
```

## SSH tunnel checks

The unit suite starts an in-process SSH server and TCP target where local listeners are permitted.
It verifies password authentication, pinned host-key verification, traffic forwarding, endpoint
routing, reconnect replacement, and cleanup. Sandboxes that prohibit local listeners skip only the
socket-forwarding case; CI must run it.

Manual checks should use a disposable bastion and database:

1. Add the bastion key to a disposable `known_hosts` file and connect with SSH agent, private-key,
   and password authentication in turn.
2. Remove the known-hosts entry and confirm connection fails closed. Repeat with the correct pinned
   SHA256 fingerprint, then with a different fingerprint and confirm the mismatch is reported.
3. Set the database host to an address resolvable only from the bastion. Confirm queries,
   PostgreSQL/MySQL backup, and restore all use the tunnel.
4. Cancel a slow connection attempt, reconnect an active profile, and disconnect it. Confirm no
   loopback listener remains and a failed replacement leaves the previous connection usable.
5. Inspect `connections.json`; SSH passwords and key passphrases must never appear.

## Credential and migration checks

Use a disposable saved profile:

1. Save a database password, SSH password, encrypted-key passphrase, and password-protected Oracle
   Wallet in representative profiles, then accept the OS keychain prompt.
2. Inspect `connections.json`; no secret may occur anywhere. Only `hasPassword`,
   `hasSshPassword`, `hasSshKeyPassphrase`, and `hasOracleWalletPassword` metadata may be true.
3. Edit another field while leaving Password blank. Reconnect successfully with the preserved
   credential.
4. Enter a replacement password, save, and reconnect.
5. Use the two-step removal actions for database, SSH, and Oracle Wallet credentials. Reopen the
   profile and confirm it requests the removed secret without affecting the other credentials.
6. Test old version-1 through version-5 profile fixtures on a disposable OS account/keychain.
   Migration to version 6 must rewrite metadata only after the keychain accepts every plaintext
   secret.

Automated tests cover restrictive file modes, plaintext absence, fail-safe legacy migration,
backend-only secret resolution, and credential removal.

For PostgreSQL maintenance, also confirm the child environment contains `PGPASSFILE` but not
`PGPASSWORD`, the password file mode is `0600`, and the file no longer exists after completion.
Frontend tests also enforce the semantic color contract recorded in the repository
[agent guide](../AGENTS.md).

## Manual application matrix

Repeat critical workflows for PostgreSQL, MySQL/MariaDB, SQLite, Oracle, and SQL Server where
applicable. Native backup/security/activity checks currently apply only to the engines whose
capabilities expose those tools:

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
- Add, alter, rename, and drop columns; create/drop indexes and constraints; verify the SQL preview
  and stale-preview rejection before applying.
- Compare disposable source/target schemas for each engine. Exercise non-destructive sync first,
  then opt into drop operations and confirm complex constraint drift remains manual.
- Back up and restore each engine. Change the selected backup after preview and confirm apply is
  rejected; cancel an external restore and verify the partial-restore warning remains visible.
- For Oracle, choose a visible DIRECTORY object, round-trip a disposable application schema through
  a `.dmp`, and confirm temporary dump/log files are removed from the server directory. Test both a
  direct endpoint and a reviewed TNS alias; test `cwallet.sso` auto-login and password-protected
  `ewallet.p12` against disposable TCPS endpoints.
- Create a disposable role/user, grant and revoke table privileges and membership, alter it, then
  drop it. Confirm password text is redacted from previews.
- Start a long query from a second client, inspect it in Activity, cancel the statement, and test
  session termination. Rolling Thunder's own protected session must not expose a destructive action.
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
- an unexpectedly unsigned Windows/Linux tagged package or failed checksum/provenance verification.
