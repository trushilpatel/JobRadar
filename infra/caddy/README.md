# Caddy reverse proxy

Single entry point for HTTPS. Terminates TLS (Let's Encrypt), applies security
headers, rate-limits, and the CrowdSec bouncer, then proxies to `api`, `console`,
and `grafana` services on the internal compose network.

## Build

Plugin versions are pinned via build args in the Dockerfile:

- `CADDY_VERSION` (default `2.8.4`)
- `PLUGIN_RATELIMIT_VERSION` (default `v0.1.0`) — `github.com/mholt/caddy-ratelimit`
- `PLUGIN_CROWDSEC_VERSION` (default `v0.8.0`) — `github.com/hslatman/caddy-crowdsec-bouncer`

Rebuild the image to bump plugin versions.

## Env vars

| Var                         | Default                                                      | Purpose                                                                 |
| --------------------------- | ------------------------------------------------------------ | ----------------------------------------------------------------------- |
| `DOMAIN`                    | (required)                                                   | Apex domain. Sites resolve to `app.$DOMAIN` and `grafana.$DOMAIN`.      |
| `ACME_EMAIL`                | (required)                                                   | Contact email for Let's Encrypt.                                        |
| `ACME_CA`                   | `https://acme-v02.api.letsencrypt.org/directory`             | ACME directory URL. Override to LE staging in dev.                      |
| `SUPABASE_PROJECT_HOST`     | (required)                                                   | Supabase host, e.g. `abc.supabase.co`. Used only in CSP `connect-src`.  |
| `RATE_LIMIT_API_PER_MIN`    | `30`                                                         | Per-IP rate limit for `/api/*` requests (requests per minute).          |
| `RATE_LIMIT_CALLBACK_PER_MIN` | `5`                                                        | Per-IP rate limit for `/auth/callback` (requests per minute).           |
| `CROWDSEC_LAPI_URL`         | `http://crowdsec:8080`                                       | LAPI endpoint for bouncer decisions.                                    |
| `CROWDSEC_LAPI_KEY`         | (required)                                                   | Bouncer API key registered via `cscli bouncers add`.                    |

## Logs

JSON access logs write to `/var/log/caddy/app.log` and `/var/log/caddy/grafana.log`.
Mount a volume at `/var/log/caddy` so Promtail and CrowdSec can tail them.

## Validate

```
docker run --rm \
  -e DOMAIN=example.com -e ACME_EMAIL=a@example.com \
  -e SUPABASE_PROJECT_HOST=abc.supabase.co \
  -e CROWDSEC_LAPI_KEY=placeholder \
  -v "$(pwd)/infra/caddy:/etc/caddy" \
  caddy:2.8-alpine caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

Note: `validate` against stock `caddy:2.8-alpine` will flag the `rate_limit` and
`crowdsec` directives as unknown because those plugins are only in the custom
image. Use the custom image (or `caddy adapt`) to fully validate.
