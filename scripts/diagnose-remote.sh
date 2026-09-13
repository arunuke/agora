#!/usr/bin/env bash
#
# Host-side half of `make diagnose-host`, streamed over SSH with `bash -s`.
#
# Lives in a script rather than inline in the Makefile because the IMDS calls
# need nested quoting that is unreadable (and easy to get subtly wrong) once
# make, ssh and sh have each had a turn at escaping it.
#
# Argument: the published host port.
PORT="${1:-8080}"
log() { printf '   %s\n' "$*"; }

echo
echo "3. is the container running?"
docker ps -a --filter name=agora --format '   {{.Names}}  {{.Status}}  ports={{.Ports}}' 2>/dev/null \
  || log "docker ps failed — is docker installed and is this user in the docker group?"

echo
echo "4. healthy INSIDE the container?"
if docker exec agora curl -sf --max-time 3 http://localhost:8080/healthz >/dev/null 2>&1; then
  log "OK — the application is fine"
else
  log "FAIL — the application itself is unhealthy"
fi

echo
echo "5. reachable on the HOST's own loopback? (tests port publishing)"
if curl -sf --max-time 3 "http://localhost:$PORT/healthz" >/dev/null 2>&1; then
  log "OK — published correctly on the host"
else
  log "FAIL — not published; check the ports: mapping in docker-compose.yml"
fi

echo
echo "6. what is listening on $PORT?"
ss -tlnp 2>/dev/null | grep ":$PORT" | sed 's/^/   /' || log "nothing is listening on $PORT"

echo
echo "7. local firewall"
if command -v ufw >/dev/null 2>&1; then
  sudo -n ufw status 2>/dev/null | head -4 | sed 's/^/   /' || log "ufw present (status needs sudo)"
else
  log "ufw not installed"
fi
iptables_rules=$(sudo -n iptables -S INPUT 2>/dev/null | grep -c DROP || true)
[ "${iptables_rules:-0}" -gt 0 ] && log "note: $iptables_rules DROP rule(s) in iptables INPUT"

echo
echo "8. which security groups are ACTUALLY attached to this instance?"
# This is the check that resolves the most common case: the rule was added to a
# security group the instance does not use. Instance metadata is authoritative —
# it reports what is attached, not what you believe you edited.
TOKEN=$(curl -sX PUT "http://169.254.169.254/latest/api/token" \
          -H "X-aws-ec2-metadata-token-ttl-seconds: 60" --max-time 2 2>/dev/null || true)
imds() {
  if [ -n "$TOKEN" ]; then
    curl -s -H "X-aws-ec2-metadata-token: $TOKEN" --max-time 2 "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null
  else
    curl -s --max-time 2 "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null
  fi
}
SGS=$(imds security-groups)
if [ -z "$SGS" ]; then
  log "instance metadata unavailable (not EC2, or IMDS is disabled)"
else
  echo "$SGS" | sed 's/^/   security group: /'
  MAC=$(imds network/interfaces/macs/ | head -n1)
  [ -n "$MAC" ] && {
    log "vpc:    $(imds "network/interfaces/macs/${MAC}vpc-id")"
    log "subnet: $(imds "network/interfaces/macs/${MAC}subnet-id")"
    ids=$(imds "network/interfaces/macs/${MAC}security-group-ids")
    [ -n "$ids" ] && echo "$ids" | sed 's/^/   sg id:  /'
  }
  log "compare the above with the security group you edited — if they differ,"
  log "that is the answer."
fi
