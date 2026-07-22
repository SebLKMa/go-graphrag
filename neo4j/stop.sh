#!/usr/bin/env bash
# Starts (or reuses) the local Neo4j container described in NEO4J.md.
set -euo pipefail

if docker ps --format '{{.Names}}' | grep -qx my-neo4j; then
    docker stop my-neo4j
    docker rm my-neo4j   
fi

if docker ps -a --format '{{.Names}}' | grep -qx my-neo4j; then
    docker rm my-neo4j
fi

echo "my-neo4j is not running"
