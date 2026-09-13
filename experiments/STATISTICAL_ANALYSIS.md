### Plan de análisis estadístico

Este documento define el plan de H2 para analizar la campaña experimental final
de RaftKV. La campaña fue ejecutada antes de redactar este plan, por lo que el
documento no constituye una preregistración. Su función es fijar las decisiones
estadísticas antes de implementar el análisis inferencial detallado y evitar
elegir pruebas en función de resultados particulares observados durante H2.

La revisión experimental analizada es:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

La metodología experimental, los criterios de validez y las definiciones de
métricas permanecen fijados por `experiments/METHODOLOGY.md`.

#### Unidad de análisis

La unidad estadística es un run válido completo.

La campaña contiene 30 runs válidos por escenario:

```text
sin fallas
follower caído
leader caído
partición de leader
```

Las operaciones individuales de un mismo run no se tratan como observaciones
independientes para la inferencia entre escenarios. Hacerlo produciría
pseudorreplicación, porque las operaciones comparten el mismo clúster, ventana
temporal, estado distribuido y condiciones de ejecución.

Los percentiles p50, p95 y p99 se calculan primero dentro de cada run, de acuerdo
con `experiments/METHODOLOGY.md`. La inferencia se realiza posteriormente sobre
los 30 valores por escenario.

#### Población analizada

H2 utiliza exclusivamente los 120 runs que satisfacen los criterios de validez
definidos en H1.

Los intentos conservados bajo `invalid/` y la ejecución interrumpida no se
incorporan a la inferencia.

No se excluyen runs válidos por presentar latencias extremas, throughput bajo,
operaciones inciertas u otros resultados desfavorables. Un valor extremo se
conserva mientras el run cumpla el protocolo experimental.

#### Comparaciones

El escenario `sin fallas` es el baseline.

Las comparaciones inferenciales previstas son:

```text
follower caído      vs sin fallas
leader caído        vs sin fallas
partición de leader vs sin fallas
```

H2 no realizará inicialmente las seis comparaciones posibles entre todos los
pares de escenarios. Las tres comparaciones anteriores responden directamente a
las preguntas experimentales definidas en H1 y reducen comparaciones
innecesarias.

Los runs no se consideran pareados. Compartir la misma configuración y semilla
de workload no crea correspondencia estadística run a run entre escenarios.

#### Métricas

Las métricas cuantitativas analizadas son:

```text
throughput
latency_p50
latency_p95
latency_p99
uncertain_writes
omitted_operations
```

La restauración se reporta descriptivamente como número de runs restaurados
sobre runs válidos. No se aplicará una prueba inferencial si un escenario tiene
resultado constante, por ejemplo 30/30 restauraciones.

Una métrica sin variación entre todos los grupos se reportará como invariante en
esta campaña y no recibirá una prueba inferencial artificial.

#### Resumen descriptivo

Para cada métrica y escenario se calcularán al menos:

```text
n
mínimo
Q1
mediana
Q3
máximo
IQR
MAD
```

`MAD` representa la mediana de las desviaciones absolutas respecto de la
mediana.

La media y la desviación estándar podrán incluirse como información
complementaria, pero no sustituirán las medidas robustas anteriores.

No se utilizará una prueba de normalidad como mecanismo para decidir
retrospectivamente entre un análisis paramétrico y uno no paramétrico. H2 adopta
desde este plan procedimientos de remuestreo y tamaños de efecto que no requieren
suponer normalidad de las métricas por run.

#### Estimación del efecto

Para cada comparación contra el baseline se reportará la diferencia de medianas:

```text
delta_mediana = mediana(falla) - mediana(baseline)
```

Para métricas positivas con mediana del baseline distinta de cero se reportará
también el cambio relativo:

```text
cambio_relativo_porcentaje =
    100 * (mediana(falla) / mediana(baseline) - 1)
```

No se calculará un cambio porcentual cuando el denominador sea cero.

Además se calculará Cliff's delta como tamaño de efecto no paramétrico entre dos
grupos independientes:

```text
delta_cliff =
    (pares donde falla > baseline - pares donde falla < baseline)
    / total de pares
```

El signo se interpreta con la orientación `falla - baseline`.

H2 reportará el valor numérico del tamaño de efecto y su intervalo de confianza.
No se dependerá únicamente de etiquetas cualitativas como pequeño, mediano o
grande.

#### Intervalos de confianza

La diferencia de medianas y Cliff's delta utilizarán intervalos de confianza
bootstrap percentil del 95 %.

El remuestreo se realizará independientemente dentro de cada grupo, preservando
30 runs por escenario en cada réplica.

La configuración final prevista es:

```text
bootstrap_replicas: 100000
confidence_level: 0.95
analysis_seed: 20260912
```

La semilla de análisis es distinta conceptualmente de `semilla: 42`, que
pertenece al workload experimental.

Los intervalos bootstrap se interpretarán como estimaciones de incertidumbre de
la campaña bajo el esquema de remuestreo. No corrigen sesgos derivados del orden
de ejecución, hardware no registrado ni otras amenazas de diseño.

#### Prueba de hipótesis

Cuando una métrica tenga variación suficiente, H2 utilizará una prueba de
permutación bilateral sobre la diferencia de medianas.

Hipótesis nula:

```text
las etiquetas de escenario son intercambiables para la métrica analizada
```

Estadístico:

```text
T = mediana(falla) - mediana(baseline)
```

La prueba utilizará permutaciones Monte Carlo porque el número total de
particiones posibles de 60 observaciones es demasiado grande para enumerarlo
exhaustivamente.

Configuración final prevista:

```text
permutation_replicas: 100000
analysis_seed: 20260912
alternative: two-sided
```

El valor p Monte Carlo utilizará corrección de una unidad:

```text
p = (extremos + 1) / (replicas + 1)
```

para evitar reportar un valor p igual a cero debido únicamente a un número
finito de permutaciones.

#### Multiplicidad

Las comparaciones inferenciales no se interpretarán mediante valores p sin
corregir de forma aislada.

H2 aplicará el procedimiento de Holm a la familia completa de pruebas
inferenciales efectivamente realizadas en el análisis final.

Las métricas constantes, para las cuales no se ejecute una prueba, no se
incorporarán artificialmente a la familia.

Se conservarán en la salida tanto el valor p Monte Carlo original como el valor
ajustado mediante Holm.

El nivel de referencia será:

```text
alpha: 0.05
```

El análisis no reducirá sus conclusiones a la dicotomía significativo/no
significativo. Las estimaciones, intervalos de confianza y tamaños de efecto
tendrán prioridad interpretativa.

#### Datos discretos e inciertas

`uncertain_writes` y `omitted_operations` son conteos por run y pueden presentar
muchos empates o valores cero.

Se conservarán como conteos. No se aplicará una transformación destinada
solamente a obtener normalidad.

La prueba de permutación y Cliff's delta admiten empates; los empates no se
cuentan como victorias ni derrotas en Cliff's delta.

Si una de estas métricas es constante en todos los runs comparados, H2 la
reportará descriptivamente y omitirá una prueba sin información.

#### Restauración

La restauración es una condición de validez para los escenarios con falla y
también un resultado descriptivo de la campaña.

Dado que G4 registró restauración completa en los runs válidos de los tres
escenarios con falla, H2 no transformará 30/30 en una afirmación probabilística
sobre una población ilimitada de ejecuciones.

Se reportará exactamente la evidencia observada.

#### Reproducibilidad del análisis

El análisis estadístico deberá consumir directamente los JSON crudos de:

```text
experiments/raw/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final/
```

Las métricas por run se recalcularán reutilizando las definiciones ya
implementadas en `internal/experiment`, en lugar de volver a definir throughput
o percentiles en un script independiente.

La herramienta de H2 deberá validar antes de analizar:

```text
revisión Git experimental
árbol limpio registrado
30 runs válidos por escenario
configuración comparable
120 operaciones programadas por run
restauración en escenarios con falla
```

Los resultados inferenciales serán artefactos derivados. Los JSON crudos no se
modificarán.

#### Salidas previstas

El análisis final de H2 deberá producir, como mínimo:

```text
experiments/processed/<revision>/final/statistics/
|-- per-run.csv
|-- descriptive.csv
|-- comparisons.csv
`-- analysis-metadata.txt
```

`per-run.csv` contendrá una fila por run válido y las métricas recalculadas.

`descriptive.csv` contendrá los resúmenes por escenario.

`comparisons.csv` contendrá las comparaciones contra el baseline, diferencias de
medianas, cambios relativos cuando correspondan, Cliff's delta, intervalos de
confianza, valores p Monte Carlo y valores p ajustados mediante Holm.

`analysis-metadata.txt` fijará la revisión, número de runs, semillas de análisis,
número de réplicas y parámetros estadísticos usados.

#### Determinismo del reprocesamiento

Con los mismos JSON crudos, la misma revisión de la herramienta y la misma
semilla de análisis, los artefactos derivados de H2 deberán regenerarse de forma
idéntica.

La salida deberá tener orden estable de escenarios, métricas y comparaciones, y
no depender de iteración no determinista sobre mapas.

#### Supuestos y límites

Las pruebas de permutación tratan los runs comparados como observaciones
intercambiables bajo la hipótesis nula.

Sin embargo, la campaña ejecutó los escenarios secuencialmente por bloques y no
aleatorizó su orden. Esto puede introducir dependencia temporal o deriva del
entorno entre escenarios.

H2 no ocultará esta limitación ni intentará corregirla retrospectivamente con una
prueba estadística. Su impacto se discutirá formalmente en H3 - Threats to
Validity.

La inferencia de H2 corresponde a esta campaña experimental. No implica
generalización automática a otras máquinas, cargas, tamaños de clúster,
versiones de Docker, sistemas operativos ni configuraciones de RaftKV.

#### Interpretación

Una conclusión de H2 deberá combinar:

```text
dirección del efecto
magnitud del efecto
intervalo de confianza
valor p ajustado
consistencia con el comportamiento del sistema
limitaciones del diseño experimental
```

No se presentará una diferencia como científicamente relevante únicamente porque
su valor p ajustado sea menor que 0.05.

Del mismo modo, un valor p mayor o igual que 0.05 no se interpretará como prueba
de igualdad entre escenarios.
