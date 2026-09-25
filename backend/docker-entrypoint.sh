#!/usr/bin/env bash
#
# Prepares DATA_DIR before starting the backend.
#
# DATA_DIR is a Docker volume that starts empty on a fresh install, so the
# layout has to be created at runtime rather than baked into the image:
#
#   logs/     config.yaml's log.file lives here and the backend does NOT create
#             the parent directory — a missing logs/ is a startup failure.
#   rules/    skills/  prompts/  tools/   read and written by the UI.
#
# mcp.json and the tools/ defaults (per-mode allow lists and the action plan tool
# schemas) are seeded from the image only when absent, so user edits made through
# the UI survive restarts and upgrades. They have to be real
# files inside the volume: the backend rewrites them with a temp file +
# rename(2), which fails with EBUSY when the path is a bind-mounted single file.

set -euo pipefail

DATA_DIR="${DATA_DIR:-/app/data}"
SEED_DIR="${SEED_DIR:-/app/seed}"

log() { printf 'entrypoint: %s\n' "$1" >&2; }

mkdir -p \
	"${DATA_DIR}/logs" \
	"${DATA_DIR}/rules" \
	"${DATA_DIR}/skills" \
	"${DATA_DIR}/prompts" \
	"${DATA_DIR}/tools"

if [ ! -e "${DATA_DIR}/mcp.json" ]; then
	if [ -f "${SEED_DIR}/mcp.json" ]; then
		cp "${SEED_DIR}/mcp.json" "${DATA_DIR}/mcp.json"
		log "seeded ${DATA_DIR}/mcp.json"
	else
		printf '{\n  "mcpServers": {}\n}\n' > "${DATA_DIR}/mcp.json"
		log "created empty ${DATA_DIR}/mcp.json"
	fi
fi

# The action plan tool schemas live in tools/schemas/. Copied file by file so a
# default added in a later release lands in an existing volume without touching
# the files the operator edited.
#
# The per-mode allow lists are NOT seeded here any more: copy-if-absent cannot
# add a newly shipped tool to a list that already exists, which left new tools
# dead on every upgraded install. They are embedded in the binary and reconciled
# at startup instead (see mode.SeedAllowLists).
if [ -d "${SEED_DIR}/tools" ]; then
	while IFS= read -r src; do
		rel="${src#"${SEED_DIR}/tools/"}"
		dest="${DATA_DIR}/tools/${rel}"
		if [ ! -e "${dest}" ]; then
			mkdir -p "$(dirname "${dest}")"
			cp "${src}" "${dest}"
			log "seeded ${dest}"
		fi
	done < <(find "${SEED_DIR}/tools" -type f -name '*.json')
fi

# nib never runs as root: api_call and execute_command run whatever the model asks for with the
# backend's own uid. Root is held only this long, for two things the image cannot do ahead of
# time: handing over a volume a root-running release left root-owned, and joining the group that
# owns the Docker socket, whose gid is the host's. A container already started as someone else
# (a Kubernetes securityContext) is left as it is.
NIB_USER=nib

if [ "$(id -u)" != "0" ]; then
	log "DATA_DIR=${DATA_DIR} ready"
	exec /usr/local/bin/nib "$@"
fi

find "${DATA_DIR}" \( ! -user "${NIB_USER}" -o ! -group "${NIB_USER}" \) -exec chown -h "${NIB_USER}:${NIB_USER}" {} + \
	|| log "warning: could not hand ${DATA_DIR} over to ${NIB_USER}; the backend may fail to write it"

groups="$(id -g "${NIB_USER}")"
docker_sock=/var/run/docker.sock
case "${DOCKER_HOST:-}" in
	unix://*) docker_sock="${DOCKER_HOST#unix://}" ;;
esac
if [ -S "${docker_sock}" ]; then
	sock_gid="$(stat -c %g "${docker_sock}")"
	groups="${groups},${sock_gid}"
	if [ "${sock_gid}" = "0" ]; then
		# Docker Desktop hands the socket over as root:root. The group grants no capabilities,
		# but it does open every root-group file in the image.
		log "warning: ${docker_sock} is owned by gid 0, joining the root group to reach it"
	else
		log "joining gid ${sock_gid} for ${docker_sock}"
	fi
fi

log "DATA_DIR=${DATA_DIR} ready, starting as ${NIB_USER}"

HOME="$(getent passwd "${NIB_USER}" | cut -d: -f6)"
export HOME
exec setpriv --reuid="${NIB_USER}" --regid="${NIB_USER}" --groups="${groups}" \
	--inh-caps=-all --no-new-privs /usr/local/bin/nib "$@"
