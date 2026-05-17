#!/bin/sh
set -e

log() { echo "[start] $*"; }

# ── Bootstrap step-ca trust ───────────────────────────────────────────────────
log "Bootstrapping step-ca CA trust"
step ca bootstrap \
  --ca-url="${STEP_CA_URL}" \
  --fingerprint="${STEP_CA_FINGERPRINT}" \
  --force

mkdir -p /etc/certs

# ── CA certificate ────────────────────────────────────────────────────────────
log "Fetching CA certificate"
step ca root --force /etc/certs/ca.crt
chmod 644 /etc/certs/ca.crt

# ── Client certificate (presented to backend services over mTLS) ──────────────
# sys-ai is a worker — it makes outbound mTLS connections, not inbound.
# The client cert CN=sys-ai must be in SERVER_TLS_ALLOWED_CNS of every service
# it calls (currently: sys-backend-restaurant-n-shopping).
log "Enrolling client certificate"
EXTRA_SAN_ARG=""
if [ -n "${STEP_CERT_EXTRA_SAN:-}" ]; then
  EXTRA_SAN_ARG="--san=${STEP_CERT_EXTRA_SAN}"
fi
# shellcheck disable=SC2086
step ca certificate "sys-ai" \
  /etc/certs/client.crt \
  /etc/certs/client.key \
  --provisioner="${STEP_PROVISIONER:-service-accounts}" \
  --provisioner-password-file=/etc/step/provisioner.password \
  --san=sys-ai \
  --san=sys-ai.serveyourstay.com \
  --san=localhost \
  ${EXTRA_SAN_ARG} \
  --not-after=168h \
  --force
chmod 600 /etc/certs/client.key

# ── Renewal daemon ────────────────────────────────────────────────────────────
log "Starting client cert renewal daemon"
step ca renew --daemon --renew-period=48h \
  /etc/certs/client.crt /etc/certs/client.key &

# ── Cert expiry watchdog ──────────────────────────────────────────────────────
(
  while true; do
    sleep 15m
    if ! step certificate verify /etc/certs/client.crt \
        --roots /etc/certs/ca.crt 2>/dev/null; then
      log "Certificate invalid or expired — exiting to trigger container restart and re-enrollment"
      kill -TERM 1 2>/dev/null
      break
    fi
  done
) &

# ── Service ───────────────────────────────────────────────────────────────────
log "Starting sys-ai reviewer"
exec /app/server
