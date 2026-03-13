#!/usr/bin/env

# Create an admin role.
ROLE_ID="$(curl -XPOST http://localhost:8085/api/rbac/v2/roles/ \
  -H "Content-Type: application/json" \
  -d '{
      "name": "ACM Admin role",
      "description": "All privileges role",
      "permissions": [
        {
          "application": "acm",
          "resource_type": "cluster",
          "permission": "read"
        },
        {
          "application": "acm",
          "resource_type": "cluster",
          "permission": "write"
        }
      ]
    }' | jq '.id' -r)"

# Create a group to hold the admin accounts.
GROUP_ID="$(curl -XPOST http://localhost:8085/api/rbac/v2/groups/ \
  -H "Content-Type: application/json" \
  -d '{
        "name": "admin group"
      }' | jq '.id' -r)"


# Add users to admin group
curl -XPOST "http://localhost:8085/api/rbac/v2/groups/${GROUP_ID}/principals" \
  -H "Content-Type: application/json" \
  -d '{
        "principals": [ "admin", "jdoe" ]
      }'

# Create a role binding
DEFAULT_WORKSPACE_ID="$(curl "http://localhost:8085/api/rbac/v2/workspaces?type=default" | jq ".data[0].id" -r)"
curl -XPUT "http://localhost:8085/api/rbac/v2/role-bindings/by-subject?resource_type=rbac/workspace&resource_id=${DEFAULT_WORKSPACE_ID}&subject_type=group&subject_id=${GROUP_ID}" \
  -H "Content-Type: application/json" \
  -d "{
        \"roles\": [ { \"id\": \"${ROLE_ID}\" } ]
      }"
