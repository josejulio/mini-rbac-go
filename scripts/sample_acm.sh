#!/usr/bin/env sh

# This assumes the following services are running an accepting unauthenticated requests
# - mini-rbac: running on localhost:8085 and configured to talk with relations-api
# - inventory-api: running on localhost:9081 (grpc) and configured to connect with relations-api
# - relations api using this schema: https://github.com/josejulio/rbac-config-acm/commit/0e37ec8114764e4062ea0a76ceff3342954419f7
# Make the needed changes to accommodate to your running services.


# Create an admin role.
ROLE_ID="$(curl -XPOST http://localhost:8085/api/rbac/v2/roles/ \
  -H "Content-Type: application/json" \
  -d '{
      "name": "ACM Admin role",
      "description": "All privileges role",
      "permissions": [
        {
          "application": "acm",
          "resource_type": "k8s_cluster",
          "permission": "read"
        },
        {
          "application": "acm",
          "resource_type": "k8s_cluster",
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

# Create a role binding (default workspace, admin group, admin role)
DEFAULT_WORKSPACE_ID="$(curl "http://localhost:8085/api/rbac/v2/workspaces?type=default" | jq ".data[0].id" -r)"
curl -XPUT "http://localhost:8085/api/rbac/v2/role-bindings/by-subject?resource_type=rbac/workspace&resource_id=${DEFAULT_WORKSPACE_ID}&subject_type=group&subject_id=${GROUP_ID}" \
  -H "Content-Type: application/json" \
  -d "{
        \"roles\": [ { \"id\": \"${ROLE_ID}\" } ]
      }"

# Create a engineering workspace under the root workspace
ROOT_WORKSPACE_ID="$(curl "http://localhost:8085/api/rbac/v2/workspaces?type=root" | jq ".data[0].id" -r)"
ENGINEERING_WP="$(curl -XPOST "http://localhost:8085/api/rbac/v2/workspaces" \
  -H "Content-Type: application/json" \
  -d "{
        \"name\": \"Engineering\",
        \"parent_id\": \"${ROOT_WORKSPACE_ID}\"
      }" | jq '.id' -r)"

# Report a k8s_cluster (acm) and link it to the root workspace
grpcurl -plaintext -d "{
            \"type\": \"k8s_cluster\",
            \"reporterType\": \"acm\",
            \"reporterInstanceId\": \"1234\",
            \"representations\": {
              \"metadata\": {
                \"localResourceId\": \"cluster-1\",
                \"apiHref\": \"http://somewhere\",
                \"reporterVersion\": \"1.0.0\"
              },
              \"common\": {
                \"workspace_id\": \"${ENGINEERING_WP}\"
              },
              \"reporter\": {
                \"external_cluster_id\": \"cluster-1\",
                \"cluster_status\": \"READY\",
                \"cluster_reason\": \"running\",
                \"kube_version\": \"1.0.0\",
                \"kube_vendor\": \"OPENSHIFT\",
                \"vendor_version\": \"3.0.0\",
                \"cloud_platform\": \"BAREMETAL_UPI\",
                \"nodes\": []
              }
            }
          }" localhost:9081 kessel.inventory.v1beta2.KesselInventoryService.ReportResource

# Check access on the resource itself
grpcurl -plaintext \
  -d '{
       "object": {
         "resource_type": "k8s_cluster", "resource_id": "cluster-1", "reporter": {"type": "acm"}
       },
       "relation": "view",
       "subject": {
         "resource": {"resource_type": "principal", "resource_id": "admin", "reporter": {"type": "rbac"}}
       }
     }' \
  localhost:9081 kessel.inventory.v1beta2.KesselInventoryService.Check

## Should be denied - as admin group has permissions over the default workspace
# Grant access to engineering workspace
curl -XPUT "http://localhost:8085/api/rbac/v2/role-bindings/by-subject?resource_type=rbac/workspace&resource_id=${ENGINEERING_WP}&subject_type=group&subject_id=${GROUP_ID}" \
  -H "Content-Type: application/json" \
  -d "{
        \"roles\": [ { \"id\": \"${ROLE_ID}\" } ]
      }"

## Wait a couple seconds and try again
sleep 2

grpcurl -plaintext \
  -d '{
       "object": {
         "resource_type": "k8s_cluster", "resource_id": "cluster-1", "reporter": {"type": "acm"}
       },
       "relation": "view",
       "subject": {
         "resource": {"resource_type": "principal", "resource_id": "admin", "reporter": {"type": "rbac"}}
       }
     }' \
  localhost:9081 kessel.inventory.v1beta2.KesselInventoryService.Check

# Check access for alice (not in admin group)

grpcurl -plaintext \
  -d '{
       "object": {
         "resource_type": "k8s_cluster", "resource_id": "cluster-1", "reporter": {"type": "acm"}
       },
       "relation": "view",
       "subject": {
         "resource": {"resource_type": "principal", "resource_id": "alice", "reporter": {"type": "rbac"}}
       }
     }' \
  localhost:9081 kessel.inventory.v1beta2.KesselInventoryService.Check

# Grant access to alice
curl -XPUT "http://localhost:8085/api/rbac/v2/role-bindings/by-subject?resource_type=rbac/workspace&resource_id=${ENGINEERING_WP}&subject_type=user&subject_id=alice" \
  -H "Content-Type: application/json" \
  -d "{
        \"roles\": [ { \"id\": \"${ROLE_ID}\" } ]
      }"

sleep 2

grpcurl -plaintext \
  -d '{
       "object": {
         "resource_type": "k8s_cluster", "resource_id": "cluster-1", "reporter": {"type": "acm"}
       },
       "relation": "view",
       "subject": {
         "resource": {"resource_type": "principal", "resource_id": "alice", "reporter": {"type": "rbac"}}
       }
     }' \
  localhost:9081 kessel.inventory.v1beta2.KesselInventoryService.Check
