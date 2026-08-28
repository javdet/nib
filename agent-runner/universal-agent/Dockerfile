FROM node:24-trixie-slim AS downloader

ARG GH_VERSION=2.97.0
ARG CODEX_VERSION=0.147.0
ARG TARGETARCH

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/* \
  && case "${TARGETARCH}" in \
       amd64) GH_ARCH=amd64; CODEX_ARCH=x86_64 ;; \
       arm64) GH_ARCH=arm64; CODEX_ARCH=aarch64 ;; \
       *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
     esac \
  && curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
    | tar -xz -C /tmp \
  && mv "/tmp/gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh" /tmp/gh \
  && curl -fsSL "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/codex-${CODEX_ARCH}-unknown-linux-musl.tar.gz" \
    | tar -xz -C /tmp \
  && mv "/tmp/codex-${CODEX_ARCH}-unknown-linux-musl" /tmp/codex

FROM node:24-trixie-slim

ARG IMAGE_VERSION
ENV IMAGE_VERSION=${IMAGE_VERSION}
ENV DEBIAN_FRONTEND=noninteractive

# AGENT_TYPE selects the runtime: claude-code (default) or codex.
ENV AGENT_TYPE=claude-code

# Claude Code defaults: OpenRouter via ANTHROPIC_API_KEY (mapped to
# ANTHROPIC_AUTH_TOKEN in the entrypoint). No /v1 suffix — Claude Code appends
# /v1/messages itself. Override with CLAUDE_CODE_OAUTH_TOKEN for subscription auth.
ENV ANTHROPIC_BASE_URL=https://openrouter.ai/api
ENV ANTHROPIC_MODEL=anthropic/claude-opus-5

# Codex defaults: OpenRouter Responses API via OPENAI_API_KEY.
ENV OPENAI_BASE_URL=https://openrouter.ai/api/v1
ENV OPENAI_MODEL=openai/gpt-5.3-codex
ENV CODEX_HOME=/home/node/.codex

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    bubblewrap \
    ca-certificates \
    curl \
    gettext-base \
    git \
    jq \
    less \
  && rm -rf /var/lib/apt/lists/*

COPY --from=downloader /tmp/gh /usr/local/bin/gh
COPY --from=downloader /tmp/codex /usr/local/bin/codex

ARG CLAUDE_CODE_VERSION=2.1.220
RUN npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
  && npm cache clean --force

COPY --chmod=0755 agent-entrypoint.sh /usr/local/bin/agent-entrypoint.sh

RUN mkdir -p /workspace "${CODEX_HOME}" \
  && chown -R node:node /workspace "${CODEX_HOME}"

WORKDIR /workspace
USER node

ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]
