### Matriz final de evidencia de correctness

Este documento cierra la Fase I de RaftKV y consolida la trazabilidad entre:

```text
propiedad
-> mecanismo
-> archivo de producción
-> prueba ejecutable
-> tipo de evidencia
-> alcance
-> limitación
```

La matriz no convierte pruebas de software en demostraciones formales.

La regla de interpretación permanece:

```text
implementado != probado formalmente
testeado     != demostrado
observado    != garantizado
```

#### Fuentes de la Fase I

La matriz integra:

```text
CORRECTNESS_INVARIANTS.md
LIVENESS_MODEL.md
```

y las pruebas agregadas durante I2-I6:

```text
internal/raft/election_safety_test.go
internal/raft/log_matching_invariant_test.go
internal/raft/leader_completeness_test.go
internal/raft/state_machine_safety_test.go
internal/raft/liveness_test.go
```

También reutiliza pruebas previas de RaftKV:

```text
internal/raft/cluster_election_test.go
internal/raft/cluster_replication_test.go
internal/raft/cluster_partition_test.go
internal/raft/cluster_log_conflict_test.go
internal/raft/append_entries_regression_test.go
internal/raft/failover_test.go
internal/raft/raft_test.go
internal/raft/wal_test.go
internal/kv/deduplication_integration_test.go
internal/kv/store_test.go
cmd/raftnode/main_test.go
```

#### Niveles de evidencia

La matriz utiliza estas abreviaturas:

| Código | Nivel | Significado |
| --- | --- | --- |
| I | Implementado | Existe un mecanismo explícito en producción. |
| U | Unit tested | Una prueba aislada verifica un caso concreto. |
| X | Integration tested | Varios componentes o nodos participan en la ejecución. |
| F | Fault tested | La propiedad se observa bajo caída, partición, failover o recovery. |
| E | Experimental | Existe evidencia operacional de la Fase H. |
| P | Formal proof | Existe demostración o verificación formal del modelo declarado. |

En la revisión actual:

```text
P = no para todas las propiedades
```

#### Resumen ejecutivo

| ID | Propiedad | Evidencia | Estado |
| --- | --- | --- | --- |
| E1 | Election Safety | I U X F | fuerte, no exhaustiva |
| E2 | Persistencia de voto y frescura del log | I U X | fuerte |
| L1 | Log Matching | I U X F | fuerte |
| L2 | Prefijo confirmado inmutable | I U X F | fuerte |
| C1 | Commit solo por quorum | I U X F | fuerte |
| C2 | Leader Completeness | I X F | fuerte en ejecuciones dirigidas |
| S1 | State Machine Safety | I U X F | fuerte en ejecuciones dirigidas |
| S2 | Aplicación ordenada y no reaplicación local | I U X | fuerte |
| P1 | Persistencia y crash recovery | I U X F | fuerte para WAL local |
| R1 | Lecturas linealizables protegidas por quorum | I U X F | fuerte para el modelo probado |
| D1 | Deduplicación de efectos de cliente | I U X | fuerte bajo contrato de cliente |
| V1 | Liveness con quorum eventualmente estable | I X F E | condicional al failure model |

Ninguna fila debe interpretarse como `P`.

#### E1: Election Safety

Propiedad:

```text
Para cada término, como máximo un servidor puede convertirse en leader.
```

Mecanismos de producción:

```text
currentTerm
votedFor persistente
autovoto del candidate
mayoría estricta
rechazo de términos antiguos
```

Archivos principales:

```text
internal/raft/raft.go
internal/raft/wal.go
```

Evidencia ejecutable:

```text
TestInitialLeaderElection
TestLeaderReplacementAfterFailure
TestSingleLeaderPerTerm
TestVotePersistsAcrossRestartAndRejectsDifferentCandidateSameTerm
TestNewTermCanReplacePersistedVoteAfterRestart
```

Qué cubre:

```text
elección inicial
cambio de leader
varios términos observados
persistencia de votedFor
restart
rechazo de un segundo candidate en el mismo término
permiso para votar a otro candidate en un término posterior
```

Nivel:

```text
I U X F
```

Límite:

```text
TestSingleLeaderPerTerm observa ejecuciones concretas y no enumera todos los
interleavings posibles.
```

Conclusión permitida:

```text
RaftKV implementa y prueba de forma dirigida los mecanismos que sostienen
Election Safety bajo el modelo ejecutado.
```

Conclusión no permitida:

```text
Election Safety está demostrada formalmente para todos los schedules.
```

#### E2: persistencia de voto y frescura del log

Propiedad de soporte:

```text
Un servidor no concede votos incompatibles dentro del mismo término y solo vota
por un candidate cuyo log sea al menos tan actualizado como el propio.
```

Mecanismos:

```text
PersistentState.CurrentTerm
PersistentState.VotedFor
LastLogTerm
LastLogIndex
```

Archivos:

```text
internal/raft/raft.go
internal/raft/wal.go
```

Evidencia:

```text
TestVotePersistsAcrossRestartAndRejectsDifferentCandidateSameTerm
TestNewTermCanReplacePersistedVoteAfterRestart
TestRejectedStaleLogDoesNotConsumeVote
TestStaleTermIsRejected
TestWALSaveAndLoadState
```

Qué cubre:

```text
voto persistente
reinicio
segundo candidate rechazado
mismo candidate aceptado de nuevo
nuevo término permite nuevo voto
candidate con término de log inferior es rechazado
rechazo no consume votedFor
```

Nivel:

```text
I U X
```

Límite:

```text
No existe model checking de todas las combinaciones posibles de votos y logs.
```

#### L1: Log Matching

Propiedad:

```text
Si dos logs contienen una entrada con el mismo índice y término, comparten el
mismo prefijo hasta ese índice.
```

Mecanismos:

```text
PrevLogIndex
PrevLogTerm
rechazo de prefijo no coincidente
reemplazo desde el primer término conflictivo
nextIndex
matchIndex
```

Archivos:

```text
internal/raft/raft.go
internal/raft/wal.go
```

Evidencia:

```text
validateLogMatching
TestLogMatchingCheckerDetectsDivergentPrefixWithSharedIndexAndTerm
TestLogMatchingHoldsAcrossConflictingSuffixRepair
TestConflictingSuffixIsRepaired
TestAppendEntriesReplacesOnlyConflictingSuffix
TestDelayedAppendEntriesPreservesSuffix
TestFollowerCatchUp
```

Qué cubre:

```text
checker explícito del invariante
checker negativo que detecta una violación artificial
sufijo conflictivo real
AppendEntries retrasado
catch-up
reparación en memoria
reparación persistente en WAL
```

Nivel:

```text
I U X F
```

Límite:

```text
El checker observa estados alcanzados por los tests; no explora exhaustivamente
el espacio de estados del protocolo.
```

#### L2: prefijo confirmado inmutable

Propiedad:

```text
Una entrada que pertenece al prefijo confirmado no puede ser sustituida por un
conflicto posterior.
```

Mecanismo:

```text
HandleAppendEntries rechaza reemplazo cuando el conflicto cae antes de
commitIndex.
```

Archivo:

```text
internal/raft/raft.go
```

Evidencia:

```text
TestCommittedEntryIsPreserved
TestAppendEntriesCommitsOnlyMatchedPrefix
TestCommittedPrefixRejectsConflictAndSurvivesPendingSuffixRepair
```

Qué cubre:

```text
rechazo de conflicto dentro del prefijo committed
preservación de commitIndex
preservación de lastApplied
preservación de efectos aplicados
preservación en WAL
reparación válida de un sufijo pendiente posterior
```

Nivel:

```text
I U X F
```

#### C1: commit solo por quorum

Propiedad de servicio:

```text
Una propuesta no se considera confirmada hasta que un quorum suficiente respalda
el índice correspondiente.
```

Mecanismos:

```text
matchIndex
advanceCommitIndex
mayoría estricta
restricción al término actual
pendingCommits
```

Archivo:

```text
internal/raft/raft.go
```

Evidencia:

```text
TestMajorityCommit
TestSingleEntryReplication
TestMultipleEntryReplication
TestLeaderPartition
TestMinorityPartition
```

Caso crítico cubierto:

```text
clúster de 5 nodos

3 de 5:
    puede confirmar

2 de 5:
    puede almacenar una entrada
    no puede avanzar commitIndex
    no puede aplicar la entrada
```

Nivel:

```text
I U X F
```

Límite:

```text
La evidencia verifica configuraciones fijas de 3 y 5 nodos; RaftKV no implementa
membresía dinámica.
```

#### C2: Leader Completeness

Propiedad:

```text
Si una entrada está committed en un término, debe aparecer en el log de cualquier
leader de un término posterior.
```

Mecanismos de soporte:

```text
commit por mayoría
frescura de log en RequestVote
barrera NOOP del término del nuevo leader
reparación de logs por AppendEntries
```

Archivos:

```text
internal/raft/raft.go
internal/raft/wal.go
```

Evidencia:

```text
TestAcknowledgedWriteSurvivesLeaderFailure
TestOldLeaderRejoins
TestConflictingSuffixIsRepaired
TestCommittedEntryAppearsInSuccessiveLeaders
```

Escenario I4:

```text
5 nodos
commit con quorum mínimo 3/5
2 followers inicialmente stale
cambio a un primer leader posterior
catch-up de un follower stale
segundo cambio de leader
verificación de la entrada committed antes de nuevas escrituras
heal
convergencia final
```

Nivel:

```text
I X F
```

Límite:

```text
La prueba fuerza elecciones sucesivas concretas; no constituye una inducción
formal sobre todos los términos posibles.
```

#### S1: State Machine Safety

Propiedad:

```text
Si dos servidores aplican una entrada en el mismo índice, no deben aplicar
comandos diferentes en ese índice.
```

Mecanismos:

```text
commitIndex
lastApplied
applyCommitted
prefijo confirmed inmutable
Log Matching
```

Archivo:

```text
internal/raft/raft.go
```

Evidencia:

```text
validateStateMachineSafety
TestStateMachineSafetyCheckerDetectsDifferentCommandsAtSameIndex
TestStateMachineSafetyAcrossLeaderChangeAndConflictRepair
TestUncommittedConflictIsNeverApplied
TestMultipleEntryReplication
TestCommittedEntryIsPreserved
```

El checker compara explícitamente:

```text
Index
Term
Command
```

para cada índice aplicado común entre nodos.

También exige:

```text
0 <= lastApplied <= commitIndex <= len(log)
```

Nivel:

```text
I U X F
```

Límite:

```text
Se inspeccionan historiales alcanzados por las pruebas. No existe todavía un
checker externo de linealizabilidad sobre historiales concurrentes completos.
```

#### S2: aplicación ordenada y no reaplicación local

Propiedad:

```text
Cada nodo aplica secuencialmente el prefijo confirmado y no vuelve a aplicar un
índice que ya alcanzó lastApplied durante la misma reconstrucción.
```

Mecanismo:

```text
while lastApplied < commitIndex
lastApplied++
entry = log[lastApplied-1]
```

Archivos:

```text
internal/raft/raft.go
internal/raft/wal.go
```

Evidencia:

```text
TestMultipleEntryReplication
TestDelayedAppendEntriesPreservesSuffix
TestNewNodeRebuildsStateMachineFromCommittedLog
TestRecoveredEntriesAreNotReapplied
TestUncommittedConflictIsNeverApplied
```

Nivel:

```text
I U X
```

#### P1: persistencia y crash recovery

Propiedad de resiliencia:

```text
Después de reiniciar con el mismo almacenamiento persistente, el nodo recupera
el estado Raft y reconstruye la máquina de estados desde el prefijo confirmed.
```

Estado persistente:

```text
CurrentTerm
VotedFor
CommitIndex
Log
```

Archivos:

```text
internal/raft/wal.go
internal/raft/raft.go
```

Evidencia:

```text
TestWALSaveAndLoadState
TestWALAppendAndLoadLog
TestWALRewriteLog
TestNewNodeRestoresCommitIndex
TestNewNodeLimitsRecoveredCommitIndexToLog
TestNewNodeRebuildsStateMachineFromCommittedLog
TestRecoveredEntriesAreNotReapplied
TestCommittedStateSurvivesNodeRestart
```

Nivel:

```text
I U X F
```

Fuera de alcance:

```text
corrupción arbitraria del WAL
fallo parcial de fsync
pérdida física de almacenamiento
fallas bizantinas del storage
```

#### R1: lecturas linealizables protegidas por quorum

Objetivo:

```text
Un nodo aislado o stale no debe servir una lectura que se anuncie como
linealizable.
```

Mecanismos:

```text
barrera NOOP del término actual
ConfirmLeadership
ronda de AppendEntries posterior a la llamada
mayoría del mismo término
ReadIndex
```

Archivos:

```text
internal/raft/raft.go
cmd/raftnode/main.go
```

Evidencia:

```text
TestBecomeLeaderAppendsCurrentTermBarrier
TestConfirmLeadershipWithMajority
TestConfirmLeadershipRequiresMajority
TestFollowerCannotConfirmLeadership
TestReadIndexIncludesPreviousCommittedWrite
TestFollowerCannotGetReadIndex
TestIsolatedLeaderCannotServeLinearizableRead
TestLeaderPartition
TestMinorityPartition
```

La capa HTTP ejecuta `store.Get` solo después de que `ReadIndex` termine con
éxito.

Nivel:

```text
I U X F
```

Límite:

```text
No existe todavía un checker externo que tome historiales concurrentes de GET y
SET y verifique linealizabilidad completa.
```

#### D1: deduplicación de efectos de cliente

Objetivo:

```text
Un reintento de una solicitud ya aplicada no debe repetir su efecto en la máquina
de estados bajo el contrato de sesión declarado.
```

Mecanismos:

```text
ClientID
RequestID
ClientSession.LastRequestID
actualización atómica de dato y sesión bajo el mismo lock
```

Archivo:

```text
internal/kv/store.go
```

Evidencia:

```text
TestStoreDeduplicatesRequests
TestLostResponseRetryDoesNotApplyTwice
```

Casos cubiertos:

```text
reintento sin restart
respuesta perdida simulada
otra escritura entre original y retry
reintento después de reconstruir desde WAL
nuevo RequestID posterior aceptado
```

Nivel:

```text
I U X
```

Límite:

```text
No es exactly-once universal. Requiere ClientID estable, RequestID monotónico y
el contrato de cliente documentado.
```

#### V1: liveness bajo failure model declarado

Afirmación condicional:

```text
Con una mayoría operativa y comunicación eventualmente estable entre esa
mayoría, RaftKV muestra recuperación de leader, progreso de escrituras y
lecturas, y catch-up en las ejecuciones cubiertas.
```

Modelo detallado:

```text
LIVENESS_MODEL.md
```

Mecanismos:

```text
election timeout aleatorio
reintentos de elección
heartbeats y AppendEntries
nextIndex
matchIndex
ConfirmLeadership
commit por quorum
catch-up después de reconexión
```

Archivos:

```text
internal/raft/raft.go
internal/raft/transport.go
```

Evidencia I6:

```text
TestProgressResumesAfterTransientLossOfQuorum
TestStableMajorityMakesProgressWhileMinorityCannotConfirm
```

Evidencia complementaria:

```text
TestLeaderReplacementAfterFailure
TestFollowerCatchUp
TestLeaderPartition
TestMinorityPartition
TestOldLeaderRejoins
TestAcknowledgedWriteSurvivesLeaderFailure
TestConfirmLeadershipRequiresMajority
TestIsolatedLeaderCannotServeLinearizableRead
```

Frontera probada:

```text
sin quorum:
    no se exige progreso
    no se permite confirmar liderazgo

quorum estable:
    leader operativo
    escrituras confirmadas
    ReadIndex
    catch-up después de heal
```

Nivel:

```text
I X F E
```

Fuera del modelo:

```text
partición permanente sin quorum
red con retrasos indefinidos
scheduler que no ejecuta goroutines necesarias
fallas bizantinas
corrupción permanente del almacenamiento
```

#### Relación con la evidencia experimental

La Fase H aporta evidencia operacional sobre:

```text
throughput
p50
p95
p99
operaciones inciertas
restauración del entorno
```

Esa evidencia corresponde al nivel:

```text
E = Experimental
```

No debe usarse para elevar una propiedad de safety a `P`.

Ejemplo:

```text
30/30 restauraciones de un escenario
!=
demostración de liveness universal
```

De forma similar:

```text
ninguna violación observada en 30 runs
!=
demostración de safety
```

#### Cobertura por tipo de evidencia

Resumen cualitativo:

| Propiedad | I | U | X | F | E | P |
| --- | --- | --- | --- | --- | --- | --- |
| Election Safety | sí | sí | sí | sí | no | no |
| Voto y frescura | sí | sí | sí | no | no | no |
| Log Matching | sí | sí | sí | sí | no | no |
| Prefijo confirmed | sí | sí | sí | sí | no | no |
| Quorum commit | sí | sí | sí | sí | sí | no |
| Leader Completeness | sí | no | sí | sí | no | no |
| State Machine Safety | sí | sí | sí | sí | no | no |
| Aplicación ordenada | sí | sí | sí | no | no | no |
| Crash recovery | sí | sí | sí | sí | no | no |
| ReadIndex | sí | sí | sí | sí | no | no |
| Deduplicación | sí | sí | sí | no | no | no |
| Liveness condicional | sí | no | sí | sí | sí | no |

`no` no significa que la propiedad sea falsa. Significa que ese tipo específico
de evidencia no existe o no corresponde.

#### Gaps que permanecen después de I7

La Fase I no intenta cerrar estos gaps:

```text
TLA+ o PlusCal del protocolo implementado
model checking exhaustivo
checker externo de linealizabilidad
historiales concurrentes de clientes a gran escala
multi-host real
fallas de red con delay, reorder y packet loss controlados
corrupción y fallas parciales de almacenamiento
membresía dinámica
snapshots y log compaction
```

Estos puntos pertenecen a una evolución posterior y no bloquean el alcance de
RaftKV v0.1.0 como artefacto experimental y educativo.

#### Regla de mantenimiento

Si una modificación futura cambia cualquiera de estos elementos:

```text
RequestVote
AppendEntries
commitIndex
lastApplied
ReadIndex
WAL
semántica de ACK
deduplicación
failure model
```

debe revisarse esta matriz y volver a ejecutar las pruebas asociadas.

No debe conservarse una fila como evidencia vigente solo porque el nombre del
test siga existiendo.

#### Criterio de cierre de la Fase I

La Fase I puede considerarse cerrada cuando:

```text
I1  inventario explícito de propiedades
I2  Election Safety y persistencia de voto
I3  Log Matching y prefijo confirmed
I4  Leader Completeness
I5  State Machine Safety
I6  liveness bajo failure model declarado
I7  matriz final de evidencia
```

han sido integrados y `make validate` permanece verde.

#### Estado final

La conclusión correcta de la Fase I es:

```text
RaftKV posee mecanismos explícitos y evidencia ejecutable dirigida para las
principales propiedades de safety, recuperación, lectura y progreso incluidas
en su alcance actual.
```

La conclusión incorrecta sería:

```text
RaftKV está formalmente verificado.
```

La siguiente evolución de verificación debe tratar `P = Formal proof` como una
fase independiente, probablemente mediante TLA+/PlusCal, model checking y
mapeo explícito entre variables formales y estructuras Go.
