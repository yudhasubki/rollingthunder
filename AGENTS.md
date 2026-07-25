# Rolling Thunder agent guide

## Project guardrails

- Keep product identity, storage keys, provider defaults, UI timing, and other frontend runtime
  policy in `frontend/src/lib/config/application.ts`.
- Keep application identity shared by Go packages in `pkg/application/metadata.go`.
- Keep backend operational timeouts in `internal/db/runtime_policy.go` and driver-specific pool
  defaults beside their driver. Do not introduce unexplained magic values in workflows.
- Use the semantic tokens defined in `frontend/src/app.css`. Components must not use literal colors
  or raw framework palette classes.
- Color must communicate state: neutral for structure/metadata/idle, info for active or navigable,
  success for completed and verified, warning for pending/elevated risk, and danger for failures,
  destructive actions, or production risk.
- Database providers and decorative categories stay neutral. Connection profiles use
  `unclassified`, `development`, `staging`, or `production`; do not restore arbitrary profile
  colors.

## Security boundaries

- Validate connection metadata before it reaches a driver or an external command. Keep supported
  TLS and SSH modes allowlisted and ports within `1..65535`.
- Never persist database passwords, SSH passwords, or key passphrases in profile JSON. Use the
  operating-system credential store.
- Never pass database secrets in command arguments or broadly inherited environment variables.
  MySQL/MariaDB maintenance uses a temporary `0600` defaults file; PostgreSQL maintenance uses a
  temporary `0600` password file, and both must be removed on every exit path.
- Preserve atomic restrictive writes for profiles and diagnostics. Configuration-directory errors
  must fail closed rather than falling back to the working directory.
- SSH host verification stays strict. Destructive database operations require a reviewed preview
  and explicit confirmation.

## Regression gate

Run before handing off a completed change:

```bash
go test ./...
go test -race ./internal/db ./internal/diagnostics ./pkg/database/...
go vet ./...

cd frontend
npm test
npm run lint
npm run build
```

Build the desktop bundle with Wails before a release. Frontend tests include a static semantic-color
contract; update the centralized design tokens instead of bypassing that test.
