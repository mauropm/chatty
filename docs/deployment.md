# Chatty — Deployment guide

## Prerequisites

- Ubuntu host with **Docker Engine** + **Docker Compose v2**.
- The **Mimir** inference stack already running. Its container (`mimir-inference`) must be attached
  to a network named **`mimir_mimir`** with an alias resolvable as **`inference`** (the default for
  the Mimir compose project). Chatty reuses this network — it does **not** start a second model.
- Free host port **3333**.

## Install

```bash
cd chatty
cp .env.example .env
# set a strong shared DB password:
sed -i "s#replace-with-a-long-random-secret#$(head -c 24 /dev/urandom | base64 | tr -d '/+=' | cut -c1-28)#" .env
sudo docker compose up -d --build
```

Compose starts two services:

- **`chatty`** — the Go app (multi-stage build → static `scratch` image, non-root UID 65532),
  published on `0.0.0.0:3333`, attached to the private `chatty` network **and** the external
  `mimir_mimir` network.
- **`chatty-db`** — PostgreSQL 17 with a named volume `chatty-db-data`, attached **only** to the
  private `chatty` network. **No host port is published.**

## Access

| Network | URL |
|---|---|
| Localhost | `http://localhost:3333` |
| LAN | `http://<host-lan-ip>:3333` |
| Tailscale | `http://<tailscale-ip>:3333` |

Both the LAN interface and the Tailscale interface reach the published port. There is no
authentication, so access is governed purely by who can route to `:3333`. Use firewall rules and/or
Tailscale ACLs to scope this.

## Configuration reference

Set in `.env` (compose interpolates `${CHATTY_DB_PASSWORD}` into both the app and Postgres, so the
DB secret has a single source of truth).

| Var | Default | Notes |
|---|---|---|
| `APP_ENV` | `production` | |
| `APP_PORT` | `3333` | bound to `0.0.0.0` |
| `LOG_LEVEL` | `info` | debug/info/warn/error |
| `DATABASE_HOST/PORT/NAME/USER` | `chatty-db/5432/chatty/chatty` | overridden in compose |
| `CHATTY_DB_PASSWORD` | — | **required**; shared app↔db secret |
| `DATABASE_SSLMODE` | `disable` | internal network |
| `MIMIR_BASE_URL` | `http://inference:8080/v1` | via `mimir_mimir` |
| `MIMIR_MODEL` | *(empty)* | empty/`auto` → discovered from `/v1/models` |
| `MIMIR_API_KEY` | *(empty)* | this llama.cpp build has no key |
| `MIMIR_TIMEOUT` | `300` | seconds (bare number) or Go duration |
| `MIMIR_THINKING` | `false` | Qwen3 reasoning |
| `MIMIR_BACKEND` | `auto` | `llamacpp` disables thinking via `chat_template_kwargs` |
| `CHAT_TEMPERATURE` | `0.7` | |
| `CHAT_MAX_TOKENS` | `1024` | auto-clamped to the model window |
| `CHAT_MAX_CONTEXT_MESSAGES` | `40` | history window |
| `CHAT_MAX_CONTEXT_TOKENS` | `1400` | prompt trim target |

> **Context budget:** the deployed `Qwen3-1.7B` slot has `n_ctx = 2048`. Chatty auto-discovers this
> and clamps `max_tokens` + trims history so a request cannot overflow it. Keep `CHAT_MAX_TOKENS`
> ≤ ~1500 in practice.

## Operations

```bash
sudo docker compose ps           # status (both should be "healthy")
sudo docker compose logs -f chatty
sudo docker compose restart chatty
sudo docker compose up -d --build   # rebuild after code changes
sudo docker compose down           # stop (volume + data preserved)
sudo docker compose down -v         # ⚠️ also delete the database volume
```

Data persists in the `chatty-db-data` volume across restarts, container recreation, and host reboots
(`restart: unless-stopped`).

## Verification checklist

```bash
# Health (DB connected + Mimir connected + real stats)
curl -s localhost:3333/health | python3 -m json.tool

# Containers
sudo docker compose ps

# PostgreSQL is NOT exposed to the host
sudo docker port chatty-db            # -> empty
ss -ltn | grep ':5432' || echo "not exposed (good)"

# Real streaming generation
CID=$(curl -s -X POST localhost:3333/api/conversations -d '{}' -H 'Content-Type: application/json' | jq -r .id)
curl -N -X POST localhost:3333/api/conversations/$CID/generate \
     -H 'Content-Type: application/json' -d '{"content":"Say hello in five words."}'

# Analytics
curl -s localhost:3333/api/stats | python3 -m json.tool
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Health `"mimir":"unreachable"` | `mimir_mimir` network missing or Mimir down | `sudo docker network inspect mimir_mimir`; ensure `mimir-inference` is up and healthy |
| Health `"database":"unreachable"` | Postgres not ready / wrong password | Check `chatty-db` logs; confirm `CHATTY_DB_PASSWORD` matches |
| Port 3333 in use | Another process | Change `APP_PORT` (compose maps `3333:3333`) |
| Empty replies / very short | Context overflow on the 2048 window | Lower `CHAT_MAX_TOKENS` / `CHAT_MAX_CONTEXT_MESSAGES` |
| Model not found | Discovery failed at startup | Check `MIMIR_BASE_URL`; re-check with `curl http://inference:8080/v1/models` from a container on `mimir_mimir` |

## Build details

The image is a two-stage build: `golang:1.27-alpine` compiles a `CGO_ENABLED=0` static binary
(front-end embedded via `go:embed`), copied into `FROM scratch`. The container runs as UID 65532,
ships CA certs + tzdata, and self-heals via `HEALTHCHECK` calling `/chatty healthcheck` (there is no
shell in `scratch`). Final image ≈ the size of the 12 MB binary + certs/tzdata.
