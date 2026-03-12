#!/usr/bin/env


# Create an admin role.
curl -XPOST http://localhost:8085/api/rbac/v2/roles/ \
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
    }'

# Create a group to hold the admin accounts.
ROLE_ID="$(curl -XPOST http://localhost:8085/api/rbac/v2/groups/ \
  -H "Content-Type: application/json" \
  -d '{
        "name": "admin group"
      }' | jq '.id' -r)"


# Add users to said group

curl -XPOST "http://localhost:8085/api/rbac/v2/groups/${ROLE_ID}/principals" \
  -H "Content-Type: application/json" \
  -d '{
        "principals": [ "admin", "jdoe" ]
      }'
