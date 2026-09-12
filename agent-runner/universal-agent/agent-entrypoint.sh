#!/usr/bin/env bash
# Universal headless coding agent entrypoint for k8s Jobs.
# AGENT_TYPE selects claude-code (default) or codex; shared env contract for
# repo clone, MCP, git/PR, and webhook callback.
set -euo pipefail

AGENT_EXIT_CODE=0
WEBHOOK_FAILED=0
WEBHOOK_SENT=0

AGENT_TYPE="claude-code"
AGENT_RESULT=""
AGENT_IS_ERROR=false
AGENT_NUM_TURNS=0
AGENT_TOTAL_COST_USD=0
AGENT_DURATION_MS=0
AGENT_SESSION_ID=""
AGENT_START_MS=0
CODEX_INPUT_TOKENS=0
CODEX_OUTPUT_TOKENS=0

REPO_PUSHED=false
PR_URL=""

# Derived from REPO_URL by resolve_git_host so self-hosted GitLab and GitHub
# Enterprise authenticate the same way gitlab.com/github.com do.
GIT_PROVIDER="${GIT_PROVIDER:-github}"
GIT_HOST=""
GIT_REPO_PATH=""
GIT_REMOTE_BASE=""

WORK_DIR=""
LOG_FILE="/tmp/agent.log"
MCP_RESOLVED_FILE="/tmp/mcp-config.json"
CODEX_CONFIG_FILE=""
CODEX_LAST_MESSAGE_FILE="/tmp/codex-last-message.txt"

WEBHOOK_RESULT_FILE="/tmp/webhook-result.txt"
WEBHOOK_LOG_TAIL_FILE="/tmp/webhook-log-tail.txt"

: "${WEBHOOK_RESULT_MAX_BYTES:=16384}"
: "${WEBHOOK_LOG_TAIL_MAX_BYTES:=16384}"

CLAUDE_ARGS=()
CODEX_ARGS=()

log() {
  printf '[agent] %s\n' "$*" >&2
}

die() {
  log "ERROR: $*"
  AGENT_EXIT_CODE=1
  exit 1
}

is_codex_agent() {
  [ "${AGENT_TYPE}" = "codex" ]
}

normalize_agent_type() {
  local raw="${AGENT_TYPE:-claude-code}"
  raw="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"
  case "$raw" in
    claude|claude-code) AGENT_TYPE="claude-code" ;;
    codex) AGENT_TYPE="codex" ;;
    *)
      die "unsupported AGENT_TYPE: ${AGENT_TYPE:-unset} (expected claude-code or codex)"
      ;;
  esac
  log "Agent runtime: ${AGENT_TYPE}"
}

# Mirrors executor.NormalizeGitProvider in the backend: the setting behind
# GIT_PROVIDER is a free-text company field, so match loosely and treat anything
# unrecognised — blank included — as GitHub.
normalize_git_provider() {
  local raw
  raw="$(printf '%s' "${GIT_PROVIDER:-}" | tr '[:upper:]' '[:lower:]')"
  case "$raw" in
    *gitlab*) GIT_PROVIDER="gitlab" ;;
    *) GIT_PROVIDER="github" ;;
  esac
  export GIT_PROVIDER
  log "Git provider: ${GIT_PROVIDER}"
}

# The username half of the token credential: GitHub wants x-access-token,
# GitLab wants oauth2. Both take the token as the password.
git_credential_username() {
  if [ "${GIT_PROVIDER}" = "gitlab" ]; then
    printf 'oauth2'
  else
    printf 'x-access-token'
  fi
}

setup_auth_claude() {
  if [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
    log "Auth: CLAUDE_CODE_OAUTH_TOKEN (Anthropic subscription)"
    unset ANTHROPIC_BASE_URL
    unset ANTHROPIC_AUTH_TOKEN
    unset ANTHROPIC_API_KEY
    case "${ANTHROPIC_MODEL:-}" in
      */*) unset ANTHROPIC_MODEL ;;
    esac
    return
  fi

  log "Auth: OpenRouter / ANTHROPIC_AUTH_TOKEN (ANTHROPIC_BASE_URL=${ANTHROPIC_BASE_URL:-unset})"
  if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
    export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_API_KEY}"
  fi
  export ANTHROPIC_API_KEY=""
}

toml_quote() {
  jq -Rn --arg v "$1" '"\($v)"'
}

write_codex_model_config() {
  local model="${OPENAI_MODEL:-gpt-5.3-codex}"
  local base_url="${OPENAI_BASE_URL:-}"

  printf 'model = %s\n' "$(toml_quote "$model")"

  if [ -z "$base_url" ] || [[ "$base_url" == *"api.openai.com"* ]]; then
    printf 'model_provider = "openai"\n'
    return
  fi

  printf 'model_provider = "gateway"\n\n'
  printf '[model_providers.gateway]\n'
  printf 'name = "gateway"\n'
  printf 'base_url = %s\n' "$(toml_quote "$base_url")"
  printf 'env_key = "OPENAI_API_KEY"\n'
  printf 'wire_api = "responses"\n'
}

setup_auth_codex() {
  if [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
    die "CLAUDE_CODE_OAUTH_TOKEN is not supported with AGENT_TYPE=codex; use OPENAI_API_KEY or CODEX_API_KEY"
  fi

  local api_key="${OPENAI_API_KEY:-${CODEX_API_KEY:-}}"
  if [ -z "$api_key" ]; then
    die "OPENAI_API_KEY or CODEX_API_KEY is required for AGENT_TYPE=codex"
  fi

  export OPENAI_API_KEY="$api_key"
  export CODEX_API_KEY="$api_key"

  mkdir -p "${CODEX_HOME:-/home/node/.codex}"
  CODEX_CONFIG_FILE="${CODEX_HOME}/config.toml"
  write_codex_model_config > "${CODEX_CONFIG_FILE}"

  log "Auth: Codex (OPENAI_BASE_URL=${OPENAI_BASE_URL:-api.openai.com}, model=${OPENAI_MODEL:-gpt-5.3-codex})"
}

setup_auth() {
  if is_codex_agent; then
    setup_auth_codex
  else
    setup_auth_claude
  fi
}

setup_git_identity() {
  : "${GIT_AUTHOR_NAME:=-}"
  : "${GIT_AUTHOR_EMAIL:=-}"
  export GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL
  export GIT_COMMITTER_NAME="${GIT_AUTHOR_NAME}"
  export GIT_COMMITTER_EMAIL="${GIT_AUTHOR_EMAIL}"
  export GIT_TERMINAL_PROMPT=0
}

validate_env() {
  normalize_agent_type
  normalize_git_provider

  if [ -z "${PROMPT:-}" ]; then
    die "PROMPT is required"
  fi

  if [ -n "${REPO_URL:-}" ]; then
    if [ -z "${TARGET_BRANCH:-}" ]; then
      die "TARGET_BRANCH is required when REPO_URL is set"
    fi
    # One credential arrives under whichever name the caller knows; GITHUB_TOKEN
    # stays the documented wire name because the Kubernetes path takes it from an
    # operator-managed Secret this image does not control.
    GIT_TOKEN="${GIT_TOKEN:-${GITHUB_TOKEN:-${GITLAB_TOKEN:-}}}"
    if [ -z "${GIT_TOKEN}" ]; then
      die "a git API token is required when REPO_URL is set (GITHUB_TOKEN, GITLAB_TOKEN or GIT_TOKEN)"
    fi
    # Re-export it under every name the tooling looks for: gh reads
    # GH_TOKEN/GITHUB_TOKEN, glab reads GITLAB_TOKEN/GL_TOKEN, and MCP configs
    # still expand ${GITHUB_TOKEN}.
    export GIT_TOKEN
    export GITHUB_TOKEN="${GIT_TOKEN}"
    export GH_TOKEN="${GIT_TOKEN}"
    export GITLAB_TOKEN="${GIT_TOKEN}"
    export GL_TOKEN="${GIT_TOKEN}"
  fi

  : "${SYSTEM_PROMPT_MODE:=append}"
}

# Splits REPO_URL into the host and the project path. A bare "host/owner/repo"
# is accepted because GitBaseURL is operator-typed and often pasted without a
# scheme; an ssh/scp address is refused outright rather than cloned without
# credentials, which used to fail several steps later with "could not read Username".
resolve_git_host() {
  local rest="${REPO_URL%.git}" scheme="https"
  case "$rest" in
    https://*) rest="${rest#https://}" ;;
    http://*) scheme="http"; rest="${rest#http://}" ;;
    *://*) die "REPO_URL must be an http(s) URL: ${REPO_URL}" ;;
    *@*:*) die "REPO_URL must be an http(s) clone URL; an ssh address cannot be authenticated with a token: ${REPO_URL}" ;;
  esac

  case "$rest" in
    */*) GIT_REPO_PATH="${rest#*/}" ;;
    *) die "REPO_URL has no project path: ${REPO_URL}" ;;
  esac

  local hostpart="${rest%%/*}"
  GIT_HOST="${hostpart#*@}"
  if [ -z "${GIT_HOST}" ] || [ -z "${GIT_REPO_PATH}" ]; then
    die "could not derive a host and project path from REPO_URL: ${REPO_URL}"
  fi
  GIT_REMOTE_BASE="${scheme}://${GIT_HOST}"
  log "Git host: ${GIT_HOST} (project ${GIT_REPO_PATH})"
}

authenticated_clone_url() {
  # Splices the credentials into GIT_REMOTE_BASE rather than assuming https, so
  # a plain-http host keeps its scheme and still matches the credential helper
  # key that prepare_workspace registers under GIT_REMOTE_BASE.
  printf '%s://%s:%s@%s/%s' \
    "${GIT_REMOTE_BASE%%://*}" "$(git_credential_username)" "${GIT_TOKEN}" \
    "${GIT_HOST}" "${GIT_REPO_PATH}"
}

# glab reads GITLAB_TOKEN/GL_TOKEN and GITLAB_HOST straight from the
# environment, so no `glab auth login` and no writable config dir are needed.
setup_glab() {
  [ "${GIT_PROVIDER}" = "gitlab" ] || return 0

  if [ "${GIT_HOST}" != "gitlab.com" ]; then
    export GITLAB_HOST="${GIT_HOST}"
    export GITLAB_URI="${GIT_REMOTE_BASE}"
    log "glab targeting self-hosted ${GIT_REMOTE_BASE}"
  fi
  if ! glab auth status >/dev/null 2>&1; then
    log "WARNING: glab auth status reported a problem; merge request creation may fail"
  fi
}

prepare_workspace() {
  if [ -n "${REPO_URL:-}" ]; then
    resolve_git_host
    setup_glab
    WORK_DIR="/workspace/repo"
    rm -rf "${WORK_DIR}"
    if [ -n "${BASE_BRANCH:-}" ]; then
      log "Cloning ${REPO_URL} (branch ${BASE_BRANCH})"
      git clone --branch "${BASE_BRANCH}" --single-branch "$(authenticated_clone_url)" "${WORK_DIR}"
    else
      log "Cloning ${REPO_URL} (remote default branch)"
      git clone --single-branch "$(authenticated_clone_url)" "${WORK_DIR}"
    fi
    cd "${WORK_DIR}"
    if [ -z "${BASE_BRANCH:-}" ]; then
      BASE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
      if [ -z "${BASE_BRANCH}" ] || [ "${BASE_BRANCH}" = "HEAD" ]; then
        die "could not detect default branch after clone"
      fi
      log "Detected default BASE_BRANCH=${BASE_BRANCH}"
    fi
    git remote set-url origin "${REPO_URL}"
    # Keyed to the repository's own host, not a hardcoded github.com, so a
    # self-hosted GitLab or GitHub Enterprise push is authenticated too. The
    # username is provider-dependent and interpolated now; ${GIT_TOKEN} is
    # escaped so git's own shell expands it from the environment at push time.
    git config --local --replace-all "credential.${GIT_REMOTE_BASE}.helper" ""
    git config --local --add "credential.${GIT_REMOTE_BASE}.helper" \
      "!f() { echo \"username=$(git_credential_username)\"; echo \"password=\${GIT_TOKEN}\"; }; f"

    local ls_remote_status=0
    git ls-remote --exit-code --heads origin "${TARGET_BRANCH}" >/dev/null || ls_remote_status=$?
    case "${ls_remote_status}" in
      0)
        log "Reusing existing remote branch ${TARGET_BRANCH}"
        git fetch origin "${TARGET_BRANCH}:refs/remotes/origin/${TARGET_BRANCH}"
        git checkout -b "${TARGET_BRANCH}" "origin/${TARGET_BRANCH}"
        ;;
      2)
        log "Remote branch ${TARGET_BRANCH} does not exist yet; creating it from ${BASE_BRANCH}"
        git checkout -b "${TARGET_BRANCH}"
        ;;
      *)
        die "git ls-remote origin ${TARGET_BRANCH} failed with exit ${ls_remote_status}"
        ;;
    esac
  else
    WORK_DIR="/workspace/scratch"
    mkdir -p "${WORK_DIR}"
    cd "${WORK_DIR}"
    log "No REPO_URL; working in ${WORK_DIR} (MCP-only mode)"
  fi
}

resolve_system_prompt_file() {
  if [ -z "${SYSTEM_PROMPT_FILE:-}" ]; then
    return 0
  fi

  local resolved="${SYSTEM_PROMPT_FILE}"
  if [[ "${SYSTEM_PROMPT_FILE}" != /* ]]; then
    resolved="${WORK_DIR}/${SYSTEM_PROMPT_FILE}"
  fi

  if [ ! -f "${resolved}" ]; then
    die "SYSTEM_PROMPT_FILE not found: ${resolved}"
  fi

  SYSTEM_PROMPT_RESOLVED="${resolved}"
  log "System prompt file: ${SYSTEM_PROMPT_RESOLVED}"
}

resolve_mcp_raw() {
  MCP_CONFIG_RAW=""

  if [ -n "${MCP_CONFIG:-}" ]; then
    printf '%s' "${MCP_CONFIG}" > "${MCP_RESOLVED_FILE}.raw"
    MCP_CONFIG_RAW="${MCP_RESOLVED_FILE}.raw"
  elif [ -n "${MCP_CONFIG_FILE:-}" ]; then
    if [ ! -f "${MCP_CONFIG_FILE}" ]; then
      log "MCP_CONFIG_FILE not found (${MCP_CONFIG_FILE}); continuing without MCP"
      return 0
    fi
    cp "${MCP_CONFIG_FILE}" "${MCP_RESOLVED_FILE}.raw"
    MCP_CONFIG_RAW="${MCP_RESOLVED_FILE}.raw"
  fi
}

substitute_mcp_secrets() {
  if [ -z "${MCP_CONFIG_RAW:-}" ]; then
    return 0
  fi

  local required_vars=()
  if grep -q '\${MCP_GW_TOKEN}' "${MCP_CONFIG_RAW}"; then
    required_vars+=("MCP_GW_TOKEN")
  fi
  if grep -q '\${GITHUB_TOKEN}' "${MCP_CONFIG_RAW}"; then
    required_vars+=("GITHUB_TOKEN")
  fi

  local var
  for var in "${required_vars[@]}"; do
    if [ -z "${!var:-}" ]; then
      die "${var} is required by MCP config but is empty/unset"
    fi
    if [[ "${!var}" == vault:* ]]; then
      die "${var} still looks like a vault: reference; vault-env did not resolve it"
    fi
    local val="${!var}"
    log "${var} is set (len=${#val})"
  done

  if [ "${#required_vars[@]}" -gt 0 ]; then
    # shellcheck disable=SC2016
    envsubst "$(printf '${%s} ' "${required_vars[@]}")" \
      < "${MCP_CONFIG_RAW}" > "${MCP_RESOLVED_FILE}"
  else
    cp "${MCP_CONFIG_RAW}" "${MCP_RESOLVED_FILE}"
  fi

  if ! jq -e '.mcpServers' "${MCP_RESOLVED_FILE}" >/dev/null 2>&1; then
    die "MCP config must be JSON with a mcpServers object"
  fi

  jq -r '
    .mcpServers
    | to_entries[]
    | "MCP server \(.key): type=\(.value.type // "MISSING") url=\(.value.url // "MISSING")"
  ' "${MCP_RESOLVED_FILE}" | while IFS= read -r line; do
    log "${line}"
  done
}

prepare_mcp_config_claude() {
  MCP_CONFIG_RESOLVED=""

  resolve_mcp_raw
  if [ -z "${MCP_CONFIG_RAW:-}" ]; then
    return 0
  fi

  substitute_mcp_secrets
  MCP_CONFIG_RESOLVED="${MCP_RESOLVED_FILE}"
  log "MCP config prepared (${MCP_CONFIG_RESOLVED})"
}

export_codex_mcp_header_envs() {
  CODEX_MCP_HDR_MAP_FILE="/tmp/codex-mcp-hdr-map.json"
  echo "{}" > "${CODEX_MCP_HDR_MAP_FILE}"

  local server
  while IFS= read -r server; do
    [ -n "$server" ] || continue
    local header_lines
    header_lines="$(jq -r --arg s "$server" '
      (.mcpServers[$s].headers // {}) | to_entries[]
      | [.key, .value] | @tsv
    ' "${MCP_RESOLVED_FILE}" 2>/dev/null || true)"
    [ -n "$header_lines" ] || continue

    local hdr_idx=0
    while IFS=$'\t' read -r hname hval; do
      [ -n "$hname" ] || continue
      hdr_idx=$((hdr_idx + 1))
      local env_name="CODEX_MCP_HDR_${server}_${hdr_idx}"
      env_name="${env_name//[^A-Za-z0-9_]/_}"
      export "${env_name}=${hval}"
      jq --arg s "$server" --arg h "$hname" --arg e "$env_name" \
        '.[$s][$h] = $e' "${CODEX_MCP_HDR_MAP_FILE}" > "${CODEX_MCP_HDR_MAP_FILE}.tmp" \
        && mv "${CODEX_MCP_HDR_MAP_FILE}.tmp" "${CODEX_MCP_HDR_MAP_FILE}"
    done <<< "$header_lines"
  done < <(jq -r '.mcpServers | keys[]' "${MCP_RESOLVED_FILE}")
}

write_codex_mcp_servers_toml() {
  local require_mcp="${REQUIRE_MCP:-1}"
  local required_toml="false"
  if [ "$require_mcp" = "1" ] || [ "$require_mcp" = "true" ]; then
    required_toml="true"
  fi

  jq -r --arg required "$required_toml" --argjson hdr_map "$(cat "${CODEX_MCP_HDR_MAP_FILE}")" '
    def toml_str: @json;
    def emit_http_headers($server):
      ($hdr_map[$server] // {}) as $map
      | if ($map | length) == 0 then empty
        else "env_http_headers = { "
          + ([$map | to_entries[] | "\(.key) = \"\(.value)\""] | join(", "))
          + " }\n"
        end;
    def emit_env($env):
      if ($env | length) == 0 then empty
      else "env = { "
        + ([$env | to_entries[] | "\(.key) = \(.value | toml_str)"] | join(", "))
        + " }\n"
      end;
    def emit_args($args):
      if ($args | length) == 0 then empty
      else "args = " + ($args | toml_str) + "\n"
      end;
    .mcpServers | to_entries[] | .key as $name | .value as $srv |
    "[mcp_servers.\($name)]\n"
    + (if ($srv.url // "") != "" then
         "url = \($srv.url | toml_str)\n"
         + emit_http_headers($name)
       elif ($srv.command // "") != "" then
         "command = \($srv.command | toml_str)\n"
         + emit_args($srv.args // [])
         + emit_env($srv.env // {})
       else
         empty
       end)
    + "required = \($required)\n"
  ' "${MCP_RESOLVED_FILE}"
}

apply_codex_tool_filters() {
  local allowed="${ALLOWED_TOOLS:-}"
  local disallowed="${DISALLOWED_TOOLS:-}"
  local has_mcp_allow=0
  local logged_builtin=0

  declare -A MCP_ALLOW_ALL=()
  declare -A MCP_ENABLED=()
  declare -A MCP_DISABLED=()
  declare -A MCP_MENTIONED=()

  _parse_tool_list() {
    local list="$1"
    local mode="$2"
    local tool rest server mcp_tool

    IFS=',' read -ra _tools <<< "$list"
    for tool in "${_tools[@]}"; do
      tool="${tool#"${tool%%[![:space:]]*}"}"
      tool="${tool%"${tool##*[![:space:]]}"}"
      [ -n "$tool" ] || continue

      if [[ "$tool" != mcp__* ]]; then
        if [ "$logged_builtin" -eq 0 ] && [ "$mode" = "allow" ]; then
          log "Codex ignores Claude builtin tool names in ALLOWED_TOOLS/DISALLOWED_TOOLS; sandbox mode governs file/shell access"
          logged_builtin=1
        fi
        continue
      fi

      rest="${tool#mcp__}"
      if [[ "$rest" != *"__"* ]]; then
        log "Skipping malformed MCP tool entry: ${tool}"
        continue
      fi

      server="${rest%%__*}"
      mcp_tool="${rest#*__}"
      MCP_MENTIONED["$server"]=1

      if [ "$mode" = "allow" ]; then
        has_mcp_allow=1
        if [ "$mcp_tool" = "*" ]; then
          MCP_ALLOW_ALL["$server"]=1
        else
          if [ -n "${MCP_ENABLED[$server]:-}" ]; then
            MCP_ENABLED["$server"]+=",${mcp_tool}"
          else
            MCP_ENABLED["$server"]="${mcp_tool}"
          fi
        fi
      else
        if [ -n "${MCP_DISABLED[$server]:-}" ]; then
          MCP_DISABLED["$server"]+=",${mcp_tool}"
        else
          MCP_DISABLED["$server"]="${mcp_tool}"
        fi
      fi
    done
  }

  if [ -n "$allowed" ]; then
    _parse_tool_list "$allowed" "allow"
  fi
  if [ -n "$disallowed" ]; then
    _parse_tool_list "$disallowed" "deny"
  fi

  local servers
  servers="$(jq -r '.mcpServers | keys[]' "${MCP_RESOLVED_FILE}" 2>/dev/null || true)"
  [ -n "$servers" ] || return 0

  while IFS= read -r server; do
    [ -n "$server" ] || continue

    {
      printf '\n[mcp_servers.%s]\n' "$server"

      if [ "$has_mcp_allow" -eq 1 ] && [ -z "${MCP_MENTIONED[$server]:-}" ]; then
        printf 'enabled = false\n'
      fi

      if [ -n "${MCP_ENABLED[$server]:-}" ] && [ -z "${MCP_ALLOW_ALL[$server]:-}" ]; then
        printf 'enabled_tools = ['
        local first=1
        local t
        IFS=',' read -ra _enabled <<< "${MCP_ENABLED[$server]}"
        for t in "${_enabled[@]}"; do
          t="${t#"${t%%[![:space:]]*}"}"
          t="${t%"${t##*[![:space:]]}"}"
          [ -n "$t" ] || continue
          if [ "$first" -eq 1 ]; then first=0; else printf ', '; fi
          printf '%s' "$(toml_quote "$t")"
        done
        printf ']\n'
      fi

      if [ -n "${MCP_DISABLED[$server]:-}" ]; then
        printf 'disabled_tools = ['
        first=1
        IFS=',' read -ra _disabled <<< "${MCP_DISABLED[$server]}"
        for t in "${_disabled[@]}"; do
          t="${t#"${t%%[![:space:]]*}"}"
          t="${t%"${t##*[![:space:]]}"}"
          [ -n "$t" ] || continue
          if [ "$first" -eq 1 ]; then first=0; else printf ', '; fi
          printf '%s' "$(toml_quote "$t")"
        done
        printf ']\n'
      fi
    } >> "${CODEX_CONFIG_FILE}"
  done <<< "$servers"
}

append_codex_extra_config() {
  if [ -n "${CODEX_CONFIG_TOML:-}" ]; then
    printf '\n%s\n' "${CODEX_CONFIG_TOML}" >> "${CODEX_CONFIG_FILE}"
  fi
  if [ -n "${CODEX_CONFIG_FILE_PATH:-}" ] && [ -f "${CODEX_CONFIG_FILE_PATH}" ]; then
    cat "${CODEX_CONFIG_FILE_PATH}" >> "${CODEX_CONFIG_FILE}"
  fi
}

prepare_mcp_config_codex() {
  resolve_mcp_raw
  if [ -z "${MCP_CONFIG_RAW:-}" ]; then
    append_codex_extra_config
    log "Codex config: ${CODEX_CONFIG_FILE}"
    return 0
  fi

  substitute_mcp_secrets
  export_codex_mcp_header_envs
  write_codex_mcp_servers_toml >> "${CODEX_CONFIG_FILE}"
  apply_codex_tool_filters
  append_codex_extra_config
  log "Codex MCP config written to ${CODEX_CONFIG_FILE}"
}

prepare_mcp_config() {
  if is_codex_agent; then
    prepare_mcp_config_codex
  else
    prepare_mcp_config_claude
  fi
}

build_claude_args() {
  CLAUDE_ARGS=(
    -p "${PROMPT}"
    --output-format stream-json
    --verbose
  )

  if [ -n "${SYSTEM_PROMPT_RESOLVED:-}" ]; then
    case "${SYSTEM_PROMPT_MODE}" in
      replace)
        CLAUDE_ARGS+=(--system-prompt-file "${SYSTEM_PROMPT_RESOLVED}")
        ;;
      append|*)
        CLAUDE_ARGS+=(--append-system-prompt-file "${SYSTEM_PROMPT_RESOLVED}")
        ;;
    esac
  fi

  if [ -n "${MCP_CONFIG_RESOLVED:-}" ]; then
    CLAUDE_ARGS+=(--mcp-config "${MCP_CONFIG_RESOLVED}" --strict-mcp-config)
    if [ "${DEBUG_MCP:-}" = "1" ] || [ "${DEBUG_MCP:-}" = "true" ]; then
      CLAUDE_ARGS+=(--debug "mcp")
    fi
  fi

  if [ -n "${ALLOWED_TOOLS:-}" ]; then
    CLAUDE_ARGS+=(--allowedTools)
    local tool
    IFS=',' read -ra _allowed_tools <<< "${ALLOWED_TOOLS}"
    for tool in "${_allowed_tools[@]}"; do
      tool="${tool#"${tool%%[![:space:]]*}"}"
      tool="${tool%"${tool##*[![:space:]]}"}"
      if [ -n "${tool}" ]; then
        CLAUDE_ARGS+=("${tool}")
      fi
    done
  fi

  if [ -n "${DISALLOWED_TOOLS:-}" ]; then
    CLAUDE_ARGS+=(--disallowedTools)
    local tool
    IFS=',' read -ra _disallowed_tools <<< "${DISALLOWED_TOOLS}"
    for tool in "${_disallowed_tools[@]}"; do
      tool="${tool#"${tool%%[![:space:]]*}"}"
      tool="${tool%"${tool##*[![:space:]]}"}"
      if [ -n "${tool}" ]; then
        CLAUDE_ARGS+=("${tool}")
      fi
    done
  fi

  if [ -n "${PERMISSION_MODE:-}" ]; then
    CLAUDE_ARGS+=(--permission-mode "${PERMISSION_MODE}")
  fi
}

build_codex_sandbox_args() {
  CODEX_ARGS=()

  if [ -n "${CODEX_SANDBOX_MODE:-}" ]; then
    case "${CODEX_SANDBOX_MODE}" in
      bypass|yolo|danger-full-access)
        CODEX_ARGS+=(--dangerously-bypass-approvals-and-sandbox)
        ;;
      *)
        CODEX_ARGS+=(-s "${CODEX_SANDBOX_MODE}")
        ;;
    esac
    return
  fi

  case "${PERMISSION_MODE:-acceptEdits}" in
    plan)
      CODEX_ARGS+=(-s read-only)
      ;;
    bypassPermissions)
      CODEX_ARGS+=(--dangerously-bypass-approvals-and-sandbox)
      ;;
    acceptEdits|default|*)
      CODEX_ARGS+=(-s workspace-write)
      if [ "${CODEX_NETWORK_ACCESS:-1}" != "0" ] && [ "${CODEX_NETWORK_ACCESS:-}" != "false" ]; then
        CODEX_ARGS+=(-c 'sandbox_workspace_write.network_access=true')
      fi
      ;;
  esac
}

build_codex_args() {
  build_codex_sandbox_args

  CODEX_ARGS+=(--json -o "${CODEX_LAST_MESSAGE_FILE}")
  CODEX_ARGS+=(--skip-git-repo-check --cd "${WORK_DIR}")

  if [ -n "${OPENAI_MODEL:-}" ]; then
    CODEX_ARGS+=(-m "${OPENAI_MODEL}")
  fi

  if [ -n "${CODEX_EXTRA_ARGS:-}" ]; then
    # shellcheck disable=SC2206
    local _extra=(${CODEX_EXTRA_ARGS})
    CODEX_ARGS+=("${_extra[@]}")
  fi
}

build_codex_prompt() {
  local prompt="${PROMPT}"
  if [ -n "${SYSTEM_PROMPT_RESOLVED:-}" ]; then
    case "${SYSTEM_PROMPT_MODE}" in
      replace)
        prompt="$(cat "${SYSTEM_PROMPT_RESOLVED}")"
        ;;
      append|*)
        prompt="$(cat "${SYSTEM_PROMPT_RESOLVED}")${prompt}"
        ;;
    esac
  fi
  printf '%s' "$prompt"
}

check_claude_mcp_init_status() {
  if [ ! -s "${LOG_FILE}" ] || [ -z "${MCP_CONFIG_RESOLVED:-}" ]; then
    return 0
  fi

  local init_line
  init_line="$(jq -c 'select(.type == "system" and .subtype == "init")' "${LOG_FILE}" 2>/dev/null | head -n1 || true)"
  if [ -z "${init_line}" ]; then
    return 0
  fi

  local errors connected failed_count total
  errors="$(jq -r '
    [
      (.mcp_servers // [] | map(select(.status == "failed" or .status == "needs-auth") | "\(.name)=\(.status)")),
      (.mcp_server_errors // [] | map("\(.name): \(.type // "error") — \(.message // "")"))
    ]
    | add
    | unique
    | .[]
  ' <<< "${init_line}" 2>/dev/null || true)"
  connected="$(jq -r '[.mcp_servers // [] | .[] | select(.status == "connected")] | length' <<< "${init_line}" 2>/dev/null || echo 0)"
  failed_count="$(jq -r '[.mcp_servers // [] | .[] | select(.status == "failed" or .status == "needs-auth")] | length' <<< "${init_line}" 2>/dev/null || echo 0)"
  total="$(jq -r '[.mcp_servers // [] | .[]] | length' <<< "${init_line}" 2>/dev/null || echo 0)"

  if [ -n "${errors}" ]; then
    log "MCP server issues detected (connected=${connected}/${total}):"
    printf '%s\n' "${errors}" | while IFS= read -r line; do
      log "  ${line}"
    done
  fi

  if [ "${total}" -gt 0 ] && [ "${connected}" -eq 0 ]; then
    if [ "${REQUIRE_MCP:-1}" = "1" ] || [ "${REQUIRE_MCP:-}" = "true" ]; then
      AGENT_EXIT_CODE=1
      die "no MCP servers connected (failed=${failed_count}/${total}; set REQUIRE_MCP=0 to ignore)"
    fi
  fi
}

check_codex_mcp_init_status() {
  if [ ! -s "${LOG_FILE}" ] || [ -z "${MCP_CONFIG_RAW:-}" ]; then
    return 0
  fi

  local mcp_errors
  mcp_errors="$(jq -r '
    select(.type == "turn.failed" or (.type == "item.completed" and .item.type == "error"))
    | .error.message // .item.message // empty
  ' "${LOG_FILE}" 2>/dev/null | grep -i mcp || true)"

  if [ -n "${mcp_errors}" ]; then
    log "Codex MCP errors detected:"
    printf '%s\n' "${mcp_errors}" | while IFS= read -r line; do
      log "  ${line}"
    done
  fi
}

check_mcp_init_status() {
  if is_codex_agent; then
    check_codex_mcp_init_status
  else
    check_claude_mcp_init_status
  fi
}

parse_claude_log() {
  if [ ! -s "${LOG_FILE}" ]; then
    AGENT_RESULT=""
    return
  fi

  local result_line
  result_line="$(jq -s 'map(select(.type == "result")) | last // empty' "${LOG_FILE}" 2>/dev/null || true)"
  if [ -z "${result_line}" ]; then
    AGENT_RESULT="$(tail -n 50 "${LOG_FILE}")"
    return
  fi

  AGENT_RESULT="$(jq -r '.result // ""' <<< "${result_line}")"
  AGENT_IS_ERROR="$(jq -r '.is_error // false' <<< "${result_line}")"
  AGENT_NUM_TURNS="$(jq -r '.num_turns // 0' <<< "${result_line}")"
  AGENT_TOTAL_COST_USD="$(jq -r '.total_cost_usd // 0' <<< "${result_line}")"
  AGENT_DURATION_MS="$(jq -r '.duration_ms // 0' <<< "${result_line}")"
  AGENT_SESSION_ID="$(jq -r '.session_id // ""' <<< "${result_line}")"

  normalize_usage_numbers
}

parse_codex_log() {
  AGENT_RESULT=""
  CODEX_INPUT_TOKENS=0
  CODEX_OUTPUT_TOKENS=0

  if [ -f "${CODEX_LAST_MESSAGE_FILE}" ] && [ -s "${CODEX_LAST_MESSAGE_FILE}" ]; then
    AGENT_RESULT="$(cat "${CODEX_LAST_MESSAGE_FILE}")"
  elif [ -s "${LOG_FILE}" ]; then
    AGENT_RESULT="$(jq -rs '
      map(select(.type == "item.completed" and .item.type == "agent_message"))
      | last
      | .item.text // ""
    ' "${LOG_FILE}" 2>/dev/null || true)"
  fi

  if [ -s "${LOG_FILE}" ]; then
    AGENT_SESSION_ID="$(jq -r 'select(.type == "thread.started") | .thread_id' "${LOG_FILE}" 2>/dev/null | head -n1 || true)"
    if jq -e 'select(.type == "turn.failed")' "${LOG_FILE}" >/dev/null 2>&1; then
      AGENT_IS_ERROR=true
    else
      AGENT_IS_ERROR=false
    fi
    AGENT_NUM_TURNS="$(jq -sc '[.[] | select(.type == "turn.completed")] | length' "${LOG_FILE}" 2>/dev/null || echo 0)"
    CODEX_INPUT_TOKENS="$(jq -sc '[.[] | select(.type == "turn.completed") | (.usage.input_tokens // 0)] | add // 0' "${LOG_FILE}" 2>/dev/null || echo 0)"
    CODEX_OUTPUT_TOKENS="$(jq -sc '[.[] | select(.type == "turn.completed") | (.usage.output_tokens // 0)] | add // 0' "${LOG_FILE}" 2>/dev/null || echo 0)"
  fi

  AGENT_TOTAL_COST_USD=0
  if [ "${AGENT_START_MS}" -gt 0 ]; then
    local now_ms
    now_ms="$(date +%s%3N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1000))')"
    AGENT_DURATION_MS=$((now_ms - AGENT_START_MS))
  fi

  normalize_usage_numbers
}

parse_agent_log() {
  if is_codex_agent; then
    parse_codex_log
  else
    parse_claude_log
  fi
}

normalize_usage_numbers() {
  case "${AGENT_NUM_TURNS}" in
    ''|*[!0-9]*) AGENT_NUM_TURNS=0 ;;
  esac
  case "${AGENT_DURATION_MS}" in
    ''|*[!0-9]*) AGENT_DURATION_MS=0 ;;
  esac
  case "${CODEX_INPUT_TOKENS}" in
    ''|*[!0-9]*) CODEX_INPUT_TOKENS=0 ;;
  esac
  case "${CODEX_OUTPUT_TOKENS}" in
    ''|*[!0-9]*) CODEX_OUTPUT_TOKENS=0 ;;
  esac
  if ! printf '%s' "${AGENT_TOTAL_COST_USD}" | grep -Eq '^-?[0-9]+(\.[0-9]+)?$'; then
    AGENT_TOTAL_COST_USD=0
  fi
  case "${REPO_PUSHED}" in
    true|false) ;;
    *) REPO_PUSHED=false ;;
  esac
}

run_claude_agent() {
  build_claude_args
  : > "${LOG_FILE}"

  log "Starting Claude Code"
  set +e
  if [ -n "${TIMEOUT_SECONDS:-}" ]; then
    timeout "${TIMEOUT_SECONDS}" claude "${CLAUDE_ARGS[@]}" 2>&1 | tee "${LOG_FILE}"
    AGENT_EXIT_CODE=$?
  else
    claude "${CLAUDE_ARGS[@]}" 2>&1 | tee "${LOG_FILE}"
    AGENT_EXIT_CODE=$?
  fi
  set -e
}

run_codex_agent() {
  build_codex_args
  : > "${LOG_FILE}"
  : > "${CODEX_LAST_MESSAGE_FILE}"

  local codex_prompt
  codex_prompt="$(build_codex_prompt)"

  log "Starting Codex"
  AGENT_START_MS="$(date +%s%3N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1000))')"

  set +e
  if [ -n "${TIMEOUT_SECONDS:-}" ]; then
    printf '%s' "$codex_prompt" | timeout "${TIMEOUT_SECONDS}" codex exec "${CODEX_ARGS[@]}" - 2>&1 | tee "${LOG_FILE}"
    AGENT_EXIT_CODE=$?
  else
    printf '%s' "$codex_prompt" | codex exec "${CODEX_ARGS[@]}" - 2>&1 | tee "${LOG_FILE}"
    AGENT_EXIT_CODE=$?
  fi
  set -e
}

run_agent() {
  if is_codex_agent; then
    run_codex_agent
  else
    run_claude_agent
  fi

  parse_agent_log
  check_mcp_init_status

  if [ "${AGENT_EXIT_CODE}" -eq 0 ] && [ "${AGENT_IS_ERROR}" = "true" ]; then
    AGENT_EXIT_CODE=1
  fi

  if [ "${AGENT_EXIT_CODE}" -eq 124 ]; then
    log "Agent timed out after ${TIMEOUT_SECONDS}s"
  elif [ "${AGENT_EXIT_CODE}" -ne 0 ]; then
    log "Agent exited with code ${AGENT_EXIT_CODE}"
  else
    log "Agent finished successfully"
  fi
}

git_commit_push_pr() {
  if [ -z "${REPO_URL:-}" ]; then
    return 0
  fi

  if [ "${AGENT_EXIT_CODE}" -ne 0 ]; then
    log "Skipping git/PR steps because agent failed"
    return 0
  fi

  cd "${WORK_DIR}"

  if [ -z "$(git status --porcelain)" ]; then
    log "No file changes to commit"
    return 0
  fi

  local commit_msg pr_title pr_body
  pr_title="${PR_TITLE:-$(printf '%s' "${PROMPT}" | head -n1)}"
  pr_body="${PR_BODY:-${PROMPT}}"
  commit_msg="${pr_title}"

  git add -A
  git commit -m "${commit_msg}"

  log "Pushing branch ${TARGET_BRANCH}"
  git push -u origin "${TARGET_BRANCH}"
  REPO_PUSHED=true

  open_change_request "${pr_title}" "${pr_body}"
}

# PR_URL keeps its name whichever provider opened the request: it is the
# repo.pr_url field of the webhook payload, which the plan panel links from.
open_change_request() {
  if [ "${GIT_PROVIDER}" = "gitlab" ]; then
    open_merge_request "$1" "$2"
  else
    open_pull_request "$1" "$2"
  fi
}

open_pull_request() {
  local pr_title="$1" pr_body="$2"

  PR_URL="$(gh pr list --head "${TARGET_BRANCH}" --state all --json url --jq '.[0].url // empty' 2>/dev/null || true)"
  if [ -n "${PR_URL}" ]; then
    log "PR already exists: ${PR_URL}"
    return 0
  fi

  log "Creating PR (${TARGET_BRANCH} -> ${BASE_BRANCH})"
  PR_URL="$(gh pr create \
    --base "${BASE_BRANCH}" \
    --head "${TARGET_BRANCH}" \
    --title "${pr_title}" \
    --body "${pr_body}")"
  log "PR created: ${PR_URL}"
}

# glab flag spellings have moved between majors, so the URL is taken from the
# command output first and only then re-queried; losing it would cost the plan
# panel its link even though the MR exists.
glab_existing_mr_url() {
  glab mr list --source-branch "${TARGET_BRANCH}" --all --output json 2>/dev/null \
    | jq -r 'if type == "array" then (.[0].web_url // empty) else empty end' 2>/dev/null \
    || true
}

open_merge_request() {
  local mr_title="$1" mr_body="$2"
  local out="/tmp/glab-mr.out"

  PR_URL="$(glab_existing_mr_url)"
  if [ -n "${PR_URL}" ]; then
    log "MR already exists: ${PR_URL}"
    return 0
  fi

  log "Creating MR (${TARGET_BRANCH} -> ${BASE_BRANCH})"
  if ! glab mr create \
    --source-branch "${TARGET_BRANCH}" \
    --target-branch "${BASE_BRANCH}" \
    --title "${mr_title}" \
    --description "${mr_body}" \
    --yes >"${out}" 2>&1; then
    cat "${out}" >&2
    die "glab mr create failed"
  fi
  cat "${out}" >&2

  PR_URL="$(grep -Eo 'https?://[^[:space:]]+/-/merge_requests/[0-9]+' "${out}" | head -n1 || true)"
  if [ -z "${PR_URL}" ]; then
    PR_URL="$(glab_existing_mr_url)"
  fi
  if [ -n "${PR_URL}" ]; then
    log "MR created: ${PR_URL}"
  else
    log "MR created, but its URL could not be captured"
  fi
}

prepare_webhook_payload_files() {
  printf '%s' "${AGENT_RESULT}" \
    | head -c "${WEBHOOK_RESULT_MAX_BYTES}" > "${WEBHOOK_RESULT_FILE}" || true

  if [ -s "${LOG_FILE}" ]; then
    tail -n 200 "${LOG_FILE}" \
      | tail -c "${WEBHOOK_LOG_TAIL_MAX_BYTES}" > "${WEBHOOK_LOG_TAIL_FILE}" || true
  else
    : > "${WEBHOOK_LOG_TAIL_FILE}"
  fi

  [ -f "${WEBHOOK_RESULT_FILE}" ] || : > "${WEBHOOK_RESULT_FILE}"
  [ -f "${WEBHOOK_LOG_TAIL_FILE}" ] || : > "${WEBHOOK_LOG_TAIL_FILE}"
}

send_webhook() {
  if [ "${WEBHOOK_SENT}" -eq 1 ]; then
    return 0
  fi
  WEBHOOK_SENT=1

  if [ -z "${WEBHOOK_URL:-}" ]; then
    log "WEBHOOK_URL not set; skipping webhook"
    return 0
  fi

  local status payload curl_args=()
  if [ "${AGENT_EXIT_CODE}" -eq 0 ]; then
    status="success"
  else
    status="failed"
  fi

  normalize_usage_numbers
  prepare_webhook_payload_files

  if ! payload="$(jq -n \
    --arg status "${status}" \
    --argjson exit_code "${AGENT_EXIT_CODE}" \
    --arg agent_type "${AGENT_TYPE}" \
    --rawfile result "${WEBHOOK_RESULT_FILE}" \
    --rawfile log_tail "${WEBHOOK_LOG_TAIL_FILE}" \
    --arg job_name "${JOB_NAME:-}" \
    --arg pod_name "${POD_NAME:-${HOSTNAME:-}}" \
    --arg pod_namespace "${POD_NAMESPACE:-}" \
    --arg image_version "${IMAGE_VERSION:-}" \
    --arg repo_url "${REPO_URL:-}" \
    --arg base_branch "${BASE_BRANCH:-}" \
    --arg target_branch "${TARGET_BRANCH:-}" \
    --argjson repo_pushed "${REPO_PUSHED}" \
    --arg pr_url "${PR_URL}" \
    --argjson duration_ms "${AGENT_DURATION_MS}" \
    --argjson num_turns "${AGENT_NUM_TURNS}" \
    --argjson total_cost_usd "${AGENT_TOTAL_COST_USD}" \
    --argjson input_tokens "${CODEX_INPUT_TOKENS}" \
    --argjson output_tokens "${CODEX_OUTPUT_TOKENS}" \
    --arg session_id "${AGENT_SESSION_ID}" \
    --arg thread_root_id "${THREAD_ROOT_ID:-}" \
    --arg task_id "${TASK_ID:-}" \
    --arg chat_id "${CHAT_ID:-}" \
    '{
      status: $status,
      exit_code: $exit_code,
      agent_type: $agent_type,
      result: $result,
      log_tail: $log_tail,
      task_id: $task_id,
      thread_root_id: $thread_root_id,
      chat_id: $chat_id,
      job: {
        name: $job_name,
        pod: $pod_name,
        namespace: $pod_namespace,
        image_version: $image_version
      },
      repo: {
        url: $repo_url,
        base_branch: $base_branch,
        target_branch: $target_branch,
        pushed: $repo_pushed,
        pr_url: $pr_url
      },
      usage: {
        duration_ms: $duration_ms,
        num_turns: $num_turns,
        total_cost_usd: $total_cost_usd,
        input_tokens: $input_tokens,
        output_tokens: $output_tokens,
        session_id: $session_id
      }
    }')"; then
    log "Failed to build the full webhook payload; falling back to a minimal one"
    payload="$(jq -n \
      --arg status "failed" \
      --argjson exit_code "${AGENT_EXIT_CODE}" \
      --arg agent_type "${AGENT_TYPE}" \
      --arg result "webhook payload could not be built inside the agent container; see Job logs" \
      --arg job_name "${JOB_NAME:-}" \
      --arg pod_name "${POD_NAME:-${HOSTNAME:-}}" \
      --arg pod_namespace "${POD_NAMESPACE:-}" \
      --arg pr_url "${PR_URL}" \
      --arg task_id "${TASK_ID:-}" \
      --arg thread_root_id "${THREAD_ROOT_ID:-}" \
      --arg chat_id "${CHAT_ID:-}" \
      '{
        status: $status,
        exit_code: $exit_code,
        agent_type: $agent_type,
        result: $result,
        log_tail: "",
        task_id: $task_id,
        thread_root_id: $thread_root_id,
        chat_id: $chat_id,
        job: {
          name: $job_name,
          pod: $pod_name,
          namespace: $pod_namespace
        },
        repo: { pr_url: $pr_url }
      }')"
  fi

  curl_args=(
    --fail-with-body
    --silent
    --show-error
    --retry 3
    --retry-all-errors
    --max-time 30
    -X POST
    -H "Content-Type: application/json"
    --data-binary @-
  )

  if [ -n "${WEBHOOK_AUTH_HEADER:-}" ]; then
    curl_args+=(-H "${WEBHOOK_AUTH_HEADER}")
  fi

  log "Sending webhook to ${WEBHOOK_URL} (payload ${#payload} bytes)"
  if ! printf '%s' "${payload}" | curl "${curl_args[@]}" "${WEBHOOK_URL}"; then
    log "Webhook delivery failed after retries"
    WEBHOOK_FAILED=1
  else
    log "Webhook delivered"
  fi
}

trap send_webhook EXIT

validate_env
setup_auth
setup_git_identity
prepare_workspace
resolve_system_prompt_file
prepare_mcp_config
run_agent
git_commit_push_pr

if [ "${WEBHOOK_FAILED}" -eq 1 ] && [ "${AGENT_EXIT_CODE}" -eq 0 ]; then
  exit 1
fi

exit "${AGENT_EXIT_CODE}"
