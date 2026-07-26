#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 2 ]]; then
  echo "usage: $0 <container-id> <output-directory>" >&2
  exit 2
fi

container_id="$1"
output_directory="$2"
server_tls_directory="/var/opt/mssql/secrets/rolling-thunder-test-tls"
server_certificate="$server_tls_directory/server-cert.pem"
server_key="$server_tls_directory/server-key.pem"
health_attempts=90
health_delay_seconds=2

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
    -server-common-name sqlserver.test
)

docker exec --user root "$container_id" \
  mkdir -p "$server_tls_directory"
docker cp \
  "$output_directory/server-cert.pem" \
  "$container_id:$server_certificate"
docker cp \
  "$output_directory/server-key.pem" \
  "$container_id:$server_key"
docker exec --user root "$container_id" \
  chown -R mssql:mssql "$server_tls_directory"
docker exec --user root "$container_id" \
  chmod 0700 "$server_tls_directory"
docker exec --user root "$container_id" \
  chmod 0400 "$server_certificate" "$server_key"

docker exec --user root "$container_id" \
  /opt/mssql/bin/mssql-conf set network.tlscert "$server_certificate"
docker exec --user root "$container_id" \
  /opt/mssql/bin/mssql-conf set network.tlskey "$server_key"
docker exec --user root "$container_id" \
  /opt/mssql/bin/mssql-conf set network.tlsprotocols 1.2
docker exec --user root "$container_id" \
  /opt/mssql/bin/mssql-conf set network.forceencryption 1

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
  echo "SQL Server did not become healthy after enabling TLS" >&2
  docker logs "$container_id" >&2
  exit 1
fi
