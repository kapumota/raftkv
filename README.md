### RaftKV

[![CI](https://github.com/kapumota/raftkv/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kapumota/raftkv/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/tag/kapumota/raftkv?sort=semver&label=release)](https://github.com/kapumota/raftkv/tags)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/github/license/kapumota/raftkv)](LICENSE)

RaftKV es un key-value store distribuido experimental escrito en Go que
implementa Raft desde cero, sin `hashicorp/raft` ni otra biblioteca de consenso.
El proyecto combina implementación del protocolo, persistencia mediante WAL,
inyección de fallas, pruebas dirigidas de correctness y evaluación experimental
reproducible.

`v0.1.0` es el primer release experimental del proyecto. Su objetivo es ofrecer
un artefacto pequeño, auditable y reproducible para estudiar consenso,
replicación, failover, recuperación y evaluación de sistemas distribuidos.

RaftKV no se presenta como una base de datos lista para producción ni como una
implementación formalmente verificada de Raft.

#### Estado de v0.1.0

El release consolida tres capas:

```text
implementación Raft
        |
        v
evidencia ejecutable de correctness
        |
        v
evaluación experimental reproducible
```

La regla de interpretación del proyecto es:

```text
implementado != probado formalmente
testeado     != demostrado
observado    != garantizado
```

La matriz completa de propiedades y evidencia está en
[`EVIDENCE_MATRIX.md`](EVIDENCE_MATRIX.md).

#### Características

RaftKV `v0.1.0` incluye:

- elección de `leader` con timeouts aleatorios y mayoría;
- persistencia de `currentTerm`, `votedFor`, `commitIndex` y log;
- restricción de voto según frescura de `LastLogTerm` y `LastLogIndex`;
- replicación mediante `AppendEntries`;
- reparación de sufijos conflictivos con `nextIndex` y `matchIndex`;
- protección del prefijo confirmado;
- avance de `commitIndex` por quorum;
- restricción de commit directo a entradas del término actual;
- barrera `NOOP` del término del leader;
- `ConfirmLeadership` y `ReadIndex`;
- confirmación HTTP de escrituras únicamente después de `Propose` exitoso;
- reconstrucción de la máquina de estados desde el log confirmado;
- deduplicación de efectos mediante `ClientID` y `RequestID`;
- failover, catch-up y recuperación después de particiones;
- pruebas con detector de carreras;
- campañas de fallas reproducibles;
- análisis estadístico reproducible;
- figuras SVG deterministas y manifests SHA-256.

#### Alcance y límites

El release mantiene deliberadamente un alcance compacto:

- membresía fija;
- sin joint consensus;
- sin snapshots;
- sin compactación del log;
- WAL potencialmente creciente;
- nodos no bizantinos;
- almacenamiento local operativo dentro del failure model;
- evaluación experimental en un único host con Docker bridge;
- sin modelado exhaustivo de delay, reorder o packet loss;
- sin corrupción arbitraria de almacenamiento;
- sin checker externo de linealizabilidad sobre historiales concurrentes;
- sin especificación TLA+/PlusCal;
- sin model checking exhaustivo;
- sin garantías de uso en producción.

La deduplicación de la máquina de estados requiere `ClientID` estable,
`RequestID` creciente y una solicitud pendiente por cliente. No constituye una
garantía exactly-once general para cualquier cliente HTTP.

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
   +----------- RequestVote / AppendEntries -----------+
   |                                                   |
   v                                                   v
followers                                          WAL por nodo
   |                                                   |
   +---------------------- quorum ----------------------+
                              |
                              v
                         commitIndex
                              |
                              v
                    máquina de estados KV
```

El despliegue Docker de referencia usa cinco nodos `raft-node-*`, volúmenes WAL
independientes y la red bridge `raftnet`.

#### Estructura del repositorio

```text
raftkv/
|-- .github/
|   `-- workflows/
|       `-- ci.yml
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
|   |-- BENCHMARKS.md
|   |-- METHODOLOGY.md
|   |-- METRICS.md
|   |-- README.md
|   |-- RESULTS.md
|   |-- RUNNER.md
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
|-- go.sum
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

La reproducción histórica de la campaña final tiene requisitos adicionales
documentados en
[`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md).

#### Validación local

La validación principal es:

```bash
make validate
```

Incluye:

```text
gofmt check
go vet ./...
go test -race ./...
go build ./...
bash -n de los scripts experimentales
```

También se pueden ejecutar targets individuales:

```bash
make fmt-check
make vet
make test
make test-race
make build
make scripts-check
```

GitHub Actions ejecuta CI para cada `push` y `pull_request`.

#### Levantar el clúster completo

```bash
make up
```

Este target ejecuta el Compose principal, incluidos los cinco nodos y
`client-driver`.

Para seguir los logs:

```bash
make logs
```

Para detener el despliegue:

```bash
make down
```

#### Demo manual sin client-driver

Para probar la API sin carga generada por `client-driver`:

```bash
docker compose up -d --build \
  raft-node-1 \
  raft-node-2 \
  raft-node-3 \
  raft-node-4 \
  raft-node-5
```

Consulta los cinco nodos:

```bash
for i in 1 2 3 4 5; do
  docker compose exec "raft-node-$i" \
    curl -s http://localhost:8080/status
  echo
done
```

Una respuesta típica es:

```json
{
  "commit_index": 3,
  "id": "raft-node-2",
  "state": "leader",
  "term": 2,
  "voted_for": "raft-node-2"
}
```

Usa el nodo cuyo campo `state` sea `leader` para las operaciones KV.

#### Escrituras

Suponiendo que `raft-node-2` es el leader:

```bash
docker compose exec raft-node-2 curl -s -X POST \
  http://localhost:8080/kv/set \
  -H 'Content-Type: application/json' \
  -d '{"key":"nombre","value":"kapumota"}'
```

Una escritura confirmada responde:

```json
{"estado":"confirmado"}
```

`200 OK` se devuelve después de que `Propose` termina con éxito.

Si el nodo no es leader:

```text
503 Service Unavailable
```

```json
{"error":"no_es_lider"}
```

Si no puede alcanzarse quorum dentro del timeout de la API:

```text
504 Gateway Timeout
```

```json
{"error":"tiempo_de_espera_agotado"}
```

#### Lecturas

Sobre el leader:

```bash
docker compose exec raft-node-2 curl -s \
  'http://localhost:8080/kv/get?key=nombre'
```

Respuesta:

```json
{"key":"nombre","value":"kapumota"}
```

Antes de consultar la máquina de estados, `/kv/get` ejecuta `ReadIndex` y
confirma liderazgo mediante quorum.

Un follower devuelve `503`. Un leader que no puede confirmar quorum dentro del
timeout devuelve `504`.

#### Correctness

La Fase I documenta la correspondencia entre propiedad, mecanismo de producción
y evidencia ejecutable.

Documentos principales:

- [`CORRECTNESS_INVARIANTS.md`](CORRECTNESS_INVARIANTS.md): inventario de
  propiedades e invariantes;
- [`EVIDENCE_MATRIX.md`](EVIDENCE_MATRIX.md): matriz final de trazabilidad;
- [`LIVENESS_MODEL.md`](LIVENESS_MODEL.md): failure model y alcance de las
  afirmaciones de progreso.

La evidencia cubre, dentro de su alcance:

| Propiedad | Evidencia |
| --- | --- |
| Election Safety | Implementación, unit, integración y fallas |
| Persistencia de voto y frescura | Implementación, unit e integración |
| Log Matching | Implementación, unit, integración y fallas |
| Prefijo confirmado inmutable | Implementación, unit, integración y fallas |
| Commit por quorum | Implementación, unit, integración y fallas |
| Leader Completeness | Implementación, integración y fallas |
| State Machine Safety | Implementación, unit, integración y fallas |
| Crash recovery | Implementación, unit, integración y fallas |
| Lecturas protegidas por quorum | Implementación, unit, integración y fallas |
| Liveness | Condicional al failure model declarado |

Ninguna de estas filas debe interpretarse como una demostración formal.

#### Failure model de liveness

La afirmación de progreso requiere, entre otros supuestos:

```text
membresía fija
nodos no bizantinos
WAL local operativo
mayoría disponible
comunicación eventualmente estable entre esa mayoría
```

Por tanto:

```text
sin quorum:
    no se exige progreso

quorum + comunicación eventualmente estable:
    se espera recuperación y progreso en el modelo probado
```

Consulta [`LIVENESS_MODEL.md`](LIVENESS_MODEL.md) para el alcance completo.

#### Evaluación experimental

La campaña final corresponde a la revisión:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

Diseño:

```text
5 nodos
4 escenarios
30 runs válidos por escenario
120 runs válidos
60 segundos por run
16 clientes
2 operaciones/s
seed de workload = 42
falla aproximadamente en t=20 s durante 10 s
```

Escenarios:

```text
sin fallas
follower caído
leader caído
partición de leader
```

La unidad estadística es el run completo.

El análisis incluye:

```text
18 comparaciones inferenciales
100000 réplicas bootstrap
100000 permutaciones
confidence level = 0.95
alpha = 0.05
corrección de Holm
analysis seed = 20260912
```

#### Resultados principales

De las 18 comparaciones planificadas, 6 mantienen evidencia después de la
corrección de Holm.

| Escenario | Hallazgos con evidencia después de Holm |
| --- | --- |
| Follower caído | Ninguna de las seis métricas |
| Leader caído | Throughput, p95, p99, operaciones inciertas |
| Partición de leader | Throughput, operaciones inciertas |

Para la caída del leader:

```text
throughput: -8.82 %
p95:        +41.72 %
p99:        +57.73 %
inciertas:  +11 en diferencia de medianas
```

Para la partición del leader:

```text
throughput: -8.40 %
inciertas:  +10 en diferencia de medianas
```

Los resultados completos, intervalos bootstrap, Cliff's delta y valores p
ajustados están en
[`experiments/RESULTS.md`](experiments/RESULTS.md).

Estos resultados describen exclusivamente el protocolo experimental estudiado.
No demuestran universalmente safety, liveness ni capacidad máxima del sistema.

#### Documentación experimental

La documentación autoritativa de la campaña final es:

- [`experiments/METHODOLOGY.md`](experiments/METHODOLOGY.md): diseño y protocolo;
- [`experiments/STATISTICAL_ANALYSIS.md`](experiments/STATISTICAL_ANALYSIS.md):
  análisis H2;
- [`experiments/THREATS_TO_VALIDITY.md`](experiments/THREATS_TO_VALIDITY.md):
  límites de validez;
- [`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md):
  integridad y reproducción;
- [`experiments/RESULTS.md`](experiments/RESULTS.md): resultados finales.

[`experiments/BENCHMARKS.md`](experiments/BENCHMARKS.md) conserva la evolución
histórica de G2/G3. [`experiments/RUNNER.md`](experiments/RUNNER.md) documenta
ejecuciones manuales del runner.

#### Nueva campaña experimental

Para ejecutar una campaña nueva desde la revisión actual:

```bash
make experiment RUNS=2 CAMPAIGN=smoke
```

Para la campaña por defecto:

```bash
make experiment
```

`make experiment` exige un árbol Git limpio, ejecuta validación y no sobrescribe
una campaña existente.

Cada run de la campaña automatizada parte de un despliegue con volúmenes nuevos.
Los resultados quedan bajo:

```text
experiments/raw/<revision>/<campaign>/
experiments/processed/<revision>/<campaign>/
```

Las nuevas campañas locales están ignoradas por Git hasta que se decida
versionarlas explícitamente.

#### Reproducción del artefacto publicado

No debe ejecutarse `make reproduce` sobre el `HEAD` de `v0.1.0` suponiendo que
ese código es la revisión experimental original.

La campaña final fue generada desde otra revisión y su reproducción histórica
usa un `git worktree` y una revisión de evidencia específica.

Sigue exactamente:

[`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md)

La guía distingue:

```text
verificación de integridad
reproducción histórica de G4
reproducción computacional de H2
nueva replicación experimental
```

El análisis estadístico congelado puede comprobarse con:

```bash
make analyze \
  REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b \
  RUNS=30 \
  CAMPAIGN=final
```

#### Integridad de la evidencia

La campaña conserva manifests SHA-256 para vincular:

```text
JSON crudos
configuraciones
fuentes del análisis
artefactos estadísticos
figuras
```

Las figuras deterministas se encuentran en:

```text
experiments/figures/
```

La verificación completa está documentada en
[`experiments/ARTIFACT_REPRODUCTION.md`](experiments/ARTIFACT_REPRODUCTION.md).

#### CI

El workflow:

```text
.github/workflows/ci.yml
```

ejecuta:

```text
gofmt check
go vet
go test -race ./...
go build ./...
make scripts-check
```

El badge `CI` de la cabecera refleja el estado de `main`.

El badge `Release` usa los tags SemVer del repositorio. Después de crear el tag
`v0.1.0`, mostrará automáticamente esa versión.

#### Convenciones

- Identificadores, tipos, funciones y métodos Go se mantienen en inglés.
- Comentarios, documentación y mensajes dirigidos a personas se escriben en
  español.
- `RequestVote`, `AppendEntries`, `leader`, `follower`, `commitIndex`,
  `lastApplied`, `nextIndex` y `matchIndex` se conservan como vocabulario Raft.
- Los títulos principales de documentación usan `###` y los subtítulos `####`.
- Se evita presentar evidencia experimental como garantía formal.

#### Roadmap posterior a v0.1.0

`v0.1.0` congela el alcance actual. Las extensiones posteriores quedan fuera de
este release:

```text
TLA+ / PlusCal
model checking
checker externo de linealizabilidad
snapshots y log compaction
membresía dinámica
fallas con delay/reorder/loss
evaluación multi-host
```

#### Licencia

RaftKV se distribuye bajo Apache License 2.0.

Consulta [`LICENSE`](LICENSE).
