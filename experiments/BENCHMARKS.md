### Benchmarks de tres y cinco nodos

G2 añade `nodos` (3 o 5), `despliegue` (`local` o `benchmark`) y el tipo de falla
`ninguna`. Los YAML anteriores conservan cinco nodos y despliegue local por defecto.
Sin fallas se exige objetivo vacío e instante y duración de falla iguales a cero.

Los nuevos despliegues tienen nombres, redes, puertos y volúmenes independientes.
Tres nodos usan 18381 a 18383 y cinco usan 18581 a 18585, siempre en localhost.
No se simula un clúster de tres apagando dos miembros de uno de cinco.

#### Validación

```bash
make fmt
go test -race ./internal/experiment -count=1 -v
make validate
```

Confirme G2 antes de ejecutar. Ambas mediciones deben usar la misma revisión y
un árbol limpio, incluidos archivos sin seguimiento. Mueva los parches fuera del
repositorio antes del benchmark. Guarde las salidas en `/tmp` por ahora.

```bash
go build -o /tmp/raftkv-experiment ./cmd/experiment-runner
go build -o /tmp/raftkv-compare ./cmd/benchmark-compare
/tmp/raftkv-experiment -scenario experiments/configs/normal-3.yaml -check
/tmp/raftkv-experiment -scenario experiments/configs/normal-5.yaml -check
```

#### Ejecución secuencial

Detenga primero el clúster anterior para evitar carga de fondo:

```bash
docker compose -f docker-compose.yml -f docker-compose.experiments.yml stop
```

Los nombres de proyecto siguientes deben ser nuevos. Permiten partir de volúmenes
independientes sin eliminar datos anteriores. Si ya ejecutó esta pareja, cambie
el sufijo `01` por otro en todos los comandos de proyecto y resultados.

```bash
docker compose -p raftkv-g2-3-01 -f docker-compose.benchmarks-3.yml up -d --build
/tmp/raftkv-experiment -scenario experiments/configs/normal-3.yaml -output /tmp/raftkv-normal-3-01.json
docker compose -p raftkv-g2-3-01 -f docker-compose.benchmarks-3.yml down
```

`down` elimina contenedores y red de ese proyecto, pero conserva sus volúmenes.
No use `-v`. No arranque otra ejecución del mismo tamaño mientras sus contenedores
sigan presentes: sus nombres son fijos para que el runner los identifique.

```bash
docker compose -p raftkv-g2-5-01 -f docker-compose.benchmarks-5.yml up -d --build
/tmp/raftkv-experiment -scenario experiments/configs/normal-5.yaml -output /tmp/raftkv-normal-5-01.json
docker compose -p raftkv-g2-5-01 -f docker-compose.benchmarks-5.yml down
```

Ejecute cada comando después de comprobar el éxito del anterior. Si el runner
falla, conserve su JSON y detenga ese despliegue antes de iniciar el siguiente.

#### Comparación

```bash
/tmp/raftkv-compare -three /tmp/raftkv-normal-3-01.json -five /tmp/raftkv-normal-5-01.json
```

El comando exige igual revisión, árboles limpios, cargas equivalentes, todas las
operaciones programadas registradas y ejecuciones completas. Muestra throughput,
percentiles, operaciones inciertas y omitidas sin modificar los JSON originales.

Los escenarios tienen la misma semilla, nombre, duración de 60 segundos, cuatro
clientes y tasa global objetivo de 100 operaciones por segundo. La tasa efectiva
puede ser menor por saturación; no se calcula throughput usando la tasa solicitada.

Los percentiles y la ventana temporal conservan las definiciones de METRICS.md.
No hay fase separada de calentamiento: son mediciones del recorrido completo del
runner después de preparar el clúster. Mantenga condiciones de CPU, disco y carga
externa comparables. Los hashes iniciales documentan el estado, pero no prueban
por sí solos equivalencia de todas las condiciones experimentales.

Una pareja de ejecuciones es una comprobación funcional y una comparación
descriptiva, no evidencia suficiente para concluir qué tamaño es más rápido.
G3 añadirá ejecuciones repetidas por configuración; G4 organizará los resultados.
La instrumentación pendiente de G1 permanece explícita: G2 no convierte las
métricas de seguridad o tiempos no observados en ceros.

#### G3 - Benchmark bajo fallas

G3 conserva el benchmark normal de G2 y añade una campaña repetida sobre cinco
nodos. La carga es idéntica entre escenarios: misma semilla, duración, clientes,
tasa y nombre de experimento. Solo cambia la falla.

Los escenarios son:

```text
sin fallas
follower caído
leader caído
partición de leader
```

La recuperación no se modela como un quinto tipo de falla. El runner ya restaura
el nodo o la conectividad durante cada escenario. Una ejecución bajo falla solo
entra al resumen G3 si contiene el evento `restauracion_completada`.

Los tres escenarios con falla la aplican a los 20 segundos durante 10 segundos.
Esto deja una ventana previa de 20 segundos y una ventana posterior de 30 segundos.

La carga final de G3 usa 40 clientes lógicos y 5 operaciones por segundo. `runLoad`
mantiene como máximo una escritura activa por cliente y cada escritura tiene un timeout
de 7 segundos. Con 5 operaciones por segundo pueden acumularse hasta 35 solicitudes
pendientes si todas alcanzan el timeout; 40 clientes dejan margen sin convertir el
generador en el cuello de botella.

Esta calibración reemplaza la configuración inicial de 4 clientes y 100 operaciones
por segundo. En el smoke inicial esa carga produjo miles de `omitida_saturacion`; aumentar
la concurrencia a cientos de clientes llegó a impedir que `/status` respondiera. La
campaña definitiva evita ambos extremos y mantiene idéntica carga entre escenarios.

G3 resume la mediana de las métricas obtenidas por ejecución. No concatena todas
las operaciones para fabricar una única distribución y no sustituye métricas no
observadas por cero.

#### Validación de G3

Antes del commit:

```bash
make fmt
go test -race ./internal/experiment -count=1 -v
make validate
```

La campaña real exige un árbol Git limpio porque cada JSON registra la revisión
y los cambios del repositorio. Por ello, aplique y valide G3, haga el commit y
ejecute después los benchmarks.

Prueba corta de dos repeticiones:

```bash
RUNS=2 OUT_DIR=/tmp/raftkv-g3-smoke ./scripts/benchmark-failures.sh
```

Campaña prevista:

```bash
RUNS=30 OUT_DIR=/tmp/raftkv-g3 ./scripts/benchmark-failures.sh
```

Cada repetición usa el despliegue de cinco nodos de G2. Entre ejecuciones se hace
`down -v` de forma deliberada para descartar WAL y estado persistente de la
repetición anterior. Los resultados permanecen en `OUT_DIR`, fuera del
repositorio.

El script rechaza resultados preexistentes para evitar sobrescribir evidencia.

#### Resumen de G3

Para resumir resultados ya generados:

```bash
go build -o /tmp/raftkv-failures ./cmd/benchmark-failures
/tmp/raftkv-failures -dir /tmp/raftkv-g3 -runs 30
```

La herramienta exige exactamente el número indicado de ejecuciones para cada
escenario, la misma revisión Git, árbol limpio, carga equivalente y restauración
completada en todas las ejecuciones con falla.

El resultado sigue siendo descriptivo. La comparación inferencial, intervalos de
confianza y organización de artefactos reproducibles quedan fuera de G3.
G4 organizará los resultados crudos y procesados.
