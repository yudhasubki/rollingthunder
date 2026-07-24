# Storage and migration policy

Rolling Thunder stores connection metadata, operating-system credentials, diagnostics preferences,
and local workspace state separately. Every durable structured format that can affect user work
must be versioned and migrated explicitly.

## Current formats

| Data                      | Current version | Location                                              | Contains secrets    |
| ------------------------- | --------------: | ----------------------------------------------------- | ------------------- |
| Saved connection profiles |               2 | `connections.json` in the OS config directory         | No                  |
| Connection passwords      |      OS-managed | Keychain, Credential Manager, or Secret Service       | Yes                 |
| Query workspaces          |               1 | Webview local storage, `rollingthunder.workspace`     | Query text          |
| Named queries             |               1 | Webview local storage, `rollingthunder.saved-queries` | Query text          |
| Diagnostics preferences   |               1 | `diagnostics.json` in the OS config directory         | No                  |
| Diagnostic reports        |    1 per report | OS cache directory                                    | Redacted error data |

The application config directory is normally:

- macOS: `~/Library/Application Support/RollingThunder`
- Windows: `%AppData%\RollingThunder`
- Linux: `$XDG_CONFIG_HOME/RollingThunder` or `~/.config/RollingThunder`

Passwords use the credential service `RollingThunder.DatabaseProfiles` and the opaque saved-profile
ID as their account key. They are never returned by the saved-profile API.

## Compatibility rules

1. Additive optional fields may be read by the current version without a format bump.
2. A semantic or destructive schema change requires a version bump and a tested migration from
   every version still supported.
3. Migrations run one version at a time, validate their output, and write atomically only after all
   steps succeed.
4. Secrets are moved before plaintext is removed. If the OS credential store rejects a legacy
   password, the original profile file remains unchanged and the application reports the failure.
5. Unknown future versions fail closed. Rolling Thunder does not guess at their structure or
   silently downgrade them.
6. Migrations must be idempotent. Restarting after an interrupted migration must not duplicate,
   erase, or weaken stored data.
7. New releases may drop invalid individual workspace entries, but must not execute restored SQL or
   restore staged destructive operations. Only query tabs are currently restored.
8. Downgrade migrations are not automatic. Restore a backup created by the older release when
   rolling back.

## Version 1 to version 2 saved profiles

Early builds stored a raw JSON array and could include `config.password`. Version 2:

- wraps profiles in `{ "version": 2, "connections": [...] }`;
- writes metadata atomically with mode `0600` and its directory with mode `0700` where supported;
- stores each password in the operating-system credential store;
- persists only `hasPassword: true|false`, never the secret;
- preserves the plaintext source file if credential migration or the metadata rewrite fails.

The migration runs automatically the first time profiles are loaded. A keychain unlock prompt can
therefore appear after upgrading an older installation.

## Backup before upgrade

Close Rolling Thunder so pending query-tab writes have completed, then:

1. Copy the complete `RollingThunder` OS config directory.
2. Export or copy Webview site data if unsaved query tabs or named queries are important. Query text
   is not stored in `connections.json`.
3. Confirm database credentials are recoverable independently. OS credential stores may not be
   included in a normal file copy or may require the same operating-system account.
4. Keep normal database backups. Rolling Thunder profile/workspace backups do not back up database
   contents.

Diagnostic archives are not application backups.

## Recovery and rollback

- If a profile migration fails, keep the reported `connections.json` untouched, unlock or repair the
  OS credential service, and retry with the same application version.
- If workspace state is incompatible, copy the Webview local-storage value before opening or editing
  tabs. An unknown version loads as an empty safe workspace.
- To roll back the application, close it and restore the config and Webview data captured before the
  upgrade. Credentials may remain in the OS store and are harmless when no profile references them.
- Never hand-edit a password into `connections.json`. Save it through Manage connections so it goes
  to the OS credential store.

## Checklist for a new migration

- Define the new envelope version and a pure old-to-new conversion.
- Preserve the original until credential writes and validation have succeeded.
- Use an atomic temporary-file replacement and restrictive permissions.
- Add fixtures for success, malformed input, unknown future versions, interrupted writes, and
  dependency failure.
- Document user-visible changes and downgrade behavior here.
- Run the complete test matrix in [TESTING.md](TESTING.md).
