# Linking a console to an OpenPak account (pairing codes)

A Wii or DS cannot sign in, so it is linked with a device code.

1. An unlinked console logs in to `wfc-gpcm`. For Mario Kart Wii the login reply's
   `wl:motd` (the Wi-Fi menu message the game already shows) ends with
   `Link this Wii at openpak.org/link` and a code `XXXX-XXXX`. Other titles and the DS show
   no message, so they keep the friend-code path.
2. The player enters the code at `openpak.org/link` (or the app). The website calls this
   service, makes the `wfc` core link (subject = WFC `user_id`), then spends the code.

Codes: eight symbols from a 32-symbol alphabet (no I, O, 0, 1), stored in Postgres
(`wfc_pairing_codes`), valid 30 minutes, single use. A live code is reused for the same console
instead of issuing another. `wfc_pairing_linked` remembers linked consoles so they are not
offered a code. Codes are never logged.

## Endpoints (POST, JSON, `secret` = `WFC_API_SECRET`)

| Path | Body | Result |
| --- | --- | --- |
| `/api/pair/lookup` | `{secret, code}` | `{user_id, gsbrcd, name, expires_at}`, no side effect; 404 `invalid_pairing_code` if unknown, expired or used |
| `/api/pair/redeem` | `{secret, code}` | spends the code, marks the console linked; `{user_id}` |
| `/api/pair/forget` | `{secret, user_id}` | 204; the console is offered a code again (called on unlink) |

The website rate limits attempts per account and per address.
