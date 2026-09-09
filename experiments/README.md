# Escenarios de inyección de fallas

F1 define el formato declarativo. F2 incorpora el parser y el runner Go para
Linux y Docker local. La guía [RUNNER.md](RUNNER.md) describe cómo validar
escenarios y ejecutar experimentos conservando los volúmenes existentes.

## Escenarios

| Archivo | Objetivo | Falla programada | Restauración programada | Fin |
| --- | --- | --- | --- | --- |
| `leader_failure.yaml` | Líder | Caída a los 20 s | 30 s | 60 s |
| `follower_failure.yaml` | Seguidor | Caída a los 20 s | 30 s | 60 s |
| `leader_partition.yaml` | Líder | Partición a los 20 s | 30 s | 60 s |
| `recovery.yaml` | Seguidor | Caída a los 15 s | 45 s | 90 s |

La recuperación prolonga la ausencia del seguidor y deja una ventana posterior
para observar cómo alcanza el log confirmado. Todos los escenarios mantienen
carga antes, durante y después de la falla.

## Contrato de configuración

Todos los campos son obligatorios. El parser deberá rechazar campos desconocidos,
claves duplicadas, documentos múltiples y valores con tipos incorrectos.

- `version`: entero; la única versión admitida inicialmente será `1`.
- `nombre`: texto no vacío para identificar el experimento en los resultados.
- `semilla`: entero no negativo de 64 bits para generar la carga reproducible.
- `duracion_segundos`: entero positivo; ventana total de generación de carga.
- `clientes`: entero positivo; máximo de clientes con una operación pendiente cada uno.
- `operaciones_por_segundo`: entero positivo; tasa objetivo global, no por cliente.
- `falla.tipo`: `caida` o `particion`.
- `falla.objetivo`: `lider` o `seguidor`.
- `falla.instante_segundos`: entero positivo medido desde el inicio de la carga.
- `falla.duracion_segundos`: entero positivo durante el cual se mantiene la falla.

La suma del instante y la duración de la falla debe ser menor que la duración
total, con validación que evite desbordamientos. Debe quedar tiempo tanto antes
de la falla como después de la restauración.

La primera versión generará escrituras SET con claves distintas por operación.
La semilla determina sus datos; los clientes reciben las operaciones en orden
cíclico. La tasa solicitada no garantiza la tasa alcanzada: si el cliente
asignado sigue ocupado, la operación programada se registra como omitida por
saturación, sin acumular una cola ilimitada ni generar una ráfaga posterior.

Los errores y timeouts se registran sin reintentos automáticos en esta primera
versión. Un timeout no demuestra que la escritura no se haya confirmado.

## Selección y restauración

El runner debe esperar que el clúster esté disponible antes de iniciar su reloj.
En el instante de la falla resolverá el rol solicitado usando el estado actual
de los nodos. Si no puede identificar un líder único, el experimento termina
con error; no se sustituye silenciosamente por otro nodo.

Para un seguidor se selecciona el primero por identificador en orden
lexicográfico entre los seguidores disponibles. El nodo elegido queda fijado:
la restauración afecta a ese mismo nodo aunque cambien los roles. El resultado
debe registrar su identificador, término observado y los tiempos reales.

`caida` significa terminar abruptamente el proceso y volver a iniciar el mismo
nodo conservando su volumen WAL. `particion` significa cortar su comunicación
con los demás nodos sin detener el proceso y después restaurar sus enlaces.

La duración de la falla se cuenta desde que su aplicación termina correctamente.
El runner debe registrar cualquier desviación respecto de los tiempos
programados. Al terminar la ventana de carga deja de emitir operaciones nuevas,
resuelve las pendientes con plazos acotados y realiza la limpieza necesaria.
La restauración debe intentarse también ante error o cancelación; si falla,
el resultado debe indicarlo y no declarar el experimento exitoso.

## Despliegue y reproducibilidad

Las direcciones, contenedores y red pertenecen a la configuración de despliegue,
no a estos escenarios. El Compose actual usa los nodos `raft-node-1` a
`raft-node-5`, el puerto interno `8080` y la red lógica `raftnet`; no publica
puertos HTTP en el host. F2 deberá resolver tanto el acceso HTTP como el control
de Docker. El nombre efectivo de la red depende del proyecto Compose.

Cada resultado deberá identificar el escenario, la semilla, la revisión de Git,
el estado inicial de los WAL, los nodos, los eventos reales y las operaciones
confirmadas, fallidas, de resultado incierto u omitidas. Los experimentos no
deben borrar volúmenes existentes para preparar su ejecución.

Repetir el escenario y la semilla reproduce el plan y los datos de carga.
No garantiza elecciones, latencias ni intercalados idénticos. La comparación
entre ejecuciones requiere condiciones iniciales equivalentes y registrar las
desviaciones observadas.
