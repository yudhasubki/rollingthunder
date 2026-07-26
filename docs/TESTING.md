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

PostgreSQL, MySQL/MariaDB, Oracle, and SQL Server conformance also read activity and security
metadata when the driver advertises those capabilities. In disposable CI containers,
`ROLLINGTHUNDER_TEST_PRIVILEGED=1` additionally creates and drops a uniquely named test role so
`ApplySecurityChange` is covered. Do not enable that flag against a shared or production server.

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

Every PostgreSQL matrix job then enables TLS on its disposable container and runs a required live
TLS suite. The fixture generator uses Go's `crypto/x509` standard library, writes private keys with
mode `0600`, and does not require repository secrets. The setup rewrites the disposable server's
TLS and host-authentication configuration, rejects plaintext TCP connections, and restarts it:

```bash
bash .github/scripts/configure-postgres-tls.sh \
  rollingthunder-postgres \
  .database-ci-tls/postgres \
  rolling_tls
```

Run the TLS suite with:

```bash
ROLLINGTHUNDER_POSTGRES_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_POSTGRES_TEST_PORT=5432 \
ROLLINGTHUNDER_POSTGRES_TEST_USER=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_PASSWORD=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_DATABASE=rolling \
ROLLINGTHUNDER_POSTGRES_TEST_REQUIRE_TLS=1 \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_ROOT_CERT="$PWD/.database-ci-tls/postgres/ca-cert.pem" \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_WRONG_ROOT_CERT="$PWD/.database-ci-tls/postgres/wrong-ca-cert.pem" \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_CERT="$PWD/.database-ci-tls/postgres/client-cert.pem" \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_KEY="$PWD/.database-ci-tls/postgres/client-key.pem" \
ROLLINGTHUNDER_POSTGRES_TEST_TLS_CLIENT_ROLE=rolling_tls \
go test ./pkg/database/postgres -run TestPostgresTLSLiveConformance -count=1 -v
```

It proves `require`, `verify-ca`, and `verify-full` create encrypted sessions; rejects plaintext,
an unrelated CA, and a wrong hostname; and verifies PostgreSQL `cert` authentication both rejects a
missing client certificate and exposes the expected client DN after a successful connection. CI
then reruns `TestPostgresLiveConformance` with `SSL_MODE=verify-full` so the complete shared driver
contract, not only the handshake probes, executes over verified TLS.

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

The required MySQL/MariaDB TLS gate uses the same Go-generated certificate model. Run the setup
only against a disposable container: it installs a private server key, enables
`require_secure_transport`, and restarts the server.

```bash
bash .github/scripts/configure-mysql-tls.sh \
  rollingthunder-mysql \
  .database-ci-tls/mysql \
  rolling_tls
```

Then run:

```bash
ROLLINGTHUNDER_MYSQL_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_MYSQL_TEST_PORT=3306 \
ROLLINGTHUNDER_MYSQL_TEST_USER=root \
ROLLINGTHUNDER_MYSQL_TEST_PASSWORD=rolling \
ROLLINGTHUNDER_MYSQL_TEST_DATABASE=rolling \
ROLLINGTHUNDER_MYSQL_TEST_REQUIRE_TLS=1 \
ROLLINGTHUNDER_MYSQL_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_MYSQL_TEST_TLS_ROOT_CERT="$PWD/.database-ci-tls/mysql/ca-cert.pem" \
ROLLINGTHUNDER_MYSQL_TEST_TLS_WRONG_ROOT_CERT="$PWD/.database-ci-tls/mysql/wrong-ca-cert.pem" \
ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_CERT="$PWD/.database-ci-tls/mysql/client-cert.pem" \
ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_KEY="$PWD/.database-ci-tls/mysql/client-key.pem" \
ROLLINGTHUNDER_MYSQL_TEST_TLS_CLIENT_USER=rolling_tls \
go test ./pkg/database/mysql -run TestMySQLTLSLiveConformance -count=1 -v
```

The suite checks the negotiated session cipher, all three supported TLS modes, trust and hostname
failures, plaintext rejection, and a disposable `REQUIRE X509` account with and without its client
certificate. CI runs it for every supported MySQL and MariaDB image, then repeats the complete
`TestMySQLLiveConformance` contract over `verify-full`.

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

The stable package conformance gate additionally runs the shared driver contract as a disposable
application user with a bounded tablespace quota, round-trips LOB/binary/Unicode/time-zone/quoted
identifier values, interrupts real long-running queries, reconnects, and connects through TCP TNS,
verified TLS, auto-login/password Wallets, and TNS over TCPS. It creates and removes users, roles,
tables, and other test objects, so run it only against a disposable Oracle instance.

Use the full Oracle Database Free image, not `-lite`, because the lite image omits XDB components
used by Data Pump and DBMS metadata. Give the container at least `1g` of shared memory. Colima users
should allocate at least 4 CPUs and 8 GiB while running the Oracle suite:

```bash
docker run --rm --name rollingthunder-oracle \
  --cap-add=SYS_NICE \
  --shm-size=1g \
  -p 1521:1521 \
  -p 2484:2484 \
  -e ORACLE_PWD=RollingThunder_2026 \
  container-registry.oracle.com/database/free:23.26.0.0
```

After the database reports healthy, create the disposable TCPS listener and Wallet fixtures. The
script writes a random Wallet password to a private `0600` file and never requires a repository
secret:

```bash
bash .github/scripts/configure-oracle-tcps.sh \
  rollingthunder-oracle \
  .oracle-ci-wallet \
  2484
```

Then run the required stable gate:

```bash
ROLLINGTHUNDER_ORACLE_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_ORACLE_TEST_PORT=1521 \
ROLLINGTHUNDER_ORACLE_TEST_USER=system \
ROLLINGTHUNDER_ORACLE_TEST_PASSWORD=RollingThunder_2026 \
ROLLINGTHUNDER_ORACLE_TEST_SERVICE=FREEPDB1 \
ROLLINGTHUNDER_ORACLE_TEST_SSL_MODE=disable \
ROLLINGTHUNDER_ORACLE_TEST_PRIVILEGED=1 \
ROLLINGTHUNDER_ORACLE_TEST_REQUIRE_SECURE=1 \
ROLLINGTHUNDER_ORACLE_TEST_TLS_PORT=2484 \
ROLLINGTHUNDER_ORACLE_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_ORACLE_TEST_TLS_ROOT_CERT="$PWD/.oracle-ci-wallet/server-cert.pem" \
ROLLINGTHUNDER_ORACLE_TEST_WALLET_PATH="$PWD/.oracle-ci-wallet" \
ROLLINGTHUNDER_ORACLE_TEST_WALLET_PASSWORD_FILE="$PWD/.oracle-ci-wallet/wallet-password" \
go test ./pkg/database/oracle -run TestOracleLiveConformance -count=1 -v
```

The standard activity and security read paths are exercised through the privileged connection;
the second contract proves ordinary core workflows do not require those administration privileges.
Pull requests run this gate once. Weekly and manual workflows repeat it three times and separately
run the extended Data Pump round-trip:

```bash
ROLLINGTHUNDER_ORACLE_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_ORACLE_TEST_PORT=1521 \
ROLLINGTHUNDER_ORACLE_TEST_USER=system \
ROLLINGTHUNDER_ORACLE_TEST_PASSWORD=RollingThunder_2026 \
ROLLINGTHUNDER_ORACLE_TEST_SERVICE=FREEPDB1 \
ROLLINGTHUNDER_ORACLE_TEST_SSL_MODE=disable \
ROLLINGTHUNDER_ORACLE_TEST_DATAPUMP=1 \
go test ./pkg/database/oracle -run TestOracleDataPumpLiveConformance -count=1 -v
```

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
variables. The privileged and native-backup flags create/drop a disposable role and database, write
a `.bak` on the server, mutate the database, restore it, and verify the pre-backup value. Enable
them only against a disposable instance:

```bash
ROLLINGTHUNDER_SQLSERVER_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_SQLSERVER_TEST_PORT=1433 \
ROLLINGTHUNDER_SQLSERVER_TEST_USER=sa \
ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD='RollingThunder_2026!' \
ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE=master \
ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE=disable \
ROLLINGTHUNDER_TEST_PRIVILEGED=1 \
ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP=1 \
go test ./pkg/database/sqlserver -run TestSQLServerLiveConformance -count=1 -v
```

Set `ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP_PATH` when the server does not expose
`/var/opt/mssql/data/rt_native_backup_conformance.bak`. The path is evaluated on the SQL Server
host and its service account must be able to create and restore that file.

Every SQL Server matrix job next installs a short-lived CA-signed server certificate, restricts its
private key to the `mssql` account, enables TLS 1.2 and `network.forceencryption`, and restarts the
disposable container. The setup follows
[Microsoft's SQL Server on Linux TLS settings](https://learn.microsoft.com/en-us/sql/linux/security/encrypted-connections):

```bash
bash .github/scripts/configure-sqlserver-tls.sh \
  rollingthunder-sqlserver \
  .database-ci-tls/sqlserver
```

Run the required transport probes with:

```bash
ROLLINGTHUNDER_SQLSERVER_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_SQLSERVER_TEST_PORT=1433 \
ROLLINGTHUNDER_SQLSERVER_TEST_USER=sa \
ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD='RollingThunder_2026!' \
ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE=master \
ROLLINGTHUNDER_SQLSERVER_TEST_REQUIRE_TLS=1 \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_ROOT_CERT="$PWD/.database-ci-tls/sqlserver/ca-cert.pem" \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_WRONG_ROOT_CERT="$PWD/.database-ci-tls/sqlserver/wrong-ca-cert.pem" \
go test ./pkg/database/sqlserver -run TestSQLServerTLSLiveConformance -count=1 -v
```

That command covers required encrypted TDS 7.4 behavior on both matrix entries and skips the
platform-specific Strict group. On the SQL Server 2025 Linux fixture, enable the Strict group to
prove a successful `08000000` protocol session before its negative trust cases:

```bash
ROLLINGTHUNDER_SQLSERVER_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_SQLSERVER_TEST_PORT=1433 \
ROLLINGTHUNDER_SQLSERVER_TEST_USER=sa \
ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD='RollingThunder_2026!' \
ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE=master \
ROLLINGTHUNDER_SQLSERVER_TEST_REQUIRE_TLS=1 \
ROLLINGTHUNDER_SQLSERVER_TEST_TDS8_STRICT=1 \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_ROOT_CERT="$PWD/.database-ci-tls/sqlserver/ca-cert.pem" \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_WRONG_ROOT_CERT="$PWD/.database-ci-tls/sqlserver/wrong-ca-cert.pem" \
go test ./pkg/database/sqlserver \
  -run 'TestSQLServerTLSLiveConformance/strict' -count=1 -v
```

Then run the complete driver contract over TDS 8.0 Strict:

```bash
ROLLINGTHUNDER_SQLSERVER_TEST_HOST=127.0.0.1 \
ROLLINGTHUNDER_SQLSERVER_TEST_PORT=1433 \
ROLLINGTHUNDER_SQLSERVER_TEST_USER=sa \
ROLLINGTHUNDER_SQLSERVER_TEST_PASSWORD='RollingThunder_2026!' \
ROLLINGTHUNDER_SQLSERVER_TEST_DATABASE=master \
ROLLINGTHUNDER_SQLSERVER_TEST_SSL_MODE=strict \
ROLLINGTHUNDER_SQLSERVER_TEST_SSL_ROOT_CERT="$PWD/.database-ci-tls/sqlserver/ca-cert.pem" \
ROLLINGTHUNDER_SQLSERVER_TEST_TLS_SERVER_NAME=localhost \
ROLLINGTHUNDER_TEST_PRIVILEGED=1 \
ROLLINGTHUNDER_SQLSERVER_TEST_BACKUP=1 \
go test ./pkg/database/sqlserver -run TestSQLServerLiveConformance -count=1 -v
```

The suite verifies that `require`, `verify-ca`, and `verify-full` negotiate encrypted TDS 7.4
sessions, while `strict` negotiates TDS 8.0 and verifies both the server certificate and hostname.
Plaintext, an unrelated CA, and a wrong hostname fail closed. CI runs the shared verified-TLS
contract on SQL Server 2022 and 2025, then repeats the Strict transport and full driver contracts on
the pinned SQL Server 2025 CU Linux image, including privileged security and native backup/restore.
SQL Server 2022 Linux rejects client-requested TDS 8.0; use `verify-full` for that platform. This
matches the behavior tracked in
[Microsoft's SQL Server container issue #878](https://github.com/microsoft/mssql-docker/issues/878).
Compatible SQL Server 2022 Windows and Azure SQL deployments can still use Strict. Rolling Thunder
never downgrades a Strict request to TDS 7.4. SQL Server TLS authenticates the server; its database
connection does not use the PostgreSQL/MySQL-style client certificate fields. Windows Integrated
and Microsoft Entra authentication retain their separate environment-specific checks. See
[Microsoft's TDS 8.0 documentation](https://learn.microsoft.com/en-us/sql/relational-databases/security/networking/tds-8)
for the server compatibility boundary.

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
6. Test old version-1 through version-6 profile fixtures on a disposable OS account/keychain.
   Migration to version 7 must rewrite metadata only after the credential store accepts every
   plaintext secret or obsolete-credential removal. A failed removal must keep the original profile
   and credential usable.

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
- For SQL Server, use a disposable non-system database and an absolute server-side `.bak` path.
  Confirm an existing destination is explicitly disclosed, a changed backup is rejected after
  review, open transactions are rolled back, the database returns to multi-user mode, and the
  connection works after restore. Verify an Azure-managed edition reports the unsupported
  `BACKUP TO URL` requirement instead of offering a local path.
- On Windows, test SQL Server Integrated authentication with the current identity. Separately test
  each configured Microsoft Entra flow over encrypted TLS and verify passwordless modes do not
  retain or request a database credential.
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
