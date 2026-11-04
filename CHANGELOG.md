# Changelog

## v0.2.0 - 2026-11-04

- `POST /api/pair/lookup`, `/api/pair/redeem`, `/api/pair/forget`: the pairing-code endpoints behind openpak.org/link for a Wii or DS (wfc-common v0.5.0). Secret-gated; refused outright when no API secret is set. See docs/pairing.md. The friend-code `/api/profile` path is unchanged.

## v0.1.4 - 2026-11-03

- Internal `/internal/` polls are counted and summarised once a minute instead of logged per call (wfc-common v0.3.1).

## v0.1.3 - 2026-11-03

- Every HTTP request now logs one line to stderr, `http <method> <path> -> <status> <ms>ms` (path only, never the query, headers or bodies; `/healthz` skipped), through wfc-common v0.3.0 `httplog`.

