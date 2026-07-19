#!/usr/bin/env bash

# Create a compact, deterministic fixture in the pre-upgrade Homebox instance.
# The API calls in this script are compatible with both v0.26.2 and current
# Homebox. Authentication tokens remain in memory and are never written out.

set -Eeuo pipefail
umask 077

HOMEBOX_URL="${HOMEBOX_URL:-http://localhost:7745}"
API_URL="${HOMEBOX_URL}/api/v1"
TEST_DATA_FILE="${TEST_DATA_FILE:-/tmp/test-users.json}"
TEST_PASSWORD="TestPassword123!"

GROUP1_OWNER_EMAIL="upgrade-owner@homebox.test"
GROUP1_MEMBER_EMAIL="upgrade-member@homebox.test"
GROUP2_OWNER_EMAIL="upgrade-isolated@homebox.test"

GROUP1_LOCATION_NAME="UPGRADE G1 Location"
GROUP1_LOCATION_DESCRIPTION="Location created before the upgrade"
GROUP1_TAG_NAME="UPGRADE G1 Tag"
GROUP1_TAG_DESCRIPTION="Tag created before the upgrade"
GROUP1_TAG_COLOR="#3366CC"
GROUP1_ITEM_NAME="UPGRADE G1 Laptop"
GROUP1_ITEM_DESCRIPTION="Entity created before the upgrade"
GROUP1_NOTIFIER_NAME="UPGRADE G1 Notifier"
GROUP1_NOTIFIER_URL="discord://homeboxupgradetest"
GROUP1_ATTACHMENT_TITLE="upgrade-laptop-receipt.txt"
GROUP2_ITEM_NAME="UPGRADE G2 Monitor"
GROUP2_ITEM_DESCRIPTION="Isolated entity created before the upgrade"

attachment_file=""

cleanup() {
  if [[ -n "$attachment_file" && -f "$attachment_file" ]]; then
    rm -f -- "$attachment_file"
  fi
}
trap cleanup EXIT

print_error_body() {
  local response_file=$1

  if jq -e . "$response_file" >/dev/null 2>&1; then
    jq -c 'del(.token, .attachmentToken, .password)' "$response_file" >&2
  else
    head -c 1000 "$response_file" >&2
    echo >&2
  fi
}

api_request() {
  local method=$1
  local endpoint=$2
  local expected_status=$3
  local data=${4:-}
  local token=${5:-}
  local response_file headers_file status content_type
  local -a curl_args

  response_file=$(mktemp)
  headers_file=$(mktemp)
  curl_args=(
    --silent
    --show-error
    --output "$response_file"
    --dump-header "$headers_file"
    --write-out "%{http_code}"
    --request "$method"
  )

  if [[ -n "$data" ]]; then
    curl_args+=(--header "Content-Type: application/json" --data "$data")
  fi
  if [[ -n "$token" ]]; then
    curl_args+=(--header "Authorization: $token")
  fi

  if ! status=$(curl "${curl_args[@]}" "${API_URL}${endpoint}"); then
    echo "API request failed: ${method} ${endpoint}" >&2
    print_error_body "$response_file"
    rm -f -- "$response_file" "$headers_file"
    return 1
  fi

  if [[ "$status" != "$expected_status" ]]; then
    echo "API request returned HTTP ${status}; expected ${expected_status}: ${method} ${endpoint}" >&2
    print_error_body "$response_file"
    rm -f -- "$response_file" "$headers_file"
    return 1
  fi

  if [[ "$expected_status" != "204" ]]; then
    content_type=$(
      awk 'tolower($1) == "content-type:" { print tolower($2) }' "$headers_file" |
        tail -n 1 |
        tr -d '\r'
    )
    if [[ "$content_type" != application/json* ]]; then
      echo "API request returned non-JSON content type '${content_type}': ${method} ${endpoint}" >&2
      rm -f -- "$response_file" "$headers_file"
      return 1
    fi
    if ! jq -e . "$response_file" >/dev/null; then
      echo "API request returned invalid JSON: ${method} ${endpoint}" >&2
      rm -f -- "$response_file" "$headers_file"
      return 1
    fi
    cat "$response_file"
  fi

  rm -f -- "$response_file" "$headers_file"
}

require_json_value() {
  local json=$1
  local filter=$2
  local description=$3
  local value

  if ! value=$(jq -er "$filter | select(type == \"string\" and length > 0)" <<<"$json"); then
    echo "Missing ${description} in API response" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

register_user() {
  local email=$1
  local name=$2
  local invitation_token=${3:-}
  local payload

  payload=$(
    jq -n \
      --arg email "$email" \
      --arg name "$name" \
      --arg password "$TEST_PASSWORD" \
      --arg token "$invitation_token" \
      '{email: $email, name: $name, password: $password}
       + if $token == "" then {} else {token: $token} end'
  )

  api_request POST "/users/register" 204 "$payload" >/dev/null
  echo "Registered ${email}"
}

login_user() {
  local email=$1
  local payload response token

  payload=$(
    jq -n \
      --arg username "$email" \
      --arg password "$TEST_PASSWORD" \
      '{username: $username, password: $password, stayLoggedIn: false}'
  )
  response=$(api_request POST "/users/login" 200 "$payload")
  token=$(require_json_value "$response" '.token' "login token")
  if [[ "$token" != "Bearer "* ]]; then
    echo "Login token for ${email} did not include the expected Bearer prefix" >&2
    return 1
  fi
  printf '%s\n' "$token"
}

sha256_file() {
  local path=$1

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
  else
    shasum -a 256 "$path" | awk '{print $1}'
  fi
}

echo "Creating pre-upgrade fixture at ${HOMEBOX_URL}"

register_user "$GROUP1_OWNER_EMAIL" "Upgrade Owner"
group1_owner_token=$(login_user "$GROUP1_OWNER_EMAIL")

invitation_payload=$(jq -n '{uses: 1}')
invitation_response=$(api_request POST "/groups/invitations" 201 "$invitation_payload" "$group1_owner_token")
invitation_token=$(require_json_value "$invitation_response" '.token' "group invitation token")

register_user "$GROUP1_MEMBER_EMAIL" "Upgrade Member" "$invitation_token"
group1_member_token=$(login_user "$GROUP1_MEMBER_EMAIL")

register_user "$GROUP2_OWNER_EMAIL" "Upgrade Isolated Owner"
group2_owner_token=$(login_user "$GROUP2_OWNER_EMAIL")

# Confirm every account has a working authenticated session before creating data.
api_request GET "/users/self" 200 "" "$group1_owner_token" >/dev/null
api_request GET "/users/self" 200 "" "$group1_member_token" >/dev/null
api_request GET "/users/self" 200 "" "$group2_owner_token" >/dev/null

entity_types=$(api_request GET "/entity-types" 200 "" "$group1_owner_token")
group1_location_type_id=$(
  require_json_value "$entity_types" '[.[] | select(.isLocation == true)][0].id' "group 1 location entity type"
)

location_payload=$(
  jq -n \
    --arg name "$GROUP1_LOCATION_NAME" \
    --arg description "$GROUP1_LOCATION_DESCRIPTION" \
    --arg entity_type_id "$group1_location_type_id" \
    '{
      name: $name,
      description: $description,
      quantity: 1,
      entityTypeId: $entity_type_id,
      tagIds: []
    }'
)
location_response=$(api_request POST "/entities" 201 "$location_payload" "$group1_owner_token")
group1_location_id=$(require_json_value "$location_response" '.id' "group 1 location ID")

tag_payload=$(
  jq -n \
    --arg name "$GROUP1_TAG_NAME" \
    --arg description "$GROUP1_TAG_DESCRIPTION" \
    --arg color "$GROUP1_TAG_COLOR" \
    '{name: $name, description: $description, color: $color, icon: ""}'
)
tag_response=$(api_request POST "/tags" 201 "$tag_payload" "$group1_owner_token")
group1_tag_id=$(require_json_value "$tag_response" '.id' "group 1 tag ID")

# Omitting entityTypeId lets both v0.26.2 and current Homebox resolve the
# group's default Item type, including groups where it has not yet been used.
group1_item_payload=$(
  jq -n \
    --arg name "$GROUP1_ITEM_NAME" \
    --arg description "$GROUP1_ITEM_DESCRIPTION" \
    --arg parent_id "$group1_location_id" \
    --arg tag_id "$group1_tag_id" \
    '{
      name: $name,
      description: $description,
      quantity: 2,
      parentId: $parent_id,
      tagIds: [$tag_id]
    }'
)
group1_item_response=$(api_request POST "/entities" 201 "$group1_item_payload" "$group1_owner_token")
group1_item_id=$(require_json_value "$group1_item_response" '.id' "group 1 item ID")

notifier_payload=$(
  jq -n \
    --arg name "$GROUP1_NOTIFIER_NAME" \
    --arg url "$GROUP1_NOTIFIER_URL" \
    '{name: $name, url: $url, isActive: true}'
)
notifier_response=$(api_request POST "/notifiers" 201 "$notifier_payload" "$group1_owner_token")
group1_notifier_id=$(require_json_value "$notifier_response" '.id' "group 1 notifier ID")

attachment_file=$(mktemp)
printf '%s\n' "homebox-upgrade-attachment-marker-v1" >"$attachment_file"
attachment_sha256=$(sha256_file "$attachment_file")
attachment_response_file=$(mktemp)
attachment_headers_file=$(mktemp)
attachment_status=$(
  curl \
    --silent \
    --show-error \
    --output "$attachment_response_file" \
    --dump-header "$attachment_headers_file" \
    --write-out "%{http_code}" \
    --request POST \
    --header "Authorization: ${group1_owner_token}" \
    --form "file=@${attachment_file};type=text/plain" \
    --form "name=${GROUP1_ATTACHMENT_TITLE}" \
    --form "type=attachment" \
    --form "primary=false" \
    "${API_URL}/entities/${group1_item_id}/attachments"
)
if [[ "$attachment_status" != "201" ]]; then
  echo "Attachment upload returned HTTP ${attachment_status}; expected 201" >&2
  print_error_body "$attachment_response_file"
  rm -f -- "$attachment_response_file" "$attachment_headers_file"
  exit 1
fi
attachment_response=$(<"$attachment_response_file")
rm -f -- "$attachment_response_file" "$attachment_headers_file"
group1_attachment_id=$(
  require_json_value \
    "$attachment_response" \
    ".attachments[] | select(.title == \"${GROUP1_ATTACHMENT_TITLE}\") | .id" \
    "group 1 attachment ID"
)

group2_item_payload=$(
  jq -n \
    --arg name "$GROUP2_ITEM_NAME" \
    --arg description "$GROUP2_ITEM_DESCRIPTION" \
    '{name: $name, description: $description, quantity: 1, tagIds: []}'
)
group2_item_response=$(api_request POST "/entities" 201 "$group2_item_payload" "$group2_owner_token")
group2_item_id=$(require_json_value "$group2_item_response" '.id' "group 2 item ID")

mkdir -p -- "$(dirname "$TEST_DATA_FILE")"
test_data_temp=$(mktemp "${TEST_DATA_FILE}.XXXXXX")
jq -n \
  --arg password "$TEST_PASSWORD" \
  --arg group1_owner_email "$GROUP1_OWNER_EMAIL" \
  --arg group1_member_email "$GROUP1_MEMBER_EMAIL" \
  --arg group2_owner_email "$GROUP2_OWNER_EMAIL" \
  --arg group1_location_id "$group1_location_id" \
  --arg group1_location_name "$GROUP1_LOCATION_NAME" \
  --arg group1_location_description "$GROUP1_LOCATION_DESCRIPTION" \
  --arg group1_tag_id "$group1_tag_id" \
  --arg group1_tag_name "$GROUP1_TAG_NAME" \
  --arg group1_tag_description "$GROUP1_TAG_DESCRIPTION" \
  --arg group1_tag_color "$GROUP1_TAG_COLOR" \
  --arg group1_item_id "$group1_item_id" \
  --arg group1_item_name "$GROUP1_ITEM_NAME" \
  --arg group1_item_description "$GROUP1_ITEM_DESCRIPTION" \
  --arg group1_notifier_id "$group1_notifier_id" \
  --arg group1_notifier_name "$GROUP1_NOTIFIER_NAME" \
  --arg group1_notifier_url "$GROUP1_NOTIFIER_URL" \
  --arg group1_attachment_id "$group1_attachment_id" \
  --arg group1_attachment_title "$GROUP1_ATTACHMENT_TITLE" \
  --arg group1_attachment_sha256 "$attachment_sha256" \
  --arg group2_item_id "$group2_item_id" \
  --arg group2_item_name "$GROUP2_ITEM_NAME" \
  --arg group2_item_description "$GROUP2_ITEM_DESCRIPTION" \
  '{
    users: [
      {
        key: "group1Owner",
        email: $group1_owner_email,
        password: $password,
        group: "group1",
        role: "owner"
      },
      {
        key: "group1Member",
        email: $group1_member_email,
        password: $password,
        group: "group1",
        role: "member"
      },
      {
        key: "group2Owner",
        email: $group2_owner_email,
        password: $password,
        group: "group2",
        role: "owner"
      }
    ],
    groups: {
      group1: {
        location: {
          id: $group1_location_id,
          name: $group1_location_name,
          description: $group1_location_description
        },
        tag: {
          id: $group1_tag_id,
          name: $group1_tag_name,
          description: $group1_tag_description,
          color: $group1_tag_color
        },
        item: {
          id: $group1_item_id,
          name: $group1_item_name,
          description: $group1_item_description,
          quantity: 2,
          locationId: $group1_location_id,
          tagId: $group1_tag_id
        },
        notifier: {
          id: $group1_notifier_id,
          name: $group1_notifier_name,
          url: $group1_notifier_url
        },
        attachment: {
          id: $group1_attachment_id,
          entityId: $group1_item_id,
          title: $group1_attachment_title,
          type: "attachment",
          sha256: $group1_attachment_sha256
        }
      },
      group2: {
        item: {
          id: $group2_item_id,
          name: $group2_item_name,
          description: $group2_item_description,
          quantity: 1
        }
      }
    }
  }' >"$test_data_temp"
mv -- "$test_data_temp" "$TEST_DATA_FILE"

echo "Pre-upgrade fixture created successfully"
echo "  users: 3 across 2 isolated groups"
echo "  group 1: 1 location, 1 tag, 1 item, 1 notifier, 1 attachment"
echo "  group 2: 1 isolated item"
echo "  verification metadata: ${TEST_DATA_FILE} (credentials and IDs only; no tokens)"
