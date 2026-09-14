#!/bin/bash
# Selects which MinIO Docker image to use, based on this host's CPU.
#
# The official quay.io/minio/minio image requires x86-64-v2 support and
# crashes with "Fatal glibc error: CPU does not support x86-64-v2" on
# older CPUs (see minio/Dockerfile for background). This script detects
# that and writes/removes docker-compose.override.yml accordingly, so
# any `docker compose` invocation in this directory - the CLI,
# docker-build.sh, or a NAS UI like Portainer/Synology Container Manager -
# automatically picks the right image without needing any -f flags.
#
# Run this once per host (e.g. after cloning/pulling on a new machine, or
# as a one-off setup step on a NAS). It's idempotent, safe to re-run.

set -e

cd "$(dirname "${BASH_SOURCE[0]}")"

cpu_supports_x86_64_v2() {
    [[ -r /proc/cpuinfo ]] || return 1
    local flags
    flags=" $(grep -m1 '^flags' /proc/cpuinfo | cut -d: -f2) "
    for f in cx16 lahf_lm popcnt sse4_1 sse4_2 ssse3; do
        [[ $flags == *" $f "* ]] || return 1
    done
    return 0
}

if [[ "$(uname -m)" == "x86_64" ]] && ! cpu_supports_x86_64_v2; then
    echo "CPU does not support x86-64-v2 — MinIO will be built from source (docker-compose.override.yml)"
    cp docker-compose.minio-source.yml docker-compose.override.yml
else
    echo "CPU supports x86-64-v2 — using the official MinIO image"
    rm -f docker-compose.override.yml
fi
