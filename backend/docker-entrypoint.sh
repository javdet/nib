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

# Per-mode allow lists live in tools/, the action plan tool schemas in
# tools/schemas/. Copied file by file so a default added in a later release
# lands in an existing volume without touching the files the operator edited.
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

log "DATA_DIR=${DATA_DIR} ready"

exec /usr/local/bin/nib "$@"
