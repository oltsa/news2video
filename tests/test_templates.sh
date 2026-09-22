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
# Style: "Noir", Gritty, Serious
# Source Inspiration: Your screenshot about the Nurmijärvi case
# ==============================================================================

JSON_CRIME=$(cat <<EOF
{
  "templateId": "news_feature",
  "backgroundImageUrl": "https://picsum.photos/1080/1920",
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
# 2. POLITICS / ECONOMY (Template: kitchen_sink)
# Style: Info-heavy, Metadata (Location/Time), Serious
# Source Inspiration: Your screenshot about Marin/Ministry of Finance
# ==============================================================================

# JSON_POLITICS=$(cat <<EOF
# {
#   "templateId": "kitchen_sink",
#   "backgroundImageUrl": "https://picsum.photos/1080/1920",
#   "payload": {
#     "title": "Valtiovarainministeriön mukaan VTV:n raportti liioittelee Marinin hallituksen menolisäyksiä",
#     "website": "vidgen.news/politics",
#     "location": "Eduskunta",
#     "datetime": "Torstai 12:41",
#     "texts": [
#       "Valtiovarainministeriön mukaan Marinin hallituksen menolisäykset olivat vain noin kolme miljardia euroa.",
#       "VTV:n aiempi raportti arvioi summan huomattavasti suuremmaksi.",
#       "Erimielisyys laskentatavoista on aiheuttanut kiivasta väittelyä täysistunnossa.",
#       "Asiantuntijat varoittavat julkisen talouden kestävyysvajeesta."
#     ]
#   }
# }
# EOF
# )
# submit_job "kitchen_sink" "POLITICS: Ministry of Finance Report" "$JSON_POLITICS"


# ==============================================================================
# 3. LIFESTYLE / CONSUMER TEST (Template: news_culture)
# Style: Clickbaity, Listicle, "We tested X so you don't have to"
# ==============================================================================

JSON_LIFESTYLE=$(cat <<EOF
{
  "templateId": "news_culture",
  "backgroundImageUrl": "https://picsum.photos/1080/1920",
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
# 4. OPINION / LEADER (Template: news_editorial)
# Style: Authoritative, text-heavy, Quote-focused
# ==============================================================================

JSON_OPINION=$(cat <<EOF
{
  "templateId": "news_editorial",
  "backgroundImageUrl": "https://picsum.photos/1080/1920",
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

echo -e "\n${GREEN}All realistic jobs submitted.${NC}"