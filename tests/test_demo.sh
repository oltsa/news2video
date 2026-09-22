#!/bin/bash

API_URL="http://localhost:8080/render"
API_KEY_DEMO_PROJ="11111111-1111-1111-1111-111111111111"

GREEN="\033[0;32m"
YELLOW="\033[1;33m"
NC="\033[0m"

make_request() {
    local api_key=$1; local template_id=$2; local description=$3;
    local topic=$4; local title=$5; shift 5
    local texts_json=$(printf '"%s",' "$@" | sed 's/,$//')

    echo -e "\n${YELLOW}--- ${description} ---${NC}"

    JSON_PAYLOAD=$(cat <<EOF
{
  "templateId": "${template_id}",
  "backgroundImageUrl": "https://picsum.photos/1080/1920?random=$(date +%s%N)",
  "payload": {
    "topic": "${topic}",
    "title": "${title}",
    "texts": [${texts_json}]
  }
}
EOF
)
    echo "Submitting job with Template ID: '${template_id}'..."
    curl --silent -X POST -H "Content-Type: application/json" -H "X-API-Key: ${api_key}" \
        -d "${JSON_PAYLOAD}" "${API_URL}" | jq .
    echo "---------------------------------------------------"
}

echo "Starting demo render..."

make_request "${API_KEY_DEMO_PROJ}" "demo_template" \
"Client Demo (demo_template)" \
"LIVE DEMO" \
"This System Automates Video Creation from Simple Data. This long title will shrink to fit inside its box." \
"This is the first carousel item with a white background." \
"This is the second item." \
"It's simple, fast, and reliable."

echo -e "\n${GREEN}Demo render has been submitted.${NC}"