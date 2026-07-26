#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <container-id> <output-directory> <tcps-port>" >&2
  exit 2
fi

container_id="$1"
output_directory="$2"
tcps_port="$3"
wallet_directory="/opt/oracle/rolling-thunder-test-wallet"
database_ready_attempts=90
database_ready_delay_seconds=2
listener_ready_attempts=30
listener_ready_delay_seconds=1

umask 077
mkdir -p "$output_directory"
wallet_password="$(openssl rand -hex 24)"
printf '%s' "$wallet_password" >"$output_directory/wallet-password"

oracle_home="$(docker exec "$container_id" printenv ORACLE_HOME)"
if [[ -z "$oracle_home" ]]; then
  echo "Oracle service container does not expose ORACLE_HOME" >&2
  exit 1
fi

database_ready="false"
for _ in $(seq 1 "$database_ready_attempts"); do
  if printf 'WHENEVER SQLERROR EXIT SQL.SQLCODE\nSELECT 1 FROM dual;\nEXIT;\n' |
    docker exec -i --user oracle "$container_id" \
      "$oracle_home/bin/sqlplus" -s / as sysdba >/dev/null 2>&1; then
    database_ready="true"
    break
  fi
  sleep "$database_ready_delay_seconds"
done
if [[ "$database_ready" != "true" ]]; then
  echo "Oracle Database did not become ready for TCPS configuration" >&2
  exit 1
fi

docker exec --user root "$container_id" \
  mkdir -p "$wallet_directory"
docker exec --user root "$container_id" \
  chown oracle:oinstall "$wallet_directory"
docker exec --user root "$container_id" \
  rm -f \
  "$wallet_directory/cwallet.sso" \
  "$wallet_directory/ewallet.p12" \
  "$wallet_directory/server-cert.pem"
docker exec --user oracle "$container_id" \
  "$oracle_home/bin/orapki" wallet create \
  -wallet "$wallet_directory" \
  -pwd "$wallet_password" \
  -auto_login
docker exec --user oracle "$container_id" \
  "$oracle_home/bin/orapki" wallet add \
  -wallet "$wallet_directory" \
  -dn "CN=localhost" \
  -keysize 2048 \
  -self_signed \
  -validity 3650 \
  -addext_san "DNS:localhost,IPV4Address:127.0.0.1" \
  -pwd "$wallet_password"
docker exec --user oracle "$container_id" \
  "$oracle_home/bin/orapki" wallet export \
  -wallet "$wallet_directory" \
  -dn "CN=localhost" \
  -cert "$wallet_directory/server-cert.pem" \
  -pwd "$wallet_password"

listener_configuration="$(mktemp)"
sqlnet_configuration="$(mktemp)"
trap 'rm -f "$listener_configuration" "$sqlnet_configuration"' EXIT
cat >"$listener_configuration" <<EOF
LISTENER =
  (DESCRIPTION_LIST =
    (DESCRIPTION =
      (ADDRESS = (PROTOCOL = IPC)(KEY = EXTPROC1))
      (ADDRESS = (PROTOCOL = TCP)(HOST = 0.0.0.0)(PORT = 1521))
      (ADDRESS = (PROTOCOL = TCPS)(HOST = 0.0.0.0)(PORT = ${tcps_port}))
    )
  )

WALLET_LOCATION =
  (SOURCE =
    (METHOD = FILE)
    (METHOD_DATA =
      (DIRECTORY = ${wallet_directory})
    )
  )

SSL_CLIENT_AUTHENTICATION = FALSE
DEDICATED_THROUGH_BROKER_LISTENER = ON
DIAG_ADR_ENABLED = OFF
EOF

cat >"$sqlnet_configuration" <<EOF
NAMES.DIRECTORY_PATH = (TNSNAMES, EZCONNECT, HOSTNAME)
DISABLE_OOB = ON
SQLNET.EXPIRE_TIME = 3

WALLET_LOCATION =
  (SOURCE =
    (METHOD = FILE)
    (METHOD_DATA =
      (DIRECTORY = ${wallet_directory})
    )
  )

SSL_CLIENT_AUTHENTICATION = FALSE
EOF

docker cp "$listener_configuration" \
  "$container_id:/tmp/rolling-thunder-listener.ora"
docker cp "$sqlnet_configuration" \
  "$container_id:/tmp/rolling-thunder-sqlnet.ora"
docker exec --user root "$container_id" \
  mv /tmp/rolling-thunder-listener.ora \
  "$oracle_home/network/admin/listener.ora"
docker exec --user root "$container_id" \
  mv /tmp/rolling-thunder-sqlnet.ora \
  "$oracle_home/network/admin/sqlnet.ora"
docker exec --user root "$container_id" \
  chown oracle:oinstall \
  "$oracle_home/network/admin/listener.ora" \
  "$oracle_home/network/admin/sqlnet.ora"

docker exec --user oracle "$container_id" \
  "$oracle_home/bin/lsnrctl" stop LISTENER >/dev/null
docker exec --user oracle "$container_id" \
  "$oracle_home/bin/lsnrctl" start LISTENER >/dev/null
printf 'WHENEVER SQLERROR EXIT SQL.SQLCODE\nALTER SYSTEM REGISTER;\nEXIT;\n' |
  docker exec -i --user oracle "$container_id" \
    "$oracle_home/bin/sqlplus" -s / as sysdba >/dev/null

docker cp "$container_id:$wallet_directory/." "$output_directory"
chmod -R u+rwX,go-rwx "$output_directory"

listener_ready="false"
for _ in $(seq 1 "$listener_ready_attempts"); do
  if openssl s_client \
    -connect "127.0.0.1:${tcps_port}" \
    -servername localhost \
    -CAfile "$output_directory/server-cert.pem" \
    -verify_hostname localhost \
    -verify_return_error </dev/null >/dev/null 2>&1; then
    listener_ready="true"
    break
  fi
  sleep "$listener_ready_delay_seconds"
done
if [[ "$listener_ready" != "true" ]]; then
  echo "Oracle TCPS listener did not become ready" >&2
  docker exec --user oracle "$container_id" \
    "$oracle_home/bin/lsnrctl" status LISTENER >&2 || true
  exit 1
fi
