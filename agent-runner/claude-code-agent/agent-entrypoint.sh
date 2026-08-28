#!/usr/bin/env bash
# Headless Claude Code agent entrypoint for k8s Jobs.
set -euo pipefail

AGENT_EXIT_CODE=0
WEBHOOK_FAILED=0
WEBHOOK_SENT=0

AGENT_RESULT=""
AGENT_IS_ERROR=false
AGENT_NUM_TURNS=0
AGENT_TOTAL_COST_USD=0
AGENT_DURATION_MS=0
AGENT_SESSION_ID=""

REPO_PUSHED=false
PR_URL=""

WORK_DIR=""
LOG_FILE="/tmp/claude-agent.log"
MCP_RESOLVED_FILE="/tmp/mcp-config.json"

# jq receives large strings through files, not through --arg: a single
# command-line argument is capped at MAX_ARG_STRLEN (32 pages = 128 KiB on
# Linux), and one stream-json line carrying a tool_result easily exceeds it.
# That cap is what "Argument list too long" means when jq is invoked with a
# large --arg value.
WEBHOOK_RESULT_FILE="/tmp/webhook-result.txt"
WEBHOOK_LOG_TAIL_FILE="/tmp/webhook-log-tail.txt"

# Byte caps are a separate concern from MAX_ARG_STRLEN: the receiving workflow
# posts these into Mattermost, which has its own message size limit.
: "${WEBHOOK_RESULT_MAX_BYTES:=16384}"
: "${WEBHOOK_LOG_TAIL_MAX_BYTES:=16384}"

log() {
  printf '[agent] %s\n' "$*" >&2
}

die() {
  log "ERROR: $*"
  AGENT_EXIT_CODE=1
  exit 1
}

# Auth modes (prefer OAuth when present):
# 1) CLAUDE_CODE_OAUTH_TOKEN — Claude.ai subscription (Pro/Max/Team/Enterprise).
#    Requires api.anthropic.com; image OpenRouter defaults must be cleared because
#    ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY take precedence over the OAuth token.
# 2) ANTHROPIC_API_KEY (+ ANTHROPIC_BASE_URL) — OpenRouter / custom gateway via Bearer.
setup_auth() {
  if [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
    log "Auth: CLAUDE_CODE_OAUTH_TOKEN (Anthropic subscription)"
    unset ANTHROPIC_BASE_URL
    unset ANTHROPIC_AUTH_TOKEN
    unset ANTHROPIC_API_KEY
    # Dockerfile default model is OpenRouter-style (provider/model). Clear any
    # slash-form id so Claude Code picks a subscription-compatible model.
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

setup_git_identity() {
  : "${GIT_AUTHOR_NAME:=-}"
  : "${GIT_AUTHOR_EMAIL:=-}"
  export GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL
  export GIT_COMMITTER_NAME="${GIT_AUTHOR_NAME}"
  export GIT_COMMITTER_EMAIL="${GIT_AUTHOR_EMAIL}"
  # There is no tty in a Job pod: fail fast with a real error instead of git
  # trying to prompt for a username.
  export GIT_TERMINAL_PROMPT=0
}

validate_env() {
  if [ -z "${PROMPT:-}" ]; then
    die "PROMPT is required"
  fi

  if [ -n "${REPO_URL:-}" ]; then
    if [ -z "${TARGET_BRANCH:-}" ]; then
      die "TARGET_BRANCH is required when REPO_URL is set"
    fi
    if [ -z "${GITHUB_TOKEN:-}" ]; then
      die "GITHUB_TOKEN is required when REPO_URL is set"
    fi
    # The credential helper set up in prepare_workspace reads GITHUB_TOKEN from
    # the environment at push time, so it has to be exported.
    export GITHUB_TOKEN
    export GH_TOKEN="${GITHUB_TOKEN}"
  fi

  : "${SYSTEM_PROMPT_MODE:=append}"
}

github_clone_url() {
  local url="${REPO_URL%.git}"
  case "$url" in
    https://github.com/*)
      echo "https://x-access-token:${GITHUB_TOKEN}@github.com/${url#https://github.com/}"
      ;;
    http://github.com/*)
      echo "https://x-access-token:${GITHUB_TOKEN}@github.com/${url#http://github.com/}"
      ;;
    github.com/*)
      echo "https://x-access-token:${GITHUB_TOKEN}@github.com/${url#github.com/}"
      ;;
    *)
      echo "$url"
      ;;
  esac
}

prepare_workspace() {
  if [ -n "${REPO_URL:-}" ]; then
    WORK_DIR="/workspace/repo"
    rm -rf "${WORK_DIR}"
    if [ -n "${BASE_BRANCH:-}" ]; then
      log "Cloning ${REPO_URL} (branch ${BASE_BRANCH})"
      git clone --branch "${BASE_BRANCH}" --single-branch "$(github_clone_url)" "${WORK_DIR}"
    else
      # No BASE_BRANCH: clone the remote default branch, then use it for work + PR base.
      log "Cloning ${REPO_URL} (remote default branch)"
      git clone --single-branch "$(github_clone_url)" "${WORK_DIR}"
    fi
    cd "${WORK_DIR}"
    if [ -z "${BASE_BRANCH:-}" ]; then
      BASE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
      if [ -z "${BASE_BRANCH}" ] || [ "${BASE_BRANCH}" = "HEAD" ]; then
        die "could not detect default branch after clone"
      fi
      log "Detected default BASE_BRANCH=${BASE_BRANCH}"
    fi
    # Keep the token out of .git/config, but keep fetch/push authenticated: the
    # remote goes back to the plain URL and a credential helper supplies the
    # token from the environment on demand. Without this, push has no creds, no
    # askpass and no tty, and dies with "could not read Username".
    git remote set-url origin "${REPO_URL}"
    # The empty value first resets any inherited helper list: git queries every
    # configured helper in order and takes the first answer, so without the
    # reset a system/global helper could shadow this one.
    git config --local --replace-all credential.https://github.com.helper ""
    git config --local --add credential.https://github.com.helper \
      '!f() { echo "username=x-access-token"; echo "password=${GITHUB_TOKEN}"; }; f'

    # ls-remote separates "branch does not exist" (exit 2) from real failures
    # such as a bad token, which the previous `2>/dev/null` swallowed — that
    # turned an auth error into a silently recreated branch and a rejected push.
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

prepare_mcp_config() {
  MCP_CONFIG_RESOLVED=""

  if [ -n "${MCP_CONFIG:-}" ]; then
    printf '%s' "${MCP_CONFIG}" > "${MCP_RESOLVED_FILE}.raw"
  elif [ -n "${MCP_CONFIG_FILE:-}" ]; then
    if [ ! -f "${MCP_CONFIG_FILE}" ]; then
      log "MCP_CONFIG_FILE not found (${MCP_CONFIG_FILE}); continuing without MCP"
      return 0
    fi
    cp "${MCP_CONFIG_FILE}" "${MCP_RESOLVED_FILE}.raw"
  else
    return 0
  fi

  # Only substitute known MCP secrets so unrelated shell env cannot corrupt JSON.
  # Claude Code also expands ${VAR} itself; we expand here so the resolved file is
  # inspectable in the Job and missing tokens fail before the agent starts.
  local required_vars=()
  if grep -q '\${MCP_GW_TOKEN}' "${MCP_RESOLVED_FILE}.raw"; then
    required_vars+=("MCP_GW_TOKEN")
  fi
  if grep -q '\${GITHUB_TOKEN}' "${MCP_RESOLVED_FILE}.raw"; then
    required_vars+=("GITHUB_TOKEN")
  fi

  local var
  for var in "${required_vars[@]}"; do
    if [ -z "${!var:-}" ]; then
      die "${var} is required by MCP config but is empty/unset"
    fi
    # Reject unresolved vault-env placeholders.
    if [[ "${!var}" == vault:* ]]; then
      die "${var} still looks like a vault: reference; vault-env did not resolve it"
    fi
    local val="${!var}"
    log "${var} is set (len=${#val})"
  done

  if [ "${#required_vars[@]}" -gt 0 ]; then
    # shellcheck disable=SC2016
    envsubst "$(printf '${%s} ' "${required_vars[@]}")" \
      < "${MCP_RESOLVED_FILE}.raw" > "${MCP_RESOLVED_FILE}"
  else
    cp "${MCP_RESOLVED_FILE}.raw" "${MCP_RESOLVED_FILE}"
  fi

  if ! jq -e '.mcpServers' "${MCP_RESOLVED_FILE}" >/dev/null 2>&1; then
    die "MCP config must be JSON with a mcpServers object"
  fi

  # Log server names/urls without dumping secrets from headers.
  jq -r '
    .mcpServers
    | to_entries[]
    | "MCP server \(.key): type=\(.value.type // "MISSING") url=\(.value.url // "MISSING")"
  ' "${MCP_RESOLVED_FILE}" | while IFS= read -r line; do
    log "${line}"
  done

  MCP_CONFIG_RESOLVED="${MCP_RESOLVED_FILE}"
  log "MCP config prepared (${MCP_CONFIG_RESOLVED})"
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

check_mcp_init_status() {
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

parse_agent_log() {
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

normalize_usage_numbers() {
  # jq --argjson rejects anything that is not valid JSON. A truncated log, a
  # missing result line or a null field leaves these empty or set to "null",
  # which would make the whole webhook payload fail to build.
  case "${AGENT_NUM_TURNS}" in
    ''|*[!0-9]*) AGENT_NUM_TURNS=0 ;;
  esac
  case "${AGENT_DURATION_MS}" in
    ''|*[!0-9]*) AGENT_DURATION_MS=0 ;;
  esac
  if ! printf '%s' "${AGENT_TOTAL_COST_USD}" | grep -Eq '^-?[0-9]+(\.[0-9]+)?$'; then
    AGENT_TOTAL_COST_USD=0
  fi
  case "${REPO_PUSHED}" in
    true|false) ;;
    *) REPO_PUSHED=false ;;
  esac
}

run_agent() {
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

prepare_webhook_payload_files() {
  # head for the result: the agent's conclusion is at the front.
  printf '%s' "${AGENT_RESULT}" \
    | head -c "${WEBHOOK_RESULT_MAX_BYTES}" > "${WEBHOOK_RESULT_FILE}" || true

  # tail for the log: the failure is at the end. `tail -n` alone is not enough,
  # a single stream-json line can be hundreds of kilobytes, so cap bytes too.
  if [ -s "${LOG_FILE}" ]; then
    tail -n 200 "${LOG_FILE}" \
      | tail -c "${WEBHOOK_LOG_TAIL_MAX_BYTES}" > "${WEBHOOK_LOG_TAIL_FILE}" || true
  else
    : > "${WEBHOOK_LOG_TAIL_FILE}"
  fi

  # --rawfile needs the files to exist even when everything above was empty.
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

  # Large strings go in through --rawfile, never through --arg: --arg would put
  # them on the command line and blow past MAX_ARG_STRLEN ("Argument list too
  # long"). --rawfile reads the file and binds its content as a string.
  if ! payload="$(jq -n \
    --arg status "${status}" \
    --argjson exit_code "${AGENT_EXIT_CODE}" \
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
    --arg session_id "${AGENT_SESSION_ID}" \
    --arg thread_root_id "${THREAD_ROOT_ID:-}" \
    --arg task_id "${TASK_ID:-}" \
    --arg chat_id "${CHAT_ID:-}" \
    '{
      status: $status,
      exit_code: $exit_code,
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
        session_id: $session_id
      }
    }')"; then
    # A payload that cannot be built must never swallow the whole webhook.
    # send_webhook runs from an EXIT trap under `set -e`: an unguarded failure
    # here aborts the trap, no callback is delivered, and the task stays in
    # `executing` forever - the next run then hits HTTP 409 AlreadyExists.
    log "Failed to build the full webhook payload; falling back to a minimal one"
    payload="$(jq -n \
      --arg status "failed" \
      --argjson exit_code "${AGENT_EXIT_CODE}" \
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

  # The body goes through stdin for the same reason jq reads from files:
  # -d "${payload}" would put the whole JSON document on the command line.
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