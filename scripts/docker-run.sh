#!/usr/bin/env bash

set -euo pipefail

IMAGE="${IMAGE:-ghcr.io/llovely45/komari-bot:latest}"
CONTAINER_NAME="${CONTAINER_NAME:-komari-tg-bot}"
ENV_URL="${ENV_URL:-https://raw.githubusercontent.com/llovely45/komari-bot/main/.env.example}"
ENV_PATH="${ENV_PATH:-.env}"
DATA_DIR="${DATA_DIR:-./data}"

required_vars=(
  TELEGRAM_BOT_TOKEN
  TELEGRAM_ADMIN_IDS
  KOMARI_URL
  KOMARI_KEY
)

for var_name in "${required_vars[@]}"; do
  if [[ -z "${!var_name:-}" ]]; then
    echo "missing required env: ${var_name}" >&2
    exit 1
  fi
done

if [[ -z "${TELEGRAM_NOTIFY_CHAT_IDS:-}" ]]; then
  TELEGRAM_NOTIFY_CHAT_IDS="${TELEGRAM_ADMIN_IDS}"
fi

escape_sed() {
  printf '%s' "$1" | sed 's/[\/&]/\\&/g'
}

echo "downloading env template from ${ENV_URL}"
curl -fsSL "${ENV_URL}" -o "${ENV_PATH}"

sed -i.bak \
  -e "s#^TELEGRAM_BOT_TOKEN=.*#TELEGRAM_BOT_TOKEN=$(escape_sed "${TELEGRAM_BOT_TOKEN}")#" \
  -e "s#^TELEGRAM_ADMIN_IDS=.*#TELEGRAM_ADMIN_IDS=$(escape_sed "${TELEGRAM_ADMIN_IDS}")#" \
  -e "s#^TELEGRAM_NOTIFY_CHAT_IDS=.*#TELEGRAM_NOTIFY_CHAT_IDS=$(escape_sed "${TELEGRAM_NOTIFY_CHAT_IDS}")#" \
  -e "s#^KOMARI_URL=.*#KOMARI_URL=$(escape_sed "${KOMARI_URL}")#" \
  -e "s#^KOMARI_KEY=.*#KOMARI_KEY=$(escape_sed "${KOMARI_KEY}")#" \
  "${ENV_PATH}"

rm -f "${ENV_PATH}.bak"
mkdir -p "${DATA_DIR}"

echo "pulling image ${IMAGE}"
docker pull "${IMAGE}"

if docker ps -a --format '{{.Names}}' | grep -Fxq "${CONTAINER_NAME}"; then
  echo "removing existing container ${CONTAINER_NAME}"
  docker rm -f "${CONTAINER_NAME}" >/dev/null
fi

echo "starting container ${CONTAINER_NAME}"
docker run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  --env-file "${ENV_PATH}" \
  -v "$(cd "${DATA_DIR}" && pwd):/app/data" \
  "${IMAGE}"

echo
echo "done"
echo "env file: ${ENV_PATH}"
echo "data dir: $(cd "${DATA_DIR}" && pwd)"
echo "logs: docker logs -f ${CONTAINER_NAME}"
