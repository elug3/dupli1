# Sign in as a manager and export an API token into the current shell.
# Source it — running it would set the variables in a child shell that exits:
#
#   source scripts/dupli1-login.sh [email]          # asks for the email if omitted
#   curl -H "Authorization: Bearer $DUPLI1_TOKEN" "$DUPLI1_API/api/v1/orders"
#   dupli1_api GET /api/v1/orders                   # same, refreshing when needed
#
# Exports:
#   DUPLI1_API            base URL (keeps an existing value; default https://dupli1.com)
#   DUPLI1_TOKEN          access token, valid 15 minutes
#   DUPLI1_REFRESH_TOKEN  rotates on every refresh; dupli1_refresh keeps it current
#
# The password is read without echo and never passed on a command line, so it
# stays out of shell history and `ps`. The tokens live only in this shell.

if ! (return 0 2>/dev/null); then
  echo "source this file instead: source ${BASH_SOURCE[0]} [email]" >&2
  exit 1
fi

export DUPLI1_API=${DUPLI1_API:-https://dupli1.com}

# _dupli1_post <path>: POST stdin as JSON; prints "<status>\n<body>".
_dupli1_post() {
  curl -sS -m 15 -w '\n%{http_code}' -X POST "$DUPLI1_API$1" \
    -H 'Content-Type: application/json' --data-binary @- |
    python3 -c 'import sys; b, s = sys.stdin.read().rsplit("\n", 1); print(s); print(b)'
}

# _dupli1_field <name>: read a top-level JSON string field from stdin.
_dupli1_field() {
  python3 -c 'import json, sys
try: print(json.load(sys.stdin).get(sys.argv[1]) or "")
except Exception: print("")' "$1"
}

# dupli1_refresh: swap the refresh token for a new access + refresh token.
dupli1_refresh() {
  [[ -n ${DUPLI1_REFRESH_TOKEN:-} ]] || { echo "not signed in; source dupli1-login.sh" >&2; return 1; }
  local res status body
  res=$(REFRESH="$DUPLI1_REFRESH_TOKEN" python3 -c 'import json, os; print(json.dumps({"refresh_token": os.environ["REFRESH"]}))' |
    _dupli1_post /api/v1/auth/refresh) || return 1
  status=${res%%$'\n'*}; body=${res#*$'\n'}
  case $status in
    200)
      export DUPLI1_TOKEN DUPLI1_REFRESH_TOKEN
      DUPLI1_TOKEN=$(_dupli1_field token <<<"$body")
      DUPLI1_REFRESH_TOKEN=$(_dupli1_field refresh_token <<<"$body")
      ;;
    503) echo "refresh unavailable (503); token kept, try again shortly" >&2; return 1 ;;
    *)
      echo "refresh refused ($status); sign in again" >&2
      unset DUPLI1_TOKEN DUPLI1_REFRESH_TOKEN
      return 1
      ;;
  esac
}

# dupli1_api <METHOD> <path> [json body]: call the API, refreshing once on 401.
dupli1_api() {
  local method=$1 path=$2 attempt status out
  local -a data=()
  [[ -n ${3:-} ]] && data=(-H 'Content-Type: application/json' --data-binary "$3")
  for attempt in 1 2; do
    out=$(curl -sS -m 30 -w '\n%{http_code}' -X "$method" "$DUPLI1_API$path" \
      -H "Authorization: Bearer ${DUPLI1_TOKEN:-}" "${data[@]}") || return 1
    status=${out##*$'\n'}
    if [[ $status == 401 && $attempt == 1 ]]; then
      dupli1_refresh || return 1
      continue
    fi
    printf '%s\n' "${out%$'\n'*}"
    [[ $status == 2?? ]] || { echo "HTTP $status" >&2; return 1; }
    return 0
  done
}

_dupli1_login() {
  local email=${1:-} password res status body refresh
  if [[ -z $email ]]; then
    read -rp "Email [admin@dupli1.com]: " email
    email=${email:-admin@dupli1.com}
  fi
  read -rsp "Password for $email: " password; echo
  [[ -n $password ]] || { echo "no password given" >&2; return 1; }

  res=$(EMAIL="$email" PASSWORD="$password" python3 -c 'import json, os
print(json.dumps({"email": os.environ["EMAIL"], "password": os.environ["PASSWORD"], "client": "manage"}))' |
    _dupli1_post /api/v1/auth/login) || return 1
  password=
  status=${res%%$'\n'*}; body=${res#*$'\n'}
  if [[ $status != 200 ]]; then
    echo "login failed ($status): $(_dupli1_field error <<<"$body")" >&2
    return 1
  fi
  refresh=$(_dupli1_field refresh_token <<<"$body")
  [[ -n $refresh ]] || { echo "login returned no refresh token" >&2; return 1; }

  export DUPLI1_REFRESH_TOKEN=$refresh
  dupli1_refresh || return 1
  echo "signed in as $email at $DUPLI1_API; DUPLI1_TOKEN valid for 15 min (dupli1_refresh renews it)"
}

_dupli1_login "$@"
