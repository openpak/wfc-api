# Changelog

## v0.3.0 - 2026-11-04

- `DELETE /internal/accounts/{account_id}/data` (bearer `ACCOUNT_ERASE_KEY`, 503 when unset, on the existing HTTP listener, port 20209): erases an account's data from the shared WFC database for every WFC service (profiles, game stats, sake records, Mario Kart Wii times and ghosts, pairing codes; wfc-common v0.6.0). Console ids come from the account core's `wfc` links, so `WFC_CORE_ADDRESS`/`WFC_CORE_KEY` must be set or the call fails (the website then retries). Nothing authored is kept: ghosts and sake rows hold the Mii name and profile id. No `account_deleted` event hook: the core drops the links first and no WFC table carries the account id. See docs/erasure.md.

## v0.2.0 - 2026-11-04

- `POST /api/pair/lookup`, `/api/pair/redeem`, `/api/pair/forget`: the pairing-code endpoints behind openpak.org/link for a Wii or DS (wfc-common v0.5.0). Secret-gated; refused outright when no API secret is set. See docs/pairing.md. The friend-code `/api/profile` path is unchanged.

## v0.1.4 - 2026-11-03

- Internal `/internal/` polls are counted and summarised once a minute instead of logged per call (wfc-common v0.3.1).

## v0.1.3 - 2026-11-03

- Every HTTP request now logs one line to stderr, `http <method> <path> -> <status> <ms>ms` (path only, never the query, headers or bodies; `/healthz` skipped), through wfc-common v0.3.0 `httplog`.

