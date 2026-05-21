# redis/ — Redis client bootstrap

Single-purpose package: wraps `go-redis/v9` with a ping-on-connect `New(ctx, url)`.

The `url` is a full connection string parsed by `goredis.ParseURL`:

- `redis://...` — plain TCP (local dev)
- `rediss://...` — TLS (Upstash and any managed Redis)

Username, password, host, port, and db index all live in the URL.

The returned `*Client` is shared across consumers in each binary:

- `apps/api` — feeds `platform/cache.RedisCache` + `platform/ratelimit.RedisLimiter`
- `apps/filter` and `apps/analyzer` — feed `service/llm/store/` for per-slot rate-limit + cooldown state

No other package in the codebase should construct its own Redis client.
