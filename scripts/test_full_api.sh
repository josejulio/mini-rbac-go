#!/bin/bash

# Mini RBAC Go - Full API Test Script
# Tests all V2 endpoints: Roles, Groups, and Role Bindings

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source test helpers
source "$SCRIPT_DIR/test_helpers.sh"

# Port configuration (can be overridden via environment)
RBAC_PORT=${RBAC_PORT:-8080}
API_BASE="http://localhost:${RBAC_PORT}/api/rbac/v2"

# Tenant ID configuration (optional - for multitenancy testing)
TENANT_ID=${TENANT_ID:-}

# Build headers array for curl
CURL_HEADERS=()
if [ -n "$TENANT_ID" ]; then
    CURL_HEADERS+=(-H "TENANT_ID: $TENANT_ID")
fi

echo "🧪 Testing Mini RBAC Go - Full V2 API"
echo "======================================"
if [ -n "$TENANT_ID" ]; then
    echo "🏢 Tenant ID: $TENANT_ID"
else
    echo "🏢 Tenant ID: <none> (using default null UUID)"
fi
echo ""

# Health check
echo "1️⃣  Health Check"
echo "   GET /health"
HEALTH_RESPONSE=$(curl -sf http://localhost:${RBAC_PORT}/health 2>&1) || {
    echo -e "${RED}❌ Server not responding. Is it running on localhost:${RBAC_PORT}?${NC}"
    exit 1
}
echo "$HEALTH_RESPONSE" | jq '.'
echo ""

# ============================================================================
# ROLES API
# ============================================================================
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📋 ROLES API"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "2️⃣  Create Role"
echo "   POST $API_BASE/roles"
ROLE_RESPONSE=$(safe_curl "Create Role" -X POST "$API_BASE/roles" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Inventory Manager",
    "description": "Manage inventory resources",
    "permissions": [
      {
        "application": "inventory",
        "resource_type": "hosts",
        "permission": "read"
      },
      {
        "application": "inventory",
        "resource_type": "hosts",
        "permission": "write"
      }
    ]
  }')

check_response "$ROLE_RESPONSE" "Create Role"
echo "$ROLE_RESPONSE" | jq '.'

ROLE_ID=$(extract_id "$ROLE_RESPONSE" "Create Role")
echo ""
echo -e "   ${GREEN}✓${NC} Created role with ID: $ROLE_ID"
echo ""

echo "3️⃣  Get Role by ID"
echo "   GET $API_BASE/roles/$ROLE_ID"
GET_ROLE_RESPONSE=$(safe_curl "Get Role" "${CURL_HEADERS[@]}" "$API_BASE/roles/$ROLE_ID")
check_response "$GET_ROLE_RESPONSE" "Get Role"
echo "$GET_ROLE_RESPONSE" | jq '.'
echo ""

echo "4️⃣  List Roles"
echo "   GET $API_BASE/roles"
LIST_ROLES_RESPONSE=$(safe_curl "List Roles" "${CURL_HEADERS[@]}" "$API_BASE/roles")
echo "$LIST_ROLES_RESPONSE" | jq '.'
echo ""

# ============================================================================
# GROUPS API
# ============================================================================
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "👥 GROUPS API"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "5️⃣  Create Group"
echo "   POST $API_BASE/groups"
GROUP_RESPONSE=$(curl -sf -X POST "$API_BASE/groups" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Engineering Team",
    "description": "Engineering department group"
  }' 2>&1) || handle_error "Create Group" "$GROUP_RESPONSE"

check_response "$GROUP_RESPONSE" "Create Group"
echo "$GROUP_RESPONSE" | jq '.'

GROUP_ID=$(extract_id "$GROUP_RESPONSE" "Create Group")
echo ""
echo -e "   ${GREEN}✓${NC} Created group with ID: $GROUP_ID"
echo ""

echo "6️⃣  Get Group by ID"
echo "   GET $API_BASE/groups/$GROUP_ID"
GET_GROUP_RESPONSE=$(curl -sf "${CURL_HEADERS[@]}" "$API_BASE/groups/$GROUP_ID" 2>&1) || handle_error "Get Group" "$GET_GROUP_RESPONSE"
check_response "$GET_GROUP_RESPONSE" "Get Group"
echo "$GET_GROUP_RESPONSE" | jq '.'
echo ""

echo "7️⃣  Add Principals to Group"
echo "   POST $API_BASE/groups/$GROUP_ID/principals"
ADD_PRINCIPALS_RESPONSE=$(curl -sf -X POST "$API_BASE/groups/$GROUP_ID/principals" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "principals": ["alice@example.com", "bob@example.com"]
  }' 2>&1) || handle_error "Add Principals" "$ADD_PRINCIPALS_RESPONSE"

check_response "$ADD_PRINCIPALS_RESPONSE" "Add Principals"
echo "$ADD_PRINCIPALS_RESPONSE" | jq '.'
echo ""

echo "8️⃣  Get Group with Principals"
echo "   GET $API_BASE/groups/$GROUP_ID"
GET_GROUP_WITH_PRINCIPALS=$(curl -sf "${CURL_HEADERS[@]}" "$API_BASE/groups/$GROUP_ID" 2>&1) || handle_error "Get Group with Principals" "$GET_GROUP_WITH_PRINCIPALS"
check_response "$GET_GROUP_WITH_PRINCIPALS" "Get Group with Principals"
echo "$GET_GROUP_WITH_PRINCIPALS" | jq '.'
echo ""

echo "9️⃣  List Groups"
echo "   GET $API_BASE/groups"
LIST_GROUPS_RESPONSE=$(curl -sf "${CURL_HEADERS[@]}" "$API_BASE/groups" 2>&1) || handle_error "List Groups" "$LIST_GROUPS_RESPONSE"
echo "$LIST_GROUPS_RESPONSE" | jq '.'
echo ""

# ============================================================================
# ROLE BINDINGS API
# ============================================================================
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔗 ROLE BINDINGS API"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "🔟 Batch Create Role Bindings"
echo "   POST $API_BASE/role-bindings/:batchCreate"
BINDING_RESPONSE=$(curl -sf -X POST "$API_BASE/role-bindings/:batchCreate" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d "{
    \"requests\": [
      {
        \"resource\": {
          \"id\": \"workspace-default\",
          \"type\": \"workspace\"
        },
        \"subject\": {
          \"id\": \"$GROUP_ID\",
          \"type\": \"group\"
        },
        \"role\": {
          \"id\": \"$ROLE_ID\"
        }
      }
    ]
  }" 2>&1) || handle_error "Batch Create Bindings" "$BINDING_RESPONSE"

check_response "$BINDING_RESPONSE" "Batch Create Bindings"
echo "$BINDING_RESPONSE" | jq '.'

BINDINGS_COUNT=$(echo "$BINDING_RESPONSE" | jq -r '.role_bindings | length')
echo ""
echo -e "   ${GREEN}✓${NC} Created $BINDINGS_COUNT role binding(s)"
echo ""

echo "1️⃣1️⃣  List Role Bindings"
echo "   GET $API_BASE/role-bindings"
LIST_BINDINGS_RESPONSE=$(curl -sf "${CURL_HEADERS[@]}" "$API_BASE/role-bindings" 2>&1) || handle_error "List Bindings" "$LIST_BINDINGS_RESPONSE"
echo "$LIST_BINDINGS_RESPONSE" | jq '.'
echo ""

# ============================================================================
# CLEANUP
# ============================================================================
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧹 CLEANUP"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "1️⃣2️⃣  List Bindings by Subject"
echo "   GET $API_BASE/role-bindings/by-subject?resource_type=workspace&resource_id=workspace-default"
BY_SUBJECT_RESPONSE=$(curl -sf "${CURL_HEADERS[@]}" "$API_BASE/role-bindings/by-subject?resource_type=workspace&resource_id=workspace-default" 2>&1) || handle_error "List by Subject" "$BY_SUBJECT_RESPONSE"
echo "$BY_SUBJECT_RESPONSE" | jq '.'
SUBJECTS_COUNT=$(echo "$BY_SUBJECT_RESPONSE" | jq -r '.data | length')
echo -e "   ${GREEN}✓${NC} Found bindings for $SUBJECTS_COUNT subject(s)"
echo ""

echo "1️⃣3️⃣  Update Bindings by Subject (remove all roles)"
echo "   PUT $API_BASE/role-bindings/by-subject?resource_type=workspace&resource_id=workspace-default&subject_type=group&subject_id=$GROUP_ID"
UPDATE_BINDING_RESPONSE=$(curl -sf -X PUT "$API_BASE/role-bindings/by-subject?resource_type=workspace&resource_id=workspace-default&subject_type=group&subject_id=$GROUP_ID" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "roles": []
  }' 2>&1) || handle_error "Update Bindings" "$UPDATE_BINDING_RESPONSE"
echo "$UPDATE_BINDING_RESPONSE" | jq '.'
echo -e "   ${GREEN}✓${NC} Removed all role bindings for subject"
echo ""

echo "1️⃣4️⃣  Remove Principals from Group"
echo "   DELETE $API_BASE/groups/$GROUP_ID/principals"
REMOVE_PRINCIPALS_RESPONSE=$(curl -sf -X DELETE "$API_BASE/groups/$GROUP_ID/principals" \
  "${CURL_HEADERS[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "principals": ["alice@example.com", "bob@example.com"]
  }' 2>&1) || handle_error "Remove Principals" "$REMOVE_PRINCIPALS_RESPONSE"

echo "$REMOVE_PRINCIPALS_RESPONSE" | jq '.'
echo -e "   ${GREEN}✓${NC} Principals removed successfully"
echo ""

echo "1️⃣5️⃣  Delete Group"
echo "   DELETE $API_BASE/groups/$GROUP_ID"
DELETE_GROUP_STATUS=$(curl -sf "${CURL_HEADERS[@]}" -X DELETE "$API_BASE/groups/$GROUP_ID" -w "%{http_code}" -o /dev/null 2>&1) || handle_error "Delete Group" "Failed with status: $DELETE_GROUP_STATUS"
echo -e "   ${GREEN}✓${NC} Status: $DELETE_GROUP_STATUS"
echo ""

echo "1️⃣6️⃣  Delete Role"
echo "   DELETE $API_BASE/roles/$ROLE_ID"
DELETE_ROLE_STATUS=$(curl -sf "${CURL_HEADERS[@]}" -X DELETE "$API_BASE/roles/$ROLE_ID" -w "%{http_code}" -o /dev/null 2>&1) || handle_error "Delete Role" "Failed with status: $DELETE_ROLE_STATUS"
echo -e "   ${GREEN}✓${NC} Status: $DELETE_ROLE_STATUS"
echo ""

echo -e "${GREEN}✅ Full API test complete!${NC}"
echo ""
echo -e "${GREEN}All 16 endpoints tested successfully!${NC}"
echo ""
echo "Note: This test requires:"
echo "  - Server running on localhost:${RBAC_PORT}"
echo "  - PostgreSQL with database"
echo "  - jq installed for JSON formatting"
echo ""
echo "Environment variables:"
echo "  - RBAC_PORT: Set to use a different port (default: 8080)"
echo "  - TENANT_ID: Set to test multitenancy (optional, uses null UUID if not set)"
echo ""
echo "Example: TENANT_ID=550e8400-e29b-41d4-a716-446655440000 ./scripts/test_full_api.sh"
