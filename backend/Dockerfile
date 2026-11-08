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
# Tools — single-binary CLIs for the agent's local commands, fetched by pinned
# version and checked against the sha256 the project publishes for it.
#
# Downloaded on the builder's own platform (nothing here executes a target
# binary), so the arm64 image does not pay for QEMU. Only the binaries are
# copied into the runtime; archives and curl's state stay behind.
#
# The image has a size budget for these tools (~300 MB on top of the base), which
# is what decided what is in and what is out, in priority order: base utilities,
# then Kubernetes, then cloud CLIs, then IaC. Left out, by unpacked size: aws-cli
# v2 (~270 MB), gcloud (~320 MB even without its bundled Python), doctl (~50 MB)
# and terraform/tofu (~110-120 MB each). Clouds and state go through MCP.
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS tools

ARG TARGETARCH

ARG KUBECTL_VERSION=1.37.2
ARG KUBECTL_SHA256_AMD64=c04487523e7aa7d1493f7808174c899db93d7a9cf57531420cd76b9bd415f8f4
ARG KUBECTL_SHA256_ARM64=fcc16f63186659d9722014b7e02d5fdd2d4e669cb8c0ab66365d7d3b8ba03509

ARG HELM_VERSION=4.3.1
ARG HELM_SHA256_AMD64=0403a88ae20913d11b10a2467b50087721fea7581736da6b9d3992f5357fe54b
ARG HELM_SHA256_ARM64=8cf8f66ecbc252ac8f7f06cca68aba88508ddfde3a6a34feb1678ad1694b4f88

ARG KUSTOMIZE_VERSION=5.8.3
ARG KUSTOMIZE_SHA256_AMD64=cb9e31198d3f63b44848bd0afc4c8efc56e6cfef9c93a4a39a211940e6decd61
ARG KUSTOMIZE_SHA256_ARM64=9867b76482cfc6d25b546abc1d2b5a32bd137505aba640649b4b378f64f6d68f

ARG YQ_VERSION=4.54.2
ARG YQ_SHA256_AMD64=44c7759059b4910827a0059a2757b22001e59360cdecdb2ef8dedc4b6c7e9f32
ARG YQ_SHA256_ARM64=bd44211dcd29a5f0eb701d4b7eb5ccd0cfff3bd3966cb4f7f9bd5b13701d997e

# gh and glab for the GitHub and GitLab APIs (`gh api`, `glab api`, PRs/MRs,
# pipelines). There is deliberately no git: a repository is read through the
# API, and anything that needs a working copy belongs to the executor.
ARG GH_VERSION=2.103.3
ARG GH_SHA256_AMD64=4c806978ffce1cec70853302a56d70ac6da4f19bae7adca4e1a8ab00633dd91d
ARG GH_SHA256_ARM64=ee32df2323c8847f5fac6cb6b4b69ca75828e9329a8f22f677021f7b3306ce63

ARG GLAB_VERSION=1.128.0
ARG GLAB_SHA256_AMD64=bc024819e7598b19429a199cd87f640d98b2b67416d8ad796f19e4260b57a7c7
ARG GLAB_SHA256_ARM64=60d0512085a8c1be7c5ef51aa7e4ee28213444cea0376b3260707d90489199ea

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl \
	&& rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

WORKDIR /dl

# fetch URL SHA256 FILE — download and refuse anything whose digest differs.
RUN case "${TARGETARCH}" in \
		amd64|arm64) ;; \
		*) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
	esac \
	&& arch_upper="${TARGETARCH^^}" \
	&& sha() { local v="$1_SHA256_${arch_upper}"; printf '%s' "${!v}"; } \
	&& fetch() { curl -fsSL --retry 3 -o "$3" "$1" && echo "$2  $3" | sha256sum -c -; } \
	&& mkdir -p /out \
	&& fetch "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" "$(sha KUBECTL)" kubectl \
	&& install -m 0755 kubectl /out/kubectl \
	&& fetch "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" "$(sha HELM)" helm.tgz \
	&& tar -xzf helm.tgz --strip-components=1 "linux-${TARGETARCH}/helm" \
	&& install -m 0755 helm /out/helm \
	&& fetch "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz" "$(sha KUSTOMIZE)" kustomize.tgz \
	&& tar -xzf kustomize.tgz kustomize \
	&& install -m 0755 kustomize /out/kustomize \
	&& fetch "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_${TARGETARCH}" "$(sha YQ)" yq \
	&& install -m 0755 yq /out/yq \
	&& fetch "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${TARGETARCH}.tar.gz" "$(sha GH)" gh.tgz \
	&& tar -xzf gh.tgz --strip-components=2 "gh_${GH_VERSION}_linux_${TARGETARCH}/bin/gh" \
	&& install -m 0755 gh /out/gh \
	&& fetch "https://gitlab.com/api/v4/projects/gitlab-org%2Fcli/packages/generic/glab/${GLAB_VERSION}/glab_${GLAB_VERSION}_linux_${TARGETARCH}.tar.gz" "$(sha GLAB)" glab.tgz \
	&& tar -xzf glab.tgz --strip-components=1 bin/glab \
	&& install -m 0755 glab /out/glab

# -----------------------------------------------------------------------------
# Runtime — Debian slim with networking/debug utilities (not Alpine).
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS runtime

ENV DEBIAN_FRONTEND=noninteractive

# ansible-core comes from Debian rather than pip: it reuses the python3 already
# here and pulls in openssh-client, which the agent wants anyway.
RUN rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/* \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends \
		ansible-core \
		bash \
		ca-certificates \
		curl \
		dnsutils \
		iproute2 \
		iputils-ping \
		jq \
		mtr-tiny \
		netcat-openbsd \
		openssl \
		procps \
		python3 \
		telnet \
		tini \
	&& rm -rf /var/lib/apt/lists/* \
	&& update-ca-certificates

COPY --from=tools /out/ /usr/local/bin/

WORKDIR /app

COPY --from=build /out/nib /usr/local/bin/nib
COPY --from=build /out/toolcatalog /usr/local/bin/toolcatalog
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/config.yaml /app/config.yaml

# Defaults copied into DATA_DIR on first start (see docker-entrypoint.sh).
COPY --from=build /src/seed /app/seed

COPY --from=build /src/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# nib runs as this user, never root: the entrypoint drops to it once DATA_DIR is prepared.
# /app and config.yaml are handed over so an unmounted config.yaml stays editable, since
# config.Manager rewrites it through a temp file beside it.
RUN groupadd --system --gid 10001 nib \
	&& useradd --system --uid 10001 --gid nib --home-dir /home/nib --create-home \
		--shell /usr/sbin/nologin nib \
	&& chown nib:nib /app /app/config.yaml

ENV SERVER_HOST=0.0.0.0 \
	SERVER_PORT=8080 \
	METRICS_HOST=0.0.0.0 \
	METRICS_PORT=9090 \
	METRICS_PATH=/metrics \
	MIGRATIONS_PATH=/app/migrations \
	DATA_DIR=/app/data \
	SEED_DIR=/app/seed

# Declared so `docker run` without an explicit -v still gets a persistent
# anonymous volume instead of writing into the container layer. Compose and the
# Helm chart both override this with a named volume / PVC.
VOLUME ["/app/data"]

# 8080 is the API; 9090 serves Prometheus metrics only.
EXPOSE 8080 9090

HEALTHCHECK --interval=15s --timeout=3s --start-period=60s --retries=10 \
	CMD curl -sf http://localhost:8080/api/v1/health || exit 1

# The entrypoint creates the DATA_DIR layout as root, then execs nib with CMD as the nib user.
# tini is PID 1 so nib is not: a command an agent backgrounds outlives its shell, is reparented to
# PID 1 when the job is killed, and only an init reaps it.
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["-config", "/app/config.yaml"]
