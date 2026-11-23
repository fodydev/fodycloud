# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A or
# BUILD_WAIT come from e2e-test.sh and earlier parts (hence SC2153 is off).

section "Storage and databases: volume, PostgreSQL, Valkey"

STATIC='{"language":"static","framework":"static"}'
PG_IMAGE="$(remote "grep ^POSTGRES_IMAGE= /opt/fodycloud/.env | cut -d= -f2")"
VK_IMAGE="$(remote "grep ^VALKEY_IMAGE= /opt/fodycloud/.env | cut -d= -f2")"

# new_student PLAN -> token of a new student on PLAN (empty = free)
new_student() {
  local email="storage-$RANDOM$RANDOM@$DOMAIN"
  fc admin create-user --email "$email" >/dev/null
  [[ -n "$1" ]] && fc admin set-plan --email "$email" --plan "$1" --note e2e >/dev/null
  fc admin create-token --email "$email" --name e2e
}
# deploy_static TOKEN APP: a static site, deployed and running.
deploy_static() {
  api POST /v1/apps "$1" "{\"name\":\"$2\",\"repo_url\":\"https://github.com/octocat/Spoon-Knife\",\"branch\":\"main\",\"stack\":$STATIC}" >/dev/null
  wait_running "$1" "$2"
}
# wait_running TOKEN APP: waits for the app's latest deployment; prints its status.
wait_running() {
  local st=""
  for _ in $(seq 1 120); do
    st="$(body_of "$(api GET "/v1/apps/$2/deployments" "$1")" | jq -r '.deployments[0].status')"
    case "$st" in running|failed|cancelled) break ;; esac
    sleep 3
  done
  echo "$st"
}
ctr() { remote "docker ps --filter label=fodycloud.app=$1 --format '{{.Names}}' | head -1"; }
service_url() { body_of "$(api GET "/v1/apps/$2/services" "$1")" | jq -r ".services[] | select(.kind==\"$3\") | .url"; }
# sql APP URL SQL: runs SQL with psql from the app's own network namespace
# (as the app would), one session; prints the output.
sql() {
  local b64; b64="$(printf '%s\n' "$3" | base64 -w0)"
  remote "echo $b64 | base64 -d | docker run --rm -i --network container:$(ctr "$1") $PG_IMAGE psql -v ON_ERROR_STOP=1 -tA '$2' -f - 2>&1"
}
# on_volume APP CMD: runs a shell command with the app's volume at /data.
on_volume() { remote "docker run --rm -v fc-vol-$1:/data busybox:1.37.0 sh -c '$2' 2>&1"; }

TOK_S2="$(new_student student)"
TOK_F2="$(new_student "")"
TOK_T="$(new_student tiny)"
check "deploy a student's app" test "$(deploy_static "$TOK_S2" e2e-store)" = running
check "deploy a free app" test "$(deploy_static "$TOK_F2" e2e-store-free)" = running
check "deploy a tiny-plan app" test "$(deploy_static "$TOK_T" e2e-store-tiny)" = running

# --- Volume
check "the free plan has no volume" expect 403 "$(api POST /v1/apps/e2e-store-free/services/volume "$TOK_F2")" "doesn't include a persistent volume"
check "add a volume" expect 202 "$(api POST /v1/apps/e2e-store/services/volume "$TOK_S2")" '"path":"/data"'
check "...only once" expect 422 "$(api POST /v1/apps/e2e-store/services/volume "$TOK_S2")" "already has"
check "the app restarts with the volume" test "$(wait_running "$TOK_S2" e2e-store)" = running
check "...mounted at /data, FODYCLOUD_DATA_DIR set" remote \
  "docker inspect $(ctr e2e-store) --format '{{range .Mounts}}{{.Name}}:{{.Destination}} {{end}}{{range .Config.Env}}{{.}} {{end}}' | grep -q 'fc-vol-e2e-store:/data.*FODYCLOUD_DATA_DIR=/data'"
on_volume e2e-store 'echo kept > /data/note.txt' >/dev/null
api POST /v1/apps/e2e-store/restart "$TOK_S2" >/dev/null
check "files survive a redeployment" bash -c '[[ "$1" == running ]] && [[ "$2" == kept ]]' _ "$(wait_running "$TOK_S2" e2e-store)" "$(on_volume e2e-store 'cat /data/note.txt')"
api POST /v1/apps/e2e-store/stop "$TOK_S2" >/dev/null
api POST /v1/apps/e2e-store/start "$TOK_S2" >/dev/null
check "...and a stop and start" bash -c '[[ "$1" == kept ]]' _ "$(on_volume e2e-store 'cat /data/note.txt')"
api POST /v1/apps/e2e-store-tiny/services/volume "$TOK_T" >/dev/null
wait_running "$TOK_T" e2e-store-tiny >/dev/null
check "a volume holds files within its size (positive control)" bash -c '[[ "$1" == *"10.0MB"* ]]' _ \
  "$(on_volume e2e-store-tiny 'dd if=/dev/zero of=/data/a bs=1M count=10 2>&1 | tail -1')"
check "...and refuses more than its 20 MB" bash -c '[[ "$1" == *"No space left"* ]]' _ \
  "$(on_volume e2e-store-tiny 'dd if=/dev/zero of=/data/b bs=1M count=40 2>&1')"

# --- PostgreSQL
check "add a PostgreSQL database" expect 202 "$(api POST /v1/apps/e2e-store/services/postgres "$TOK_S2")" '"kind":"postgres"'
check "the app restarts with DATABASE_URL" bash -c '[[ "$1" == running ]]' _ "$(wait_running "$TOK_S2" e2e-store)"
URL_S="$(service_url "$TOK_S2" e2e-store postgres)"
check "...in its environment" remote "docker inspect $(ctr e2e-store) --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -qx 'DATABASE_URL=$URL_S'"
check "the app connects to its database and writes" bash -c '[[ "$1" == *"hello"* ]]' _ \
  "$(sql e2e-store "$URL_S" "CREATE TABLE notes (t text); INSERT INTO notes VALUES ('hello'); SELECT t FROM notes;")"
api POST /v1/apps/e2e-store-tiny/services/postgres "$TOK_T" >/dev/null
wait_running "$TOK_T" e2e-store-tiny >/dev/null
URL_T="$(service_url "$TOK_T" e2e-store-tiny postgres)"
DB_T="${URL_T##*/}"
check "another app's database is closed to it, even with its own password" bash -c '[[ "$1" == *"no pg_hba.conf entry"* ]]' _ \
  "$(sql e2e-store "${URL_S%/*}/$DB_T" "SELECT 1;")"
check "...while that app reaches its own (positive control)" bash -c '[[ "$1" == 1 ]]' _ "$(sql e2e-store-tiny "$URL_T" "SELECT 1;")"
check "the superuser can't log in from an app" bash -c '[[ "$1" == *"pg_hba.conf rejects"* ]]' _ \
  "$(sql e2e-store "postgresql://fcadmin:guess@${URL_S#*@}" "SELECT 1;")"
DB_S="${URL_S##*/}"
check "another app can't use this database without its password" bash -c '[[ "$1" == *"password authentication failed"* ]]' _ \
  "$(sql e2e-store-tiny "postgresql://$DB_S:wrong@${URL_S#*@}" "SELECT 1;")"
# Size limit (tiny plan: 20 MB): over it the database becomes read-only.
# About 23.5 MB: over the limit (20.97 MB), under the 120% (25.2 MB) that blocks a
# database growing while read-only (empty database 7.9 MB, about 1.2 KB a row).
sql e2e-store-tiny "$URL_T" "CREATE TABLE big AS SELECT repeat(md5(i::text), 32) AS x FROM generate_series(1, 13000) i;" >/dev/null
fc admin check-storage >/dev/null
check "a database over its size becomes read-only" bash -c '[[ "$1" == *"read-only transaction"* ]]' _ \
  "$(sql e2e-store-tiny "$URL_T" "INSERT INTO big VALUES ('x');")"
check "...the API says so" bash -c 'jq -e ".services[] | select(.kind==\"postgres\") | .read_only" <<<"$1" >/dev/null' _ \
  "$(body_of "$(api GET /v1/apps/e2e-store-tiny/services "$TOK_T")")"
EMAIL_T="$(body_of "$(api GET /v1/me "$TOK_T")" | jq -r .email)"
check "...and the student is told by email" mail_has "$EMAIL_T" "e2e-store-tiny is full" "read-only"
sql e2e-store-tiny "$URL_T" "SET default_transaction_read_only = off;
DROP TABLE big;
VACUUM FULL;" >/dev/null
fc admin check-storage >/dev/null
check "deleting data makes it writable again" bash -c '[[ "$1" == *"INSERT 0 1"* && "$1" != *read-only* ]]' _ \
  "$(sql e2e-store-tiny "$URL_T" "CREATE TABLE small (x int); INSERT INTO small VALUES (1);")"
check "...and the student is told" mail_has "$EMAIL_T" "e2e-store-tiny accepts writes again" "can change its data again"

# --- Valkey
check "add a Valkey cache" expect 202 "$(api POST /v1/apps/e2e-store/services/valkey "$TOK_S2")" '"kind":"valkey"'
check "the app restarts with REDIS_URL" test "$(wait_running "$TOK_S2" e2e-store)" = running
URL_V="$(service_url "$TOK_S2" e2e-store valkey)"
PW_V="${URL_V#redis://default:}"; PW_V="${PW_V%%@*}"
check "the app reaches its cache" bash -c '[[ "$1" == PONG ]]' _ \
  "$(remote "docker run --rm --network container:$(ctr e2e-store) $VK_IMAGE valkey-cli --no-auth-warning -u '$URL_V' ping 2>&1")"
check "another app can't reach it" bash -c '[[ "$1" != *PONG* ]]' _ \
  "$(remote "docker run --rm --network container:$(ctr e2e-store-tiny) $VK_IMAGE timeout 5 valkey-cli --no-auth-warning -u '$URL_V' ping 2>&1")"
check "its memory follows the plan (128 MB)" bash -c '[[ "$1" == *134217728* ]]' _ \
  "$(remote "docker exec -e REDISCLI_AUTH=$PW_V fc-valkey-e2e-store valkey-cli config get maxmemory 2>&1" | tr -d '\r')"
check "removing needs the app name" expect 422 "$(api DELETE "/v1/apps/e2e-store/services/valkey?confirm=wrong" "$TOK_S2")" "does not match"
check "remove the cache" expect 204 "$(api DELETE "/v1/apps/e2e-store/services/valkey?confirm=e2e-store" "$TOK_S2")"
wait_running "$TOK_S2" e2e-store >/dev/null
check "...its container and network are gone" remote "! docker ps -a --format '{{.Names}}' | grep -qx fc-valkey-e2e-store && ! docker network inspect fc-net-e2e-store >/dev/null 2>&1"

# --- Dashboard and deletion
SJ="$(mktemp)"
curl -sk "${RES[@]}" -b "$SJ" -c "$SJ" -H "Origin: $D" -o /dev/null --data-urlencode "email=$(body_of "$(api GET /v1/me "$TOK_S2")" | jq -r .email)" --data-urlencode "password=$PASSWORD" "$D/login"
check "the app page shows the storage card" bash -c 'grep -q "Storage and databases" <<<"$1" && grep -q "Show the connection URL" <<<"$1"' _ "$(curl -sk "${RES[@]}" -b "$SJ" "$D/apps/e2e-store")"
rm -f "$SJ"
check "deleting an app deletes its volume and database" expect 204 "$(api DELETE "/v1/apps/e2e-store-tiny?confirm=e2e-store-tiny" "$TOK_T")"
check "...the volume is gone" remote "! docker volume inspect fc-vol-e2e-store-tiny >/dev/null 2>&1"
check "...the database is gone" remote "cd /opt/fodycloud && ! docker compose exec -T apps-postgres psql -U fcadmin -d fcadmin -tAc \"SELECT 1 FROM pg_database WHERE datname='$DB_T'\" | grep -q 1"
# e2e-store keeps its volume and database for the backup checks.
