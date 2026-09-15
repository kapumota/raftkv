### RaftKV v0.1.0

`v0.1.0` es el primer release experimental de RaftKV, un key-value store distribuido escrito en Go que implementa Raft desde cero.

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

RaftKV no se presenta como una base de datos lista para producción ni como una implementación formalmente verificada.

#### Qué incluye

La implementación incorpora elección de `leader`, persistencia de `currentTerm`, `votedFor`, `commitIndex` y log, replicación con `AppendEntries`, reparación de sufijos conflictivos, commit por quorum, barrera `NOOP`, `ConfirmLeadership`, `ReadIndex`, reconstrucción de la máquina de estados, deduplicación bajo contrato de cliente, failover, catch-up, recovery y pruebas con detector de carreras.

#### Correctness y liveness

La evidencia dirigida cubre Election Safety, persistencia de voto y frescura del log, Log Matching, prefijo confirmado inmutable, commit por quorum, Leader Completeness, State Machine Safety, aplicación ordenada, persistencia y crash recovery, lecturas protegidas por quorum, deduplicación bajo contrato de cliente y liveness bajo quorum eventualmente estable.

Ninguna de estas propiedades se presenta como formalmente probada.

Documentos principales:

```text
CORRECTNESS_INVARIANTS.md
EVIDENCE_MATRIX.md
LIVENESS_MODEL.md
```

#### Correcciones de estabilización previas al release

Durante el freeze de `v0.1.0`, la validación repetida con `go test -race` detectó dos problemas de temporización en elecciones:

```text
1. electionTicker podía iniciar una elección usando una observación de timeout
   que había quedado obsoleta después de un heartbeat.

2. un RequestVote con término superior pero log obsoleto podía actualizar el
   término y reiniciar el election timer aunque el voto finalmente se rechazara.
```

Las correcciones conservan `startElection()` como primitiva explícita para las pruebas y aplican la revalidación únicamente al camino automático del ticker.

#### Campaña experimental

Revisión experimental:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

Ancla histórica G4:

```text
26358a950e7b40eb7e4ea36919dc60cc6ce660b1
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
```

#### Resultados principales

De 18 comparaciones inferenciales planificadas, 6 mantienen evidencia después de Holm.

Para caída del leader:

```text
throughput: -8.82 %
p95:        +41.72 %
p99:        +57.73 %
uncertain_writes: +11 en diferencia de medianas
```

Para partición del leader:

```text
throughput: -8.40 %
uncertain_writes: +10 en diferencia de medianas
```

La caída de un follower no presenta diferencias robustas frente al baseline después de Holm.

#### Reproducibilidad

Antes del release se verificó:

```text
integridad de los 120 JSON
integridad de las configuraciones
integridad de las fuentes H2
integridad de artefactos estadísticos
reproducción histórica G4 sin diff
reproducción H2 byte-for-byte
regeneración determinista de figuras H5
```

La reproducción histórica de G4 debe seguir `experiments/ARTIFACT_REPRODUCTION.md`.

#### Límites de v0.1.0

No incluye:

```text
TLA+ / PlusCal
model checking exhaustivo
checker externo de linealizabilidad
snapshots
log compaction
membresía dinámica
tolerancia bizantina
evaluación multi-host
delay/reorder/loss exhaustivos
garantías de producción
```

#### Validación del release candidate

Release candidate técnico previo a estas notas:

```text
2ed44f32c7e7e4d32a01f46a765b20216eeba0e7
```

El tag `v0.1.0` se creará únicamente después del PR final, CI remoto verde, merge a `main` y validación posterior al merge.

#### Licencia

Apache License 2.0.
