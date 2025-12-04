#!/bin/bash

# ---------------------------------------------------------
# CONFIGURATION & PATHS (The Nuclear Option)
# ---------------------------------------------------------
# We manually point to the Homebrew "keg-only" curl and standard jq
CURL_CMD="/opt/homebrew/opt/curl/bin/curl"
JQ_CMD="/opt/homebrew/bin/jq"

# Fallback: If Homebrew curl isn't found, try system curl
if [ ! -f "$CURL_CMD" ]; then
    CURL_CMD="/usr/bin/curl"
fi

# Fallback: If Homebrew jq isn't found, disable it
if [ ! -f "$JQ_CMD" ]; then
    JQ_CMD=""
fi

# Debugging: Print which tools we are using
# echo "Using curl at: $CURL_CMD"
# echo "Using jq at:   $JQ_CMD"

TOKEN_FILE=".access_token"
BASE_URL="http://localhost:8080"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# ---------------------------------------------------------
# COMMAND: LOGIN
# ---------------------------------------------------------
if [ "$1" == "login" ]; then
    echo -e "${BLUE}Running get_token.go...${NC}"
    
    # Run the Go script and capture output
    OUTPUT=$(go run get_token.go)
    
    # Extract the token (find the long string starting with eyJ)
    TOKEN=$(echo "$OUTPUT" | grep -oE 'eyJ[a-zA-Z0-9._-]+' | head -n 1)

    if [ -z "$TOKEN" ]; then
        echo -e "${RED}Failed to extract token.${NC}"
        echo "$OUTPUT"
        exit 1
    fi

    echo "$TOKEN" > "$TOKEN_FILE"
    echo -e "${GREEN}Token saved to $TOKEN_FILE${NC}"
    exit 0
fi

# ---------------------------------------------------------
# CHECK TOKEN
# ---------------------------------------------------------
if [ ! -f "$TOKEN_FILE" ]; then
    echo -e "${RED}No token found.${NC}"
    echo "Run './api.sh login' first."
    exit 1
fi

TOKEN=$(cat "$TOKEN_FILE")

# ---------------------------------------------------------
# PARSE ARGUMENTS
# ---------------------------------------------------------
METHOD="GET"
PATH=""
BODY=""

if [ -z "$1" ]; then
    echo "Usage: ./api.sh login"
    echo "Usage: ./api.sh [METHOD] /path [json_body]"
    exit 1
fi

if [[ "$1" == /* ]]; then
    METHOD="GET"
    PATH="$1"
else
    METHOD="$1"
    PATH="$2"
    BODY="$3"
fi

# ---------------------------------------------------------
# EXECUTE CURL
# ---------------------------------------------------------
echo -e "${BLUE}$METHOD $BASE_URL$PATH${NC}"

# We use the variable $CURL_CMD here instead of just 'curl'
if [ -z "$BODY" ]; then
    RESPONSE=$("$CURL_CMD" -s -X "$METHOD" "$BASE_URL$PATH" \
        -H "Authorization: Bearer $TOKEN")
else
    RESPONSE=$("$CURL_CMD" -s -X "$METHOD" "$BASE_URL$PATH" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "$BODY")
fi

# ---------------------------------------------------------
# PRETTY PRINT OUTPUT
# ---------------------------------------------------------
# Check if JQ_CMD variable is set and the file exists
if [ -n "$JQ_CMD" ] && [ -f "$JQ_CMD" ]; then
    echo "$RESPONSE" | "$JQ_CMD"
else
    # Fallback to python if jq is missing
    echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"
fi