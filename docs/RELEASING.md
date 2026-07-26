# Release process

Publishing a GitHub Release starts the native Rolling Thunder build matrix and uploads the resulting
packages to that same release. Tests remain mandatory, while Windows and Linux signing is optional.
macOS artifacts are intentionally unsigned. Manual workflow runs produce preview artifacts when the
optional release-tag input is left blank, or rebuild and upload assets to an existing release when
the input is provided.

## Automated workflows

- `ci.yml`: Go tests, race detector, vet, frontend tests/lint/build, and a Linux Wails build.
- `integration.yml`: SQLite plus PostgreSQL 14-18, MySQL 8.4/9.7 LTS with legacy 8.0
  compatibility, MariaDB 10.11/11.4/11.8/12.3 LTS, and SQL Server 2022/2025 on every relevant
  change. Every SQL Server matrix entry requires server-forced encryption, negative CA/hostname
  checks, and the full driver contract over verified TLS, including disposable security
  administration and a native backup/restore round trip. The larger official Oracle Database Free
  image runs on the weekly schedule and manual workflow dispatches.
- `release.yml`: validates the source, builds native macOS arm64/amd64, Windows amd64, and Linux
  amd64 packages, optionally signs Windows/Linux builds, creates SHA-256 checksums, emits a
  GitHub/Sigstore provenance attestation, and uploads the assets to the published release.

The numeric part of the release tag is injected into Wails package metadata before each native
build. The installed application uses that same embedded version when comparing against GitHub's
latest published stable release. The Linux build uses WebKitGTK 4.1 and the `webkit2_41` build tag.

## Optional signing secrets

No signing secret is required to build or publish a release. If one field of a signing pair is
configured, its matching field is required so the workflow never silently falls back from a broken
signing configuration. Every build still receives checksums and a GitHub provenance attestation.

### macOS

No Apple Developer account or Apple secrets are required. The workflow publishes archives named
`rollingthunder_<version>_darwin_<arch>.zip`. They are not code-signed or notarized, so macOS
Gatekeeper can require explicit user approval on first launch.

### Windows

- `WINDOWS_CERTIFICATE_BASE64`: base64-encoded Authenticode `.pfx`
- `WINDOWS_CERTIFICATE_PASSWORD`: password for that `.pfx`

When both values are configured, the final NSIS installer is timestamped, signed, and verified with
`signtool`. Without them, the workflow publishes an unsigned installer.

### Linux

- `LINUX_GPG_PRIVATE_KEY_BASE64`: base64-encoded exported private GPG key
- `LINUX_GPG_PASSPHRASE`: key passphrase

When both values are configured, the `.tar.gz` receives an armored detached `.asc` signature.
Without them, the tarball is still covered by `SHA256SUMS` and the GitHub attestation.

Store signing material only as protected repository or environment secrets. Never commit
certificates, private keys, passwords, or decoded temporary files.

## Preview a release

Open **Actions → Release → Run workflow**, select the intended branch, leave **release_tag** blank,
and run it manually. The workflow performs the complete validation and native build matrix, then
exposes a `rollingthunder-release` workflow artifact. It does not create or modify a GitHub Release.

Use preview artifacts for cross-platform smoke testing before publishing a version tag.

## Publish a release

1. Complete the manual checklist in [TESTING.md](TESTING.md), including the accessibility audit.
2. Update `README.md`, `ROADMAP.md`, migration notes, and release-facing behavior.
3. Confirm CI and all integration matrix jobs are green for the exact commit.
4. Open **Releases → Draft a new release** on GitHub.
5. Create a semantic-version tag such as `v0.1.0-beta.1`, target the exact release commit, and
   prepare the release title and notes.
6. Mark unstable versions as a pre-release, then click **Publish release**.
7. Monitor the Release workflow. The published page initially contains GitHub's source archives;
   native packages appear after every validation and build job succeeds.
8. Download the uploaded files and verify them on clean target machines before announcing the
   release.

Editing release notes does not start a new build. Re-running the original workflow run is suitable
for transient failures because it keeps the original workflow SHA and ref.

## Retry an existing release

After committing a workflow fix to the default branch, open **Actions → Release → Run workflow**,
select the branch containing the fix, enter the existing tag in **release_tag**, and run it. The
workflow validates that the release exists, checks out the exact tag, rebuilds every native package,
and replaces same-named assets using `--clobber` without moving or recreating the tag.

## Verify artifacts

From the downloaded release directory:

```bash
sha256sum --check SHA256SUMS
gh attestation verify --owner yudhasubki <artifact>
```

When a Linux `.asc` signature is present:

```bash
gpg --verify rollingthunder_<version>_linux_amd64.tar.gz.asc \
  rollingthunder_<version>_linux_amd64.tar.gz
```

When the Windows installer is signed:

```powershell
Get-AuthenticodeSignature .\rollingthunder_<version>_windows_amd64_installer.exe
```

Checksums must match and the GitHub attestation must identify this repository and the expected
release workflow. Any optional Windows/Linux signature that is present must also validate. Test
each macOS archive on a clean Mac and document the expected first-launch Gatekeeper flow in the
release notes.
