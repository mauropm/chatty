# Chatty — Architecture

## Overview

Chatty is a single Go binary that serves an embedded single-page front end, a JSON REST API, and
Server-Sent Events (SSE) streaming, backed by PostgreSQL and the existing Mimir inference service.

```
                 ┌───────────────────────────── Go binary (chatty) ─────────────────────────────┐
 Browser ──────▶ │ HTTP server (net/http)                                                       │
  (SPA)          │   • middleware: request-id · security headers/CSP · access-log · recover     │
                 │   • static assets + index.html  (go:embed from /web)                          │
                 │   • /api/* handlers  →  domain services                                       │
                 │        ├── conversations.Repo   ─┐                                            │
                 │        ├── messages.Repo         ├─▶ PostgreSQL pool (pgx/v5) → chatty-db      │
                 │        ├── statistics.Service   ─┘                                            │
                 │        ├── settings.Service      (application_settings, validated overrides)  │
                 │        └── chat.Engine  ───────────────────────────────────────────────┐      │
                 └─────────────────────────────────────────────────────────────────────── │ ────┘
                                                                                            ▼
                                                            mimir.Client → Mimir llama-server (Qwen3)
                                                            OpenAI-compatible /v1/chat/completions (SSE)
```

## Packages

| Package | Responsibility |
|---|---|
| `cmd/chatty` | Wiring, startup, graceful shutdown, `healthcheck` subcommand. |
| `internal/config` | Env parsing + validation. Unit tested. |
| `internal/observability` | `slog` JSON logging, request-id context, redaction. |
| `internal/database` | pgx pool with retry-until-ready, UTC sessions. |
| `internal/migrations` | Embedded versioned SQL + `schema_migrations` runner. |
| `internal/conversations` | Conversation CRUD, search (FTS + ILIKE), title generation. |
| `internal/messages` | Message append/list/context rules, streaming finalize, partial persistence. |
| `internal/mimir` | OpenAI-compatible client: discovery, completions, streaming, SSE parser, backend compatibility layer. |
| `internal/chat` | Orchestrator: context building, token budgeting, streaming to browser, stop/regenerate, usage. |
| `internal/statistics` | `/health` counters + `/api/stats` analytics (daily activity, model usage). |
| `internal/settings` | Runtime generation overrides (validated), stored in `application_settings`. |
| `internal/api` | Router, middleware, handlers, SSE writers, event hub. |
| `web` | Embedded front end (`index.html`, `static/`). |

## Request lifecycle: a chat turn

1. `POST /api/conversations/{id}/generate` (SSE). The handler rejects if a generation is already
   running for that conversation (409) **before** opening the stream.
2. `chat.Engine.Generate`:
   - Loads the conversation, then **registers** it in an in-flight map (per-conversation lock) and
     creates a generation context derived from the request context.
   - Persists the new **user** message; on the first turn, auto-generates a title (no LLM call).
   - Builds the model context: system prompt + the most recent *completed* user/assistant turns
     (failed/incomplete assistant messages are never replayed), trimmed to a **token budget**
     derived from the model's real context window (`n_ctx`), so the request can never provably
     overflow the 2048-token llama.cpp slot.
   - Creates a `streaming` assistant row so an interruption can persist a partial.
   - Calls `mimir.Client.ChatCompletionStream`, forwarding `delta`/`reasoning` events to the browser.
3. On completion / stop / error, the assistant message is **finalized** on a detached context
   (`complete` / `stopped` with partial / `failed`), a `usage_events` row is recorded, and the
   conversation is touched. The browser receives a terminal `done` (or `error`) event.

**Cancellation propagation:** stopping via `POST /stop`, the browser closing the SSE connection, or
the HTTP request being canceled during shutdown all cancel the generation context, which aborts the
outbound Mimir HTTP request — closing the connection upstream and freeing a llama.cpp slot. A partial
response is always persisted.

## Mimir compatibility layer

The deployed backend is llama.cpp `llama-server`. Empirically (see `docs/DISCOVERY.md`):

- Thinking (Qwen3) is disabled by sending a **top-level** `chat_template_kwargs: {"enable_thinking": false}`.
  (The OpenAI Python SDK's `extra_body` flattens to this on the wire.) A top-level `thinking:false`
  is **ignored** by this build; `reasoning_effort:"none"` also works but is not needed.
- Streaming uses `stream_options.include_usage` so a final usage-only chunk carries token counts.
- When thinking is **on**, reasoning streams in `delta.reasoning_content` and is forwarded as
  `reasoning` SSE events.

The `Backend` selector (`auto`/`llamacpp`/`openai`) switches how thinking is expressed. `auto`
detects from `/v1/models` `owned_by` (e.g. `llamacpp`) or the `.gguf` model id.

## Front end

Vanilla JS (no framework). `marked` → `DOMPurify` → `highlight.js` for safe Markdown + syntax
highlighting, all bundled locally (no CDN). Fonts (Inter, JetBrains Mono) are embedded. Streaming is
read from a `fetch` `ReadableStream` and rendered with `requestAnimationFrame` throttling; code
blocks get copy buttons and language badges. State is kept in a small in-memory store; conversation
history is loaded from the API on demand.

## Database schema

Four tables (see `internal/migrations/0001_init.up.sql`): `conversations`, `messages`,
`usage_events`, `application_settings`, with generated `tsvector` columns + GIN indexes for search
and indexes for ordering/retrieval/reporting. A trigger keeps `conversations.updated_at` fresh. All
timestamps are `TIMESTAMPTZ` (UTC); IDs are native `uuid`.
