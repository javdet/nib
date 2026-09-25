#!/usr/bin/env bash
# Interactive first-run setup for the production compose stack (make setup).
#
# Asks for the LLM provider, its key and the first project, writes .env and
# deploy/compose/config.local.yaml, then starts docker compose and prints the
# URL and API token to log in with.
#
# Written for the bash 3.2 macOS ships: no associative arrays, no ${var,,}.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV_FILE=.env
ENV_TEMPLATE=.env.example
CONFIG_TEMPLATE=deploy/compose/config.yaml
# A file of its own rather than the tracked template, so a `git pull` never
# conflicts with an install's settings.
CONFIG_FILE=deploy/compose/config.local.yaml
PGDATA_VOLUME=nib-pgdata

# Same rule as a knowledge-base collection name (internal/kbdoc/name.go): the
# project name becomes the collection that plan auto-updates write to.
NAME_RE='^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'

if [ -t 1 ]; then
	BOLD=$'\033[1m'; DIM=$'\033[2m'; RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RESET=$'\033[0m'
else
	BOLD=; DIM=; RED=; GREEN=; YELLOW=; RESET=
fi

info() { printf '%s==>%s %s\n' "$BOLD" "$RESET" "$*"; }
warn() { printf '%swarning:%s %s\n' "$YELLOW" "$RESET" "$*" >&2; }
die() { printf '%serror:%s %s\n' "$RED" "$RESET" "$*" >&2; exit 1; }

# ask VAR "Prompt" [default] — reads one line; an empty answer takes the default.
# End of input aborts, or every loop re-asking for a valid answer would spin.
ask() {
	local __var=$1 __prompt=$2 __default=${3:-} __reply=
	if [ -n "$__default" ]; then
		read -r -p "$__prompt [$__default]: " __reply || [ -n "$__reply" ] || die "no more input"
	else
		read -r -p "$__prompt: " __reply || [ -n "$__reply" ] || die "no more input"
	fi
	__reply=$(printf '%s' "${__reply:-$__default}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
	printf -v "$__var" '%s' "$__reply"
}

# ask_secret VAR "Prompt" — like ask, without echoing, and never empty.
ask_secret() {
	local __var=$1 __prompt=$2 __reply=
	while [ -z "$__reply" ]; do
		read -r -s -p "$__prompt: " __reply || [ -n "$__reply" ] || die "no more input"
		printf '\n'
		__reply=$(printf '%s' "$__reply" | tr -d '[:space:]')
		[ -n "$__reply" ] || warn "a value is required"
	done
	printf -v "$__var" '%s' "$__reply"
}

# ask_name VAR "Prompt" required|optional
ask_name() {
	local __var=$1 __prompt=$2 __mode=$3 __value
	while :; do
		ask __value "$__prompt"
		if [ -z "$__value" ]; then
			[ "$__mode" = optional ] && break
			warn "a value is required"
			continue
		fi
		printf '%s' "$__value" | grep -Eq "$NAME_RE" && break
		warn "use letters, digits, '.', '_' or '-' (starting with a letter or digit, at most 64)"
	done
	printf -v "$__var" '%s' "$__value"
}

# env_get KEY [FILE] — the value KEY has in FILE (default: an existing .env),
# empty when absent.
env_get() {
	local file=${2:-$ENV_FILE}
	[ -f "$file" ] || return 0
	sed -n "s/^$1=//p" "$file" | tail -n 1
}

# env_set FILE KEY VALUE — sets KEY, uncommenting "# KEY=" if that is all
# there is, appending otherwise. awk rather than sed -i, whose flags differ
# between BSD and GNU; the value goes through ENVIRON so no character in it is
# ever interpreted.
env_set() {
	local file=$1 tmp
	tmp=$(mktemp "${file}.XXXXXX")
	K=$2 V=$3 awk '
		BEGIN { k = ENVIRON["K"]; v = ENVIRON["V"]; done = 0 }
		!done && ($0 ~ "^" k "=" || $0 ~ "^# *" k "=") { print k "=" v; done = 1; next }
		{ print }
		END { if (!done) print k "=" v }
	' "$file" >"$tmp"
	mv "$tmp" "$file"
}

sudo_cmd() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1; then
		sudo "$@"
	else
		die "root is required to run: $* (install sudo or run as root)"
	fi
}

ensure_openssl() {
	command -v openssl >/dev/null 2>&1 && return 0
	info "OpenSSL is not installed; installing it"
	if command -v brew >/dev/null 2>&1; then
		brew install openssl
	elif command -v apt-get >/dev/null 2>&1; then
		sudo_cmd apt-get update -y && sudo_cmd apt-get install -y openssl
	elif command -v dnf >/dev/null 2>&1; then
		sudo_cmd dnf install -y openssl
	elif command -v yum >/dev/null 2>&1; then
		sudo_cmd yum install -y openssl
	elif command -v apk >/dev/null 2>&1; then
		sudo_cmd apk add --no-cache openssl
	elif command -v pacman >/dev/null 2>&1; then
		sudo_cmd pacman -Sy --noconfirm openssl
	elif command -v zypper >/dev/null 2>&1; then
		sudo_cmd zypper --non-interactive install openssl
	else
		die "no supported package manager found; install openssl and run make setup again"
	fi
	command -v openssl >/dev/null 2>&1 || die "openssl is still not on PATH after installing it"
}

ensure_docker() {
	command -v docker >/dev/null 2>&1 || die "docker is not installed: https://docs.docker.com/get-docker/"
	docker compose version >/dev/null 2>&1 || die "the docker compose plugin is not installed"
	docker info >/dev/null 2>&1 || die "the Docker daemon is not reachable; start Docker and run make setup again"
}

# --- ports ---

# port_free PORT — true when nothing but this install's own stack listens on it.
port_free() {
	local owner
	# Containers first, matched on the host side of "0.0.0.0:8080->80/tcp"
	# (docker's publish= filter matches the container port instead). A re-run
	# finds the ports held by the nib stack itself, which compose replaces in
	# place; any other container's port is taken.
	owner=$(docker ps --format '{{.Label "com.docker.compose.project"}}|{{.Ports}}' 2>/dev/null |
		awk -F'|' -v p=":$1->" 'index($2, p) { print ($1 == "nib" ? "nib" : "other"); exit }')
	case $owner in
		nib) return 0 ;;
		other) return 1 ;;
	esac
	if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; then
		return 1
	fi
	# Without root lsof cannot see other users' sockets, so a connect has the
	# last word.
	! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

CHOSEN_PORTS=" "

next_free_port() {
	local port=$1 limit=$(($1 + 100))
	while [ "$port" -le "$limit" ] && [ "$port" -le 65535 ]; do
		case $CHOSEN_PORTS in *" $port "*) ;; *) if port_free "$port"; then echo "$port"; return; fi ;; esac
		port=$((port + 1))
	done
}

# choose_port VAR ENV_KEY "what it serves" — keeps the current port when it is
# free and asks for another only when it is taken.
choose_port() {
	local __var=$1 __key=$2 __label=$3 __port __base __suggest
	__port=$(env_get "$__key")
	[ -n "$__port" ] || __port=$(env_get "$__key" "$ENV_TEMPLATE")
	__base=$__port
	while :; do
		if ! printf '%s' "$__port" | grep -Eq '^[0-9]{1,5}$' || [ "$__port" -lt 1 ] || [ "$__port" -gt 65535 ]; then
			warn "'$__port' is not a port number"
		else
			case $CHOSEN_PORTS in
				*" $__port "*) warn "port $__port is already used for another nib port" ;;
				*) if port_free "$__port"; then break; fi
					warn "port $__port ($__key, $__label) is already in use on this host" ;;
			esac
		fi
		# Searched upwards from the port we started with, so a mistyped answer
		# still gets a suggestion.
		__suggest=$(next_free_port $((__base + 1)))
		ask __port "Port for $__label" "$__suggest"
	done
	CHOSEN_PORTS="$CHOSEN_PORTS$__port "
	printf -v "$__var" '%s' "$__port"
}

# --- provider presets (docs/how-to/switch-llm-provider.md#provider-presets) ---

PROVIDERS="openai openrouter gemini qwen deepseek xai kimi"

provider_label() {
	case $1 in
		openai) echo "OpenAI" ;;
		openrouter) echo "OpenRouter" ;;
		gemini) echo "Google Gemini" ;;
		qwen) echo "Alibaba Qwen (DashScope)" ;;
		deepseek) echo "DeepSeek        (embeddings need an OpenAI key)" ;;
		xai) echo "xAI Grok        (embeddings need an OpenAI key)" ;;
		kimi) echo "Moonshot Kimi   (embeddings need an OpenAI key)" ;;
	esac
}

# Sets the LLM_* and EMB_* globals the config and .env are written from.
# EMB_BASE_URL empty means embeddings come from LLM_BASE_URL.
load_preset() {
	LLM_API=chat
	LLM_EFFORT='""'
	LLM_REFERER='""'
	LLM_TITLE='""'
	EMB_BASE_URL=
	EMB_MODEL=
	EMB_DIMENSIONS=
	EMB_SEPARATE_KEY=false
	case $1 in
		openai)
			LLM_BASE_URL=https://api.openai.com/v1
			LLM_MODEL=gpt-5.6
			LLM_EMBEDDING_MODEL=text-embedding-3-small
			LLM_API=responses
			LLM_EFFORT=medium
			;;
		openrouter)
			LLM_BASE_URL=https://openrouter.ai/api/v1
			LLM_MODEL=anthropic/claude-opus-4.8
			LLM_EMBEDDING_MODEL=openai/text-embedding-3-small
			LLM_REFERER=https://github.com/javdet/nib
			LLM_TITLE=nib
			;;
		gemini)
			LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/
			LLM_MODEL=gemini-2.5-pro
			LLM_EMBEDDING_MODEL=gemini-embedding-001
			EMB_MODEL=gemini-embedding-001
			EMB_DIMENSIONS=1536
			;;
		qwen)
			LLM_BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1
			LLM_MODEL=qwen-max
			LLM_EMBEDDING_MODEL=text-embedding-v4
			EMB_MODEL=text-embedding-v4
			EMB_DIMENSIONS=1536
			;;
		deepseek | xai | kimi)
			case $1 in
				deepseek) LLM_BASE_URL=https://api.deepseek.com/v1; LLM_MODEL=deepseek-chat ;;
				xai) LLM_BASE_URL=https://api.x.ai/v1; LLM_MODEL=grok-4 ;;
				kimi) LLM_BASE_URL=https://api.moonshot.ai/v1; LLM_MODEL=kimi-k2-0905-preview ;;
			esac
			LLM_EMBEDDING_MODEL=text-embedding-3-small
			EMB_BASE_URL=https://api.openai.com/v1
			EMB_MODEL=text-embedding-3-small
			EMB_SEPARATE_KEY=true
			;;
		*) die "unknown provider: $1" ;;
	esac
}

choose_provider() {
	local i=1 p choice
	printf '\n%sLLM provider%s\n' "$BOLD" "$RESET"
	for p in $PROVIDERS; do
		printf '  %d) %s\n' "$i" "$(provider_label "$p")"
		i=$((i + 1))
	done
	while :; do
		ask choice "Choose a provider" 1
		if printf '%s' "$choice" | grep -Eq '^[1-7]$'; then
			PROVIDER=$(printf '%s\n' $PROVIDERS | sed -n "${choice}p")
			return
		fi
		warn "enter a number from 1 to 7"
	done
}

# --- config.local.yaml ---

yaml_quote() { printf '"%s"' "$1"; }

write_config() {
	local tmp env cloud_block="" env_block="" IFS_SAVE
	tmp=$(mktemp "${CONFIG_FILE}.XXXXXX")

	{
		cat <<-EOF
			# Generated by \`make setup\` from $CONFIG_TEMPLATE. Mounted as /app/config.yaml
			# through NIB_CONFIG_FILE in .env; edit it freely, \`make setup\` rewrites it.
			# Provider presets: docs/how-to/switch-llm-provider.md#provider-presets
			# API keys live in .env, never here.
			llm:
			  baseURL: $LLM_BASE_URL
			  model: $(yaml_quote "$LLM_MODEL")
			  # MUST match kb_collections.embedding_model exactly.
			  embeddingModel: $LLM_EMBEDDING_MODEL
			  api: $LLM_API
		EOF
		if [ -n "$EMB_MODEL" ]; then
			echo "  embeddings:"
			if [ -n "$EMB_BASE_URL" ]; then echo "    baseURL: $EMB_BASE_URL"; fi
			echo "    model: $EMB_MODEL"
			if [ -n "$EMB_DIMENSIONS" ]; then echo "    dimensions: $EMB_DIMENSIONS"; fi
		fi
		cat <<-EOF
			  reasoningEffort: $LLM_EFFORT
			  timeoutSeconds: 300
			  httpReferer: $LLM_REFERER
			  appTitle: $LLM_TITLE

		EOF

		# Everything between the template's llm block and its example projects,
		# so log/metrics/agent/directory settings follow the shipped defaults.
		awk '
			state == 0 && /^llm:/ { state = 1; next }
			state == 1 && /^[^[:space:]]/ { state = 2 }
			state == 2 && /^projects:/ { exit }
			state == 2 { print }
		' "$CONFIG_TEMPLATE"

		echo "projects:"
		echo "  - name: $(yaml_quote "$PROJECT")"
		echo "    description: \"\""
		if [ -n "$ENVIRONMENTS" ]; then
			echo "    environments:"
			IFS_SAVE=$IFS
			IFS=,
			for env in $ENVIRONMENTS; do
				echo "      - name: $(yaml_quote "$env")"
				echo "        description: \"\""
			done
			IFS=$IFS_SAVE
		else
			echo "    environments: []"
		fi
		if [ -n "$CLOUD" ]; then
			echo "    clouds:"
			echo "      - name: $(yaml_quote "$CLOUD")"
			echo "        description: \"\""
			if [ -n "$REGION" ]; then
				echo "        locations:"
				echo "          - $(yaml_quote "$REGION")"
			else
				echo "        locations: []"
			fi
		else
			echo "    clouds: []"
		fi
	} >"$tmp"
	mv "$tmp" "$CONFIG_FILE"
	chmod 0644 "$CONFIG_FILE"
}

# --- .env ---

write_env() {
	local api_token encryption_key pg_password webhook_token tmp

	# Anything a previous run generated is kept: a new SECRETS_ENCRYPTION_KEY
	# makes every stored secret unreadable, a new POSTGRES_PASSWORD no longer
	# opens an existing database volume, a new NIB_API_TOKEN signs every browser
	# out.
	api_token=$(env_get NIB_API_TOKEN)
	encryption_key=$(env_get SECRETS_ENCRYPTION_KEY)
	pg_password=$(env_get POSTGRES_PASSWORD)
	webhook_token=$(env_get AGENT_WEBHOOK_TOKEN)

	[ ${#api_token} -ge 32 ] || api_token=$(openssl rand -hex 32)
	[ -n "$encryption_key" ] || encryption_key=$(openssl rand -base64 32)
	if [ -z "$pg_password" ] || [ "$pg_password" = nib ]; then
		# Postgres reads the password only when it initialises an empty volume,
		# so an existing volume keeps whatever it was created with.
		if docker volume inspect "$PGDATA_VOLUME" >/dev/null 2>&1; then
			pg_password=${pg_password:-nib}
		else
			pg_password=$(openssl rand -hex 24)
		fi
	fi

	if [ -f "$ENV_FILE" ]; then
		BACKUP="$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
		cp "$ENV_FILE" "$BACKUP"
		chmod 0600 "$BACKUP"
	fi

	tmp=$(mktemp "${ENV_FILE}.XXXXXX")
	cp "$ENV_TEMPLATE" "$tmp"
	env_set "$tmp" NIB_CONFIG_FILE "./$CONFIG_FILE"
	env_set "$tmp" NIB_HTTP_PORT "$HTTP_PORT"
	env_set "$tmp" NIB_BACKEND_PORT "$BACKEND_PORT"
	env_set "$tmp" NIB_METRICS_PORT "$METRICS_PORT"
	env_set "$tmp" POSTGRES_PASSWORD "$pg_password"
	env_set "$tmp" LLM_API_KEY "$LLM_KEY"
	env_set "$tmp" NIB_API_TOKEN "$api_token"
	env_set "$tmp" SECRETS_ENCRYPTION_KEY "$encryption_key"
	# Empty lets the backend generate {DATA_DIR}/.webhook-key itself; a value
	# someone set before is kept, since changing it orphans runs in flight.
	env_set "$tmp" AGENT_WEBHOOK_TOKEN "$webhook_token"

	# kb-mcp embeds the queries, so it has to reach the same host at the same width.
	env_set "$tmp" KB_EMBEDDINGS_BASE_URL "${EMB_BASE_URL:-${LLM_BASE_URL%/}}"
	if [ -n "$EMB_DIMENSIONS" ]; then
		env_set "$tmp" KB_EMBEDDINGS_DIMENSIONS "$EMB_DIMENSIONS"
	fi
	if [ "$EMB_SEPARATE_KEY" = true ]; then
		env_set "$tmp" LLM_EMBEDDINGS_API_KEY "$EMB_KEY"
	fi

	chmod 0600 "$tmp"
	mv "$tmp" "$ENV_FILE"
	API_TOKEN=$api_token
}

# --- main ---

printf '%sNib setup%s\n' "$BOLD" "$RESET"
printf '%sWrites .env and %s, then starts docker compose.%s\n' "$DIM" "$CONFIG_FILE" "$RESET"

for f in "$ENV_TEMPLATE" "$CONFIG_TEMPLATE"; do
	[ -f "$f" ] || die "$f is missing; run make setup from a complete checkout"
done
ensure_docker
ensure_openssl

if [ -f "$ENV_FILE" ]; then
	warn "$ENV_FILE already exists. It will be backed up and rewritten; its generated tokens and passwords are kept."
	ask confirm "Continue? (y/N)" n
	case $confirm in y | Y | yes | YES) ;; *) die "aborted; nothing was changed" ;; esac
fi

choose_provider
load_preset "$PROVIDER"
while :; do
	ask LLM_MODEL "Model" "$LLM_MODEL"
	# Written into YAML inside double quotes, so those and backslashes are out.
	printf '%s' "$LLM_MODEL" | grep -Eq '^[^[:space:]"\\]+$' && break
	warn "a model id has no spaces, quotes or backslashes"
done
ask_secret LLM_KEY "$(provider_label "$PROVIDER" | sed 's/ *(.*//') API key (LLM_API_KEY)"
EMB_KEY=
if [ "$EMB_SEPARATE_KEY" = true ]; then
	printf '%sThis provider serves no /embeddings route; the knowledge base embeds through OpenAI.%s\n' "$DIM" "$RESET"
	ask_secret EMB_KEY "OpenAI API key for embeddings (LLM_EMBEDDINGS_API_KEY)"
fi

printf '\n%sFirst project%s\n' "$BOLD" "$RESET"
ask_name PROJECT "Project name" required

while :; do
	ask ENVIRONMENTS "Environments, comma-separated (optional, e.g. stage,prod)"
	ENVIRONMENTS=$(printf '%s' "$ENVIRONMENTS" | tr -d '[:space:]' | sed -e 's/,,*/,/g' -e 's/^,//' -e 's/,$//')
	bad=
	IFS_SAVE=$IFS
	IFS=,
	for e in $ENVIRONMENTS; do
		printf '%s' "$e" | grep -Eq "$NAME_RE" || bad=$e
	done
	IFS=$IFS_SAVE
	[ -z "$bad" ] && break
	warn "invalid environment name '$bad': use letters, digits, '.', '_' or '-'"
done
# Duplicate names would be refused by the Projects page, so drop them here.
ENVIRONMENTS=$(printf '%s' "$ENVIRONMENTS" | tr ',' '\n' | awk 'NF && !seen[$0]++' | paste -sd, -)

ask_name CLOUD "Cloud (optional, e.g. aws)" optional
REGION=
if [ -n "$CLOUD" ]; then
	# A region lives under a cloud in config.yaml, so it is asked only with one.
	ask_name REGION "Region (optional, e.g. us-east-1)" optional
fi

if docker volume inspect "$PGDATA_VOLUME" >/dev/null 2>&1; then
	warn "volume $PGDATA_VOLUME already exists. If this install used another embedding model before, knowledge search needs the rename in docs/how-to/switch-llm-provider.md#rename-the-embedding-model-in-the-database."
fi

choose_port HTTP_PORT NIB_HTTP_PORT "the web UI"
choose_port BACKEND_PORT NIB_BACKEND_PORT "direct API access"
choose_port METRICS_PORT NIB_METRICS_PORT "Prometheus metrics"

info "Writing $CONFIG_FILE"
write_config
info "Writing $ENV_FILE"
write_env
if [ -n "${BACKUP:-}" ]; then info "Previous $ENV_FILE saved as $BACKUP"; fi

info "Starting docker compose (first start pulls images and can take a few minutes)"
if ! docker compose up -d --wait --wait-timeout 300; then
	docker compose ps >&2 || true
	die "the stack did not become healthy; see: docker compose logs backend"
fi

printf '\n%sNib is running.%s\n\n' "$GREEN" "$RESET"
printf '  URL:        %shttp://localhost:%s%s\n' "$BOLD" "$HTTP_PORT" "$RESET"
printf '  API token:  %s%s%s\n\n' "$BOLD" "$API_TOKEN" "$RESET"
printf '%sPaste the token on the login screen. It is stored in %s as NIB_API_TOKEN.%s\n' "$DIM" "$ENV_FILE" "$RESET"
