# syntax=docker/dockerfile:1.7

# -----------------------------------------------------------------------------
# Dependencies — cache bun installs separately from source changes.
#
# deps and build are pinned to BUILDPLATFORM: the Vite output in /app/dist is
# plain static assets, identical for every target arch, so there is no reason
# to run bun under QEMU when building the linux/arm64 image.
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM oven/bun:1.3-alpine AS deps

WORKDIR /app

COPY package.json bun.lock ./

RUN --mount=type=cache,target=/root/.bun/install/cache \
	bun install --frozen-lockfile

# -----------------------------------------------------------------------------
# Build — compile TypeScript and bundle static assets with Vite.
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM oven/bun:1.3-alpine AS build

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Override at build time when the API is hosted on another origin.
# Default keeps relative /api/v1 calls (proxied by nginx at runtime).
ARG VITE_API_URL=/api/v1
ENV VITE_API_URL=${VITE_API_URL}

RUN bun run build

# -----------------------------------------------------------------------------
# Runtime — nginx serves the SPA and proxies /api to the backend.
# -----------------------------------------------------------------------------
FROM nginx:1.31-alpine AS runtime

# Upstream for /api/* — override at deploy time (Compose service name, K8s svc, etc.).
ENV API_UPSTREAM=http://backend:8080

COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD wget -qO- http://127.0.0.1/health >/dev/null 2>&1 || exit 1
