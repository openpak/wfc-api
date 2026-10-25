# Wii/DS API Server

OpenPak's Wii/DS API Server.

| | |
| --- | --- |
| Network | Wii, DS |
| Role | Operator profile, moderation and statistics HTTP API, plus the WFC core-event translator |
| Depends on | [`wfc-gpcm`](https://github.com/openpak/wfc-gpcm), [`wfc-qr2`](https://github.com/openpak/wfc-qr2), existing WFC database (PostgreSQL) |
| Used by | [`wfc-nas`](https://github.com/openpak/wfc-nas) (console HTTP) |
| Image | `ghcr.io/openpak/wfc-api` |
| Based on | [WiiLink WFC](https://github.com/WiiLink24/wfc-server) (AGPL-3.0) |
| Licence | AGPL-3.0-only |

## What it serves

| Endpoint | What |
| --- | --- |
| `GET /healthz` | Health check, unauthenticated |
| `/api/groups`, `/api/stats`, `/api/ban`, `/api/unban`, `/api/kick`, `/api/baninfo`, `/api/profile` | Operator HTTP API; mutations and profile reads keep their `WFC_API_SECRET` role checks |

## Run

```sh
GOWORK=off go test -mod=vendor ./...
GOWORK=off go vet -mod=vendor ./...
docker build -t wfc-api .
```

## Docs

- [Protocol](docs/protocol.md)
- [Evidence](docs/evidence.md)
- [Deploy](docs/deploy.md)
- [Notes](docs/notes.md)

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).
