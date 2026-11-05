# Linking a console to an OpenPak account (link-first)

A Wii or DS cannot sign in, so it is linked with a device code, and since link-first (prds#139)
it plays online only once it is linked. Design: `prds/repos/wfc-link-first-design.md`.

1. An unlinked console is refused before any game profile is made, and the refusal carries its
   pairing code (measured on Marina and melonDS, prds#139 step 0):
   - every Wii and DS title except Mario Kart Wii: wfc-nas refuses the NAS login with the link
     number as return code (4000-9999), which DWC shows as "Error Code: 24000-29999";
   - Mario Kart Wii: wfc-gpcm refuses the GPCM login with text, the 8-symbol code `XXXX-XXXX`
     and the same 5-digit number (error 22100).
2. The player enters the code or the number at `openpak.org/link` (or in the app). A number
   (6000 values, guessable) also needs a second factor: the browser on the console's network
   (keyed network tags compared, no address stored) or the console's in-game name typed. The
   website calls `preview`, shows "Link your Wii (game, name) to you?", then `approve` with the
   signed-in account. This service makes the core link (`ReserveLink`, `ActivateLink`, namespace
   `wfc`, subject `<user_id>.<unix ns>`, idempotency keys `wfcpair:<subject>:*`), writes
   `wfc_pairing_linked` and spends the code.
3. The player connects again; the gate finds the console linked.

Codes: 8 symbols (no I, O, 0, 1) and a link number (4000-9999, shown +20000, unique among live
codes), valid 15 minutes, single use; a live code is reused. At most 6 new codes per console per
hour and 2000 live codes. Codes are never logged (log lines carry a salted 6-hex tag).

A fresh subject per link means a console unlinked from one account can be linked by another
(the core refuses to move a revoked subject). Up to 8 consoles per account.

## Endpoints (POST, JSON, `secret` = `WFC_API_SECRET`)

| Path | Body | Result |
| --- | --- | --- |
| `/api/pair/preview` (also `/lookup`) | `{secret, code, client_ip, name}` | `{unit, gsbrcd, name, expires_at}`; 404 `invalid_pairing_code` for any miss |
| `/api/pair/approve` | `{secret, code, core_account_id, client_ip, name}` | `{status:"linked", unit, link_id, already_linked}`; 404 miss, 409 `conflict` (linked elsewhere or 8 consoles), 422 `approval_failed` (core refused; rolled back), 503 `unavailable` |
| `/api/pair/consoles` | `{secret, core_account_id}` | `{consoles:[{link_id, unit, games, linked_at}]}` (never the console id) |
| `/api/pair/unlink` | `{secret, core_account_id, link_id, erase}` | 204; core `UnlinkLink`, forget, kick, and with `erase` the console's online game data is deleted; 404 when the account has no such console |
| `/api/pair/stats` (GET, no secret) | | `{pair_codes_live, pair_issued_24h, pair_approved_24h, linked_consoles}` |
| `/api/pair/forget` | `{secret, user_id}` | 204 (pre-link-first unlink) |
| `/api/pair/redeem` | | 410 `gone` (approval is this service's now) |

`core_account_id` must be the website's signed-in account, never a value from the browser;
`client_ip` is the browser's address as the website saw it. `WFC_NET_TAG_KEY` (else
`WFC_INTERNAL_KEY`) must match wfc-nas and wfc-gpcm.

## Lifecycle

- A reaper runs every minute: an approval whose code expired half-way is finished if the core
  link is live for the same account, otherwise unlinked and released.
- `corebridge` acts on three core events: `link_unlinked` (namespace `wfc`) forgets the console
  and kicks its sessions (22101); `account_deleted` forgets every console of the account;
  `account_banned` kicks them (22002). Every other event is still dropped and counted.
- Erasure (`docs/erasure.md`) erases the consoles named by the core's links and by the local
  cache.
- `GET /healthz/pairing` is the status check: the pairing tables and the core answer.
