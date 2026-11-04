# Account erasure

`DELETE /internal/accounts/{account_id}/data` with `Authorization: Bearer $ACCOUNT_ERASE_KEY`
on the HTTP listener (`http://openpak-wfc-api:20209`, container network only). Without
`ACCOUNT_ERASE_KEY` the endpoint answers 503. Response: `{"service":"wfc","rows":N}`; one
log line `account erased account=<uuid> rows=<n> service=wfc`.

All WFC services (nas, dls, gpcm, gpsp, serverbrowser, qr2, natneg, gamestats, sake, race)
share one Postgres database, so this one call erases for all of them. It looks up the
account's `wfc` links in the core, then deletes in one transaction the console's `users`
profiles, `gamestats_public_data`, `sake_records`, `mario_kart_wii_sake`,
`mario_kart_wii_ghosts`, `sessions`, `events` naming the profile, and the pairing rows.
Nothing authored is kept. Needs `WFC_CORE_ADDRESS` and `WFC_CORE_KEY`. Leaderboard scores
are erased by the leaderboards step. Live in-memory sessions end on their own (qr2, natneg,
gpcm, gamestats keep only transient session maps).
