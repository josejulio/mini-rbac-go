#!/bin/bash

# Common test helpers for Mini RBAC Go test scripts

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Safe curl wrapper that captures both response and status code
# Usage: safe_curl "Step name" curl_args...
safe_curl() {
    local step_name="$1"
    shift

    # Create temp files for response body and error output
    local temp_file=$(mktemp)
    local error_file=$(mktemp)

    # Run curl and capture status code
    local http_code
    http_code=$(curl -s -w "%{http_code}" -o "$temp_file" "$@" 2>"$error_file")
    local curl_exit=$?
    local response=$(cat "$temp_file")
    local curl_error=$(cat "$error_file")
    rm -f "$temp_file" "$error_file"

    # Check if curl itself failed (network error, DNS, etc.)
    if [ $curl_exit -ne 0 ]; then
        echo -e "${RED}❌ Test failed at step: $step_name${NC}" >&2
        echo "Curl failed with exit code: $curl_exit" >&2
        if [ -n "$curl_error" ]; then
            echo "Error: $curl_error" >&2
        fi
        exit 1
    fi

    # Check if HTTP status is successful (2xx or 3xx)
    if [[ ! "$http_code" =~ ^[23][0-9][0-9]$ ]] && [[ ! "$http_code" = "204" ]]; then
        echo -e "${RED}❌ Test failed at step: $step_name${NC}" >&2
        echo "HTTP Status: $http_code" >&2
        echo "Response body:" >&2
        if [ -n "$response" ]; then
            (echo "$response" | jq '.' 2>/dev/null || echo "$response") >&2
        else
            echo "(empty response)" >&2
        fi
        exit 1
    fi

    # Return the response
    echo "$response"
}

# Extract and validate ID from JSON response
extract_id() {
    local response="$1"
    local step="$2"
    local id

    id=$(echo "$response" | jq -r '.id // empty')

    if [ -z "$id" ] || [ "$id" = "null" ]; then
        echo -e "${RED}❌ Failed to extract ID at step: $step${NC}" >&2
        echo "Response was:" >&2
        (echo "$response" | jq '.' 2>/dev/null || echo "$response") >&2
        exit 1
    fi

    echo "$id"
}

# Check if response contains error field
check_response() {
    local response="$1"
    local step="$2"

    if echo "$response" | jq -e '.error or .title' > /dev/null 2>&1; then
        echo -e "${RED}❌ Test failed at step: $step${NC}" >&2
        echo "Response contains error:" >&2
        echo "$response" | jq '.' >&2
        exit 1
    fi

    if [ -z "$response" ]; then
        echo -e "${RED}❌ Test failed at step: $step${NC}" >&2
        echo "Empty response" >&2
        exit 1
    fi
}

# Handle error from failed command
handle_error() {
    local step="$1"
    local response="$2"

    echo -e "${RED}❌ Test failed at step: $step${NC}" >&2
    if [ -n "$response" ]; then
        echo "Response:" >&2
        (echo "$response" | jq '.' 2>/dev/null || echo "$response") >&2
    fi
    exit 1
}
