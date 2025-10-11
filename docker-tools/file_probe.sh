#!/bin/sh

PROBE_DIR="/tmp"
if [ -n "$SHARED_VOLUME_PATH" ]; then
  PROBE_DIR=${SHARED_VOLUME_PATH}
fi
if [ -z "$PROBE_FILE" ]; then
  echo "PROBE_FILE variable not defined" >&2
  exit 10
fi
PROBE_PATH="${PROBE_DIR}/${PROBE_FILE}"

if [ -f "${PROBE_PATH}" ]; then
  exit 0
else
  echo "Probe file ${PROBE_PATH} not found"
  exit 1
fi
