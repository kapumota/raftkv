# Métricas base

Este primer parche de G1 procesa los resultados de F2 sin modificar el JSON
original ni ejecutar nuevas fallas. El resultado conserva la revisión de Git,
los cambios locales, el escenario y el SHA-256 del archivo de entrada.

```bash
go run ./cmd/experiment-metrics \
  -input /tmp/raftkv-caida-lider-01.json \
  -output /tmp/raftkv-caida-lider-01-metrics.json
```

La salida debe ser un archivo nuevo. Se rechazan ventanas temporales inválidas,
secuencias repetidas y resultados de operación desconocidos.

## Definiciones

| Campo | Definición |
| --- | --- |
| `throughput` | Escrituras confirmadas divididas por segundos entre `inicio` y `fin`. |
| `latency_p50` | Percentil 50 de duración de escrituras confirmadas, en milisegundos. |
| `latency_p95` | Percentil 95 de la misma población, en milisegundos. |
| `latency_p99` | Percentil 99 de la misma población, en milisegundos. |

Se utiliza el rango más próximo por techo: posición `ceil(p * N)` en la lista
ordenada, contando desde uno. No se interpolan muestras. Los percentiles excluyen
operaciones inciertas, rechazadas y omitidas; estas categorías conservan sus
propios contadores para evitar ocultar fallos o saturación.

La ventana de F2 incluye drenaje y limpieza posteriores a la generación de carga.
No equivale necesariamente a la duración configurada. Esta definición mide el
throughput global observado, no el throughput durante un intervalo estable.

Sin confirmaciones, el throughput de una ejecución completa es cero y sus
percentiles son `null`. Si la ejecución terminó con error, `partial_run` es
verdadero y el throughput es `null`; los percentiles describen solo las muestras
disponibles y no deben agregarse como una ejecución completa.

## Instrumentación pendiente de G1

Los JSON de F2 no contienen evidencia suficiente para medir los siguientes
campos. Se incluyen como `null` junto con su motivo en `no_disponible`:

- `election_duration_ms`: faltan inicio y final observados de cada elección.
- `failover_duration_ms`: faltan instantes de confirmación posteriores a la falla.
- `recovery_duration_ms`: falta observar cuándo el nodo alcanza el estado confirmado.
- `committed_writes_lost`: falta verificar las escrituras confirmadas tras recuperar.
- `duplicate_applications`: falta instrumentar las aplicaciones en la máquina de estados.

El éxito de un comando Docker no mide recuperación del log. Un timeout no prueba
pérdida de datos. Repetir un registro en el JSON tampoco demuestra doble aplicación.
No se reemplazan valores ausentes por cero ni se incluyen en promedios.

G1 aún requiere esa instrumentación antes de declarar disponibles las nueve
métricas del plan. G2 adaptará el despliegue y el runner, actualmente fijados en
cinco nodos, para comparar tres y cinco. G3 repetirá cada configuración; los
percentiles por ejecución no se promediarán para presentarlos como un percentil
global. G4 organizará datos originales, derivados y comandos de reproducción.
