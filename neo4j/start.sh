#!/usr/bin/env bash
# Starts (or reuses) the local Neo4j container described in NEO4J.md.
set -euo pipefail

if docker ps -a --format '{{.Names}}' | grep -qx my-neo4j; then
    if ! docker ps --format '{{.Names}}' | grep -qx my-neo4j; then
        docker start my-neo4j
    fi
    echo "my-neo4j is running"
    exit 0
fi

docker run \
    --name my-neo4j \
    -p 7474:7474 -p 7687:7687 \
    -d \
    -v "$HOME/neo4j/data:/data" \
    -v "$HOME/neo4j/logs:/logs" \
    -v "$HOME/neo4j/import:/import" \
    -v "$HOME/neo4j/plugins:/plugins" \
    -e NEO4J_AUTH=neo4j/graph4fun \
    neo4j:latest

echo "my-neo4j started; browser at http://localhost:7474 (user: neo4j, password: graph4fun)"
