# AI instruction: switch card checkout to NANO key-in (수기결제) on VENUS

**Where:** VENUS, the production host (`serial` user, Docker stack in `/opt/dupli1/`).
**Audience:** an AI agent or operator with a shell on VENUS.
**Linear:** SER-106. **Design:** [payment-nano-key-in.md](payment-nano-key-in.md). **Host runbook:** [deployment-venus.md](deployment-venus.md).

---

## Goal

Turn on key-in card payment in production: the shopper types the card on `dupli1.com/checkout/pay/:id`, and payment sends it to NANO `approval.io` once. Nothing else changes.

The code is already merged (dupli1 #383, dupli1-web #191) and deployed by CI. It stays on the old 인증결제 window until two values are set in `/opt/dupli1/.env`, so this whole job is: **set two env values, restart one container, check it.**

## Stop and ask the human if

- You were not given **production** `NANO_ENC_KEY` and `NANO_ENC_IV` from the NANO contract. Never invent them. Never use the test key and IV from NANO's guide: they only work on `dev3.nanopay.co.kr`, while production points at `pay.nanopay.co.kr`.
- Any check in step 1 fails.
- The key is not exactly 32 bytes or the IV not exactly 16 bytes.
- Anything asks you to charge a real card. A live test payment is the human's call (step 5).

## Rules

- **Never print, log or commit the key or IV.** Don't `cat` or `grep` the `.env` lines that hold them, don't echo them, don't paste them into chat, a PR, Linear or a file in a repository. Check them only by length.
- Change only `/opt/dupli1/.env`. Do not edit `/opt/dupli1/repo` (the deploy checkout): the next deploy discards edits there.
- Restart only `payment`. No other service reads these values.
- Do not remove the 인증결제 bridge (`/nano/checkout`, `/nano/return`, `/webhooks/nano`). That is a separate code change, one release later.

Command prefix:

```bash
DC="docker compose -f /opt/dupli1/repo/deploy/venus/docker-compose.yml --env-file /opt/dupli1/.env"
```

---

## 1. Check that the new code is running

```bash
# The deploy checkout includes the key-in merge (ebab67d) or later
git -C /opt/dupli1/repo merge-base --is-ancestor ebab67d HEAD && echo "repo ok"

# The compose file passes the new variables to payment
grep -c 'NANO_ENC_' /opt/dupli1/repo/deploy/venus/docker-compose.yml   # expect 2 or more

# Payment reports the feature (false for now: the key is not set yet)
curl -s -H 'Host: dupli1.com' http://127.0.0.1/api/v1/payments/settings | jq '.features.nano_key_in'
# expect: false   (null means the old payment image is still running)

# NANO is already configured for 인증결제 (prints names only, not values)
grep -oE '^NANO_(API_KEY|SHOPCODE|LOGIN_ID|VER)=.' /opt/dupli1/.env | cut -d= -f1
# expect all four names
```

Check the storefront too: its last deploy job in `elug3/dupli1-web` (Actions → images.yml on `master`) must be from #191's merge or later. If it isn't, stop: the old storefront still works, but shoppers would keep seeing the NANO window.

If `nano_key_in` is `null`, or the repo check fails, the backend deploy for #383 has not finished. Stop and ask; don't deploy by hand.

## 2. Back up `.env`

```bash
sudo cp -p /opt/dupli1/.env /opt/dupli1/.env.bak-$(date +%Y%m%d-%H%M)
```

(`/opt/dupli1` is mode 700; drop `sudo` if you are its owner.)

## 3. Add the key and IV

Have the human type them, or read them into variables without echoing. Check the lengths before writing anything:

```bash
read -rs -p 'NANO_ENC_KEY: ' K; echo
read -rs -p 'NANO_ENC_IV: '  V; echo
printf %s "$K" | wc -c   # must be 32
printf %s "$V" | wc -c   # must be 16
```

Payment uses the bytes as given (no hex or base64 decoding), so 32 and 16 are the raw string lengths. If NANO gave them as hex or base64, stop and ask.

Then, only if neither name is already in the file:

```bash
grep -qE '^NANO_ENC_(KEY|IV)=' /opt/dupli1/.env && echo "ALREADY SET: stop and ask"

{
  echo ''
  echo '# NANO key-in (수기결제), SER-106. docs/payment-nano-key-in.md'
  printf "NANO_ENC_KEY='%s'\n" "$K"
  printf "NANO_ENC_IV='%s'\n"  "$V"
} | sudo tee -a /opt/dupli1/.env >/dev/null
unset K V
```

Single quotes keep Compose from expanding a `$` in the values. If a value contains a single quote, stop and ask.

## 4. Restart payment and check it

```bash
$DC up -d payment
sleep 10
docker logs --since 2m dupli1-payment-1 2>&1 | grep -E 'key-in|NANO_ENC'
```

Expect exactly:

```
payment: nano card checkout is key-in (수기결제, approval.io)
```

If you see `WARNING NANO_ENC_KEY must be 32 bytes and NANO_ENC_IV 16 bytes`, the values are the wrong length: go to **Roll back**.

Then:

```bash
curl -s -H 'Host: dupli1.com' http://127.0.0.1/api/v1/payments/settings | jq '.features.nano_key_in'   # true
curl -s -o /dev/null -w '%{http_code}\n' -H 'Host: dupli1.com' http://127.0.0.1/api/v1/payments/health   # 200
docker ps --filter name=dupli1-payment-1 --format '{{.Status}}'   # Up, not Restarting
```

## 5. Hand over to the human

Report: the time key-in went on (KST), the log line, `nano_key_in: true`, and the backup file name. Don't report the values.

The human then does one real card payment of a small amount on `https://dupli1.com`:

- `/checkout/pay/:id` shows the card form, not the NANO window.
- After paying, the order shows as paid in manage-web.
- `docker logs dupli1-payment-1` shows the masked card number only, never a full one.
- Refund it by canceling the order in manage-web.

## Roll back

Back to the 인증결제 window:

```bash
sudo sed -i '/^NANO_ENC_KEY=/d; /^NANO_ENC_IV=/d' /opt/dupli1/.env
$DC up -d payment
curl -s -H 'Host: dupli1.com' http://127.0.0.1/api/v1/payments/settings | jq '.features.nano_key_in'   # false
```

New card payments open the NANO window again. A payment created while key-in was on has no window to open, so its card form shows an error; its order auto-cancels after 5 minutes unpaid and the shopper orders again.

## After it is on

- **"카드 수기결제 응답 없음" on Telegram:** NANO never answered an approval and the payment is held. Look the payment id up in the NANO console as `compOrderNo`. If NANO approved it, refund it there. The order auto-cancels either way. See [payment-nano-key-in.md](payment-nano-key-in.md) → "Never paying twice".
- **Kill switch:** blocking credit card in manage-web `/settings` stops key-in too, without touching `.env`.
- **PCI:** card numbers now pass through VENUS (storefront SSR → payment → NANO). Keep request-body logging off on the edge and the gateway.
