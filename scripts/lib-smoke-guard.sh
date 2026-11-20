# shellcheck shell=bash
# Sourced by the smoke scripts that write data (smoke-money-path.sh,
# smoke-promotion-campaign.sh). They create products, customers and real
# orders, so they run only against a local dev stack unless the caller says,
# in so many words, that writing real data is intended.
#
# Refused unless SMOKE_ALLOW_REAL_DATA=yes:
#   - a BASE whose host is not localhost / 127.0.0.1 / [::1]
#   - a BASE without an explicit port, or on port 80/443 (the dev gateway is
#     :8080; on VENUS the production edge listens on 127.0.0.1:80)
#   - any run on a host with /opt/dupli1/.env (VENUS, which serves production)
smoke_guard() {
  local base=$1
  [ "${SMOKE_ALLOW_REAL_DATA:-}" = "yes" ] && return 0
  local rest=${base#*://}
  local hostport=${rest%%/*}
  local host port
  if [[ $hostport == \[*\]* ]]; then
    host=${hostport%%]*}]; port=${hostport#"$host"}; port=${port#:}
  else
    host=${hostport%%:*}; port=${hostport#"$host"}; port=${port#:}
  fi
  local why=""
  case $host in
    localhost | 127.0.0.1 | "[::1]") ;;
    *) why="BASE host '$host' is not local" ;;
  esac
  if [ -z "$why" ] && { [ -z "$port" ] || [ "$port" = 80 ] || [ "$port" = 443 ]; }; then
    why="BASE has no dev-stack port (the local gateway is :8080; :80 on VENUS is production)"
  fi
  if [ -z "$why" ] && [ -e /opt/dupli1/.env ]; then
    why="this host has /opt/dupli1/.env, so it is VENUS and serves production"
  fi
  [ -z "$why" ] && return 0
  cat >&2 <<MSG
refusing to run: $why.
This script writes real data (a product, a customer account, an order).
Run it against a local dev stack (sudo docker compose up --build; BASE=http://localhost:8080).
To check a production deploy read-only, see docs/deployment-venus.md → "Checking a deploy without writing data".
If writing real data really is intended, set SMOKE_ALLOW_REAL_DATA=yes.
MSG
  exit 2
}
