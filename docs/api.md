# Chatty — API reference

Base URL: `http://<host>:3333`. All JSON request/response bodies are `application/json; charset=utf-8`.
UUIDs are canonical strings. Timestamps are RFC3339 UTC. Errors are `{"error": "<message>"}` with a
semantic HTTP status. Responses carry `X-Request-Id`.

> **Security:** there is no authentication. Anyone who can reach the endpoint can use the app —
> restrict at the network layer.

## Health & analytics

### `GET /health`
Returns real dependency status and usage counters. **503** when the database (critical) is
unreachable; **200** with `"status":"degraded"` when only Mimir is unavailable.

```json
{
  "status": "healthy", "service": "chatty", "version": "1.0.0", "uptime_seconds": 123,
  "database": { "status": "connected", "latency_ms": 2 },
  "mimir":    { "status": "connected", "model": "/models/Qwen3-1.7B-Q4_K_M.gguf", "latency_ms": 15 },
  "statistics": { "total_conversations": 100, "total_messages": 1500, "total_user_messages": 750,
    "total_assistant_messages": 750, "total_tokens_prompt": 100000, "total_tokens_completion": 200000,
    "total_requests": 750, "successful_requests": 740, "failed_requests": 10 }
}
```

### `GET /api/stats`
Full report: `messages_by_role`, `model_usage[]`, `daily_activity_7d[]`, `daily_activity_30d[]`,
`avg_latency_ms`, plus the overview counters.

### `GET /api/models`
Discovers models from Mimir; returns `{ "models":[{id, owned_by, context, active, backend, thinking}], "active_model": "…" }`.

## Conversations

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/conversations` | Lists active conversations. `?q=` searches title **and** message content; `?include_archived=true`; `?limit=`. |
| `POST` | `/api/conversations` | Body `{ "title"?: string }`. Returns the created conversation (201). |
| `GET` | `/api/conversations/{id}` | One conversation (with `message_count`). |
| `PATCH` | `/api/conversations/{id}` | Body `{ "title"?: string, "archived"?: bool }`. |
| `DELETE` | `/api/conversations/{id}` | 204. 409 if it is currently generating. |
| `GET` | `/api/conversations/{id}/export` | `?format=markdown|json` (attachment). |

## Messages & generation

| Method | Path | Body | Result |
|---|---|---|---|
| `GET` | `/api/conversations/{id}/messages` | — | `{ "messages": [Message] }` chronological. |
| `POST` | `/api/conversations/{id}/messages` | `{ role?, content }` | Appends a user/system message (201). Does not generate. |
| `POST` | `/api/conversations/{id}/generate` | `{ content? }` | **SSE stream** of a new assistant reply. |
| `POST` | `/api/conversations/{id}/regenerate` | `{}` | **SSE stream** replacing the last assistant message. |
| `POST` | `/api/conversations/{id}/stop` | — | `{ "stopped": true|false }` — cancels an in-flight generation. |

`Message`:
```json
{ "id","conversation_id","role","content","status","model",
  "prompt_tokens","completion_tokens","latency_ms","error",
  "created_at","completed_at" }
```
`status ∈ pending|streaming|complete|failed|stopped`.

### Generation stream (SSE)

`POST …/generate` responds `text/event-stream`. Each frame is `event: <type>` + `data: <json>`.

| Event | Data |
|---|---|
| `meta` | `{ message_id, conversation_id, model, created_at }` (assistant row created) |
| `delta` | `{ content }` incremental answer text |
| `reasoning` | `{ content }` (only when thinking is enabled) |
| `done` | `{ message_id, finish_reason, prompt_tokens, completion_tokens, latency_ms, status, content }` |
| `error` | `{ message, retryable, status }` (hard failure; a stop emits `done` with `status:"stopped"`) |

Concurrency: a second `generate`/`regenerate` while one is running returns **409**.

## Settings

| Method | Path | Body |
|---|---|---|
| `GET` | `/api/settings` | — → `{ generation, defaults, model, system, database, mimir }` |
| `PATCH` | `/api/settings` | Partial generation override: `{ temperature?, max_tokens?, thinking?, max_context_messages?, max_context_tokens? }`. Validated; out-of-range → **400**. |

## Global events

### `GET /api/events`
Long-lived SSE. Sends `event: hello {service,version,model,active,server}`, a heartbeat every ~20s,
and lifecycle broadcasts (`conversation.created`, `conversation.deleted`).
