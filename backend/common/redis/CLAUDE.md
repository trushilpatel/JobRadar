# redis/ — Redis client bootstrap

Single-purpose package: wraps `go-redis/v9` with a ping-on-connect `New(ctx, addr, db)`.

Used by `service/llm/store/` for per-slot rate-limit + cooldown state. No other package in the codebase should construct its own Redis client.
