#!/bin/bash

# --- Configuration ---
# This is the default API key for 'publication_a' from your init.sql
API_KEY="11111111-1111-1111-1111-111111111111"
ORCHESTRATOR_URL="http://localhost:8080"

# --- Helper: Check for jq ---
if ! command -v jq &> /dev/null; then
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    echo "!!! ERROR: 'jq' command is not installed.                !!!"
    echo "!!! This script requires 'jq' to parse the Job ID from   !!!"
    echo "!!! the API response. Please install it and try again.   !!!"
    echo "!!! (e.g., 'sudo apt-get install jq' or 'brew install jq') !!!"
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    exit 1
fi

# --- Step 1: Submit a new render job ---
echo "▶️  Step 1: Submitting a new render job to get a Job ID..."
RESPONSE=$(curl -s -X POST "$ORCHESTRATOR_URL/render" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{
        "templateId": "news_default",
        "payload": {
          "title": "Live System Test",
          "topic": "Verification",
          "texts": ["Testing endpoint...", "Testing rate limit..."]
        }
      }')

# Extract Job ID and check for errors
JOB_ID=$(echo "$RESPONSE" | jq -r '.jobId')
if [ "$JOB_ID" == "null" ]; then
    echo "Error: Failed to submit render job. The API returned:"
    echo "$RESPONSE"
    exit 1
fi
echo "Success! Got Job ID: $JOB_ID"
echo ""
sleep 1 # Brief pause

# --- Step 2: Test the job status endpoint (Happy Path) ---
echo "▶️  Step 2: Checking job status once (Happy Path)..."
curl -H "X-API-Key: $API_KEY" "$ORCHESTRATOR_URL/jobs/$JOB_ID"
echo ""
echo "Endpoint is working."
echo ""
sleep 1

# --- Step 3: Test the rate limiter ---
echo "▶️  Step 3: Testing rate limiter by sending 5 rapid requests..."
echo "    (Expecting first 2 to be HTTP 200, the rest to be 429)"
echo "----------------------------------------------------------------"
for i in {1..5}; do
  echo -n "Request #$i result: "
  # The -w flag prints the HTTP status code after the response
  curl -w " | HTTP Status: %{http_code}\n" -s -H "X-API-Key: $API_KEY" "$ORCHESTRATOR_URL/jobs/$JOB_ID"
  sleep 0.2 # Send a request every 0.2 seconds
done
echo "----------------------------------------------------------------"
echo "🏁 Test complete. If you saw HTTP Status 429, the rate limiter is working correctly."