# Chatty — Phase 1 Environment Discovery Notes

## Host
- Ubuntu LTS, Linux x86_64
- LAN IP: `<host-lan-ip>` (WiFi interface)
- Tailscale IP: `<tailscale-ip>` (tailscale0)
- Docker, Docker Compose (requires sudo)
- Go available at `/usr/local/go/bin` (not on default PATH)

## Existing Mimir stack (do NOT disrupt / do NOT redeploy)
Containers on docker network **`mimir_mimir`**:
- `mimir-inference` (mimir/inference:latest)  → `inference`/`mimir-inference` alias, port **8080/tcp** (internal)
- `mimir-backend`   (3000/tcp)
- `mimir-frontend`  (127.0.0.1:8088->80)

### Mimir inference = llama.cpp `llama-server`
- Entrypoint launches: `llama-server --model /models/Qwen3-1.7B-Q4_K_M.gguf --host 0.0.0.0 --port 8080 --ctx-size 4096 --n-gpu-layers 99 --parallel 2 ...`
- Model file: **Qwen3-1.7B-Q4_K_M.gguf** (Q4_K_M, 2.03B params)
- API key: **none** (MIMIR_API_KEY empty → no auth needed)
- Model id from `/v1/models`: **`/models/Qwen3-1.7B-Q4_K_M.gguf`**
- **n_ctx = 2048 per slot** (4096 total / 2 parallel slots). Tight context budget.

### Thinking (Qwen3) — empirical findings against this llama-server build
| Request field | Effect |
|---|---|
| `chat_template_kwargs:{"enable_thinking":false}` (top-level) | ✅ Thinking OFF, clean content, fast. **This is the correct mechanism.** |
| `reasoning_effort: "none"` | ✅ Works too |
| `thinking: false` (top-level) | ❌ Ignored — reasoning still generated |

→ Chatty sends **top-level** `chat_template_kwargs.enable_thinking` to disable thinking (note: OpenAI Python SDK's `extra_body` flattens to top-level over the wire, so this matches the spec's intent).

### Streaming (SSE)
- Standard OpenAI chunk format; `delta.content` tokens, final `delta:{}` + finish_reason, then a usage-only chunk (empty `choices`) with `usage` + llama `timings`, then `data: [DONE]`.
- `stream_options.include_usage:true` yields the usage chunk.
- When thinking is ON, reasoning streams in `delta.reasoning_content`.

### Overflow behaviour
- `max_tokens > n_ctx`: llama clamps gracefully, `finish_reason:stop` (no error).
- Overlong prompt: llama truncates the prompt (does not hard-error on this build). Still, Chatty trims history in-engine.

### Extra endpoint
- `POST /tokenize` → accurate token counts.

## Integration plan
- Chatty attaches to **`mimir_mimir`** external network (reuse, no second Mimir).
- `MIMIR_BASE_URL=http://inference:8080/v1` (alias `inference` resolves on the shared network).
- Model auto-discovered via `/v1/models` when `MIMIR_MODEL` empty or `auto`.
- Context budget kept conservative (n_ctx=2048) via in-engine token-budget trimming.
