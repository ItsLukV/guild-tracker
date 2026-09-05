#!/usr/bin/env bash
set -euo pipefail

UUID="${1:?usage: fetch_profiles.sh <player-uuid> [output-file]}"

if [ -z "${HYPIXEL_API_KEY:-}" ]; then
    ENV_FILE="$(dirname "$0")/../.env"
    if [ -f "$ENV_FILE" ]; then
        HYPIXEL_API_KEY="$(grep -E '^(export )?HYPIXEL_API_KEY=' "$ENV_FILE" | tail -n1 | cut -d= -f2-)"
    fi
fi

if [ -z "${HYPIXEL_API_KEY:-}" ]; then
    echo "error: HYPIXEL_API_KEY not set and not found in .env" >&2
    exit 1
fi

RESPONSE=$(curl -sS -G "https://api.hypixel.net/v2/skyblock/profiles" \
    -H "API-Key: ${HYPIXEL_API_KEY}" \
    --data-urlencode "uuid=${UUID}")

if command -v jq >/dev/null 2>&1; then
    RESPONSE=$(echo "$RESPONSE" | jq .)
fi


echo "$RESPONSE"
