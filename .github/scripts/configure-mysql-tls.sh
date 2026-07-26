#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <container-id> <output-directory> <client-common-name>" >&2
  exit 2
fi

container_id="$1"
output_directory="$2"
client_common_name="$3"
server_tls_directory="/etc/mysql/rolling-thunder-test-tls"
server_config="/etc/mysql/conf.d/rolling-thunder-test-tls.cnf"
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
    -client-common-name "$client_common_name"
)

temporary_configuration="$(mktemp)"
trap 'rm -f "$temporary_configuration"' EXIT
cat >"$temporary_configuration" <<EOF
[mysqld]
ssl_ca=${server_tls_directory}/ca-cert.pem
ssl_cert=${server_tls_directory}/server-cert.pem
ssl_key=${server_tls_directory}/server-key.pem
require_secure_transport=ON
EOF

docker exec --user root "$container_id" \
  mkdir -p "$server_tls_directory" /etc/mysql/conf.d
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
  "$temporary_configuration" \
  "$container_id:$server_config"
docker exec --user root "$container_id" \
  chown -R mysql:mysql "$server_tls_directory"
docker exec --user root "$container_id" \
  chmod 0600 "$server_tls_directory/server-key.pem"
docker exec --user root "$container_id" \
  chmod 0644 \
  "$server_tls_directory/ca-cert.pem" \
  "$server_tls_directory/server-cert.pem" \
  "$server_config"

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
  echo "MySQL-compatible database did not become healthy after enabling TLS" >&2
  docker logs "$container_id" >&2
  exit 1
fi
