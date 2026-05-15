#!/usr/bin/env bash
set -euo pipefail

################################
# CONFIG
################################

IFACE="$1"
# Data-plane port ranges (matched by world-controller's managedPortRanges).
# 8080 is intentionally NOT here (control-plane: experiment-executor, events-webapp, web-ui).
PORT_RANGES=(4000-5000 9000-9999)
# Kept for the ip_profile helper below; defaults to the first range.
export PORT_RANGE="${PORT_RANGES[0]}"

OUT_LIMIT=4mbit

echo "Using interface: $IFACE"

################################
# CLEANUP
################################
tc qdisc del dev "$IFACE" root 2>/dev/null || true
tc qdisc replace dev "$IFACE" root pfifo_fast || true
tc qdisc del dev "$IFACE" root || true

################################
# ROOT QDISC
################################
tc qdisc add dev "$IFACE" root handle 1: htb default 9999

###############################
# DEFAULT DROP CLASS
################################
tc class add dev "$IFACE" parent 1: classid 1:900 htb rate $OUT_LIMIT ceil $OUT_LIMIT
tc qdisc add dev "$IFACE" parent 1:900 handle 900: netem loss 100%

################################
# NORMAL TRAFFIC CLASS
################################
tc class add dev "$IFACE" parent 1: classid 1:9999 htb rate 10gbit

################################
# DEFAULT BLOCK RULES (OUTGOING ONLY)
################################

for range in "${PORT_RANGES[@]}"; do
    tc filter add dev "$IFACE" parent 1: protocol ip prio 100 flower \
        ip_proto tcp dst_port "$range" \
        flowid 1:900
    tc filter add dev "$IFACE" parent 1: protocol ip prio 100 flower \
        ip_proto udp dst_port "$range" \
        flowid 1:900
done

tc filter add dev "$IFACE" parent 1: protocol ip prio 100 flower \
    ip_proto icmp \
    flowid 1:900

############################################
# PER-IP PROFILE FUNCTION (SIMPLE & SAFE)
############################################
# Usage:
#   ip_profile <dst_ip> <class_id_suffix> <rate> <delay> <loss> [ports]
# - dst_ip: destination IP to allow
# - class_id_suffix: numeric suffix for HTB class (e.g., 101 -> class 1:101)
# - rate: egress rate for this class (e.g., 100kbit, 10mbit)
# - delay: netem delay (e.g., 0ms, 20ms)
# - loss: packet loss percent (e.g., 0%, 1%)
# - ports (optional): single port or range (e.g., 5201 or 4000-5000). Defaults to PORT_RANGE (first managed range).
#
# Example to allow iperf3 default port (5201) to a specific server:
#   ip_profile 15.235.13.240 101 100kbit 0ms 0% 5201
ip_profile() {
    local IP="$1"
    local CID="$2"
    local RATE="$3"
    local DELAY="$4"
    local LOSS="$5"
    local PORTS="${6:-$PORT_RANGE}"

    local CLASS="1:${CID}"

    echo "Applying profile for $IP (CID=$CID, ports=$PORTS)"

    # Class
    tc class replace dev "$IFACE" parent 1: classid "$CLASS" \
        htb rate "$RATE" ceil "$RATE"

    # Netem
    tc qdisc replace dev "$IFACE" parent "$CLASS" handle "${CID}:" \
        netem delay "$DELAY" loss "$LOSS"

    # TCP unblock
    tc filter replace dev "$IFACE" parent 1: protocol ip prio 5 handle "${CID}1" flower \
        ip_proto tcp dst_ip "$IP" dst_port $PORTS \
        flowid "$CLASS"

    # UDP unblock
    tc filter replace dev "$IFACE" parent 1: protocol ip prio 5 handle "${CID}2" flower \
        ip_proto udp dst_ip "$IP" dst_port $PORTS \
        flowid "$CLASS"

    # ICMP unblock
    tc filter replace dev "$IFACE" parent 1: protocol ip prio 5 handle "${CID}3" flower \
        ip_proto icmp dst_ip "$IP" \
        flowid "$CLASS"
}

#ip_profile 15.235.13.240 101 1mbit 0ms 0%

echo "Setup complete."
