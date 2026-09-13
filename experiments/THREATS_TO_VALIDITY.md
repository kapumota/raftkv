### Threats to Validity

Este documento identifica las principales amenazas a la validez de la campaña
experimental de RaftKV y del análisis estadístico H2.

La evidencia analizada corresponde a la revisión experimental:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

La metodología detallada se encuentra en:

```text
experiments/METHODOLOGY.md
```

El plan estadístico y los artefactos de H2 se encuentran en:

```text
experiments/STATISTICAL_ANALYSIS.md
experiments/processed/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final/statistics/
```

Este documento no modifica retrospectivamente el protocolo ni los resultados.
Su propósito es delimitar con precisión qué conclusiones soporta la evidencia y
qué factores pueden limitar su interpretación.

#### Validez interna

La validez interna se refiere a si las diferencias observadas pueden atribuirse
razonablemente al escenario de falla y no a factores concurrentes.

#### Orden fijo de los escenarios

Los escenarios se ejecutaron secuencialmente en un orden fijo:

```text
sin fallas
follower caído
leader caído
partición de leader
```

El orden no fue aleatorizado ni contrabalanceado.

Esto permite que una deriva temporal del host, temperatura, presión de memoria,
cachés, actividad externa u otras condiciones no registradas se confunda con el
efecto del escenario.

Mitigaciones existentes:

- todos los escenarios utilizaron la misma revisión experimental;
- cada run recreó el despliegue;
- los volúmenes del run anterior se eliminaron mediante `down -v`;
- los escenarios utilizaron la misma carga controlada.

Limitación residual:

No es posible separar retrospectivamente un efecto de orden de un efecto del
tratamiento. Una campaña futura debería aleatorizar o contrabalancear el orden
de los escenarios.

#### Ejecución sobre un mismo host

Los cinco nodos y el generador de carga se ejecutaron dentro del mismo entorno
de host.

Por tanto, CPU, memoria, almacenamiento y scheduling pueden introducir
dependencias compartidas entre procesos que no existirían de la misma forma en
un clúster físicamente distribuido.

La recreación de contenedores y volúmenes reduce contaminación entre runs, pero
no aísla completamente recursos físicos compartidos.

#### Metadata incompleta del host

La campaña final no conserva de forma verificable todos los metadatos del host.

Entre los datos que no deben reconstruirse por inferencia se encuentran:

- modelo exacto de CPU;
- número de cores e hilos;
- cantidad exacta de RAM;
- dispositivo y características de almacenamiento;
- sistema operativo y kernel exactos del host;
- versión exacta de Docker Engine;
- versión exacta de Docker Compose;
- versión exacta del toolchain Go usado desde el host.

Esta ausencia limita la capacidad para controlar o reproducir posibles efectos
dependientes del entorno.

No se inventan valores para completar esta información.

#### Carga externa no observada

El protocolo no registra de forma continua la utilización global del host ni la
actividad de procesos externos.

Una carga concurrente ajena a RaftKV podría afectar latencias o throughput de un
run válido sin producir necesariamente un error experimental.

La campaña se diseñó para ejecutarse sin manipulación externa concurrente, pero
esa condición no fue instrumentada como una variable observada.

#### Intentos inválidos y selección condicional

La campaña conserva dos intentos que no pudieron resolver un leader único dentro
del plazo establecido para seleccionar el objetivo de la falla.

También se conserva evidencia de una ejecución interrumpida del escenario sin
fallas.

Estos intentos fueron excluidos mediante criterios de validez definidos por el
protocolo y no por el valor posterior de las métricas.

Sin embargo, los 30 runs válidos de cada escenario representan ejecuciones que
sí consiguieron satisfacer las precondiciones del protocolo.

Por tanto, los resultados de los escenarios de falla deben interpretarse de
forma condicional a que el runner pueda observar el estado requerido para
aplicar correctamente el tratamiento.

La exclusión metodológica evita mezclar tratamientos mal definidos, pero puede
subestimar la frecuencia operacional con la que el sistema atraviesa estados
ambiguos durante la preparación de una falla.

#### Leader hint durante cambios de liderazgo

Durante la carga, el runner conserva el último leader observado y solo actualiza
el hint cuando detecta posteriormente un leader único.

Una observación transitoria sin leader único no elimina inmediatamente el hint.

Como consecuencia, durante un cambio de liderazgo pueden enviarse operaciones al
último leader conocido.

Este comportamiento forma parte de la revisión experimental y contribuye a la
semántica de `uncertain_writes`.

No debe interpretarse la cantidad de operaciones inciertas como una medición
aislada exclusivamente del tiempo de elección de Raft.

#### Momento efectivo de la falla

La falla está programada aproximadamente a los 20 segundos.

El runner debe resolver primero el objetivo y aplicar la modificación del
entorno. La restauración se programa a partir de la finalización efectiva de la
aplicación de la falla.

Por tanto, los límites reales de las ventanas pre-falla, falla y
post-restauración pueden desplazarse ligeramente entre runs.

Los eventos reales quedan registrados, pero la tabla principal de G3/G4 no
modela explícitamente esta variación temporal.

#### Validez de constructo

La validez de constructo se refiere a si las métricas utilizadas representan
adecuadamente los conceptos que se pretende estudiar.

#### Throughput

El throughput se define como:

```text
escrituras confirmadas / ventana temporal observada
```

La ventana se extiende desde el inicio hasta el final registrado del run.

Puede incluir drenaje de operaciones y limpieza posteriores al periodo nominal
de generación de carga.

Por tanto, el throughput no representa simplemente:

```text
confirmadas / 60 segundos
```

ni equivale a la tasa configurada de 2 operaciones/s.

Esta definición es consistente entre escenarios, pero debe mantenerse al
comparar resultados.

#### Percentiles de latencia

p50, p95 y p99 se calculan exclusivamente sobre operaciones confirmadas.

Las operaciones inciertas, rechazadas u omitidas no participan en esa
distribución.

En escenarios con falla, esto puede producir una forma de censura operacional:
las operaciones que sufren los efectos más severos pueden terminar clasificadas
como inciertas en lugar de contribuir a la cola de latencia confirmada.

Por tanto, latencia y `uncertain_writes` deben interpretarse conjuntamente.

#### Percentiles por run

Cada run calcula primero sus propios percentiles.

El análisis entre escenarios utiliza los 30 valores de percentil por run y no
concatena todas las operaciones de todos los runs.

Esta decisión evita pseudorreplicación, pero significa que la distribución
analizada en H2 es una distribución de estadísticas por run, no una distribución
global de latencias por operación.

#### Operaciones inciertas

Una operación incierta indica que la observación del cliente no permite afirmar
si la escritura fue definitivamente confirmada o no.

No significa automáticamente:

```text
escritura perdida
```

ni:

```text
escritura no committed
```

Un timeout o una pérdida de respuesta puede dejar el resultado final
desconocido desde el cliente.

Por tanto, `uncertain_writes` mide ambigüedad observable del resultado y no una
violación de seguridad de Raft.

#### Operaciones omitidas

Una operación omitida representa un trabajo programado que no pudo despacharse
según la cadencia prevista.

No equivale a una escritura rechazada por Raft ni a una operación perdida
después del commit.

La mediana de omisiones igual a cero no implica que todos los runs carezcan de
omisiones.

#### Restauración

`restauracion_completada` demuestra que la acción de restauración del entorno
finalizó correctamente.

No demuestra por sí sola:

- convergencia completa del log;
- catch-up completo del nodo;
- recuperación total de la máquina de estados;
- ausencia de entradas divergentes;
- duración del recovery lógico.

Por tanto, 30/30 restauraciones no debe presentarse como 100 % de recuperación
semántica del clúster.

#### Validez de conclusión

La validez de conclusión se refiere a si el análisis estadístico justifica las
conclusiones extraídas de la campaña.

#### Unidad estadística

La unidad de análisis es el run válido.

Las operaciones individuales dentro de un run no se consideran observaciones
independientes.

Esto evita inflar artificialmente el tamaño muestral mediante
pseudorreplicación.

Cada escenario aporta:

```text
n = 30 runs
```

#### Comparaciones planificadas

H2 compara únicamente cada escenario de falla contra el baseline.

No se realizan todas las comparaciones posibles entre pares de escenarios.

Esto reduce comparaciones innecesarias y mantiene la inferencia alineada con las
preguntas experimentales fijadas en H1/H2.

#### Multiplicidad

La campaña ejecuta 18 comparaciones inferenciales:

```text
3 escenarios de falla * 6 métricas
```

Los valores p obtenidos mediante permutación se ajustan conjuntamente mediante
Holm.

Las conclusiones inferenciales utilizan el valor p ajustado y no el valor p
Monte Carlo aislado.

#### Remuestreo Monte Carlo

Los intervalos bootstrap y las pruebas de permutación utilizan 100000 réplicas.

La semilla de análisis es:

```text
20260912
```

Esto hace determinista el reprocesamiento computacional, pero no elimina el
error Monte Carlo inherente a una aproximación finita.

Los valores p extremadamente pequeños están limitados por la resolución que
permite el número de permutaciones simuladas.

#### Bootstrap percentil

Los intervalos de confianza utilizan bootstrap percentil.

Este método es reproducible y no requiere asumir normalidad, pero no corrige
automáticamente sesgo ni aceleración de la distribución del estimador.

Los intervalos deben interpretarse como una estimación de incertidumbre bajo el
esquema de remuestreo elegido.

#### Cliff's delta

Cliff's delta resume dominancia estocástica entre dos grupos independientes.

No expresa directamente una diferencia en unidades de throughput, milisegundos
o número de operaciones.

Por eso H2 reporta conjuntamente:

- diferencia de medianas;
- cambio relativo cuando está definido;
- intervalo bootstrap;
- Cliff's delta;
- intervalo de Cliff's delta;
- valor p de permutación;
- valor p ajustado mediante Holm.

#### Métricas con empates

Los conteos de operaciones inciertas y omitidas contienen numerosos empates y
valores cero.

Cliff's delta trata los empates como empates, y la prueba de permutación conserva
los valores observados.

Esto evita transformaciones artificiales para aproximar normalidad.

Sin embargo, una mediana igual a cero puede ocultar heterogeneidad entre runs,
por lo que deben consultarse también Q1, Q3, máximo, MAD y distribución completa.

#### Validez externa

La validez externa se refiere al grado en que los resultados pueden
generalizarse fuera de la campaña ejecutada.

#### Tamaño del clúster

La campaña final utiliza exactamente cinco nodos.

No se han evaluado en esta campaña otros tamaños de clúster.

Los resultados no deben extrapolarse automáticamente a configuraciones de tres,
siete o más nodos.

#### Tipo de carga

La carga final está compuesta por escrituras `SET`.

No se evalúa una mezcla realista de:

- lecturas;
- escrituras;
- scans;
- claves calientes;
- diferentes tamaños de valores;
- sesiones de clientes con patrones heterogéneos.

Los resultados describen el comportamiento bajo el workload experimental
definido y no un workload universal de un KV store.

#### Intensidad de carga

La campaña utiliza:

```text
16 clientes
2 operaciones/s
```

La tasa fue seleccionada como punto conservador mediante calibración empírica.

No representa la capacidad máxima del sistema ni un punto de saturación
estimado estadísticamente.

Los efectos de las fallas pueden cambiar bajo cargas mayores o menores.

#### Semilla única de workload

La campaña final usa:

```text
semilla: 42
```

La semilla fija los datos generados, pero no controla elecciones, scheduling,
red, disco ni intercalados concurrentes.

Aunque existen 30 runs por escenario, no se evaluó una familia de semillas de
workload diferentes.

#### Tiempo y duración de la falla

Los escenarios con falla programan:

```text
inicio aproximado: 20 s
duración: 10 s
```

No se evalúan fallas al comienzo del run, cerca del final, de duración variable o
múltiples fallas dentro del mismo run.

#### Modelo de red

Los nodos se comunican sobre una red bridge de Docker en un mismo host.

Una partición experimental desconecta al nodo de esa red.

No se modelan explícitamente:

- latencia WAN;
- jitter controlado;
- pérdida parcial de paquetes;
- duplicación;
- reordenamiento;
- bandwidth limitado;
- particiones asimétricas.

Por tanto, el escenario de partición representa una desconexión fuerte y no toda
la diversidad de fallas de red posibles.

#### Modelo de crash

La caída experimental termina abruptamente el contenedor objetivo y después
reinicia el mismo nodo.

No cubre todos los modelos posibles de fallo, como:

- corrupción parcial de almacenamiento;
- pérdida del volumen;
- power failure con comportamiento específico del filesystem;
- pausas prolongadas;
- procesos vivos pero no responsivos.

#### Generalización de hardware y software

Debido a la metadata incompleta del host y a que la campaña se ejecutó en un
único entorno, los resultados no deben generalizarse automáticamente a otro
hardware, kernel, runtime de contenedores o dispositivo de almacenamiento.

La revisión Git y los manifests permiten reproducir el software y los datos,
pero no recrean exactamente el entorno físico original.

#### Validez de reproducibilidad

La campaña conserva los JSON crudos y manifests SHA-256.

H2 añade un pipeline determinista que genera:

```text
per-run.csv
descriptive.csv
effects.csv
comparisons.csv
analysis-metadata.txt
analysis-source-manifest.sha256
analysis-manifest.sha256
```

Una segunda ejecución de `make analyze` debe producir resultados byte a byte
idénticos.

Esto protege la reproducibilidad computacional del análisis.

No obstante, reproducibilidad computacional no equivale a replicabilidad
experimental completa: volver a ejecutar la campaña puede producir nuevos
intercalados, elecciones y latencias debido a la naturaleza no determinista del
sistema distribuido.

#### Alcance de las conclusiones actuales

La evidencia permite sostener conclusiones sobre esta campaña específica.

En particular, H2 proporciona evidencia robusta de degradación del throughput
cuando el leader cae o queda particionado, así como un incremento importante de
operaciones inciertas en ambos escenarios.

La caída del leader también muestra un aumento marcado de la cola p95/p99.

La caída de un follower no muestra, bajo esta carga y este diseño, una diferencia
robusta frente al baseline después del ajuste por multiplicidad.

Estas conclusiones no equivalen a una demostración formal de propiedades de
seguridad o liveness de Raft.

Las propiedades de protocolo deben sostenerse mediante implementación,
invariantes, pruebas de integración, fault tests y, cuando corresponda,
especificación formal.
