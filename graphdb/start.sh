#!/usr/bin/env bash
# Starts (or reuses) the local Ontotext GraphDB container described in GRAPHDB.md.
# Pinned to 10.8.1: GraphDB 11+ refuses writes without a license file, while
# the 10.x free mode runs unlicensed.
set -euo pipefail

if docker ps -a --format '{{.Names}}' | grep -qx graphdb; then
    if ! docker ps --format '{{.Names}}' | grep -qx graphdb; then
        docker start graphdb
    fi
    echo "graphdb is running"
    exit 0
fi

docker run -d --name graphdb \
  -p 7200:7200 \
  -v ~/graphdb-data/home:/opt/graphdb/home \
  -v ~/graphdb-data/import:/root/graphdb-import \
  ontotext/graphdb:10.8.1

echo "graphdb started; workbench at http://localhost:7200"
