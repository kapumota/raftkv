### Changelog

Todos los cambios relevantes de RaftKV se documentan en este archivo.

#### v0.1.0 - 2026-09-14

Primer release experimental de RaftKV.

##### Added

- Implementación de Raft desde cero en Go para un clúster fijo de cinco nodos.
- Elección de leader mediante `RequestVote`, mayoría estricta y persistencia de `currentTerm` y `votedFor`.
- Replicación de log mediante `AppendEntries`, `nextIndex` y `matchIndex`.
- Commit por quorum con restricción de commit directo a entradas del término actual.
- Barrera `NOOP` del término del leader.
- `ConfirmLeadership` y `ReadIndex` para lecturas protegidas por quorum.
- Persistencia WAL y reconstrucción de la máquina de estados desde el log confirmado.
- Máquina de estados KV con `SET` y `GET`.
- Deduplicación de efectos mediante `ClientID` y `RequestID` bajo contrato de cliente.
- Pruebas dirigidas para Election Safety, Log Matching, Leader Completeness y State Machine Safety.
- Pruebas de failover, recovery, catch-up, particiones y liveness bajo un failure model explícito.
- Inyección de fallas y campañas experimentales reproducibles.
- Campaña final de 120 runs válidos en cuatro escenarios.
- Análisis estadístico reproducible con bootstrap, permutaciones, Cliff's delta y corrección de Holm.
- Figuras SVG deterministas y manifests SHA-256.
- Documentación de correctness, liveness, metodología, amenazas a la validez y reproducción del artefacto.
- CI con formato, `go vet`, `go test -race`, compilación y validación de scripts.
- Badges de CI, release, Go y licencia en el README principal.

##### Fixed

- Se evitó iniciar elecciones automáticas desde observaciones obsoletas del election timeout.
- Los `RequestVote` rechazados ya no reinician indebidamente el election timer.
- Se estabilizaron elecciones sucesivas y recuperación de progreso bajo los escenarios cubiertos por la suite de pruebas.

##### Experimental results

La campaña final usa:

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

Después de la corrección de Holm, 6 de 18 comparaciones mantienen evidencia estadística dentro del protocolo estudiado:

```text
leader caído:
  throughput
  p95
  p99
  uncertain_writes

partición de leader:
  throughput
  uncertain_writes
```

La caída de un follower no presenta diferencias robustas frente al baseline después del ajuste por multiplicidad.

##### Limits

`v0.1.0` es un artefacto experimental y educativo. No incluye:

- verificación formal,
- TLA+ o PlusCal,
- model checking exhaustivo,
- checker externo de linealizabilidad,
- snapshots,
- log compaction,
- membresía dinámica,
- tolerancia bizantina,
- evaluación multi-host,
- modelado exhaustivo de delay, reorder o packet loss,
- garantías de producción.
