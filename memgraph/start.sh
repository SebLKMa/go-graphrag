#!/usr/bin/env bash
# Starts (or reuses) the local Memgraph + Memgraph Lab containers described in
# MEMGRAPH.md. Only memgraph:2.11.0 works on Windows VMware Ubuntu.
set -euo pipefail

if ! docker network ls --format '{{.Name}}' | grep -qx memgraph-net; then
    docker network create memgraph-net
fi

if docker ps -a --format '{{.Names}}' | grep -qx memgraph; then
    if ! docker ps --format '{{.Names}}' | grep -qx memgraph; then
        docker start memgraph
    fi
    echo "memgraph is running"
else
    docker run -d --name memgraph \
      --network memgraph-net \
      -p 7687:7687 -p 7444:7444 \
      -v "$HOME/memgraph-data/lib:/var/lib/memgraph" \
      memgraph/memgraph:2.11.0 \
      --data-directory=/var/lib/memgraph
    sleep 3
fi

if docker ps -a --format '{{.Names}}' | grep -qx memgraph-lab; then
    if ! docker ps --format '{{.Names}}' | grep -qx memgraph-lab; then
        docker start memgraph-lab
    fi
    echo "memgraph-lab is running"
else
    docker run -d --name memgraph-lab \
      --network memgraph-net \
      -p 3000:3000 \
      -e QUICK_CONNECT_MG_HOST=memgraph \
      -e QUICK_CONNECT_MG_PORT=7687 \
      memgraph/lab
fi

echo "memgraph on bolt://localhost:7687 (no auth); Memgraph Lab at http://localhost:3000"
