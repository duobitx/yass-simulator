#!/usr/bin/env bash

./traffic.sh
echo "Network interface -- ${IFACE}"
./world-controller
