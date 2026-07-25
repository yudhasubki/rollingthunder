# Privacy and diagnostics

Rolling Thunder is a local desktop database client. It does not automatically upload diagnostic
reports, queries, table data, connection profiles, or credentials.

## Defaults

- Optional frontend diagnostics are **disabled by default**.
- Basic system information is **disabled by default** and can only be enabled while optional
  diagnostics are enabled.
- A minimal local crash report may be written for an unhandled top-level application panic even
  while optional diagnostics are off. It is never uploaded automatically.
- At most 10 local diagnostic/crash reports are retained; older reports are deleted first.

Privacy settings are available from the connection hub and the shield button in the application
header.

## Data recorded

When optional diagnostics are enabled, an unhandled frontend error can record:

- the error source;
- a redacted error message;
- a redacted JavaScript stack trace;
- the UTC occurrence time.

If "Include basic system information" is also enabled, reports add only:

- operating system;
- CPU architecture;
- Go runtime version;
- logical CPU count.

A local crash report records a generic panic category and a redacted Go stack trace. The raw panic
value is intentionally not stored.

Reports do not intentionally collect query text, result rows, imported/exported files, schema
metadata, saved profiles, usernames, or passwords.

## Redaction

Before a report is written, Rolling Thunder replaces:

- credentials embedded in PostgreSQL, MySQL, or MariaDB URLs;
- password, token, and secret key/value fields;
- bearer tokens;
- private-key blocks;
- single-quoted values;
- the current user's home-directory path.

Redaction is defense in depth, not a mathematical guarantee. Error text can come from third-party
drivers and operating systems. Always review every JSON file in an exported ZIP before sharing it.

## Storage and retention

Diagnostics preferences are stored as a versioned `0600` file in the Rolling Thunder OS config
directory. Reports are stored as `0600` JSON files under the Rolling Thunder OS cache directory:

- macOS: `~/Library/Caches/RollingThunder/diagnostics`
- Windows: `%LocalAppData%\RollingThunder\diagnostics`
- Linux: `$XDG_CACHE_HOME/RollingThunder/diagnostics` or
  `~/.cache/RollingThunder/diagnostics`

File permissions are applied where the operating system supports Unix-style modes. Access is still
governed by the current OS account and its filesystem policy.

## Export and deletion

- "Export local reports" opens a native destination picker and creates a ZIP only after a deliberate
  user action.
- Export does not transmit the ZIP. The user decides whether and how to share it.
- "Clear local reports" requires a second confirmation and deletes retained report files.
- Disabling optional diagnostics stops future frontend reports; it does not silently delete existing
  reports. Use Clear for that.
- Clearing reports does not remove saved profiles or keychain credentials.

## Credentials

Saved passwords are stored by the operating system:

- macOS Keychain;
- Windows Credential Manager;
- Linux Secret Service-compatible keyring.

`connections.json` contains non-secret metadata and a `hasPassword` flag only. Frontend profile
screens never receive a saved password. A blank password field preserves an existing credential;
removal requires a separate two-step action.

## Network behavior

Rolling Thunder connects to database endpoints explicitly configured by the user. The diagnostics
manager itself has no upload endpoint and performs no network request.

At application startup, Rolling Thunder makes one unauthenticated request to the public GitHub
Releases API to check the latest published stable version. The request includes the installed
application version in its user-agent and necessarily exposes ordinary connection metadata such as
the device IP address to GitHub. It does not include queries, table data, database endpoints, saved
profiles, credentials, diagnostics, or a device identifier. A failed check is ignored and never
blocks startup.

When a newer release exists, its version, publication date, and plain-text release notes are shown
in a local dialog. Choosing "Remind me tomorrow" stores only the release version and reminder
deadline in browser-local application storage. Choosing "Download update" opens the trusted
Rolling Thunder GitHub Release page in the system browser; installation is never started silently.
