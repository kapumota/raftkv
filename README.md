### RaftKV

[![CI](https://github.com/kapumota/raftkv/actions/workflows/ci.yml/badge.svg)](https://github.com/kapumota/raftkv/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/tag/kapumota/raftkv?sort=semver&label=release)](https://github.com/kapumota/raftkv/tags)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/github/license/kapumota/raftkv)](LICENSE)

RaftKV es un key-value store distribuido experimental escrito en Go que
implementa Raft desde cero, sin `hashicorp/raft` ni otra biblioteca de
consenso. El clúster de referencia utiliza cinco nodos, WAL persistente,
Docker Compose, inyección de fallas, evaluación reproducible y pruebas
dirigidas de invariantes de correctness.

`v0.1.0` es el primer release experimental congelado del proyecto. Su objetivo
es ofrecer un artefacto pequeño, auditable y reproducible para estudiar
consenso, replicación, failover, persistencia y evaluación de sistemas
distribuidos.

No pretende ser una base de datos lista para producción ni una implementación
formalmente verificada de Raft.

#### Estado del proyecto

La versión `v0.1.0` consolida tres capas de evidencia:

```text
implementación Raft
        |
        v
pruebas dirigidas de correctness
        |
        v
evaluación experimental reproducible
```

El repositorio incluye evidencia ejecutable para Election Safety, persistencia
de voto, Log Matching, quorum commit, Leader Completeness, State Machine
Safety, recuperación desde WAL, lecturas protegidas por quorum y liveness bajo
un failure model explícito.

La regla de interpretación es:

```text
implementado != probado formalmente
testeado     != demostrado
observado    != garantizado
```

#### Características principales

- Elección de `leader` con timeouts aleatorios y votación por mayoría.
- Persistencia de `currentTerm`, `votedFor`, `commitIndex` y log en WAL.
- Restricción de voto según frescura de `LastLogTerm` y `LastLogIndex`.
- Replicación mediante `AppendEntries`.
- Reparación de sufijos conflictivos con `nextIndex` y `matchIndex`.
- Protección del prefijo ya confirmado.
- Avance de `commitIndex` únicamente con quorum.
- Confirmación HTTP de escrituras solo después del commit.
- Barrera `NOOP` por término para confirmar liderazgo.
- `ReadIndex` antes de servir lecturas KV.
- Reconstrucción de la máquina de estados desde el prefijo confirmado.
- Deduplicación de efectos mediante `ClientID` y `RequestID` en la máquina de
  estados.
- Failover, catch-up y recuperación después de particiones.
- Campañas de fallas reproducibles.
- Análisis estadístico reproducible y figuras deterministas.

La deduplicación forma parte de la semántica interna de comandos Raft y no se
expone actualmente como un contrato exactly-once general de la API HTTP.

#### Alcance y límites

`v0.1.0` mantiene deliberadamente un alcance compacto:

- Membresía fija. No hay joint consensus ni cambios dinámicos de configuración.
- No hay snapshots ni compactación del log.
- El WAL puede crecer sin límite.
- El failure model considera nodos no bizantinos y almacenamiento local
  operativo.
- La campaña experimental final se ejecutó en un único host con Docker bridge.
- No se modelan exhaustivamente delay, reorder, packet loss o corrupción
  arbitraria del almacenamiento.
- No existe todavía un checker externo de linealizabilidad sobre historiales
  concurrentes completos.
- No existe todavía una especificación TLA+/PlusCal ni model checking.
- No se afirma idoneidad para cargas de producción.

Para el alcance exacto consulta
[`EVIDENCE_MATRIX.md`](EVIDENCE_MATRIX.md).

#### Arquitectura

```text
cliente
   |
   v
HTTP /kv/set, /kv/get
   |
   v
leader Raft
   |
   +---- RequestVote / AppendEntries ----+
   |                                     |
   v                                     v
followers                            WAL por nodo
   |                                     |
   +-------------- quorum ---------------+
                    |
                    v
               commitIndex
                    |
                    v
          máquina de estados KV
```

El clúster Docker de referencia contiene cinco servicios `raft-node-*` y un
`client-driver`. Cada nodo mantiene un volumen WAL independiente sobre la red
bridge `raftnet`.

#### Estructura del repositorio

```text
raftkv/
|-- .github/workflows/ci.yml
|-- cmd/
|   |-- benchmark-compare/
|   |-- benchmark-failures/
|   |-- benchmark-figures/
|   |-- benchmark-stats/
|   |-- client-driver/
|   |-- experiment-metrics/
|   |-- experiment-runner/
|   `-- raftnode/
|-- docker/
|-- experiments/
|   |-- configs/
|   |-- figures/
|   |-- processed/
|   |-- raw/
|   |-- ARTIFACT_REPRODUCTION.md
|   |-- METHODOLOGY.md
|   |-- RESULTS.md
|   |-- STATISTICAL_ANALYSIS.md
|   `-- THREATS_TO_VALIDITY.md
|-- internal/
|   |-- experiment/
|   |-- kv/
|   `-- raft/
|-- scripts/
|-- CORRECTNESS_INVARIANTS.md
|-- EVIDENCE_MATRIX.md
|-- LIVENESS_MODEL.md
|-- docker-compose.yml
|-- go.mod
|-- LICENSE
|-- Makefile
`-- README.md
```

#### Requisitos

Para desarrollo y validación:

```text
Go 1.22.x
GNU Make
Bash
Git
```

Para ejecutar el clúster y nuevas campañas:

```text
Docker Engine
Docker Compose
```

La reproducción histórica tiene requisitos específicos en
[`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md).

#### Validación local

La validación principal es:

```bash
make validate
```

Ejecuta:

```text
gofmt check
go vet ./...
go test -race ./...
go build ./...
bash -n de scripts experimentales
```

También están disponibles:

```bash
make fmt-check
make vet
make test
make test-race
make build
```

GitHub Actions ejecuta CI en cada `push` y `pull_request`.

#### Levantar el clúster

```bash
make up
```

Para seguir los logs:

```bash
make logs
```

Para detenerlo:

```bash
make down
```

#### Encontrar el leader

Consulta cada nodo:

```bash
for i in 1 2 3 4 5; do
  docker compose exec "raft-node-$i"     curl -s http://localhost:8080/status
  echo
done
```

Una respuesta típica:

```json
{
  "commit_index": 3,
  "id": "raft-node-2",
  "state": "leader",
  "term": 2,
  "voted_for": "raft-node-2"
}
```

#### Escribir un valor

Suponiendo que `raft-node-2` es el leader:

```bash
docker compose exec raft-node-2 curl -s -X POST   http://localhost:8080/kv/set   -H 'Content-Type: application/json'   -d '{"key":"nombre","value":"kapumota"}'
```

Una escritura confirmada responde:

```json
{"estado":"confirmado"}
```

La respuesta `200 OK` se produce después de que `Propose` complete con éxito.

Un nodo que no es leader devuelve:

```text
503 Service Unavailable
```

con:

```json
{"error":"no_es_lider"}
```

Si no se alcanza quorum dentro del timeout:

```text
504 Gateway Timeout
```

con:

```json
{"error":"tiempo_de_espera_agotado"}
```

#### Leer un valor

Sobre el leader:

```bash
docker compose exec raft-node-2 curl -s   'http://localhost:8080/kv/get?key=nombre'
```

Respuesta:

```json
{"key":"nombre","value":"kapumota"}
```

Antes de consultar la máquina de estados, `/kv/get` ejecuta `ReadIndex` y
confirma liderazgo mediante quorum.

Un follower devuelve `503`. Un leader que no puede confirmar quorum dentro del
timeout devuelve `504`.

#### Inyectar una partición manual

```bash
./scripts/partition.sh raft-node-3 20
```

Este script sirve para demostraciones manuales. Las campañas reproducibles usan
el runner y las configuraciones bajo `experiments/`.

#### Correctness

La Fase I documenta la correspondencia entre propiedad, mecanismo y evidencia:

- [`CORRECTNESS_INVARIANTS.md`](CORRECTNESS_INVARIANTS.md)
- [`EVIDENCE_MATRIX.md`](EVIDENCE_MATRIX.md)
- [`LIVENESS_MODEL.md`](LIVENESS_MODEL.md)

Las propiedades evaluadas incluyen:

```text
Election Safety
Log Matching
Leader Completeness
State Machine Safety
quorum commit
crash recovery
ReadIndex
liveness condicional
```

Las pruebas aportan evidencia dirigida. No constituyen formal verification.

#### Evaluación experimental

La campaña final usa la revisión experimental:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

Configuración principal:

```text
5 nodos
30 runs válidos por escenario
4 escenarios
120 runs válidos
60 segundos por run
16 clientes
2 operaciones/s
seed de workload = 42
```

Escenarios:

```text
sin fallas
follower caído
leader caído
partición de leader
```

Resultados:

[`experiments/RESULTS.md`](experiments/RESULTS.md)

Metodología:

[`experiments/METHODOLOGY.md`](experiments/METHODOLOGY.md)

Amenazas a la validez:

[`experiments/THREATS_TO_VALIDITY.md`](experiments/THREATS_TO_VALIDITY.md)

#### Análisis estadístico reproducible

```bash
make analyze   REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b   RUNS=30   CAMPAIGN=final
```

Incluye:

```text
mediana e IQR
MAD
diferencia de medianas
Cliff's delta
IC bootstrap 95 %
prueba de permutación bilateral
corrección de Holm
```

#### Reproducción del artefacto

No ejecutes `make reproduce` sobre `main` suponiendo que representa la revisión
experimental original.

La reproducción histórica de G4 usa una revisión de evidencia específica y un
`git worktree`.

Sigue:

[`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md)

La guía distingue:

```text
integridad de evidencia
reproducción histórica de G4
reproducción computacional de H2
nueva replicación experimental
```

#### Resultados principales

Dentro del protocolo experimental estudiado:

- La caída de un follower no conserva diferencias robustas frente al baseline
  después del ajuste de Holm.
- La caída del leader reduce throughput, incrementa p95 y p99 e incrementa las
  operaciones inciertas.
- La partición del leader reduce throughput e incrementa las operaciones
  inciertas.

Estos resultados describen la campaña evaluada y no demuestran universalmente
propiedades de safety o liveness.

#### Convenciones

- Identificadores, tipos, funciones y métodos de Go se mantienen en inglés.
- Comentarios, documentación y mensajes dirigidos a personas se escriben en
  español.
- Los términos `RequestVote`, `AppendEntries`, `leader`, `follower`,
  `commitIndex`, `nextIndex` y `matchIndex` se conservan.
- Los títulos principales usan `###` y los subtítulos `####`.
- La evidencia experimental no se presenta como garantía formal.

#### Roadmap después de v0.1.0

`v0.1.0` congela el artefacto actual. Las extensiones posteriores quedan fuera
del release:

```text
TLA+ / PlusCal
model checking
checker externo de linealizabilidad
snapshots y log compaction
membresía dinámica
fallas de red con delay/reorder/loss
evaluación multi-host
```

#### Licencia

RaftKV se distribuye bajo Apache License 2.0. Consulta [`LICENSE`](LICENSE).
