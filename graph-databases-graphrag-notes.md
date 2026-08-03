# Graph Databases, GraphRAG & Docker Deployment — Working Notes

A running technical Q&A covering RDF on Neo4j, building a GraphRAG system, LangGraph
orchestration, and Dockerizing Memgraph / GraphDB / Stardog on a VMware Ubuntu VM,
ending with a horizontal/vertical scaling comparison.

---

## 1. Can Neo4j use RDF?

Neo4j is a **labeled property graph** database, not a native RDF triple store, but it
supports RDF several ways:

- **Neosemantics (n10s) plugin** — the usual route. Imports RDF (Turtle, RDF/XML,
  JSON-LD, N-Triples, TriG) into Neo4j, exports graph data back out as RDF, supports
  SPARQL-like querying via a wrapper (Cypher stays primary), and can validate against
  RDFS/SHACL/OWL subsets. Each RDF resource becomes a node; predicates become
  relationships or literal properties.
- **Native semantic tooling** in newer 5.x releases leans further into knowledge-graph
  use, but storage is still property-graph — RDF is a translation layer.

**Trade-offs:** RDF's triple model doesn't map 1:1 onto property graphs, so round-tripping
isn't always lossless. If you need strict RDF semantics + SPARQL as first-class, use a
native triple store (GraphDB, Stardog, Neptune RDF mode, Apache Jena). If you want
property-graph flexibility with occasional RDF exchange, Neo4j + n10s works well.

---

## 2. Building a simple GraphRAG on Neo4j

**Core idea:** build a knowledge graph from documents (entities as nodes, relationships
as edges), then retrieve by combining **vector similarity + graph traversal**, and feed
the result to an LLM.

### Four stages

1. **Ingestion & entity/relation extraction** — chunk documents, use an LLM to extract
   entities and relationships as structured JSON. Expensive: one LLM call per chunk.
2. **Graph construction** — `MERGE` nodes (dedupe by name/type), create relationships,
   store source chunk text as a `Chunk` node linked via `MENTIONS` so you can get back to
   real text at retrieval time.
3. **Embeddings** — embed each chunk, store as a node property, build a native vector index:

   ```cypher
   CREATE VECTOR INDEX chunk_embeddings IF NOT EXISTS
   FOR (c:Chunk) ON (c.embedding)
   OPTIONS {indexConfig: {
     `vector.dimensions`: 1536,
     `vector.similarity_function`: 'cosine'
   }}
   ```
4. **Retrieval** — embed the query, vector-search for seed chunks, then traverse 1–2 hops
   out to pull in connected context pure vector search would miss:

   ```cypher
   CALL db.index.vector.queryNodes('chunk_embeddings', 5, $queryEmbedding)
   YIELD node AS seedChunk, score
   MATCH (seedChunk)-[:MENTIONS]->(e:Entity)-[:RELATED_TO*1..2]-(e2:Entity)<-[:MENTIONS]-(relatedChunk:Chunk)
   RETURN DISTINCT seedChunk, relatedChunk, score
   ORDER BY score DESC
   LIMIT 15
   ```

**Minimal stack:** Neo4j 5.11+ (native vector index, no plugin), `neo4j` Python driver,
an LLM for extraction + generation, an embedding model.

**Where complexity lives:** entity resolution (merging "Microsoft"/"MSFT"/"Microsoft Corp"),
extraction quality (schema-constrained beats open-ended), and retrieval tuning (hop count,
graph-distance vs vector-similarity weighting).

---

## 3. Ingestion from CSVs that already contain entities

If entities are already structured in CSV, **skip the LLM extraction step** — the biggest
simplification.

Typical shape: `entities.csv` (`id, name, type, description`) and `relationships.csv`
(`source_id, target_id, relation_type`).

```cypher
-- Load entities
LOAD CSV WITH HEADERS FROM 'file:///entities.csv' AS row
MERGE (e:Entity {id: row.id})
SET e.name = row.name, e.type = row.type, e.description = row.description

-- Load relationships
LOAD CSV WITH HEADERS FROM 'file:///relationships.csv' AS row
MATCH (a:Entity {id: row.source_id})
MATCH (b:Entity {id: row.target_id})
MERGE (a)-[r:RELATED_TO {type: row.relation_type}]->(b)
```

- Use `apoc.merge.node()` / `apoc.merge.relationship()` for dynamic labels/rel-types if
  `type` varies a lot.
- Embeddings, vector index, and retrieval stay the same in shape.
- Stage 3 (graph construction) collapses into stage 1; replace it with a **data-quality
  pass** (orphans, duplicate IDs, dangling FKs).

**Key question:** if the CSV gives no explicit relationships, you need an inference step
(rule-based or lightweight LLM) — otherwise it's regular RAG "wearing a graph costume."

---

## 4. Equipment + inspection CSVs (one-to-many via FK)

Schema: `equipments.csv` (PK `equipment_id`) + `equipment_health_inspections.csv`
(FK `equipment_id`). Clean one-to-many.

### Data model
```
(:Equipment {equipment_id, name, type, location, manufacturer})
   -[:HAS_INSPECTION]->
(:Inspection {inspection_id, date, status, findings})
```
The FK becomes the **relationship**, not a stored property.

### Ingestion
```cypher
CREATE INDEX equipment_id_idx IF NOT EXISTS
FOR (e:Equipment) ON (e.equipment_id);

LOAD CSV WITH HEADERS FROM 'file:///equipments.csv' AS row
MERGE (e:Equipment {equipment_id: row.equipment_id})
SET e.name = row.name, e.type = row.type, e.location = row.location,
    e.manufacturer = row.manufacturer, e.install_date = date(row.install_date);

LOAD CSV WITH HEADERS FROM 'file:///equipment_health_inspections.csv' AS row
MATCH (e:Equipment {equipment_id: row.equipment_id})
MERGE (i:Inspection {inspection_id: row.inspection_id})
SET i.date = date(row.inspection_date), i.status = row.health_status,
    i.findings = row.findings, i.inspector = row.inspector
MERGE (e)-[:HAS_INSPECTION]->(i);
```

### Embeddings
Embed the semantically rich `findings` text. Skip embeddings entirely if findings are just
status codes — plain graph queries serve better, and it isn't really GraphRAG then.

### Retrieval — the graph payoff
Find relevant findings by vector search, then traverse **up to the equipment and back down
to its full inspection history**, so the LLM sees the whole health timeline:

```cypher
CALL db.index.vector.queryNodes('inspection_embeddings', 5, $queryEmbedding)
YIELD node AS seedInspection, score
MATCH (seedInspection)<-[:HAS_INSPECTION]-(e:Equipment)-[:HAS_INSPECTION]->(allInspections:Inspection)
RETURN e.name AS equipment, e.type AS type, e.location AS location,
       collect({date: allInspections.date, status: allInspections.status,
                findings: allInspections.findings}) AS full_history,
       max(score) AS relevance
ORDER BY relevance DESC;
```

**Caveat:** a two-table parent/child structure is at the simple end of what justifies a
graph. It pays off when you add more entity types (technicians, parts, work orders) or want
cross-equipment patterns:

```cypher
MATCH (a:Equipment), (b:Equipment)
WHERE a.type = b.type AND a.manufacturer = b.manufacturer
  AND a.equipment_id < b.equipment_id
MERGE (a)-[:SAME_MODEL]->(b);
```

---

## 5. Getting an `ANTHROPIC_API_KEY` for Claude Code

The key comes from the **Anthropic Console** (console.anthropic.com), a separate account
from claude.ai chat. Steps: sign up / log in → verify phone → add a payment method (keys
won't return successful responses until billing is set up) → Settings → API keys → Create
Key → **copy immediately** (shown once).

Wire it in:
```bash
echo 'export ANTHROPIC_API_KEY="sk-ant-api03-..."' >> ~/.zshrc && source ~/.zshrc
```

**Key point:** with a Claude Pro/Max subscription you can use Claude Code via OAuth login
at no extra cost — no API key needed. Use a dedicated capped API key only if you specifically
want raw API testing / billing isolation.

### "Purchasing prepaid credits is not allowed before upgrading your plan"

Your Console account is still on the default/free tier. Sequence: add a payment method
first → "upgrade plan" here usually means activating the pay-as-you-go **Build plan**
(a usage tier, not a monthly subscription) → then **Buy credits** unlocks ($5 min).
Watch out not to accidentally buy an annual Pro subscription instead of API credits — they're
different products. If the button stays disabled with a card on file, it's an account-state
issue for Anthropic support.

---

## 6. Neo4j: create vs update a node

- **`CREATE`** — always inserts a new node. Run twice → duplicates. Use only when you know
  it doesn't exist.
- **`MATCH` + `SET`** — updates properties on an existing node; does nothing if `MATCH`
  finds nothing.
- **`MERGE`** — the upsert: match if present, create if not. Combine with `ON CREATE` /
  `ON MATCH`:

  ```cypher
  MERGE (e:Equipment {equipment_id: 'EQ-100'})
  ON CREATE SET e.name = 'Pump A', e.created_at = datetime()
  ON MATCH  SET e.updated_at = datetime()
  ```
  A plain `SET` after `MERGE` runs in **both** cases.

**The `MERGE` gotcha:** it matches on the *entire* pattern. `MERGE` on the key only, then
`SET` the rest — otherwise a mismatched extra property creates a duplicate node.

```cypher
MERGE (e:Equipment {equipment_id: 'EQ-100'})   -- key only
SET e.status = 'active', e.name = 'Pump A'      -- everything else
```

Back it with a uniqueness constraint so duplicates are impossible and `MERGE` uses an index:
```cypher
CREATE CONSTRAINT equipment_id_unique IF NOT EXISTS
FOR (e:Equipment) REQUIRE e.equipment_id IS UNIQUE;
```

**Rule of thumb:** new → `CREATE`; exists, changing → `MATCH`+`SET`; idempotent loads →
`MERGE` on key + `SET` the rest.

---

## 7. LangGraph (LangChain) — how it relates

**Terminology trap:** a LangGraph "node" is **not** a data record like a Neo4j node. It's a
**function/step in an agent workflow** that receives state and returns an update. The graph
is control flow, not a data store. No `MERGE`/`SET`; nodes aren't persisted as data.

### Three primitives
`StateGraph` (builder), **nodes** (functions: state → partial update), **edges**
(transitions, unconditional or conditional).

### "Updating a node" = updating shared state via reducers
State is a typed dict shared across nodes. Each node returns a *partial* update, merged in.
The **reducer** controls create-vs-update (the real parallel to `MERGE`/`SET`):

- **No reducer** → last write wins (overwrite, like `SET`)
- **`Annotated[list, add]`** → accumulate/append

```python
from typing import Annotated, TypedDict
from langgraph.graph import StateGraph, START, END
import operator

class State(TypedDict):
    current_step: str                        # overwrite
    findings: Annotated[list, operator.add]  # accumulate

def analyze(state: State) -> dict:
    return {"current_step": "analyzed", "findings": ["found X"]}
```

### Two things to know
- **API shift (2026):** for simple ReAct agents the low-level prebuilt path is deprecated
  in favor of `create_agent` in the `langchain` package. Drop to `StateGraph` for complex
  topologies (multi-agent, conditional routing, human-in-the-loop).
- **Persistence via checkpointers,** not by writing nodes to a DB: `MemorySaver` (dev),
  `SqliteSaver` (single-server), `PostgresSaver` (multi-instance).

### How it fits the equipment GraphRAG
- **Neo4j** = the *data* graph (the knowledge).
- **LangGraph** = the *orchestration* graph (the steps). A node runs a Cypher retrieval
  query and drops results into state; the next node feeds context to the LLM. Neo4j is the
  tool a node calls; LangGraph decides the flow.

*(A full runnable four-node agent — `embed_query → vector_search → expand_history →
generate` — was produced as `equipment_graphrag_agent.py`.)*

---

## 8. Memgraph on Docker (VMware Ubuntu) with persistence

Memgraph is in-memory but persists via **snapshots + WAL** under `/var/lib/memgraph` — mount
that for persistence.

### Images
`memgraph/memgraph` (bare), **`memgraph/memgraph-mage`** (+ MAGE algorithms, recommended),
`memgraph/lab` (web UI, separate container). Ports: `7687` Bolt, `7444` log streaming.

### Named-volume version (Docker manages ownership)
```bash
docker run -d --name memgraph \
  -p 7687:7687 -p 7444:7444 \
  -v mg_lib:/var/lib/memgraph \
  -v mg_log:/var/log/memgraph \
  -v mg_etc:/etc/memgraph \
  memgraph/memgraph-mage
```

### Custom host directory (bind mount) — needs ownership handling
Named volumes hid ownership from you; bind mounts don't. The container's `memgraph` user
must own the host dir:
```bash
mkdir -p ~/memgraph-data/lib ~/memgraph-data/log ~/memgraph-data/etc
docker run --rm --entrypoint id memgraph/memgraph-mage memgraph   # read uid/gid
sudo chown -R <uid>:<gid> ~/memgraph-data
docker run -d --name memgraph -p 7687:7687 -p 7444:7444 \
  -v ~/memgraph-data/lib:/var/lib/memgraph \
  memgraph/memgraph-mage
```
On WSL2, keep the dir in the Linux filesystem, not `/mnt/c/...`.

### Adding Memgraph Lab
Two containers on a shared network so Lab resolves `memgraph` by name:
```bash
docker network create memgraph-net
# memgraph run with: --network memgraph-net
docker run -d --name memgraph-lab --network memgraph-net -p 3000:3000 \
  -e QUICK_CONNECT_MG_HOST=memgraph -e QUICK_CONNECT_MG_PORT=7687 \
  memgraph/lab
```
Lab is stateless — no persistence needed. UI at `http://localhost:3000`. Host code still
connects via `bolt://localhost:7687`.

---

## 9. Memgraph troubleshooting on the VM

### Exit (139) = segmentation fault (SIGSEGV)
On a VMware VM this is almost always a **CPU-instruction mismatch** — Memgraph's binary
uses AVX/AVX2 that VMware masks from the guest by default.

Diagnose:
```bash
docker logs memgraph
grep -o 'avx[0-9]*' /proc/cpuinfo | sort -u   # empty = AVX hidden
sudo dmesg | tail -20                          # "invalid opcode" confirms it
```

Fix in VMware (VM powered off): enable VT-x/EPT passthrough, or edit `.vmx`:
```
cpuid.enableAVX = "TRUE"
cpuid.enableAVX2 = "TRUE"
```
Or turn off CPUID masking so the guest sees the real CPU.

Fallbacks: pin an older image (`memgraph/memgraph:2.11.0`, lower instruction baseline),
confirm architecture (`uname -m` = x86_64), raise container memory.

### "Folder for the key-value store mg_data/settings couldn't be initialized!"
`chown` didn't fix it → not (only) ownership. The path in the error is **`mg_data`**
(relative), not `/var/lib/memgraph` — Memgraph fell back to its compiled default because an
**empty bind mount on `/etc/memgraph` masked the image's `memgraph.conf`**, which normally
sets `--data-directory=/var/lib/memgraph`.

Confirm the args in play:
```bash
docker inspect memgraph --format 'Cmd: {{ .Config.Cmd }}
Entrypoint: {{ .Config.Entrypoint }}'
```

### The fix — pass the data directory explicitly
Anything after the image name goes to the entrypoint `[/usr/lib/memgraph/memgraph]` as flags:

```bash
docker run -d --name memgraph \
  -p 7687:7687 -p 7444:7444 \
  -v ~/memgraph-data/lib:/var/lib/memgraph \
  memgraph/memgraph:2.11.0 \
  --data-directory=/var/lib/memgraph
```
The flag value is the **container path** (the right side of `-v`). Dropping the
`/etc/memgraph` mount removes the masking problem. **This resolved it.**

Version note: `2.11.0` runs where newer builds segfault (older CPU baseline). It's the plain
DB (no MAGE). Don't upgrade the image against the same data dir — snapshot/WAL format is
version-specific (newer can read older, not vice versa); back up `~/memgraph-data/lib` first.

---

## 10. GraphDB (Ontotext) on Docker

A genuine **W3C RDF triple store** (SPARQL), unlike Neo4j/Memgraph. Free edition just runs
(no license gate on older versions; note GraphDB 11.0.0+ requires a free license key).

- **Image:** `ontotext/graphdb:<version>` (single image all editions since 10.0)
- **Data:** `/opt/graphdb/home` | **Import dir:** `/root/graphdb-import` | **Port:** `7200`

```bash
mkdir -p ~/graphdb-data/home ~/graphdb-data/import
docker run -d --name graphdb -p 7200:7200 \
  -v ~/graphdb-data/home:/opt/graphdb/home \
  -v ~/graphdb-data/import:/root/graphdb-import \
  ontotext/graphdb:11.0.1
```
Workbench UI at `http://localhost:7200`. Config via `-D` Java properties after the image
name, or `GDB_JAVA_OPTS`.

**VM note:** it's a JVM app, so the AVX/exit-139 problem is unlikely — but it's RAM-hungry.
Cap the heap so it isn't OOM-killed:
```bash
-e GDB_JAVA_OPTS="-Xms1g -Xmx2g"
```

---

## 11. Stardog on Docker

RDF triple store **+ OWL/SWRL reasoning**. Key difference: **a license file is mandatory
even for the free tier** — get one from Stardog or use Stardog Cloud.

- **Image:** `stardog/stardog` | **Home (data + license):** `/var/opt/stardog` | **Port:** `5820`
- Default creds `admin` / `admin`.

```bash
mkdir -p ~/stardog-home
cp /path/to/stardog-license-key.bin ~/stardog-home/
docker run -d --name stardog -p 5820:5820 \
  -v ~/stardog-home:/var/opt/stardog \
  -e STARDOG_SERVER_JAVA_ARGS="-Xmx4g -Xms4g -XX:MaxDirectMemorySize=4g" \
  stardog/stardog
```
Named volumes must be pre-populated with the license before first start.

**VM notes:** JVM app (AVX-safe) but very memory-hungry (heap + large direct memory); low
RAM causes JVM segfaults — give it more memory.

### RDF-capable options compared

| | Neo4j / Memgraph | GraphDB | Stardog |
|---|---|---|---|
| License to start | No | Free edition runs (11.0+ needs free key) | **Yes, mandatory** |
| Query language | Cypher (property graph) | SPARQL (RDF) | SPARQL (RDF) + reasoning |
| UI | Browser / Lab | Workbench :7200 | Studio :5820/8080 |
| JVM (VM-safe) | Memgraph native (AVX issue) | Yes | Yes |

For low-friction RDF/SPARQL experimentation, GraphDB free is the easiest path. Stardog is
worth it for its reasoning depth.

---

## 12. Scaling comparison: Neo4j vs GraphDB vs Stardog

**The fundamental problem:** sharding a graph is uniquely hard — a traversal crossing machine
boundaries pays a network round-trip per hop while trying to keep ACID. So historically all
three **replicate the whole graph to every node: reads scale horizontally, writes scale
vertically.**

### Neo4j
- **Vertical:** strong (native engine, more RAM = faster traversals).
- **Reads:** autonomous clustering (leader/follower); set a copy count, queries auto-route.
- **Writes:** through the single leader — don't scale horizontally in this model.
- **Fabric:** federation, not sharding — manual partition by region/type/tenant, then
  federated queries.
- **Infinigraph (Early Access, 2025):** the real answer to "too big for one box" — **property
  sharding** keeps nodes/relationships together and shards only properties, scaling past
  100 TB with full ACID and no app changes. Catch: the single topology shard can become a
  hot-spot under extreme traversal concurrency. Solves *storage* scale, not necessarily
  *traversal-throughput* scale.

### GraphDB
- **Vertical:** primary axis; optimize I/O → CPU → RAM. Honest ceiling — some query parts run
  sequentially.
- **Reads:** Raft-based cluster (leader/followers); more followers = more parallel reads.
- **Writes:** through the leader; every write replicates to all workers (full copy each).
- **No sharding** — use SPARQL / FedX federation for cross-dataset scale.

### Stardog
- **Vertical:** memory-hungry; allocate ~2× heap per cluster node for replication overhead.
- **Reads:** ZooKeeper-coordinated cluster (Coordinator + Participants); min 3 Stardog +
  3 ZooKeeper nodes.
- **Writes:** **strongest consistency** — a write must land on *all* nodes before the client
  gets a response. No sharding, no eventual consistency. Larger clusters = better reads,
  worse writes.
- **Extra dependency:** a ZooKeeper ensemble on separate nodes (more operational weight).

### Side-by-side

| | Neo4j | GraphDB | Stardog |
|---|---|---|---|
| Model | Property graph | RDF | RDF + reasoning |
| Vertical | Strong | Primary axis | Strong, RAM-heavy |
| Read scale-out | Autonomous cluster | Raft cluster | ZooKeeper cluster |
| Write scale-out | Single leader (except Infinigraph) | Single leader | Coordinator, blocks on full replication |
| True sharding | **Infinigraph** (EA) | No (federation) | **No** |
| Consistency | ACID / causal | Raft consensus | **Strongest** (synchronous) |
| Min cluster nodes | 3 primaries | Odd (3+) | 3 Stardog + 3 ZooKeeper |
| External dependency | None | None | **ZooKeeper** |

### Bottom line
- All three scale **reads** the same way (add replicas) — fine for read-heavy GraphRAG.
- **Writes** are the differentiator: Stardog strictest/safest/least write-scalable; Neo4j &
  GraphDB funnel through one leader.
- Only **Neo4j Infinigraph** truly answers "graph too big for one machine"; the others use
  federation.
- **Operational weight:** Stardog also needs a ZooKeeper ensemble; GraphDB/Neo4j clusters
  are self-contained.
- On a single VM you're in vertical-scaling territory anyway (more RAM/CPU) — horizontal only
  matters once you outgrow one machine.
