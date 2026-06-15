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
COPY nginx.conf /etc/nginx/conf.d/default.conf
RUN chown -R 101:101 /usr/share/nginx/html /etc/nginx/conf.d
USER 101

ARG GIT_SHA=dev
LABEL org.opencontainers.image.revision="${GIT_SHA}"

EXPOSE 8080
