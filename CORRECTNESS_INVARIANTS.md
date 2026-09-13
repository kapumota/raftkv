### Correctness invariants and evidence

Este documento inicia la Fase I de RaftKV y mantiene un inventario explícito de
propiedades de correctness, mecanismos de implementación y evidencia ejecutable.

Su objetivo no es convertir pruebas de software en demostraciones formales.

La regla de interpretación de esta fase es:

```text
implementado != probado formalmente
testeado     != demostrado
observado    != garantizado
```

#### Revisión de referencia

La Fase I parte de `main` después del cierre de la Fase H.

Commit de cierre de H5:

```text
02aa3ec10d13ba605e579182ad281487727ae102
```

Las propiedades de este documento deben mantenerse trazables contra código y
pruebas de la revisión actual. Si una fase posterior modifica la semántica de
Raft, esta matriz debe revisarse.

#### Niveles de evidencia

La Fase I utiliza los siguientes niveles.

| Nivel | Significado |
| --- | --- |
| Implementado | Existe un mecanismo explícito en el código que intenta preservar la propiedad. |
| Unit tested | Una prueba aislada verifica un caso concreto del mecanismo. |
| Integration tested | Una prueba con varios nodos o capas verifica una ejecución concreta. |
| Fault tested | La propiedad se observa bajo caída, partición, failover o recuperación. |
| Experimentally observed | La campaña experimental ofrece evidencia operacional, no una prueba de safety. |
| Formally proven | Existe una demostración o verificación formal que cubre el modelo declarado. |

Actualmente ninguna propiedad de RaftKV debe clasificarse como `Formally proven`.

#### Fuentes de evidencia

La implementación principal de Raft se encuentra en:

```text
internal/raft/raft.go
internal/raft/types.go
internal/raft/wal.go
```

La máquina de estados KV se encuentra en:

```text
internal/kv/store.go
```

Las pruebas distribuidas principales incluyen:

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

#### Matriz inicial

| ID | Propiedad | Mecanismo principal | Evidencia ejecutable actual | Estado I1 |
| --- | --- | --- | --- | --- |
| E1 | Election Safety | voto persistente por término y elección por mayoría | `TestSingleLeaderPerTerm`, `TestInitialLeaderElection`, `TestLeaderReplacementAfterFailure` | evidencia parcial fuerte, no exhaustiva |
| E2 | Restricción de voto y frescura del log | `votedFor` persistente y comparación `(LastLogTerm, LastLogIndex)` | `TestStaleTermIsRejected`, pruebas de WAL y elecciones | mecanismo implementado, falta prueba dirigida de doble voto tras reinicio |
| L1 | Log Matching | `PrevLogIndex`/`PrevLogTerm`, reemplazo de sufijo conflictivo y backtracking de `nextIndex` | `TestConflictingSuffixIsRepaired`, `TestAppendEntriesReplacesOnlyConflictingSuffix`, `TestDelayedAppendEntriesPreservesSuffix` | evidencia fuerte en escenarios dirigidos |
| L2 | Inmutabilidad del prefijo confirmado | rechazo de reemplazo cuando `position < commitIndex` | `TestCommittedEntryIsPreserved`, `TestAppendEntriesCommitsOnlyMatchedPrefix` | evidencia fuerte en casos dirigidos |
| C1 | Commit solo por quorum | avance del leader únicamente cuando una mayoría tiene `matchIndex >= index` y la entrada es del término actual | `TestMajorityCommit`, pruebas de propuestas y failover | evidencia fuerte en 3 y 5 nodos |
| C2 | Leader Completeness | voto solo a candidatos con log suficientemente actualizado y commit mediante mayoría | `TestAcknowledgedWriteSurvivesLeaderFailure`, `TestOldLeaderRejoins`, `TestConflictingSuffixIsRepaired` | evidencia parcial fuerte, falta test dedicado con varias elecciones |
| S1 | State Machine Safety | aplicación secuencial hasta `commitIndex`; prefijo confirmado no reemplazable | `TestMultipleEntryReplication`, `TestCommittedEntryIsPreserved`, recuperación y particiones | evidencia parcial fuerte, falta checker explícito por índice |
| S2 | Aplicación en orden y sin reaplicación local | `lastApplied` avanza monotónicamente y `applyCommitted` recorre cada índice una vez | `TestMultipleEntryReplication`, `TestRecoveredEntriesAreNotReapplied`, regresiones de AppendEntries | evidencia fuerte para ejecuciones cubiertas |
| P1 | Persistencia de estado confirmado | WAL conserva término, voto, log y `commitIndex`; `NewNode` reconstruye entradas confirmadas | `TestWALSaveAndLoadState`, `TestCommittedStateSurvivesNodeRestart`, pruebas de recuperación | evidencia fuerte de crash/restart local |
| R1 | Lectura linealizable protegida por quorum | barrera NOOP del término actual, `ConfirmLeadership` y `ReadIndex` | `TestConfirmLeadershipWithMajority`, `TestConfirmLeadershipRequiresMajority`, `TestIsolatedLeaderCannotServeLinearizableRead`, particiones | evidencia fuerte para el modelo probado |
| D1 | Deduplicación de efectos de cliente | `(ClientID, RequestID)` y sesión aplicada bajo el mismo bloqueo que el efecto KV | `TestLostResponseRetryDoesNotApplyTwice`, `TestStoreDeduplicatesRequests` | evidencia fuerte bajo el contrato declarado |
| V1 | Progreso con quorum disponible | elecciones, replicación y catch-up con mayoría conectada | `TestFollowerCatchUp`, `TestLeaderReplacementAfterFailure`, `TestMinorityPartition` | evidencia de liveness limitada al failure model probado |

#### E1: Election Safety

Propiedad de referencia:

```text
Para cada término, como máximo un servidor puede convertirse en leader.
```

El mecanismo de RaftKV incluye:

```text
currentTerm
votedFor persistente
autovoto del candidato
mayoría estricta para convertirse en leader
rechazo de términos antiguos
```

`TestSingleLeaderPerTerm` observa varios cambios de liderazgo en un clúster de
cinco nodos y registra los leaders por término.

La propia prueba documenta correctamente su límite: utiliza muestreo temporal y
no demuestra todos los intercalados posibles.

Estado I1:

```text
implementado: sí
unit/integration tested: sí
fault tested: sí, mediante aislamiento de leaders
formalmente demostrado: no
```

Gap prioritario para I2:

```text
probar explícitamente que un nodo no concede dos votos diferentes en el mismo
término, incluso después de reiniciar desde el WAL
```

#### E2: restricción de voto y frescura del log

`HandleRequestVote` concede el voto únicamente cuando:

```text
votedFor está vacío o ya corresponde al mismo candidato
```

y el log del candidato es al menos tan actualizado según:

```text
LastLogTerm
LastLogIndex
```

Esta condición es esencial para Leader Completeness.

`TestStaleTermIsRejected` verifica que solicitudes de un término anterior no
alteren el log, el estado persistente, el voto ni el estado aplicado.

`TestWALSaveAndLoadState` verifica persistencia de:

```text
CurrentTerm
VotedFor
CommitIndex
```

Gap prioritario para I2:

```text
agregar un test de voto A, restart, intento de voto B en el mismo término
```

#### L1: Log Matching

Propiedad de referencia:

```text
Si dos logs contienen una entrada con el mismo índice y término, comparten el
mismo prefijo hasta ese índice.
```

Los mecanismos principales son:

```text
PrevLogIndex
PrevLogTerm
rechazo cuando el prefijo no coincide
reemplazo solo desde el primer término conflictivo
backtracking de nextIndex
```

La implementación además evita que una petición AppendEntries retrasada que
contiene solo un prefijo ya conocido elimine un sufijo válido.

Evidencia actual:

```text
TestConflictingSuffixIsRepaired
TestAppendEntriesReplacesOnlyConflictingSuffix
TestDelayedAppendEntriesPreservesSuffix
TestFollowerCatchUp
```

Gap prioritario para I3:

```text
agregar un checker reusable que compare logs de varios nodos y falle si una
entrada con igual índice y término no implica prefijo idéntico
```

#### L2: prefijo confirmado inmutable

RaftKV rechaza una sustitución de log cuando el primer conflicto cae dentro del
prefijo ya confirmado.

Evidencia principal:

```text
TestCommittedEntryIsPreserved
TestAppendEntriesCommitsOnlyMatchedPrefix
```

Esta protección es especialmente relevante porque una reparación legítima puede
eliminar entradas pendientes, pero no debe sustituir entradas confirmadas.

Estado I1:

```text
implementado: sí
testeado: sí
formalmente demostrado: no
```

#### C1: commit solo por quorum

El leader avanza `commitIndex` solo cuando:

```text
la entrada pertenece al término actual
una mayoría tiene matchIndex >= índice
```

La restricción al término actual evita confirmar directamente por conteo una
entrada de un término anterior. Una barrera NOOP del término del nuevo leader
permite que las entradas anteriores queden confirmadas indirectamente según las
reglas de Raft.

`TestMajorityCommit` distingue explícitamente:

```text
3 de 5 nodos: puede confirmar
2 de 5 nodos: puede almacenar, pero no confirmar ni aplicar
```

El contrato de `Propose` devuelve éxito solo después de que el índice esté
confirmado.

Estado I1:

```text
implementado: sí
integration tested: sí
fault tested: sí
formalmente demostrado: no
```

#### C2: Leader Completeness

Propiedad de referencia:

```text
Si una entrada está committed en un término, esa entrada debe aparecer en el
log de cualquier leader de un término posterior.
```

La evidencia actual combina:

```text
regla de frescura de RequestVote
commit por quorum
TestAcknowledgedWriteSurvivesLeaderFailure
TestOldLeaderRejoins
TestConflictingSuffixIsRepaired
```

`TestAcknowledgedWriteSurvivesLeaderFailure` confirma una escritura, elimina el
leader inicial y verifica que un follower que conserva la entrada pueda asumir
el liderazgo y seguir sirviendo progreso.

Esto es evidencia de una ejecución relevante, pero no una demostración general
de Leader Completeness.

Gap prioritario para I4:

```text
crear una prueba dedicada con una entrada committed en quorum mínimo y varias
elecciones posteriores, verificando la presencia de esa entrada en cada leader
observado antes de permitir nuevas operaciones
```

#### S1: State Machine Safety

Propiedad de referencia:

```text
Si dos servidores aplican una entrada en un índice dado, no deben aplicar
comandos diferentes en ese mismo índice.
```

El mecanismo actual aplica únicamente índices hasta `commitIndex` y avanza
`lastApplied` secuencialmente.

Las pruebas de replicación comparan:

```text
índice
término
comando
orden de comandos aplicados
WAL persistente
```

`TestMultipleEntryReplication` detecta pérdidas, duplicados y cambios de orden
en los callbacks de aplicación.

Sin embargo, el recorder actual guarda comandos y no una tupla explícita:

```text
(index, term, command)
```

Gap prioritario para I5:

```text
instrumentar un checker de State Machine Safety por índice y usarlo bajo
elecciones, conflictos y particiones
```

#### S2: aplicación sin reaplicación local

`applyCommitted` incrementa `lastApplied` antes de invocar el callback de la
máquina de estados y solo recorre mientras:

```text
lastApplied < commitIndex
```

La recuperación reconstruye la máquina de estados desde el prefijo confirmado y
restaura `lastApplied` al mismo progreso durante la inicialización.

La evidencia incluye:

```text
TestRecoveredEntriesAreNotReapplied
TestMultipleEntryReplication
TestDelayedAppendEntriesPreservesSuffix
```

Esto respalda ausencia de reaplicación en las ejecuciones cubiertas.

#### P1: crash recovery

RaftKV persiste:

```text
currentTerm
votedFor
commitIndex
log
```

`NewNode` limita el `commitIndex` recuperado a la longitud disponible del log y
reconstruye la máquina de estados solo desde entradas confirmadas.

Evidencia principal:

```text
TestWALSaveAndLoadState
TestNewNodeRestoresCommitIndex
TestNewNodeLimitsRecoveredCommitIndexToLog
TestNewNodeRebuildsStateMachineFromCommittedLog
TestRecoveredEntriesAreNotReapplied
TestCommittedStateSurvivesNodeRestart
```

Esta evidencia cubre reinicio con el mismo WAL.

No cubre:

```text
corrupción arbitraria del WAL
pérdida parcial del almacenamiento
fsync roto
fallas bizantinas
```

#### R1: lecturas linealizables

Al convertirse en leader, RaftKV agrega una barrera NOOP del término actual.

Antes de devolver un `ReadIndex`, el nodo:

```text
exige ser leader
exige barrera del término actual
confirma liderazgo mediante mayoría
verifica que la barrera esté committed
devuelve lastApplied
```

Evidencia principal:

```text
TestBecomeLeaderAppendsCurrentTermBarrier
TestConfirmLeadershipWithMajority
TestConfirmLeadershipRequiresMajority
TestReadIndexIncludesPreviousCommittedWrite
TestIsolatedLeaderCannotServeLinearizableRead
TestLeaderPartition
TestMinorityPartition
```

La capa HTTP consulta el KV solo después de que `ReadIndex` termine con éxito.

Estado I1:

```text
implementado: sí
unit/integration tested: sí
fault tested: sí
historiales verificados con checker externo de linealizabilidad: no
formalmente demostrado: no
```

#### D1: deduplicación de cliente

La deduplicación no es una propiedad central del protocolo Raft, pero es
relevante para la semántica observable de RaftKV ante respuestas perdidas.

El contrato requiere:

```text
ClientID estable
RequestID monotónico por cliente
una solicitud pendiente por cliente
```

La máquina de estados descarta solicitudes cuyo `RequestID` no supere el último
aplicado para el mismo `ClientID`.

`TestLostResponseRetryDoesNotApplyTwice` cubre:

```text
reintento sin restart
reintento después de reconstruir desde WAL
```

No debe describirse como exactly-once universal. La garantía depende del
contrato de IDs de cliente y de que la sesión pueda reconstruirse desde el log
confirmado.

#### V1: liveness bajo el failure model declarado

La Fase I separa safety de liveness.

La evidencia actual muestra progreso cuando existe una mayoría comunicada en
escenarios como:

```text
follower aislado
leader aislado y reemplazado
partición con mayoría de 3 sobre 5
reconexión y catch-up
```

También muestra que una minoría no puede confirmar escrituras ni lecturas
linealizables.

Esta evidencia no permite afirmar liveness bajo:

```text
partición permanente sin quorum
red asíncrona sin cotas
pérdida arbitraria permanente de mensajes
fallas bizantinas
corrupción de almacenamiento
```

I6 debe declarar explícitamente el failure model para cualquier afirmación de
progreso.

#### Diferencia entre safety, API y resiliencia

La matriz no debe mezclar categorías.

Propiedades Raft de safety:

```text
Election Safety
Log Matching
Leader Completeness
State Machine Safety
```

Mecanismos de servicio construidos sobre Raft:

```text
quorum commit antes de ACK
ReadIndex y lecturas linealizables
deduplicación de efectos de cliente
crash recovery desde WAL
```

Propiedades de progreso:

```text
elección eventual con quorum
replicación eventual a followers reconectados
catch-up después de partición
```

Las campañas de la Fase H aportan evidencia operacional sobre degradación y
recuperación, pero no sustituyen las pruebas dirigidas de esta fase.

#### Roadmap de Fase I

La secuencia propuesta es:

```text
I1  Inventario de invariantes y niveles de evidencia
I2  Election Safety y persistencia de voto
I3  Log Matching y prefijo confirmado
I4  Leader Completeness bajo cambios de leader
I5  State Machine Safety por índice
I6  Liveness bajo el failure model declarado
I7  Matriz final:
    propiedad -> mecanismo -> unit test -> integration test -> fault test
```

I2-I6 deben priorizar pruebas pequeñas y dirigidas. No se modifica el algoritmo
si la evidencia nueva no revela un defecto real.

#### Criterio para cambiar código de producción

Durante la Fase I, un cambio en `internal/raft` solo está justificado cuando una
prueba nueva demuestra uno de estos casos:

```text
violación de un invariante
estado persistente incompatible con el invariante
respuesta incorrecta frente a una transición válida
posibilidad reproducible de aplicar o confirmar estado inseguro
```

Si una propiedad ya está implementada correctamente, la fase debe agregar
evidencia y documentación, no reescribir Raft sin necesidad.

#### Estado de I1

I1 se considera completo cuando:

```text
1. las propiedades principales están nombradas explícitamente;
2. cada propiedad tiene mecanismos y tests trazables;
3. los gaps de evidencia están identificados;
4. ninguna prueba se presenta como demostración formal;
5. existe un roadmap dirigido para cerrar los gaps.
```
