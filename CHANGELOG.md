# Changelog

## v0.5.0 - 2026-11-05

- Link-first (prds#139): this service owns Wii/DS linking. `/api/pair/preview` (8-symbol code, or the 5-digit number with a network tag match from `client_ip` or the in-game `name`), `/api/pair/approve` (core `ReserveLink`/`ActivateLink` with idempotency keys, rollback on a core refusal, 8 consoles per account), `/api/pair/consoles`, `/api/pair/unlink` (optional erase of the console's game data), `/api/pair/stats`; `/api/pair/lookup` is `preview`, `/api/pair/redeem` answers 410. A reaper finishes or rolls back approvals left half-done. See docs/pairing.md.
- `corebridge` acts on `link_unlinked` (namespace `wfc`), `account_deleted` and `account_banned`: the console is forgotten and its GPCM sessions are kicked through wfc-gpcm's new `kick-user` op. Every other event is still dropped and counted.
- Erasure also erases consoles named by the local link cache.
- `GET /healthz/pairing` for the status page.
- Vendors wfc-common v0.8.0. Deploy before wfc-gpcm v0.4.0 and the website.

## v0.4.0 - 2026-11-05

- Vendors wfc-common v0.7.0: the users row holds protocol ids only, email and nick are derived (prds#131). Deploy before wfc-gpcm, which drops the columns.

## v0.3.1 - 2026-11-04

- Fix: the core bridge and the erase now send `x-internal-key` too (wfc-common v0.6.1). The core rejected `X-API-Key` alone, so the bridge had been polling with Unauthenticated and the erase could not list the account's links.

## v0.3.0 - 2026-11-04

- `DELETE /internal/accounts/{account_id}/data` (bearer `ACCOUNT_ERASE_KEY`, 503 when unset, on the existing HTTP listener, port 20209): erases an account's data from the shared WFC database for every WFC service (profiles, game stats, sake records, Mario Kart Wii times and ghosts, pairing codes; wfc-common v0.6.0). Console ids come from the account core's `wfc` links, so `WFC_CORE_ADDRESS`/`WFC_CORE_KEY` must be set or the call fails (the website then retries). Nothing authored is kept: ghosts and sake rows hold the Mii name and profile id. No `account_deleted` event hook: the core drops the links first and no WFC table carries the account id. See docs/erasure.md.

## v0.2.0 - 2026-11-04

- `POST /api/pair/lookup`, `/api/pair/redeem`, `/api/pair/forget`: the pairing-code endpoints behind openpak.org/link for a Wii or DS (wfc-common v0.5.0). Secret-gated; refused outright when no API secret is set. See docs/pairing.md. The friend-code `/api/profile` path is unchanged.

## v0.1.4 - 2026-11-03

- Internal `/internal/` polls are counted and summarised once a minute instead of logged per call (wfc-common v0.3.1).

## v0.1.3 - 2026-11-03

- Every HTTP request now logs one line to stderr, `http <method> <path> -> <status> <ms>ms` (path only, never the query, headers or bodies; `/healthz` skipped), through wfc-common v0.3.0 `httplog`.

