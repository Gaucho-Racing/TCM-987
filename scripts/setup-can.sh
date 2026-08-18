#!/usr/bin/env bash
set -euo pipefail

# Bring up a CAN interface for the relay.
#
#   sudo ./scripts/setup-can.sh              # can0 @ 500k, listen-only
#   sudo ./scripts/setup-can.sh can0 500000  # explicit
#   sudo ./scripts/setup-can.sh vcan0        # virtual interface for dev
#
# Real interfaces come up in listen-only mode: the relay is a passive
# sniffer and must never ACK or transmit on the car's bus.

IFACE="${1:-can0}"
BITRATE="${2:-500000}"

if [[ "$IFACE" == vcan* ]]; then
    modprobe vcan
    ip link add dev "$IFACE" type vcan 2>/dev/null || true
    ip link set "$IFACE" up
    echo "$IFACE up (virtual)"
    exit 0
fi

ip link set "$IFACE" down 2>/dev/null || true
ip link set "$IFACE" up type can bitrate "$BITRATE" listen-only on
echo "$IFACE up @ ${BITRATE}bps (listen-only)"
