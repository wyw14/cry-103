#!/usr/bin/env sh
set -eu
docker build -f benzhi.Dockerfile -t cableguard:local .
