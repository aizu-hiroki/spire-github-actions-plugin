#!/bin/bash
set -euo pipefail

SPIRE_SERVER_ADDRESS="$1"
SPIRE_SERVER_PORT="$2"
TRUST_DOMAIN="$3"
AUDIENCE="${4:-spiffe://${TRUST_DOMAIN}}"

# Use audience default if empty string was passed
if [ -z "$AUDIENCE" ]; then
  AUDIENCE="spiffe://${TRUST_DOMAIN}"
fi

SVID_OUTPUT_DIR="/github/workspace/.spire-svid"
SOCKET="/tmp/spire-agent/public/api.sock"

echo "::group::Configure SPIRE agent"

cat > /tmp/agent.conf <<EOF
agent {
  server_address = "${SPIRE_SERVER_ADDRESS}"
  server_port    = "${SPIRE_SERVER_PORT}"
  trust_domain   = "${TRUST_DOMAIN}"
  data_dir       = "/tmp/spire-agent-data"
  log_level      = "INFO"
  insecure_bootstrap = true
}

plugins {
  NodeAttestor "github_actions" {
    plugin_cmd = "/usr/local/bin/nodeattestor-agent"
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

mkdir -p /tmp/spire-agent-data
echo "Agent config:"
cat /tmp/agent.conf
echo "::endgroup::"

echo "::group::Start SPIRE agent and attest"

spire-agent run -config /tmp/agent.conf &
AGENT_PID=$!

# Wait for node attestation
for i in $(seq 1 30); do
  if spire-agent healthcheck -socketPath "$SOCKET" 2>/dev/null; then
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
  if spire-agent api fetch x509 -socketPath "$SOCKET" -write "$SVID_OUTPUT_DIR" 2>/dev/null; then
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
SPIFFE_ID=$(spire-agent api fetch x509 -socketPath "$SOCKET" 2>/dev/null | grep "SPIFFE ID" | head -1 | awk '{print $NF}')
echo "SPIFFE ID: ${SPIFFE_ID}"

# Display SVID details
spire-agent api fetch x509 -socketPath "$SOCKET"

echo "::endgroup::"

# Stop agent
kill $AGENT_PID 2>/dev/null || true

# Set outputs (paths relative to host workspace)
WORKSPACE="${GITHUB_WORKSPACE:-.}"
echo "spiffe-id=${SPIFFE_ID}" >> "$GITHUB_OUTPUT"
echo "svid-cert=${WORKSPACE}/.spire-svid/svid.0.pem" >> "$GITHUB_OUTPUT"
echo "svid-key=${WORKSPACE}/.spire-svid/svid.0.key" >> "$GITHUB_OUTPUT"
echo "bundle=${WORKSPACE}/.spire-svid/bundle.0.pem" >> "$GITHUB_OUTPUT"

echo "Done - SVID files available at ${WORKSPACE}/.spire-svid/"
