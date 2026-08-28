FROM node:24-trixie-slim

ARG IMAGE_VERSION
ENV IMAGE_VERSION=${IMAGE_VERSION}
ENV DEBIAN_FRONTEND=noninteractive

# Default auth path: OpenRouter via ANTHROPIC_API_KEY (mapped to ANTHROPIC_AUTH_TOKEN
# in the entrypoint). No /v1 suffix — Claude Code appends /v1/messages itself.
# Override: set CLAUDE_CODE_OAUTH_TOKEN for Claude.ai subscription auth; the
# entrypoint then unsets these OpenRouter defaults and talks to api.anthropic.com.
ENV ANTHROPIC_BASE_URL=https://openrouter.ai/api
ENV ANTHROPIC_MODEL=anthropic/claude-opus-5

ARG GH_VERSION=2.97.0
ARG TARGETARCH

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gettext-base \
    git \
    jq \
    less \
  && case "${TARGETARCH}" in \
       amd64) GH_ARCH=amd64 ;; \
       arm64) GH_ARCH=arm64 ;; \
       *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
     esac \
  && curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
    | tar -xz -C /tmp \
  && mv "/tmp/gh_${GH_VERSION}_linux_${GH_ARCH}/bin/gh" /usr/local/bin/gh \
  && rm -rf "/tmp/gh_${GH_VERSION}_linux_${GH_ARCH}" \
  && rm -rf /var/lib/apt/lists/*

ARG CLAUDE_CODE_VERSION=2.1.220
RUN npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"

COPY --chmod=0755 agent-entrypoint.sh /usr/local/bin/agent-entrypoint.sh

RUN mkdir -p /workspace && chown -R node:node /workspace
WORKDIR /workspace
USER node

ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]
