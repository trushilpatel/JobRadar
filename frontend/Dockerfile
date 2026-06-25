FROM node:20-alpine AS builder

ARG VITE_CLERK_PUBLISHABLE_KEY
ENV VITE_CLERK_PUBLISHABLE_KEY=${VITE_CLERK_PUBLISHABLE_KEY}

WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build

# Final image runs as UID 101 (built-in nginx user in the unprivileged image).
FROM nginxinc/nginx-unprivileged:1.27-alpine

USER root
COPY --from=builder /app/dist /usr/share/nginx/html

# nginx config is supplied at runtime by the Helm ConfigMap
# (k8s/helm/jobradar/templates/console/configmap.yaml), mounted over
# /etc/nginx/conf.d/default.conf. The image ships no app nginx config so routing
# and cache headers have a single source of truth.

# Bake the build SHA into a static /version.json so a deploy can be verified
# against the intended commit (stale-bundle / rollout-drift detection).
ARG GIT_SHA=dev
RUN printf '{"sha":"%s"}\n' "${GIT_SHA}" > /usr/share/nginx/html/version.json

RUN chown -R 101:101 /usr/share/nginx/html
USER 101

LABEL org.opencontainers.image.revision="${GIT_SHA}"

EXPOSE 8080
