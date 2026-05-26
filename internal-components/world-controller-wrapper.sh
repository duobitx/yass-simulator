#!/usr/bin/env bash

if [ -v POD_IP ]; then
  echo "POD IP: ${POD_IP}"
fi

export IFACE=$(ip route | awk '/default/ {print $5; exit}')
echo "Network interface -- ${IFACE}"


if [ "${DISABLE_NETWORKING_MANIPULATION:-false}" = "true" ]; then
  echo "DISABLE_NETWORKING_MANIPULATION=true -- skipping traffic.sh"
else
  ./traffic.sh "${IFACE}"
fi
./world-controller
