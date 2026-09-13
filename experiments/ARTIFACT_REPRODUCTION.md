### Artifact and Reproduction Guide

Esta guía describe cómo verificar, reproducir y auditar los artefactos de la campaña experimental final de RaftKV y su análisis estadístico H2.

El objetivo es permitir que un reviewer trabaje desde un clon limpio sin necesidad de reconstruir información no registrada del host original.

La guía distingue explícitamente entre:

```text
verificación de integridad
reproducción histórica de G4
reproducción computacional de H2
replicación experimental de una nueva campaña
```

Estas actividades no son equivalentes y utilizan revisiones Git diferentes.

#### Identificadores autoritativos

La revisión sobre la que se ejecutó la campaña experimental final es:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

El commit que añade la evidencia G4 sin modificar código ni configuración respecto de esa revisión es:

```text
26358a950e7b40eb7e4ea36919dc60cc6ce660b1
```

Este segundo commit es el ancla recomendada para reproducir históricamente los artefactos G4 mediante `make reproduce`.

La evidencia estadística H2 se encuentra versionada en `main` bajo:

```text
experiments/processed/
`-- 8ba7a131c4f452aac014a4c628794062fa1a8c9b/
    `-- final/
        `-- statistics/
```

#### Fuentes metodológicas

La interpretación del artefacto debe realizarse conjuntamente con:

```text
experiments/METHODOLOGY.md
experiments/STATISTICAL_ANALYSIS.md
experiments/THREATS_TO_VALIDITY.md
experiments/README.md
```

`METHODOLOGY.md` define el protocolo experimental.

`STATISTICAL_ANALYSIS.md` fija las decisiones estadísticas.

`THREATS_TO_VALIDITY.md` delimita las conclusiones que pueden extraerse.

Este documento se concentra únicamente en reproducción y auditoría del artefacto.

#### Requisitos para verificar y reproducir artefactos

Para los niveles de reproducción que no vuelven a ejecutar la campaña se requieren:

```text
Git
GNU Make
Bash
Go compatible con el módulo del proyecto
GNU coreutils
```

Los scripts utilizan, entre otras herramientas:

```text
awk
cmp
find
install
mktemp
sha256sum
sort
wc
```

El módulo declara Go 1.22.

Esto no significa que se conozca la versión exacta del toolchain Go utilizado en el host original de la campaña. Esa versión no quedó registrada de forma verificable.

Docker no es necesario para:

```text
verificar manifests
reproducir G4 desde JSON existentes
reproducir el análisis H2 desde JSON existentes
```

Docker Engine y Docker Compose sí son necesarios para ejecutar una nueva campaña experimental.

Los scripts dependen de Bash y utilidades GNU. En sistemas que no las incluyen por defecto debe instalarse un entorno compatible antes de utilizar los comandos de esta guía.

#### Estructura del artefacto

La campaña final se organiza como:

```text
experiments/
|-- configs/
|-- raw/
|   `-- <revision>/
|       `-- final/
|-- processed/
|   `-- <revision>/
|       `-- final/
|           |-- summary.txt
|           |-- raw-manifest.sha256
|           |-- config-manifest.sha256
|           |-- metadata.txt
|           `-- statistics/
`-- README.md
```

La evidencia cruda nunca debe editarse para intentar hacer pasar una verificación.

Si un checksum no coincide, debe recuperarse una copia limpia del repositorio o investigarse la diferencia.

#### Nivel A: verificar integridad desde main

Este nivel no recalcula resultados. Verifica que los archivos versionados coincidan con sus manifests.

Desde un clon limpio:

```bash
git clone https://github.com/kapumota/raftkv.git
cd raftkv
git status --short
```

Defina las rutas:

```bash
REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b
RAW_DIR="experiments/raw/${REVISION}/final"
PROCESSED_DIR="experiments/processed/${REVISION}/final"
STATISTICS_DIR="${PROCESSED_DIR}/statistics"
ROOT="$(pwd)"
```

Debe haber exactamente 120 JSON válidos en el nivel principal de la campaña:

```bash
find "$RAW_DIR" -maxdepth 1 -type f -name '*.json' | wc -l
```

Salida esperada:

```text
120
```

Los intentos inválidos y la ejecución interrumpida se conservan aparte y no forman parte de esos 120 JSON válidos.

Verifique los JSON crudos:

```bash
(
  cd "$RAW_DIR"
  sha256sum -c "$ROOT/$PROCESSED_DIR/raw-manifest.sha256"
)
```

Las 120 entradas deben terminar en:

```text
OK
```

Verifique los cuatro YAML utilizados:

```bash
(
  cd experiments/configs
  sha256sum -c "$ROOT/$PROCESSED_DIR/config-manifest.sha256"
)
```

Verifique las fuentes utilizadas por H2:

```bash
sha256sum -c "$STATISTICS_DIR/analysis-source-manifest.sha256"
```

Verifique todos los artefactos estadísticos:

```bash
(
  cd "$STATISTICS_DIR"
  sha256sum -c analysis-manifest.sha256
)
```

No debe aparecer ningún `FAILED`.

#### Hashes esperados de H2

El manifest estadístico congelado contiene:

```text
bf220a95b789bb60665d721301e7bfe5a6c417c512ebeaa27a858f1c30d7ceb9  per-run.csv
a14e74b570a39e1ade2c04d2fdf205595292715e0b522df808e83d22e68c0525  descriptive.csv
5e9a457df3b359f9d94a24897e063b7f70ba15ea102e688c32e14114ad885490  effects.csv
12b066b4da22172dc1f3b4620ef793828466ee305526edeb43b95d5be89be1d8  comparisons.csv
0e44c4d346020c19794b01fd8efde92e8fd531dc207b62392079f1adf15d6973  analysis-metadata.txt
18162d906def33a3bc5158406494c261872a07e5db08d074deb5eda2ea844ede  analysis-source-manifest.sha256
```

El SHA-256 del manifest G4 de datos crudos registrado por H2 es:

```text
99dbf420059d2ff102755da0ce5ae5a2efcd82a50f1f8f4a6f35170a5606fb38
```

Puede comprobarse mediante:

```bash
sha256sum "$PROCESSED_DIR/raw-manifest.sha256"
```

#### Nivel B: reproducir históricamente G4

`make reproduce` protege la evidencia histórica y rechaza procesarla con código o configuración que hayan cambiado después de la revisión experimental.

Por esa razón no debe utilizarse el `main` actual para reconstruir G4.

El commit autoritativo de evidencia para esta operación es:

```text
26358a950e7b40eb7e4ea36919dc60cc6ce660b1
```

Desde el clon principal puede crearse un worktree independiente:

```bash
G4_EVIDENCE=26358a950e7b40eb7e4ea36919dc60cc6ce660b1

git worktree add --detach ../raftkv-g4 "$G4_EVIDENCE"
cd ../raftkv-g4
```

Compruebe el estado:

```bash
git status --short
git rev-parse HEAD
```

La segunda salida debe ser:

```text
26358a950e7b40eb7e4ea36919dc60cc6ce660b1
```

Puede auditarse que entre la revisión experimental y este commit no cambió código ni configuración:

```bash
git diff --name-only   8ba7a131c4f452aac014a4c628794062fa1a8c9b..HEAD   -- .   ':(exclude)experiments/raw/**'   ':(exclude)experiments/processed/**'
```

La salida esperada es vacía.

Valide el proyecto:

```bash
make validate
```

Regenere los resultados G4:

```bash
make reproduce   REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b   RUNS=30   CAMPAIGN=final
```

Salida final esperada:

```text
Resultados reproducidos en: experiments/processed/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final
```

Compruebe que la reproducción no cambió los artefactos versionados:

```bash
git diff --exit-code --   experiments/processed/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final
```

Un código de salida igual a cero y ausencia de diff indican reproducción idéntica de G4.

Al terminar:

```bash
cd ../raftkv
git worktree remove ../raftkv-g4
```

#### Por qué make reproduce falla intencionalmente en main actual

Después de G4 se añadieron metodología, estadística, inferencia y documentación.

Por diseño, `scripts/reproduce.sh` comprueba:

```text
revisión experimental
        |
        v
HEAD usado para reproducir G4
```

y permite únicamente cambios bajo:

```text
experiments/raw/
experiments/processed/
```

Por tanto, ejecutar desde `main` actual:

```bash
make reproduce   REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b   RUNS=30   CAMPAIGN=final
```

debe rechazarse porque existen cambios posteriores de código y documentación fuera de las rutas de evidencia.

Ese rechazo es una protección metodológica y no un fallo de reproducibilidad.

Use el worktree del commit `26358a...` para G4.

#### Nivel C: reproducir H2 desde main

El análisis H2 está diseñado para ejecutarse sobre la evidencia G4 congelada desde el código estadístico actual versionado.

Regrese al checkout principal:

```bash
cd raftkv
git switch main
git pull --ff-only
git status --short
```

El árbol debe estar limpio.

Ejecute:

```bash
make validate
```

Después:

```bash
make analyze   REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b   RUNS=30   CAMPAIGN=final
```

El script:

```text
verifica raw-manifest.sha256
verifica que existan 120 JSON válidos
recalcula las métricas por run
recalcula descriptivos
recalcula bootstrap y Cliff's delta
recalcula permutaciones y Holm
recalcula metadata
recalcula manifests
compara byte a byte contra los artefactos congelados
```

La salida final esperada es:

```text
Análisis H2 reproducido sin diferencias: experiments/processed/8ba7a131c4f452aac014a4c628794062fa1a8c9b/final/statistics
```

Compruebe finalmente:

```bash
git status --short
git diff --exit-code
```

Ambos deben indicar ausencia de cambios.

#### Parámetros autoritativos de H2

`analysis-metadata.txt` fija:

```text
revision_experimental=8ba7a131c4f452aac014a4c628794062fa1a8c9b
campana=final
runs_por_escenario=30
escenarios=4
runs_totales=120
metricas=6
comparaciones=18
comparaciones_testeadas=18
bootstrap_replicas=100000
permutation_replicas=100000
confidence_level=0.95
alpha=0.05
analysis_seed=20260912
quantiles=Hyndman-Fan type 7
mad=raw
stddev=sample
permutation_statistic=median_difference
permutation_alternative=two-sided
multiplicity_adjustment=Holm
```

Estos parámetros no deben ajustarse después de observar los resultados para intentar obtener otra conclusión.

#### Artefactos producidos por H2

`per-run.csv` contiene una fila por run válido y las métricas recalculadas desde los JSON crudos.

`descriptive.csv` contiene los descriptivos robustos por escenario y métrica:

```text
n
min
q1
median
q3
max
iqr
mad
mean
stddev
```

`effects.csv` contiene:

```text
diferencia de medianas
IC bootstrap de la diferencia
cambio relativo cuando está definido
Cliff's delta
IC bootstrap de Cliff's delta
```

`comparisons.csv` añade:

```text
prueba de permutación bilateral
número de resultados extremos
valor p Monte Carlo
tamaño de la familia Holm
valor p ajustado por Holm
```

`analysis-metadata.txt` fija los parámetros del análisis.

`analysis-source-manifest.sha256` fija las fuentes que producen H2.

`analysis-manifest.sha256` fija los artefactos derivados.

#### Comprobaciones semánticas mínimas

G4 debe reproducir 30 runs válidos por escenario y el siguiente resumen descriptivo redondeado a tres decimales:

```text
Escenario             Throughput  p50 ms   p95 ms    p99 ms    Inciertas  Omitidas
sin fallas                1.983   418.087  1903.720  2692.064      0          0
follower caído            1.972   423.315  1872.698  2564.284      0          0
leader caído              1.808   410.819  2697.890  4246.179     11          0
partición de leader       1.817   391.942  1951.367  3026.359     10          0
```

Los escenarios con falla deben registrar restauración completa en 30 de 30 runs válidos.

El baseline no debe registrar restauraciones.

H2 debe producir:

```text
120 filas de datos por run
24 filas descriptivas
18 filas de efectos
18 comparaciones inferenciales
18 comparaciones testeadas
```

Estos conteos excluyen las cabeceras CSV.

#### Comprobaciones inferenciales mínimas

Después del ajuste Holm, la campaña soporta dentro de su alcance evidencia para:

```text
leader caído:
  reducción de throughput
  incremento de p95
  incremento de p99
  incremento de uncertain_writes

partición de leader:
  reducción de throughput
  incremento de uncertain_writes
```

La caída de un follower no presenta una diferencia robusta frente al baseline después del ajuste por multiplicidad.

Estas comprobaciones son criterios de consistencia del artefacto, no una demostración formal de safety o liveness.

#### Nivel D: replicar experimentalmente una nueva campaña

Una nueva campaña requiere Docker Engine y Docker Compose además de los requisitos anteriores.

Para mantener separado el código experimental histórico puede utilizarse otro worktree:

```bash
EXPERIMENT_REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b

git worktree add --detach ../raftkv-replication "$EXPERIMENT_REVISION"
cd ../raftkv-replication
make validate
```

Ejecute primero un smoke independiente:

```bash
make experiment RUNS=2 CAMPAIGN=replication-smoke
```

Una replicación completa del protocolo puede ejecutarse con:

```bash
make experiment RUNS=30 CAMPAIGN=replication-final
```

No use `CAMPAIGN=final` para una nueva ejecución cuando exista riesgo de confundirla con la campaña original.

La nueva evidencia se almacenará bajo:

```text
experiments/raw/8ba7a131c4f452aac014a4c628794062fa1a8c9b/replication-final/
```

#### Qué debe y qué no debe reproducir una nueva campaña

La semilla de workload reproduce:

```text
plan de carga
claves
valores
asignación determinista de datos
```

No reproduce necesariamente:

```text
leader elegido
términos
instantes exactos de elección
scheduling
latencias
intercalados concurrentes
timeouts
orden de eventos del sistema distribuido
```

Por tanto, una nueva campaña no debe compararse byte a byte con los JSON originales.

Debe compararse mediante el protocolo, las métricas y el análisis definidos.

#### Limitación del entorno original

La campaña original no conserva de forma verificable toda la metadata necesaria para reconstruir exactamente el host físico y su software de sistema.

No están documentados con precisión suficiente:

```text
CPU exacta
cores e hilos
RAM exacta
almacenamiento
sistema operativo y kernel del host
Docker Engine exacto
Docker Compose exacto
toolchain Go exacto del host
```

Esta información no debe inventarse.

Por esta razón H4 garantiza reproducción computacional de los artefactos conservados, pero no afirma recreación exacta del entorno físico original.

#### Evidencia inválida conservada

La campaña conserva evidencia que no pertenece a los 120 runs válidos.

Incluye dos intentos donde el runner no pudo identificar un leader único para resolver el objetivo de falla dentro del plazo del protocolo.

También se conserva una ejecución baseline interrumpida:

```text
normal-faults-5-027.interrumpido.20260910T000151Z.invalid
```

Esta evidencia no debe renombrarse como run válido ni incorporarse manualmente a los CSV estadísticos.

Los criterios de inclusión están definidos en `METHODOLOGY.md`.

#### Problemas frecuentes

Si `make reproduce` informa que cambió código o configuración, no elimine la protección.

Use el worktree en:

```text
26358a950e7b40eb7e4ea36919dc60cc6ce660b1
```

Si `make analyze` informa que el árbol está sucio:

```bash
git status --short
```

y restaure un checkout limpio antes de reintentarlo.

No use `git stash` de forma automática si existen cambios que deben conservarse; revise primero su estado.

Si un SHA-256 no coincide, no regenere ni edite el manifest para ocultar la diferencia. Recupere la evidencia versionada y determine por qué cambió el archivo.

Si faltan dependencias Go en caché, la primera compilación puede requerir acceso a la fuente configurada para módulos Go. Esto es independiente de los datos experimentales.

#### Checklist para reviewer

Una revisión mínima del artefacto debe poder contestar afirmativamente:

```text
[ ] el checkout está limpio
[ ] existen 120 JSON válidos
[ ] raw-manifest.sha256 verifica los 120 JSON
[ ] config-manifest.sha256 verifica los cuatro YAML
[ ] G4 se reproduce en el commit 26358a...
[ ] la reproducción G4 no produce diff
[ ] analysis-source-manifest.sha256 verifica las fuentes H2
[ ] analysis-manifest.sha256 verifica los artefactos H2
[ ] make analyze termina sin diferencias
[ ] quedan 24 filas descriptivas
[ ] quedan 18 comparaciones de efecto
[ ] quedan 18 comparaciones inferenciales
[ ] la metadata registra 100000 bootstrap y 100000 permutaciones
[ ] el árbol Git permanece limpio después de reproducir
```

#### Alcance de la reproducibilidad

El artefacto permite auditar:

```text
configuración experimental
evidencia cruda
integridad mediante SHA-256
agregación G4
métricas por run
descriptivos
tamaños de efecto
bootstrap
Cliff's delta
permutaciones
corrección Holm
metadata del análisis
fuentes del análisis
```

No demuestra por sí mismo:

```text
linealizabilidad de todos los historiales posibles
Election Safety para todas las ejecuciones posibles
Log Matching para todos los estados alcanzables
Leader Completeness para todos los estados alcanzables
State Machine Safety mediante prueba formal
liveness bajo cualquier modelo de red
```

La clasificación explícita de invariantes y su evidencia corresponde a la Fase I.

#### Criterio de éxito de H4

H4 se considera reproducido correctamente cuando:

```text
1. los manifests SHA-256 son válidos;
2. G4 se regenera sin diff desde el commit de evidencia;
3. H2 se regenera byte a byte desde main mediante make analyze;
4. el checkout permanece limpio;
5. las limitaciones de replicación experimental se mantienen explícitas.
```

Una nueva campaña puede utilizarse para replicar el estudio, pero no sustituye la evidencia original ni debe esperarse que genere archivos idénticos.
