#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# End-to-end checks of a provisioned FodyCloud node (the local QEMU VM by default).
#
#   ./smoke-test.sh
#
# Connection settings come from .work/net.env (see testlib.sh); env overrides:
# HOST, SSH_PORT, HTTP_PORT, HTTPS_PORT, SSH_KEY, DOMAIN
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=qemu/testlib.sh
source "$HERE/testlib.sh"
cleanup() { remote 'docker rm -f fc-test-app1 fc-test-app2 >/dev/null 2>&1' || true; }
trap cleanup EXIT

section "Host"
check "platform containers all running and healthy" remote '
  cd /opt/fodycloud
  for svc in traefik socket-proxy postgres valkey control-plane; do
    docker compose ps --status running --format "{{.Service}}" | grep -qx "$svc" || { echo "$svc is not running"; exit 1; }
  done
  # every service that defines a healthcheck must report healthy (test fixtures included)
  ! docker compose ps --format "{{.Service}} {{.Status}}" | grep "(health" | grep -v "(healthy)"'
check "/var/lib/docker is XFS with project quotas" remote 'findmnt -no FSTYPE,OPTIONS /var/lib/docker | grep -q "^xfs .*prjquota"'
check "swap active" remote 'swapon --show=NAME --noheadings | grep -qx /swapfile'
check "timezone Africa/Douala" remote 'test "$(timedatectl show -p Timezone --value)" = Africa/Douala'
check "firewall table loaded, input policy drop" remote 'nft list chain inet fodycloud input | grep -q "policy drop"'
check "only ssh/http/https listen on public addresses" remote '
  ! ss -Hltn | awk "{print \$4}" | grep -vE "^(127\.|\[::1\]|\[::ffff:127)" | grep -vE ":(22|80|443)$"'
check "sshd: password login disabled" remote 'sshd -T | grep -qx "passwordauthentication no"'
check "password SSH attempt refused (publickey only)" bash -c "ssh -o BatchMode=yes -o ConnectTimeout=10 -o PubkeyAuthentication=no \
  -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p $SSH_PORT root@$HOST true 2>&1 | grep -q 'Permission denied (publickey)'"
check "fail2ban sshd jail active" remote 'fail2ban-client status sshd >/dev/null'
check "unattended-upgrades enabled" remote 'systemctl is-enabled -q unattended-upgrades'

section "Edge (Traefik)"
site() { curl -sk --max-time 10 --resolve "$1:$HTTPS_PORT:$HOST" -w '\n%{http_code} %{redirect_url}' "https://$1:$HTTPS_PORT$2"; }
check "home page on the apex domain" bash -c '[[ "$(tail -1 <<<"$1")" == "200 " ]] && grep -q "Your code. Your deployment. Your cloud." <<<"$1"' _ "$(site "$DOMAIN" /)"
check "dashboard sends visitors to its sign-in" bash -c '[[ "$(tail -1 <<<"$1")" == "303 "*/login ]]' _ "$(site "dashboard.$DOMAIN" /)"
check "guides served on the home site at /docs" bash -c 'grep -q "User guide" <<<"$1"' _ "$(site "$DOMAIN" /docs)"
check "API site serves /v1 (401 without a token)" bash -c '[[ "$(tail -1 <<<"$1")" == "401 " ]]' _ "$(site "api.$DOMAIN" /v1/me)"
check "MCP site has a home page" bash -c 'grep -q "/mcp" <<<"$1"' _ "$(site "mcp.$DOMAIN" /)"
check "admin site asks for the admin sign-in" bash -c 'grep -q "Sign in to the admin panel" <<<"$1"' _ "$(site "admin.$DOMAIN" /)"
ADMIN_PW="$(tofu -chdir="$(dirname "$0")/../terraform" output -raw admin_password 2>/dev/null || true)"
if [[ -n "$ADMIN_PW" ]]; then
  check "the installed admin account signs in to the admin panel" bash -c "curl -sk --max-time 15 --resolve admin.$DOMAIN:$HTTPS_PORT:$HOST -H 'Origin: https://admin.$DOMAIN:$HTTPS_PORT' -o /dev/null -w '%{http_code} %{redirect_url}' --data-urlencode 'email=admin@$DOMAIN' --data-urlencode \"password=\$1\" https://admin.$DOMAIN:$HTTPS_PORT/login | grep -qE '^303 https://admin\.[^/]+/\$'" _ "$ADMIN_PW"
  check "the installed admin account can't sign in to the dashboard" bash -c "curl -sk --max-time 15 --resolve dashboard.$DOMAIN:$HTTPS_PORT:$HOST -H 'Origin: https://dashboard.$DOMAIN:$HTTPS_PORT' -o /dev/null -w '%{redirect_url}' --data-urlencode 'email=admin@$DOMAIN' --data-urlencode \"password=\$1\" https://dashboard.$DOMAIN:$HTTPS_PORT/login | grep -q 'Invalid+email+or+password'" _ "$ADMIN_PW"
fi
check "HTTP redirects to HTTPS" bash -c "curl -s --max-time 10 -o /dev/null -w '%{http_code} %{redirect_url}' -H 'Host: dashboard.$DOMAIN' http://$HOST:$HTTP_PORT/ | grep -qE '^30[18] https://dashboard.$DOMAIN/'"
check "security headers present" bash -c "curl -skI --max-time 10 --resolve dashboard.$DOMAIN:$HTTPS_PORT:$HOST https://dashboard.$DOMAIN:$HTTPS_PORT/ | grep -qi '^strict-transport-security'"
if [[ "$NET" == bridge && -s "$CA_ROOT" ]]; then
  for h in "$DOMAIN" "dashboard.$DOMAIN" "admin.$DOMAIN" "api.$DOMAIN" "mcp.$DOMAIN"; do
    check "$h has a valid ACME certificate (verified against the test CA, no -k)" valid_cert "$h"
  done
  check "certificate issued by the local ACME CA (step-ca), not self-signed" bash -c "grep -q 'FodyCloud Test CA' <<<\"\$1\"" _ "$(issuer_of "dashboard.$DOMAIN")"
else
  echo "  SKIP certificate checks (needs NET=bridge and qemu/.work/ca from net-setup.sh)"
fi
check "unknown host gets 404" bash -c "curl -sk --max-time 10 -o /dev/null -w '%{http_code}' --resolve nope.$DOMAIN:$HTTPS_PORT:$HOST https://nope.$DOMAIN:$HTTPS_PORT/ | grep -qx 404"

section "Student app (simulated deployment with 'free' plan limits)"
PLAN="$(remote 'jq -c .plans.free /opt/fodycloud/config/plans.json')"
CPUS="$(jq -r .cpus <<<"$PLAN")" MEM="$(jq -r .memory <<<"$PLAN")" DISK="$(jq -r .disk <<<"$PLAN")" PIDS="$(jq -r .pids <<<"$PLAN")"
echo "  plan free: cpus=$CPUS memory=$MEM disk=$DISK pids=$PIDS"
check "deploy app1 on fodycloud-apps" remote "
  docker run -d --name fc-test-app1 --network fodycloud-apps \
    --cpus $CPUS --memory $MEM --memory-swap $MEM --pids-limit $PIDS --storage-opt size=$DISK \
    --cap-drop ALL --security-opt no-new-privileges \
    -l traefik.enable=true \
    -l 'traefik.http.routers.app1.rule=Host(\`app1.apps.$DOMAIN\`)' \
    -l traefik.http.routers.app1.entrypoints=websecure -l traefik.http.routers.app1.tls=true \
    -l traefik.http.services.app1.loadbalancer.server.port=80 \
    traefik/whoami:latest >/dev/null"
sleep 3
check "app1 reachable at https://app1.apps.$DOMAIN" bash -c "curl -sk --max-time 10 --resolve app1.apps.$DOMAIN:$HTTPS_PORT:$HOST https://app1.apps.$DOMAIN:$HTTPS_PORT/ | grep -q '^Hostname: '"
check "app1 limits applied (cpu/mem/pids/disk)" remote "
  docker inspect fc-test-app1 --format '{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}} {{.HostConfig.PidsLimit}} {{index .HostConfig.StorageOpt \"size\"}}' |
  grep -qx '$(awk "BEGIN{printf \"%d\", $CPUS*1e9}") $(numfmt --from=iec "${MEM^^}") $PIDS $DISK'"
check "disk quota enforced (100M container cannot write 300M)" remote "
  docker run --rm --storage-opt size=100M alpine:3 sh -c 'dd if=/dev/zero of=/fill bs=1M count=300' 2>&1 | grep -q 'No space left on device'"
remote 'docker run -d --name fc-test-app2 --network fodycloud-apps alpine:3 sleep 600 >/dev/null'
APP1_IP="$(remote "docker inspect -f '{{(index .NetworkSettings.Networks \"fodycloud-apps\").IPAddress}}' fc-test-app1")"
PG_IP="$(remote "docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' fodycloud-postgres-1 | awk '{print \$1}'")"
check "control: app2 can open TCP to a public host (1.1.1.1:443)" remote "docker exec fc-test-app2 nc -z -w 5 1.1.1.1 443"
check "apps cannot reach each other (app2 -> app1 $APP1_IP)" remote "! docker exec fc-test-app2 wget -q -T 3 -O /dev/null http://$APP1_IP/"
check "apps cannot reach postgres ($PG_IP:5432)" remote "! docker exec fc-test-app2 nc -z -w 3 $PG_IP 5432"
# The students' PostgreSQL server (apps-postgres) is the one exception: its
# port 5432 only, at the apps subnet's .3 address.
APPS_DB_IP="$(remote "grep ^APPS_DB_IP= /opt/fodycloud/.env | cut -d= -f2")"
check "apps reach the students' PostgreSQL ($APPS_DB_IP:5432)" remote "docker exec fc-test-app2 nc -z -w 3 $APPS_DB_IP 5432"
TRAEFIK_APPS_IP="$(remote "grep ^TRAEFIK_APPS_IP= /opt/fodycloud/.env | cut -d= -f2")"
check "control: the PostgreSQL server reaches Traefik ($TRAEFIK_APPS_IP:443)" remote \
  "cd /opt/fodycloud && docker compose exec -T apps-postgres nc -z -w 3 $TRAEFIK_APPS_IP 443"
check "...but can't open connections to apps ($APP1_IP:80)" remote \
  "cd /opt/fodycloud && ! docker compose exec -T apps-postgres nc -z -w 3 $APP1_IP 80"
# A private address that really answers: a service on the operator's LAN
# (normally the router's admin page). The VM host reaching it is the positive
# control; student apps must not. (The PC itself is shielded by NetworkManager's
# shared-mode firewall, so it cannot serve as the target.)
LAN_GW="$(ip route show default 2>/dev/null | awk '/default/ {print $3; exit}')" LAN_PORT=""
if [[ "$LAN_GW" =~ ^(10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.) ]]; then
  for p in 80 443 22 8080; do
    remote "nc -z -w 3 $LAN_GW $p" && { LAN_PORT=$p; break; }
  done
fi
if [[ -n "$LAN_PORT" ]]; then
  check "control: the VM host reaches a service on the operator's LAN ($LAN_GW:$LAN_PORT)" remote "nc -z -w 3 $LAN_GW $LAN_PORT"
  check "apps cannot reach the operator's LAN ($LAN_GW:$LAN_PORT)" remote "! docker exec fc-test-app2 nc -z -w 3 $LAN_GW $LAN_PORT"
else
  echo "  SKIP private-network check: no reachable private LAN service found (gateway: ${LAN_GW:-none})"
fi
check "apps cannot reach the host (gateway:22)" remote "! docker exec fc-test-app2 nc -z -w 3 \$(docker network inspect fodycloud-apps -f '{{(index .IPAM.Config 0).Gateway}}') 22"
check "apps have internet egress + DNS" remote "docker exec fc-test-app2 wget -q -T 10 -O /dev/null https://deb.debian.org/"

section "Data services"
check "postgres accepts connections" remote 'cd /opt/fodycloud && docker compose exec -T postgres sh -c "psql -U \$POSTGRES_USER -d \$POSTGRES_DB -tAc \"select 1\"" | grep -qx 1'
check "valkey requires a password" remote 'cd /opt/fodycloud && docker compose exec -T valkey sh -c "REDISCLI_AUTH= VALKEYCLI_AUTH= valkey-cli ping" | grep -q NOAUTH'
check "valkey answers with its password" remote 'cd /opt/fodycloud && docker compose exec -T valkey valkey-cli ping | grep -qx PONG'
check "backup job produces a dump" remote 'systemctl start fodycloud-backup.service && ls /var/backups/fodycloud/postgres-*.sql.gz >/dev/null && zcat $(ls -t /var/backups/fodycloud/postgres-*.sql.gz | head -1) | grep -q "PostgreSQL database cluster dump"'
check "backup timer scheduled" remote 'systemctl is-active -q fodycloud-backup.timer'

section "Platform alerts (health.sh)"
check "health-check timer active" remote 'systemctl is-active -q fodycloud-health.timer'
# mail_count SUBJECT-PART: messages in Mailpit whose subject contains the text.
mail_count() {
  remote "curl -s 'http://127.0.0.1:8025/api/v1/messages?limit=1000'" |
    jq --arg s "$1" '[.messages[]? | select(.Subject | contains($s))] | length'
}
# wait_mail SUBJECT-PART N: waits until at least N such messages arrived (the outbox sends in the background).
wait_mail() {
  for _ in $(seq 1 30); do (( $(mail_count "$1") >= $2 )) && return 0; sleep 2; done
  echo "only $(mail_count "$1") message(s) with \"$1\""; return 1
}
remote 'rm -rf /var/lib/fodycloud/health'
DISK0="$(mail_count "Disk / is")" CERT0="$(mail_count "Certificates expire soon")"
remote 'ALERT_DISK_PERCENT=1 ALERT_CERT_DAYS=2 /opt/fodycloud/health.sh' >/dev/null
check "a full disk emails the admins" wait_mail "Disk / is" $((DISK0 + 1))
check "certificates close to expiry email the admins (24-hour test certificates, 2-day threshold)" wait_mail "Certificates expire soon" $((CERT0 + 1))
remote 'ALERT_DISK_PERCENT=1 ALERT_CERT_DAYS=2 /opt/fodycloud/health.sh' >/dev/null
sleep 10
check "a lasting problem isn't emailed again within 24 hours" test "$(mail_count "Disk / is")" -eq $((DISK0 + 1))
RES0="$(mail_count "Resolved: Disk")"
remote '/opt/fodycloud/health.sh' >/dev/null
check "a resolved problem is announced" wait_mail "Resolved: Disk" $((RES0 + 1))

finish
