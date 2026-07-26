#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <container-id> <output-directory> <client-role>" >&2
  exit 2
fi

container_id="$1"
output_directory="$2"
client_role="$3"
server_tls_directory="/var/lib/postgresql/rolling-thunder-test-tls"
health_attempts=60
health_delay_seconds=2

if [[ ! "$client_role" =~ ^[a-z_][a-z0-9_]*$ ]]; then
  echo "PostgreSQL client role contains unsupported characters" >&2
  exit 2
fi

script_directory="$(
  cd "$(dirname "${BASH_SOURCE[0]}")"
  pwd
)"
repository_root="$(
  cd "$script_directory/../.."
  pwd
)"
if [[ "$output_directory" != /* ]]; then
  output_directory="$PWD/$output_directory"
fi
(
  cd "$repository_root"
  go run ./scripts/testcert \
    -output "$output_directory" \
    -client-common-name "$client_role"
)

postgres_data="$(
  docker exec "$container_id" printenv PGDATA
)"
if [[ -z "$postgres_data" || "$postgres_data" != /* ]]; then
  echo "PostgreSQL service container exposes an invalid PGDATA" >&2
  exit 1
fi

temporary_directory="$(mktemp -d)"
trap 'rm -rf "$temporary_directory"' EXIT

docker cp \
  "$container_id:$postgres_data/postgresql.conf" \
  "$temporary_directory/postgresql.original.conf"
docker cp \
  "$container_id:$postgres_data/pg_hba.conf" \
  "$temporary_directory/pg_hba.original.conf"

awk '
  $0 == "# rolling thunder tls begin" { skipping = 1; next }
  $0 == "# rolling thunder tls end" { skipping = 0; next }
  !skipping { print }
' "$temporary_directory/postgresql.original.conf" \
  >"$temporary_directory/postgresql.conf"
cat >>"$temporary_directory/postgresql.conf" <<EOF

# rolling thunder tls begin
ssl = on
ssl_ca_file = '${server_tls_directory}/ca-cert.pem'
ssl_cert_file = '${server_tls_directory}/server-cert.pem'
ssl_key_file = '${server_tls_directory}/server-key.pem'
# rolling thunder tls end
EOF

awk '
  $0 == "# rolling thunder tls begin" { skipping = 1; next }
  $0 == "# rolling thunder tls end" { skipping = 0; next }
  !skipping { print }
' "$temporary_directory/pg_hba.original.conf" \
  >"$temporary_directory/pg_hba.base.conf"
{
  echo "# rolling thunder tls begin"
  echo "hostssl all ${client_role} 0.0.0.0/0 cert"
  echo "hostssl all ${client_role} ::/0 cert"
  echo "hostnossl all all 0.0.0.0/0 reject"
  echo "hostnossl all all ::/0 reject"
  echo "# rolling thunder tls end"
  cat "$temporary_directory/pg_hba.base.conf"
} >"$temporary_directory/pg_hba.conf"

docker exec --user root "$container_id" \
  mkdir -p "$server_tls_directory"
docker cp \
  "$output_directory/ca-cert.pem" \
  "$container_id:$server_tls_directory/ca-cert.pem"
docker cp \
  "$output_directory/server-cert.pem" \
  "$container_id:$server_tls_directory/server-cert.pem"
docker cp \
  "$output_directory/server-key.pem" \
  "$container_id:$server_tls_directory/server-key.pem"
docker cp \
  "$temporary_directory/postgresql.conf" \
  "$container_id:$postgres_data/postgresql.conf"
docker cp \
  "$temporary_directory/pg_hba.conf" \
  "$container_id:$postgres_data/pg_hba.conf"
docker exec --user root "$container_id" \
  chown -R postgres:postgres "$server_tls_directory"
docker exec --user root "$container_id" \
  chown postgres:postgres \
  "$postgres_data/postgresql.conf" \
  "$postgres_data/pg_hba.conf"
docker exec --user root "$container_id" \
  chmod 0600 \
  "$server_tls_directory/server-key.pem" \
  "$postgres_data/postgresql.conf" \
  "$postgres_data/pg_hba.conf"

docker restart "$container_id" >/dev/null

database_ready="false"
for _ in $(seq 1 "$health_attempts"); do
  health="$(
    docker inspect \
      --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
      "$container_id"
  )"
  if [[ "$health" == "healthy" ]]; then
    database_ready="true"
    break
  fi
  sleep "$health_delay_seconds"
done
if [[ "$database_ready" != "true" ]]; then
  echo "PostgreSQL did not become healthy after enabling TLS" >&2
  docker logs "$container_id" >&2
  exit 1
fi
