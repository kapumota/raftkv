### Liveness failure model

Este documento declara las condiciones bajo las cuales RaftKV interpreta y
prueba afirmaciones de liveness.

La Fase I separa estrictamente:

```text
safety: nada incorrecto debe ocurrir
liveness: eventualmente debe ocurrir progreso bajo supuestos declarados
```

Las pruebas de liveness de RaftKV son evidencia ejecutable bajo un modelo de
fallas concreto. No constituyen una demostración formal de progreso para todos
los schedules o redes posibles.

#### Alcance del modelo

La evidencia de liveness asume:

```text
membresía fija
nodos no bizantinos
procesos que continúan ejecutando goroutines y temporizadores
WAL local disponible sin corrupción permanente
mayoría de nodos operativos para las operaciones que requieren quorum
comunicación eventualmente estable entre los nodos de esa mayoría
RPC que, después de un periodo transitorio, pueden completarse
```

La expresión `eventualmente estable` significa que las particiones, pérdidas de
conectividad o periodos de indisponibilidad relevantes terminan y dejan una
mayoría capaz de intercambiar RPC durante tiempo suficiente para:

```text
completar una elección
confirmar la barrera NOOP del término
replicar entradas
avanzar commitIndex
```

No se asume una cota matemática global para el tiempo de recuperación.

#### Fallas cubiertas

El harness y las pruebas actuales modelan principalmente:

```text
aislamiento de un follower
aislamiento de un leader
particiones de red entre grupos
pérdida temporal de quorum
restauración de enlaces
catch-up de followers atrasados
failover después de pérdida del leader
reinicio local con WAL persistente
```

Estas fallas son crash/omission-like desde la perspectiva del protocolo.

#### Fallas fuera del modelo

La Fase I no afirma liveness frente a:

```text
partición permanente sin mayoría comunicada
red asíncrona que retrasa mensajes indefinidamente
pérdida permanente de todos los mensajes relevantes
scheduler que nunca ejecuta una goroutine necesaria
fallas bizantinas
corrupción arbitraria del WAL
errores permanentes de almacenamiento
pausas indefinidas del proceso
membresía dinámica no implementada
```

En cualquiera de estos casos, ausencia de progreso no contradice el modelo de
liveness declarado.

#### Quorum como frontera de progreso

Para un clúster de tamaño `N`, RaftKV necesita más de `N/2` nodos comunicados
para las operaciones que exigen quorum.

Por tanto:

```text
mayoría disponible y eventualmente estable:
    se espera recuperación de leader y progreso

sin mayoría disponible:
    no se exige progreso
```

Una minoría puede conservar estado local o incluso un nodo puede continuar
creyendo temporalmente que es leader, pero no debe confirmar liderazgo,
escrituras ni lecturas linealizables sin quorum.

Esta distinción conecta liveness con safety:

```text
la falta de quorum puede detener progreso
pero no autoriza a relajar las reglas de confirmación
```

#### Elección eventual

RaftKV utiliza election timeouts aleatorios y reintenta elecciones cuando un
follower o candidate supera su timeout.

La evidencia ejecutable verifica que, después de recuperar una mayoría
comunicada, `WaitForLeader` observa un leader capaz de confirmar liderazgo por
quorum.

Esto respalda la afirmación:

```text
bajo las ejecuciones probadas y con mayoría eventualmente estable,
el clúster recupera un leader operativo
```

No respalda la afirmación más fuerte:

```text
para todo schedule posible, siempre se elegirá un leader dentro de una cota fija
```

#### Progreso de escrituras

`Propose` devuelve éxito únicamente cuando la entrada alcanza `commitIndex`.

Bajo una mayoría estable, las pruebas de I6 exigen que el leader pueda:

```text
replicar nuevas entradas
obtener quorum
avanzar commitIndex
aplicar los comandos
responder éxito
```

Cuando no existe quorum, I6 no exige que una propuesta complete.

Los timeouts utilizados por los tests son límites de observación del test, no
garantías temporales del protocolo.

#### Progreso de lecturas

Las lecturas linealizables dependen de `ReadIndex`.

Para completar una lectura, el leader debe:

```text
conservar la barrera NOOP del término actual
confirmar liderazgo mediante una mayoría
mantener la barrera committed
```

Por tanto, la liveness de lectura comparte la misma dependencia de quorum que
la confirmación de liderazgo.

Una minoría no tiene obligación de servir una lectura linealizable.

#### Catch-up después de restaurar conectividad

Cuando un follower vuelve a estar comunicado con un leader operativo, los
reintentos de AppendEntries y el ajuste de `nextIndex` permiten reparar y
completar el log.

I6 considera progreso de recuperación cuando, después de sanar la red:

```text
se obtiene un leader con quorum
los followers vuelven a recibir el prefijo confirmado
commitIndex y lastApplied alcanzan el prefijo esperado
```

Esto es evidencia de eventual catch-up en los escenarios probados, no una cota
formal de tiempo de convergencia.

#### Evidencia I6

I6 agrega:

```text
TestProgressResumesAfterTransientLossOfQuorum
TestStableMajorityMakesProgressWhileMinorityCannotConfirm
```

La primera prueba cubre:

```text
clúster de 3 nodos
commit inicial
pérdida temporal total de quorum
imposibilidad de confirmar liderazgo
restauración de la red
nueva elección con quorum
nueva escritura confirmada
ReadIndex posterior
```

La segunda prueba cubre:

```text
clúster de 5 nodos
partición 2/3
leader antiguo en la minoría
nuevo leader en la mayoría
varias escrituras consecutivas en la mayoría
ReadIndex en la mayoría
ausencia de nuevos efectos aplicados en la minoría
heal
catch-up completo
```

Se complementan con evidencia previa:

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

#### Afirmaciones permitidas

Después de I6, una formulación aceptable es:

```text
RaftKV muestra recuperación de liderazgo, progreso de escrituras y lecturas, y
catch-up cuando existe o se restablece una mayoría comunicada y suficientemente
estable bajo el failure model probado.
```

También es aceptable:

```text
Una minoría no puede confirmar liderazgo ni se espera que progrese hasta que se
restaure una mayoría.
```

#### Afirmaciones no permitidas

No debe escribirse:

```text
RaftKV garantiza liveness en cualquier red.
RaftKV siempre recupera un leader en tiempo fijo.
RaftKV progresa durante cualquier partición.
Las pruebas demuestran formalmente liveness.
```

#### Relación con la Fase H

La Fase H mide comportamiento operacional bajo fallas controladas.

La Fase I6 usa pruebas dirigidas para comprobar condiciones concretas de
progreso.

Por tanto:

```text
Fase H:
    cuánto se degrada y recupera el sistema en una campaña experimental

Fase I6:
    bajo qué supuestos declarados se observa progreso funcional
```

Una campaña experimental no sustituye una prueba dirigida de liveness, y una
prueba dirigida tampoco sustituye mediciones de rendimiento.

#### Estado I6

I6 se considera completo cuando:

```text
1. el failure model está declarado;
2. la ausencia de quorum queda fuera de la obligación de progreso;
3. se prueba recuperación después de pérdida temporal de quorum;
4. se prueba progreso sostenido en una mayoría estable;
5. se prueba catch-up después del heal;
6. los límites de la evidencia se documentan explícitamente.
```
