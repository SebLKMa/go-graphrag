#!/usr/bin/env bash
# Starts (or reuses) the local Neo4j container described in NEO4J.md.
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
  ontotext/graphdb:11.0.1

echo "graphdb started; browser at http://localhost:7200"
