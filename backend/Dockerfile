# syntax=docker/dockerfile:1.7

# -----------------------------------------------------------------------------
# Build — compile static Go binaries (CGO disabled for a portable executable).
#
# Pinned to BUILDPLATFORM so the compiler always runs natively on the builder;
# the target architecture comes from GOARCH. Cross-compiling is free with
# CGO_ENABLED=0 and avoids running the whole Go toolchain under QEMU.
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

# Cache module downloads separately from sources.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Align go.mod/go.sum with all packages in the module.
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-trimpath \
	-ldflags="-s -w -X github.com/javdet/nib/internal/version.value=${VERSION}" \
	-o /out/nib ./cmd/nib

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/toolcatalog ./cmd/toolcatalog

# -----------------------------------------------------------------------------
# Runtime — Debian slim with networking/debug utilities (not Alpine).
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS runtime

ENV DEBIAN_FRONTEND=noninteractive

RUN rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/* \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends \
		bash \
		ca-certificates \
		curl \
		dnsutils \
		iproute2 \
		jq \
		netcat-openbsd \
		openssl \
		procps \
		telnet \
	&& rm -rf /var/lib/apt/lists/* \
	&& update-ca-certificates

WORKDIR /app

COPY --from=build /out/nib /usr/local/bin/nib
COPY --from=build /out/toolcatalog /usr/local/bin/toolcatalog
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/config.yaml /app/config.yaml

# Defaults copied into DATA_DIR on first start (see docker-entrypoint.sh).
COPY --from=build /src/seed /app/seed

COPY --from=build /src/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV SERVER_HOST=0.0.0.0 \
	SERVER_PORT=8080 \
	MIGRATIONS_PATH=/app/migrations \
	DATA_DIR=/app/data \
	SEED_DIR=/app/seed

# Declared so `docker run` without an explicit -v still gets a persistent
# anonymous volume instead of writing into the container layer. Compose and the
# Helm chart both override this with a named volume / PVC.
VOLUME ["/app/data"]

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=60s --retries=10 \
	CMD curl -sf http://localhost:8080/api/v1/health || exit 1

# The entrypoint creates the DATA_DIR layout, then execs nib with CMD.
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["-config", "/app/config.yaml"]
