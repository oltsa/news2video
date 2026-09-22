#!/bin/bash

# ==============================================================================
# REALISTIC NEWS CONTENT TEST SCRIPT
# Based on real Finnish tabloid/news archetypes
# ==============================================================================

API_URL="http://localhost:8080/render"
# Replace with your actual API Key
API_KEY="11111111-1111-1111-1111-111111111111" 

GREEN="\033[0;32m"; YELLOW="\033[1;33m"; NC="\033[0m"

# --- Helper Function ---
submit_job() {
    local template=$1
    local desc=$2
    local json_payload=$3

    echo -e "\n${YELLOW}--- ${desc} ---${NC}"
    echo "Template: ${template}"
    
    curl --silent -X POST "${API_URL}" \
      -H "Content-Type: application/json" \
      -H "X-API-Key: ${API_KEY}" \
      -d "${json_payload}" | jq .
      
    echo "---------------------------------------------------"
    sleep 1
}

echo "Submitting Realistic News Jobs..."

# ==============================================================================
# 1. CRIME / INVESTIGATIVE (Template: news_feature)
# Style: "The Red Line" - Noir, Gritty, Serious
# ==============================================================================

JSON_CRIME=$(cat <<EOF
{
  "templateId": "news_feature",
  "backgroundImageUrl": "https://picsum.photos/seed/crime/1080/1920",
  "payload": {
    "topic": "RIKOKSET",
    "title": "Nurmijärveltä surmattuna löytyneen Katjan, 31, taustalla synkkä veriteko",
    "website": "vidgen.news/kotimaa",
    "texts": [
      "Sittemmin surmatuksi joutunut nainen ja hänen avopuolisonsa onnistuivat hämäämään Oodin vartijoita.",
      "Karu totuus selvisi myöhemmin poliisin teknisessä tutkinnassa.",
      "Poliisi epäilee teon olleen vakaasti harkittu ja erityisen raaka.",
      "Lue koko rikostoimittajan raportti verkosta."
    ]
  }
}
EOF
)
submit_job "news_feature" "TRUE CRIME: Nurmijärvi Case" "$JSON_CRIME"


# ==============================================================================
# 2. LIFESTYLE / CONSUMER (Template: news_culture)
# Style: "Modern Split" - High contrast yellow/black, centered
# ==============================================================================

JSON_LIFESTYLE=$(cat <<EOF
{
  "templateId": "news_culture",
  "backgroundImageUrl": "https://picsum.photos/seed/food/1080/1920",
  "payload": {
    "title": "Testasimme 10 uutta hittiravintolaa Helsingissä – yksi yllätti täysin, toinen oli kallis pettymys",
    "website": "vidgen.news/food",
    "texts": [
      "Luksusburgeri maistui pahvilta, mutta lisukkeet pelastivat kokemuksen.",
      "Korttelibistron hinta-laatusuhde on tällä hetkellä kaupungin paras.",
      "Tätä jälkiruokaa varten kannattaa jonottaa jopa tunti sateessa.",
      "Katso koko ranking-lista ja pisteytykset!"
    ]
  }
}
EOF
)
submit_job "news_culture" "LIFESTYLE: Restaurant Review" "$JSON_LIFESTYLE"


# ==============================================================================
# 3. OPINION / LEADER (Template: news_editorial)
# Style: "The Zebra Stack" - Red/White/Black, Authoritative
# ==============================================================================

JSON_OPINION=$(cat <<EOF
{
  "templateId": "news_editorial",
  "backgroundImageUrl": "https://picsum.photos/seed/strike/1080/1920",
  "payload": {
    "topic": "PÄÄKIRJOITUS",
    "title": "Suomen vientiteollisuus ei kestä enää yhtään uutta lakkoviikkoa.",
    "website": "vidgen.news/editorial",
    "datetime": "TORSTAI 20.11.",
    "texts": [
      "Työmarkkinoiden solmu on avattava keinolla millä hyvänsä.",
      "Vastuu on nyt sekä työnantajilla että työntekijöillä.",
      "Kyse ei ole enää vain palkoista, vaan koko hyvinvointivaltion rahoituksesta."
    ]
  }
}
EOF
)
submit_job "news_editorial" "OPINION: Labor Strike Commentary" "$JSON_OPINION"


# ==============================================================================
# 4. VERSUS / SLIDESHOW (Template: news_versus)
# Style: "Fight Night" - Split screen, sliding images
# ==============================================================================

JSON_VERSUS=$(cat <<EOF
{
  "templateId": "news_versus",
  "backgroundImageUrl": "https://picsum.photos/1080/1920",
  "payload": {
    "title": "Photo Competition #14",
    "image_1": "https://picsum.photos/seed/hki1/1300/960",
    "credit_1": "Helsinki Tourism",
    "image_2": "https://picsum.photos/seed/tre1/1300/960",
    "credit_2": "Visit Tampere",
    "image_3": "https://picsum.photos/seed/hki2/1300/960",
    "credit_3": "Jussi Valokuvaaja",
    "image_4": "https://picsum.photos/seed/tre2/1300/960",
    "credit_4": "Matti Meikäläinen"
  }
}
EOF
)
submit_job "news_versus" "VERSUS: Photo competition" "$JSON_VERSUS"

echo -e "\n${GREEN}All realistic jobs submitted.${NC}"