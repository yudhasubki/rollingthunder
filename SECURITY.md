# Security policy

Rolling Thunder is under active development. Until stable releases are published, security fixes
are provided on the current `main` branch only.

## Report a vulnerability

Use GitHub's private vulnerability reporting or a private Security Advisory for this repository.
Do not open a public issue containing credentials, connection strings, private schema/query data,
diagnostic archives, certificate material, or an exploit that could damage a database.

Include:

- affected commit or release and operating system;
- database engine/version;
- impact and realistic attack path;
- minimal reproduction using disposable data;
- whether OS credential storage, imported/exported files, or a destructive workflow is involved.

Please allow maintainers time to reproduce and prepare a coordinated fix before public disclosure.

## Security model

- Saved passwords belong in macOS Keychain, Windows Credential Manager, or Linux Secret Service,
  never in profile JSON.
- The frontend receives only non-secret profile metadata and a `hasPassword` flag.
- Database operations run with the permissions of the configured database account. Rolling Thunder
  is not a permission boundary and cannot undo committed database changes.
- Connection attempts and health checks are bounded and cancellable. Controlled reconnect installs a
  replacement only after it connects successfully.
- Export uses temporary files so cancellation/failure does not replace an existing destination.
- Optional diagnostics default off, remain local, and are redacted. Review archives before sharing.
- Tagged release packages require platform signatures and receive checksums plus build-provenance
  attestations.

Use least-privilege database accounts, TLS where supported, independent backups, and a non-production
environment for testing structural or destructive workflows.

See [docs/PRIVACY.md](docs/PRIVACY.md), [docs/MIGRATIONS.md](docs/MIGRATIONS.md), and
[docs/RELEASING.md](docs/RELEASING.md) for operational details.
