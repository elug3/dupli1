#!/bin/bash
# Move production from AWS to VENUS. See docs/deployment-venus.md.
#
#   deploy/venus/cutover.sh rehearse   # fresh dump + images → VENUS; AWS untouched
#   deploy/venus/cutover.sh cutover    # same, after stopping AWS writes; asks to confirm
#
# Both modes: run the in-VPC dump task (task def dupli1-migration-backup, the
# temp bucket and role from the 2026-09-26 backup must still exist), sync
# images, then import-backup.sh --replace into VENUS and start the stack.
# cutover additionally scales every AWS ECS service to 0 first (so nothing is
# written after the dump), enables Telegram on VENUS, and starts cloudflared.
# Nothing in AWS is deleted; rollback = scale ECS back up + point Cloudflare
# back at the ALB.
set -euo pipefail
MODE=${1:?rehearse|cutover}
HERE=$(cd "$(dirname "$0")" && pwd)
ENV_FILE=${DUPLI1_HOME:-/opt/dupli1}/.env
DC=(docker compose -f "$HERE/docker-compose.yml" --env-file "$ENV_FILE")
CLUSTER=production
TEMP_BUCKET=dupli1-migration-backup-20260926
IMAGES_BUCKET=dupli1-product-images-20260712132109100000000003
PREV_BACKUP=${PREV_BACKUP:-$HOME/backups/dupli1-2026-09-26}
OUT=$HOME/backups/dupli1-$MODE-$(date -u +%Y%m%dT%H%MZ)
APP_SERVICES=(auth product order cart payment profile notification proxy web manage-web edge)
ECS_SERVICES=(dupli1-web dupli1-manage-web dupli1-proxy dupli1-auth dupli1-product dupli1-order
              dupli1-cart dupli1-payment dupli1-profile dupli1-notification dupli1-redis dupli1-nats)

log() { echo "[$(date -u +%H:%M:%S)] $*"; }

case $MODE in
  rehearse) ;;
  cutover)
    grep -q "^CLOUDFLARE_TUNNEL_TOKEN='.\+'" "$ENV_FILE" || { echo "set CLOUDFLARE_TUNNEL_TOKEN in $ENV_FILE first"; exit 1; }
    read -r -p "This stops production on AWS (ECS → 0). Type CUTOVER to continue: " ok
    [ "$ok" = CUTOVER ] || exit 1 ;;
  *) echo "usage: $0 rehearse|cutover"; exit 1 ;;
esac

aws sts get-caller-identity --query Account --output text >/dev/null
mkdir -p -m 700 "$OUT"/{db,s3,config}

if [ "$MODE" = cutover ]; then
  log "recording ECS desired counts, then scaling to 0"
  aws ecs describe-services --cluster $CLUSTER --services "${ECS_SERVICES[@]:0:10}" --output json > "$OUT/config/ecs-services-before-1.json"
  aws ecs describe-services --cluster $CLUSTER --services "${ECS_SERVICES[@]:10}" --output json > "$OUT/config/ecs-services-before-2.json"
  for s in "${ECS_SERVICES[@]}"; do aws ecs update-service --cluster $CLUSTER --service "$s" --desired-count 0 >/dev/null; done
  aws ecs wait services-stable --cluster $CLUSTER --services "${ECS_SERVICES[@]:0:10}"
  aws ecs wait services-stable --cluster $CLUSTER --services "${ECS_SERVICES[@]:10}"
  log "AWS stopped — downtime starts"
fi

log "running dump task in the production VPC"
TASK=$(aws ecs run-task --cluster $CLUSTER --task-definition dupli1-migration-backup \
  --capacity-provider-strategy capacityProvider=dupli1-production-ec2,weight=1 \
  --network-configuration 'awsvpcConfiguration={subnets=[subnet-01fd0882721f10499,subnet-006b8428713711816],securityGroups=[sg-05526d202a1f5f56d],assignPublicIp=DISABLED}' \
  --tags key=purpose,value=migration-backup --started-by migration-$MODE --query 'tasks[0].taskArn' --output text)
aws ecs wait tasks-stopped --cluster $CLUSTER --tasks "$TASK"
code=$(aws ecs describe-tasks --cluster $CLUSTER --tasks "$TASK" --query 'tasks[0].containers[0].exitCode' --output text)
logs=$(aws logs get-log-events --log-group-name /dupli1/migration-backup --log-stream-name "dump/dump/${TASK##*/}" --query 'events[].message' --output text)
echo "$logs" | tr '\t' '\n' | grep -E '^(== |[a-z_]+: |tar |UPLOAD_OK|globals)' || true
[ "$code" = 0 ] && echo "$logs" | grep -q UPLOAD_OK || { echo "dump task failed (exit $code)"; exit 1; }
want=$(echo "$logs" | grep -o 'sha256 [0-9a-f]\{64\}' | cut -d' ' -f2)
aws s3 cp --only-show-errors "s3://$TEMP_BUCKET/rds.tar" "$OUT/db/rds.tar"
[ "$(sha256sum "$OUT/db/rds.tar" | cut -d' ' -f1)" = "$want" ] || { echo "rds.tar checksum mismatch"; exit 1; }
(cd "$OUT/db" && tar xf rds.tar && cd dupli1-rds && sha256sum -c --quiet SHA256SUMS)
log "dump downloaded and verified"

log "syncing images (starting from the previous backup's copy)"
cp -a --reflink=auto "$PREV_BACKUP/s3/$IMAGES_BUCKET" "$OUT/s3/"
aws s3 sync --only-show-errors --delete "s3://$IMAGES_BUCKET" "$OUT/s3/$IMAGES_BUCKET"
aws s3api list-objects-v2 --bucket $IMAGES_BUCKET --output json > "$OUT/s3/source-listing.json"
python3 -c "import json;[print(o['Key']) for o in json.load(open('$OUT/s3/source-listing.json')).get('Contents',[])]" |
  xargs -P 16 -I{} sh -c 'aws s3api head-object --bucket '"$IMAGES_BUCKET"' --key "$1" --query "[ContentType,CacheControl]" --output text | sed "s|^|$1\t|"' _ {} \
  > "$OUT/s3/object-metadata.tsv"

log "importing into VENUS"
"${DC[@]}" --profile tunnel stop "${APP_SERVICES[@]}" cloudflared
"$HERE/import-backup.sh" "$OUT" --replace

if [ "$MODE" = cutover ]; then
  log "enabling Telegram on VENUS (AWS notification is stopped)"
  python3 - "$ENV_FILE" <<'EOF'
import sys
p = sys.argv[1]; lines = open(p).read().splitlines()
vals = {l.split("=", 1)[0]: l.split("=", 1)[1] for l in lines if "=" in l and not l.startswith("#")}
out = []
for l in lines:
    k = l.split("=", 1)[0]
    if f"{k}_AT_CUTOVER" in vals:
        l = f"{k}={vals[k + '_AT_CUTOVER']}"
    out.append(l)
open(p, "w").write("\n".join(out) + "\n")
EOF
  "${DC[@]}" --profile tunnel up -d
else
  "${DC[@]}" up -d
fi
log "done: $OUT"
[ "$MODE" = cutover ] && echo "Now switch dupli1.com + manage.dupli1.com to the tunnel in Cloudflare (docs/deployment-venus.md, step 4)."
