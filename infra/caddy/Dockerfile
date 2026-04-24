# syntax=docker/dockerfile:1.7
# Pinned plugin versions — bump deliberately; do not leave as "latest".
# caddy-ratelimit: https://github.com/mholt/caddy-ratelimit
# caddy-crowdsec-bouncer: https://github.com/hslatman/caddy-crowdsec-bouncer
ARG CADDY_VERSION=2.8.4
ARG PLUGIN_RATELIMIT_VERSION=v0.1.0
ARG PLUGIN_CROWDSEC_VERSION=v0.8.0

FROM caddy:${CADDY_VERSION}-builder AS builder
ARG PLUGIN_RATELIMIT_VERSION
ARG PLUGIN_CROWDSEC_VERSION
RUN xcaddy build \
    --with github.com/mholt/caddy-ratelimit@${PLUGIN_RATELIMIT_VERSION} \
    --with github.com/hslatman/caddy-crowdsec-bouncer/http@${PLUGIN_CROWDSEC_VERSION} \
    --with github.com/hslatman/caddy-crowdsec-bouncer/crowdsec@${PLUGIN_CROWDSEC_VERSION}

FROM caddy:${CADDY_VERSION}-alpine

# Non-root runtime user (caddy image ships with root; we add a dedicated uid/gid).
RUN addgroup -S -g 10001 caddy_runtime && \
    adduser -S -D -H -u 10001 -G caddy_runtime caddy_runtime && \
    mkdir -p /var/log/caddy /data /config && \
    chown -R caddy_runtime:caddy_runtime /var/log/caddy /data /config

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

USER 10001:10001

ENTRYPOINT ["/usr/bin/caddy"]
CMD ["run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
