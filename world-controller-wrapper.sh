#!/usr/bin/env bash

ip route # TODO debug remove

if [ -v POD_IP ]; then
  echo "POD IP: ${POD_IP}"
fi

export IFACE=$(ip route | awk '/default/ {print $5; exit}')
echo "Network interface -- ${IFACE}"


./traffic.sh "${IFACE}"
./world-controller
