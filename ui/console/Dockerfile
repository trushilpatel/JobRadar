FROM node:20-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build

# ── Final image ───────────────────────────────────────────────────────────────

FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

ARG GIT_SHA=dev
LABEL org.opencontainers.image.revision="${GIT_SHA}"

EXPOSE 80
