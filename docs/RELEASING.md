# Release process

GitHub Actions builds Rolling Thunder on each target operating system. Tagged releases are gated on
tests and the configured Windows/Linux signing credentials. macOS artifacts are intentionally
unsigned, and manual workflow runs produce preview artifacts without publishing a GitHub release.

## Automated workflows

- `ci.yml`: Go tests, race detector, vet, frontend tests/lint/build, and a Linux Wails build.
- `integration.yml`: SQLite plus PostgreSQL 14–18, MySQL 8.0/8.4 LTS, and MariaDB 10.11/11.4 LTS.
- `release.yml`: validates the source, builds native macOS arm64/amd64, Windows amd64, and Linux
  amd64 packages, signs Windows/Linux tag builds, creates SHA-256 checksums, emits a GitHub/Sigstore
  provenance attestation, and publishes the tag release.

The numeric part of the release tag is injected into Wails package metadata before each native
build. The installed application uses that same embedded version when comparing against GitHub's
latest published stable release. The Linux build uses WebKitGTK 4.1 and the `webkit2_41` build tag.

## Required release secrets

Tagged releases stop rather than publish unsigned Windows or Linux packages when their required
secrets are missing.

### macOS

No Apple Developer account or Apple secrets are required. The workflow publishes archives named
`rollingthunder_<version>_darwin_<arch>.zip`. They are not code-signed or notarized, so macOS
Gatekeeper can require explicit user approval on first launch.

### Windows

- `WINDOWS_CERTIFICATE_BASE64`: base64-encoded Authenticode `.pfx`
- `WINDOWS_CERTIFICATE_PASSWORD`: password for that `.pfx`

The final NSIS installer is timestamped, signed, and verified with `signtool`.

### Linux

- `LINUX_GPG_PRIVATE_KEY_BASE64`: base64-encoded exported private GPG key
- `LINUX_GPG_PASSPHRASE`: key passphrase

The `.tar.gz` receives an armored detached `.asc` signature.

Store signing material only as protected repository or environment secrets. Never commit
certificates, private keys, passwords, or decoded temporary files.

## Create a release

1. Complete the manual checklist in [TESTING.md](TESTING.md), including the accessibility audit.
2. Update `README.md`, `ROADMAP.md`, migration notes, and release-facing behavior.
3. Confirm CI and all integration matrix jobs are green for the exact commit.
4. Create and push an annotated semantic-version tag, for example:

   ```bash
   git tag -a v0.4.0 -m "Rolling Thunder v0.4.0"
   git push origin v0.4.0
   ```

5. Monitor the Release workflow. It will refuse malformed tags or missing Windows/Linux signing
   credentials.
6. Download the published files and verify them on clean target machines before announcing the
   release.

Re-running a successful tag workflow replaces assets on the existing GitHub release. It does not
move or recreate the Git tag.

## Verify artifacts

From the downloaded release directory:

```bash
sha256sum --check SHA256SUMS
gh attestation verify --owner yudhasubki <artifact>
```

Linux signature verification:

```bash
gpg --verify rollingthunder_<version>_linux_amd64.tar.gz.asc \
  rollingthunder_<version>_linux_amd64.tar.gz
```

On Windows:

```powershell
Get-AuthenticodeSignature .\rollingthunder_<version>_windows_amd64_installer.exe
```

Windows/Linux signature status must be valid, checksums must match, and the GitHub attestation must
identify this repository and the expected release workflow. Test each macOS archive on a clean Mac
and document the expected first-launch Gatekeeper flow in the release notes.
