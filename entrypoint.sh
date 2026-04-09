#!/bin/bash
set -euo pipefail

SPIRE_SERVER_ADDRESS="$INPUT_SPIRE_SERVER_ADDRESS"
SPIRE_SERVER_PORT="$INPUT_SPIRE_SERVER_PORT"
TRUST_DOMAIN="$INPUT_TRUST_DOMAIN"
AUDIENCE="${INPUT_AUDIENCE:-spiffe://${TRUST_DOMAIN}}"
JWT_AUDIENCES="${INPUT_JWT_AUDIENCES:-}"

# Use audience default if empty string was passed
if [ -z "$AUDIENCE" ]; then
  AUDIENCE="spiffe://${TRUST_DOMAIN}"
fi

SVID_OUTPUT_DIR="${GITHUB_WORKSPACE}/.spire-svid"
SPIRE_DIR="${GITHUB_WORKSPACE}/.spire"
SOCKET="${SPIRE_DIR}/agent/public/api.sock"

echo "::group::Configure SPIRE agent"

mkdir -p "${SPIRE_DIR}/data"

cat > "${SPIRE_DIR}/agent.conf" <<EOF
agent {
  server_address = "${SPIRE_SERVER_ADDRESS}"
  server_port    = "${SPIRE_SERVER_PORT}"
  trust_domain   = "${TRUST_DOMAIN}"
  data_dir       = "${SPIRE_DIR}/data"
  socket_path    = "${SOCKET}"
  log_level      = "INFO"
  insecure_bootstrap = true
}

plugins {
  NodeAttestor "github_actions" {
    plugin_cmd = "${NODEATTESTOR_AGENT_PATH}"
    plugin_data {
      audience = "${AUDIENCE}"
    }
  }

  KeyManager "memory" {
    plugin_data {}
  }

  WorkloadAttestor "unix" {
    plugin_data {}
  }
}
EOF

echo "Agent config:"
cat "${SPIRE_DIR}/agent.conf"
echo "::endgroup::"

echo "::group::Start SPIRE agent and attest"

"${SPIRE_AGENT_PATH}" run -config "${SPIRE_DIR}/agent.conf" &
AGENT_PID=$!

# Wait for node attestation
for i in $(seq 1 30); do
  if "${SPIRE_AGENT_PATH}" healthcheck -socketPath "$SOCKET" 2>/dev/null; then
    echo "Node attestation successful"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "::error::Node attestation failed within timeout"
    kill $AGENT_PID 2>/dev/null || true
    exit 1
  fi
  sleep 2
done

echo "::endgroup::"

echo "::group::Fetch X.509 SVID"

mkdir -p "$SVID_OUTPUT_DIR"

for i in $(seq 1 20); do
  if "${SPIRE_AGENT_PATH}" api fetch x509 -socketPath "$SOCKET" -write "$SVID_OUTPUT_DIR" 2>/dev/null; then
    echo "SVID written to $SVID_OUTPUT_DIR"
    break
  fi
  if [ "$i" -eq 20 ]; then
    echo "::error::Failed to fetch SVID within timeout"
    kill $AGENT_PID 2>/dev/null || true
    exit 1
  fi
  sleep 2
done

# Get SPIFFE ID
SPIFFE_ID=$("${SPIRE_AGENT_PATH}" api fetch x509 -socketPath "$SOCKET" 2>/dev/null | grep "SPIFFE ID" | head -1 | awk '{print $NF}')
echo "SPIFFE ID: ${SPIFFE_ID}"

# Display SVID details
"${SPIRE_AGENT_PATH}" api fetch x509 -socketPath "$SOCKET"

echo "::endgroup::"

# Fetch JWT-SVIDs if audiences specified
JWT_JSON="{}"
if [ -n "$JWT_AUDIENCES" ]; then
  echo "::group::Fetch JWT-SVIDs"

  IFS=',' read -ra AUDIENCES <<< "$JWT_AUDIENCES"
  for aud in "${AUDIENCES[@]}"; do
    aud=$(echo "$aud" | xargs)  # trim whitespace
    if [ -z "$aud" ]; then
      continue
    fi
    echo "Fetching JWT-SVID for audience: ${aud}"
    JWT_TOKEN=""
    for i in $(seq 1 10); do
      JWT_OUTPUT=$("${SPIRE_AGENT_PATH}" api fetch jwt -audience "$aud" -socketPath "$SOCKET" 2>/dev/null || true)
      JWT_TOKEN=$(echo "$JWT_OUTPUT" | grep -A1 "token(" | tail -1 | xargs || true)
      if [ -n "$JWT_TOKEN" ] && [ "$JWT_TOKEN" != ")" ]; then
        break
      fi
      if [ "$i" -eq 10 ]; then
        echo "::warning::Failed to fetch JWT-SVID for audience: ${aud}"
      fi
      sleep 2
    done
    if [ -n "$JWT_TOKEN" ] && [ "$JWT_TOKEN" != ")" ]; then
      # Escape audience for JSON key
      ESCAPED_AUD=$(echo "$aud" | sed 's/"/\\"/g')
      if [ "$JWT_JSON" = "{}" ]; then
        JWT_JSON="{\"${ESCAPED_AUD}\":\"${JWT_TOKEN}\"}"
      else
        JWT_JSON="${JWT_JSON%\}},\"${ESCAPED_AUD}\":\"${JWT_TOKEN}\"}"
      fi
      echo "✓ JWT-SVID fetched for: ${aud}"
    fi
  done

  echo "::endgroup::"
fi

# Stop agent
kill $AGENT_PID 2>/dev/null || true

# Set outputs
echo "spiffe-id=${SPIFFE_ID}" >> "$GITHUB_OUTPUT"
echo "svid-cert=.spire-svid/svid.0.pem" >> "$GITHUB_OUTPUT"
echo "svid-key=.spire-svid/svid.0.key" >> "$GITHUB_OUTPUT"
echo "bundle=.spire-svid/bundle.0.pem" >> "$GITHUB_OUTPUT"
echo "jwt-svids=${JWT_JSON}" >> "$GITHUB_OUTPUT"

echo "Done - SVID files written to workspace/.spire-svid/"
