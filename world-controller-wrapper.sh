#!/usr/bin/env bash

ip route # TODO debug remove
./traffic.sh
echo "Network interface -- ${IFACE}"
./world-controller
