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

1. Save a database password, SSH password, and encrypted-key passphrase in representative profiles,
   then accept the OS keychain prompt.
2. Inspect `connections.json`; no secret may occur anywhere. Only `hasPassword`,
   `hasSshPassword`, and `hasSshKeyPassphrase` metadata may be true.
3. Edit another field while leaving Password blank. Reconnect successfully with the preserved
   credential.
4. Enter a replacement password, save, and reconnect.
5. Use the two-step removal actions for database and SSH credentials. Reopen the profile and confirm
   it requests the removed secret without affecting the other credential.
6. Test old version-1/version-2 profile fixtures on a disposable OS account/keychain. Migration to
   version 4 must rewrite metadata only after the keychain accepts every plaintext secret.

Automated tests cover restrictive file modes, plaintext absence, fail-safe legacy migration,
backend-only secret resolution, and credential removal.

For PostgreSQL maintenance, also confirm the child environment contains `PGPASSFILE` but not
`PGPASSWORD`, the password file mode is `0600`, and the file no longer exists after completion.
Frontend tests also enforce the semantic color contract recorded in the repository
[agent guide](../AGENTS.md).

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
- Add, alter, rename, and drop columns; create/drop indexes and constraints; verify the SQL preview
  and stale-preview rejection before applying.
- Compare disposable source/target schemas for each engine. Exercise non-destructive sync first,
  then opt into drop operations and confirm complex constraint drift remains manual.
- Back up and restore each engine. Change the selected backup after preview and confirm apply is
  rejected; cancel an external restore and verify the partial-restore warning remains visible.
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
