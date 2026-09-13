# Metodología experimental

Este documento define el protocolo científico correspondiente a la campaña
experimental final de RaftKV realizada en la revisión Git
`8ba7a131c4f452aac014a4c628794062fa1a8c9b`.

Su objetivo es fijar de forma explícita las decisiones metodológicas necesarias
para interpretar y reproducir los resultados de G3/G4. No modifica el
comportamiento de RaftKV ni redefine retrospectivamente los datos obtenidos.

## Objetivo

El experimento evalúa, de forma controlada y reproducible, cómo cuatro
condiciones operativas afectan el comportamiento observable de un clúster
RaftKV de cinco nodos bajo una carga de escrituras moderada y constante:

- operación normal sin fallas;
- caída temporal de un follower;
- caída temporal del leader;
- partición temporal del leader.

El experimento no pretende estimar la capacidad máxima de RaftKV. La tasa de
carga utilizada es un punto de operación conservador elegido mediante
calibración empírica para evitar saturación permanente del baseline y permitir
observar el efecto de las fallas.

## Preguntas experimentales

La campaña responde descriptivamente las siguientes preguntas:

1. ¿Cómo cambia el throughput observado cuando falla temporalmente un follower?
2. ¿Cómo cambia el throughput observado cuando falla temporalmente el leader?
3. ¿Cómo cambian las latencias p50, p95 y p99 bajo cada escenario?
4. ¿Cuántas operaciones quedan con resultado incierto durante fallas que afectan
   al leader?
5. ¿Se producen operaciones omitidas bajo la carga seleccionada?
6. ¿Los escenarios con fallas completan la restauración programada dentro de
   cada run válido?

Estas preguntas se responden en H1 mediante definiciones y estadística
descriptiva. La inferencia estadística, los intervalos de confianza y los
tamaños de efecto pertenecen a H2.

## Diseño experimental

### Variable independiente

La variable independiente principal es el escenario de falla, con cuatro
niveles:

| Escenario | Tipo de falla | Objetivo | Inicio programado | Duración |
| --- | --- | --- | --- | --- |
| sin fallas | ninguna | ninguno | no aplica | no aplica |
| follower caído | caída | follower | 20 s | 10 s |
| leader caído | caída | leader | 20 s | 10 s |
| partición de leader | partición | leader | 20 s | 10 s |

Los tres escenarios con falla mantienen la misma carga y difieren únicamente
en el tratamiento aplicado al clúster.

### Variables dependientes

Las variables dependientes disponibles en G3/G4 son:

- throughput por run;
- latencia p50 por run;
- latencia p95 por run;
- latencia p99 por run;
- número de operaciones inciertas por run;
- número de operaciones omitidas por run;
- indicador de restauración completada.

La campaña también conserva operaciones rechazadas y evidencia cruda por
operación, aunque no forman parte de la tabla descriptiva principal de G3/G4.

### Variables controladas

La campaña final mantiene constantes entre escenarios:

```yaml
nodos: 5
duracion_segundos: 60
clientes: 16
operaciones_por_segundo: 2
semilla: 42
despliegue: benchmark
nombre: benchmark_fallas
```

También se mantiene la misma revisión Git experimental y se exige un árbol Git
limpio al iniciar la campaña.

## Topología

La topología experimental consta de cinco procesos RaftKV:

```text
raft-bench-5-node-1
raft-bench-5-node-2
raft-bench-5-node-3
raft-bench-5-node-4
raft-bench-5-node-5
```

Cada nodo conoce a los otros cuatro peers. Los nodos se ejecutan en contenedores
Docker unidos a una red bridge dedicada y cada nodo posee un volumen persistente
independiente para el WAL y el estado Raft.

La configuración de benchmark publica los puertos HTTP exclusivamente sobre
`127.0.0.1`, desde `18581` hasta `18585`.

## Workload

Cada run programa exactamente:

```text
60 s * 2 operaciones/s = 120 operaciones
```

La carga está compuesta por escrituras `SET` con claves distintas por secuencia.
La secuencia se distribuye cíclicamente sobre 16 clientes lógicos. Cada cliente
mantiene como máximo una escritura activa.

El valor de cada escritura se deriva determinísticamente de la semilla y del
número de secuencia. Repetir la misma configuración reproduce el plan de carga y
los datos generados, pero no fuerza el mismo intercalado distribuido, elección
de leader, latencia ni temporización del sistema.

Cada llamada de escritura utiliza un timeout de 7 segundos. No se realizan
reintentos automáticos de una misma operación.

### Calibración de carga

La configuración final de 16 clientes y 2 operaciones/s es el resultado de una
calibración empírica previa. Su propósito fue seleccionar un punto de operación
en el que el escenario sin fallas no permaneciera saturado y en el que las
fallas pudieran observarse sin que el generador de carga dominara el resultado.

Esta calibración no constituye una estimación estadística de la capacidad máxima
del sistema ni debe interpretarse como throughput de saturación.

Documentación histórica de G3 menciona configuraciones preliminares, entre ellas
40 clientes y 5 operaciones/s. Esa configuración no corresponde a la campaña
final G4. Para la campaña final, los archivos YAML versionados en la revisión
experimental y la evidencia cruda son las fuentes autoritativas.

## Cronología de un run

El runner espera inicialmente:

1. que respondan los cinco nodos;
2. que exista un leader único;
3. que todos los nodos hayan observado progreso del log (`commitIndex > 0`).

Después registra el estado inicial y comienza el reloj experimental.

Para los escenarios con falla:

```text
0 s                 20 s              ~30 s                  60 s
|--------------------|------------------|----------------------|
     pre-falla          falla activa       post-restauración
```

A los 20 segundos se intenta resolver el objetivo de la falla a partir del
estado observado del clúster. El runner exige quorum visible y un leader único.
La resolución del objetivo dispone de un timeout de 3 segundos.

La duración de 10 segundos se cuenta desde que la aplicación de la falla termina
correctamente, por lo que los instantes reales pueden desviarse levemente de los
instantes programados. La evidencia registra los eventos reales.

En una caída se termina abruptamente el contenedor objetivo mediante `SIGKILL` y
posteriormente se inicia de nuevo el mismo nodo. En una partición se desconecta
el nodo de la red Docker y después se restauran su conectividad, dirección y
aliases.

## Leader observado durante la carga

Al comenzar la carga se conserva el leader detectado como hint. Durante el run,
el runner consulta periódicamente el estado del clúster y actualiza ese hint
únicamente cuando observa un leader único.

Una observación transitoria sin leader único no borra automáticamente el último
hint válido. Por tanto, durante cambios de liderazgo pueden existir intentos
contra el último leader conocido; su resultado se clasifica según la respuesta
observada. Esta definición corresponde al comportamiento de la revisión
experimental utilizada por la campaña final.

## Definición de run válido

Un run se considera válido para G3/G4 cuando satisface todas las condiciones que
verifica el pipeline de agregación:

- pertenece a uno de los cuatro escenarios esperados;
- usa cinco nodos y despliegue `benchmark`;
- utiliza la misma revisión Git que los demás runs;
- fue ejecutado con árbol Git limpio;
- contiene la misma carga controlada que el baseline;
- finaliza sin error experimental;
- registra las 120 operaciones programadas;
- para escenarios con falla, contiene el evento
  `restauracion_completada`;
- para el escenario sin fallas, no registra restauraciones.

El agregador exige exactamente 30 runs válidos por escenario.

## Intentos inválidos

Durante la campaña final se conservaron intentos que no satisficieron las
precondiciones metodológicas. Estos intentos no se cuentan como runs válidos y
no se eliminan silenciosamente.

Dos intentos fallaron al intentar resolver el objetivo de la falla porque no
pudo observarse un leader único dentro del plazo de resolución. La evidencia se
conserva en:

```text
experiments/raw/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final/invalid/
```

Además existe evidencia de una ejecución interrumpida del escenario sin fallas.
Una ejecución interrumpida no forma parte del conjunto de runs completos
utilizado por el agregador.

La exclusión de estos intentos se basa en criterios definidos por el protocolo,
no en el valor de las métricas obtenidas.

## Semilla

La campaña usa `semilla: 42`.

La semilla determina los datos generados para las escrituras, pero no controla
toda la no determinación del sistema distribuido. En particular, no fija:

- temporización de elecciones;
- scheduling del host;
- latencias de red o disco;
- orden exacto de goroutines;
- intercalados entre nodos.

Por tanto, la semilla mejora la reproducibilidad del workload, pero no convierte
la ejecución distribuida en determinista.

## Aislamiento entre ejecuciones

Los runs se ejecutan secuencialmente.

Antes de cada run, el script de campaña ejecuta:

```bash
docker compose ... down -v
```

y luego recrea el despliegue con:

```bash
docker compose ... up -d --no-build
```

El uso de `down -v` elimina deliberadamente los volúmenes del run anterior. De
esta forma, cada repetición comienza con volúmenes nuevos y no hereda el WAL ni
el estado persistente de la repetición previa.

El runner dispone además de un bloqueo local para evitar dos ejecuciones
simultáneas del runner sobre el mismo despliegue. Este mecanismo no impide que
un proceso externo ejecute comandos Docker manualmente, por lo que la campaña
debe realizarse sin carga ni manipulación externa concurrente.

## Revisión Git

La revisión experimental final es:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

`make experiment` exige un árbol Git limpio y registra la revisión completa en
cada JSON crudo.

Los cuatro escenarios de la campaña final deben compartir esa misma revisión.

## Reproducibilidad

La evidencia está organizada como:

```text
experiments/
|-- configs/
|-- raw/
|-- processed/
`-- README.md
```

La ejecución y el reprocesamiento se separan intencionalmente:

```bash
make experiment
```

ejecuta RaftKV y produce evidencia cruda, mientras que:

```bash
make reproduce \
  REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b \
  RUNS=30 \
  CAMPAIGN=final
```

recalcula los resultados procesados a partir de los JSON existentes, sin
levantar contenedores ni volver a ejecutar RaftKV.

El pipeline de reproducción genera manifests SHA-256 para los JSON crudos y para
los YAML de configuración. La reproducción se acepta únicamente si el código y
la configuración relevantes no cambiaron desde la revisión experimental.

## Definición de throughput

Para cada run completo:

```text
throughput =
    número de escrituras confirmadas
    --------------------------------
    segundos entre inicio y fin del run
```

La ventana temporal utilizada es la observada entre `inicio` y `fin` del
resultado experimental. Puede incluir drenaje de operaciones y limpieza
posteriores a la generación de carga; por tanto, no equivale necesariamente a
los 60 segundos configurados.

El throughput mide escrituras confirmadas por segundo. No se calcula a partir de
la tasa solicitada de 2 operaciones/s.

Para un run interrumpido o fallido, el throughput se considera no disponible y
ese run no se agrega como benchmark completo.

## Definición de p50, p95 y p99

Los percentiles se calculan exclusivamente sobre la duración de escrituras
confirmadas.

Se aplica el método nearest-rank:

```text
posición = ceil(p * N)
```

sobre las latencias ordenadas, usando índices desde uno. No se interpola entre
muestras.

Las operaciones inciertas, rechazadas y omitidas no participan en la población
de latencias confirmadas.

En G3/G4 cada run produce sus propios percentiles. El resumen de 30 repeticiones
reporta la mediana de esos percentiles por run; no concatena todas las
operaciones de todos los runs para construir una distribución artificialmente
única.

## Operaciones inciertas

Una operación se clasifica como `incierta` cuando la observación del cliente no
permite afirmar que la escritura fue confirmada ni que definitivamente no lo
fue.

El caso incluye errores o timeouts en los que el estado final de la escritura no
puede inferirse a partir de la respuesta observada. Un timeout no demuestra que
la escritura se haya perdido.

El contador de operaciones inciertas se conserva por separado y no se convierte
en cero por ausencia de instrumentación adicional.

## Operaciones omitidas

Una operación programada puede registrarse como omitida por dos causas:

- `omitida_saturacion`: el cliente lógico asignado todavía tiene una operación
  activa o el instante programado ya no puede atenderse dentro de la cadencia
  prevista;
- `omitida_sin_lider`: el runner no dispone de un leader hint utilizable al
  despachar la operación.

Las operaciones omitidas se contabilizan explícitamente. No se posponen en una
cola ilimitada ni se reinyectan posteriormente como una ráfaga.

## Restauración

Para los escenarios con falla, la restauración forma parte de la validez del
run.

El runner intenta restaurar el nodo o la conectividad tanto en el flujo normal
como ante errores posteriores a la modificación de Docker. Un escenario bajo
falla solo puede entrar en el resumen G3/G4 si registra
`restauracion_completada`.

Este indicador demuestra que la acción de restauración del entorno se completó.
No demuestra por sí mismo que el nodo haya alcanzado completamente el estado
confirmado del clúster ni mide un tiempo de recuperación del log.

## Agregación descriptiva de G3/G4

Para cada escenario se agregan 30 runs válidos.

El resumen reporta:

- mediana del throughput por run;
- mediana del p50 por run;
- mediana del p95 por run;
- mediana del p99 por run;
- mediana del número de operaciones inciertas por run;
- mediana del número de operaciones omitidas por run;
- número de runs que completaron restauración.

Para un número par de observaciones, la mediana se calcula como el promedio de
los dos valores centrales después de ordenar las métricas por run.

## Alcance estadístico

G3/G4 constituyen análisis descriptivo.

Los resultados obtenidos permiten caracterizar las observaciones de esta campaña
y comparar descriptivamente escenarios bajo una configuración controlada. No
permiten todavía afirmar significancia estadística, estimar intervalos de
confianza, reportar tamaños de efecto ni generalizar a otras cargas, plataformas
o configuraciones.

Ese análisis corresponde a H2.

## Metadata de entorno pendiente

La evidencia versionada permite identificar la revisión Git, la configuración,
los contenedores e imágenes observadas por el runner, pero H1 no dispone de
metadata suficiente para afirmar de manera científicamente trazable:

- modelo de CPU del host;
- número de cores/hilos disponibles;
- RAM total y disponible;
- dispositivo y características de almacenamiento;
- sistema operativo y versión del host;
- versión exacta de Docker Engine;
- versión exacta de Docker Compose;
- versión exacta del toolchain Go usado en el host de la campaña.

El repositorio declara Go 1.22 y las imágenes de los nodos se construyen con
`golang:1.22-alpine` y ejecutan sobre `alpine:3.20`, pero eso no sustituye la
metadata del host experimental.

Esta metadata debe capturarse explícitamente en una campaña futura o recuperarse
de evidencia externa verificable si existe. No se infiere ni se inventa
retrospectivamente.

## Fuentes normativas dentro del repositorio

Para interpretar la campaña final, el orden de autoridad es:

1. revisión experimental
   `8ba7a131c4f452aac014a4c628794062fa1a8c9b`;
2. archivos `experiments/configs/*.yaml` usados por G4;
3. código del runner, cálculo de métricas y agregación de esa revisión;
4. JSON crudos de la campaña final;
5. manifests y resultados procesados;
6. documentación histórica de F2/G1/G2/G3.

Cuando una descripción histórica difiera del código, la configuración o la
evidencia de la revisión experimental, H1 adopta estos últimos como fuente
normativa y conserva la discrepancia como deuda documental.
