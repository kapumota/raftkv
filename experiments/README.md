### Evaluación experimental reproducible

Este directorio contiene configuraciones, evidencia cruda, artefactos derivados,
metodología y resultados de la evaluación experimental de RaftKV.

Para la campaña final, las fuentes principales son:

- [METHODOLOGY.md](METHODOLOGY.md): protocolo experimental.
- [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md): plan y análisis H2.
- [THREATS_TO_VALIDITY.md](THREATS_TO_VALIDITY.md): límites de interpretación.
- [ARTIFACT_REPRODUCTION.md](ARTIFACT_REPRODUCTION.md): reproducción verificable.
- [RESULTS.md](RESULTS.md): resultados y figuras finales.

[RUNNER.md](RUNNER.md) documenta el runner manual. [BENCHMARKS.md](BENCHMARKS.md)
conserva la evolución histórica G2/G3 y no sustituye la metodología final.

#### Escenarios de la campaña final

| Archivo | Objetivo | Falla | Restauración | Fin |
| --- | --- | --- | --- | --- |
| `normal-faults-5.yaml` | Ninguno | Sin falla | No aplica | 60 s |
| `follower-down-5.yaml` | Follower | Caída a los 20 s durante 10 s | Aproximadamente 30 s | 60 s |
| `leader-down-5.yaml` | Leader | Caída a los 20 s durante 10 s | Aproximadamente 30 s | 60 s |
| `leader-partition-5.yaml` | Leader | Partición a los 20 s durante 10 s | Aproximadamente 30 s | 60 s |

Los cuatro escenarios finales usan cinco nodos, 16 clientes, 2 operaciones por
segundo y semilla de workload 42.

#### Contrato de configuración

El parser rechaza campos desconocidos, claves duplicadas, documentos múltiples y
valores con tipos incorrectos.

Los escenarios versionados utilizan:

- `version`: versión del formato; actualmente `1`.
- `nombre`: identificador lógico del experimento.
- `semilla`: entero no negativo para la carga determinista.
- `nodos`: `3` o `5` según el despliegue.
- `despliegue`: `local` o `benchmark`.
- `duracion_segundos`: ventana total de carga.
- `clientes`: máximo de clientes con una operación pendiente cada uno.
- `operaciones_por_segundo`: tasa objetivo global.
- `falla.tipo`: `ninguna`, `caida` o `particion`.
- `falla.objetivo`: vacío, `lider` o `seguidor` según el tipo.
- `falla.instante_segundos`: instante programado de la falla.
- `falla.duracion_segundos`: duración programada de la falla.

La versión 1 genera escrituras `SET` con claves determinadas por escenario,
semilla y secuencia. Los clientes reciben operaciones en orden cíclico. Si el
cliente asignado sigue ocupado, la operación se registra como omitida por
saturación; no se acumula una cola ilimitada.

Los errores y timeouts se registran sin reintentos automáticos. Un timeout no
demuestra que una escritura no se haya confirmado.

#### Selección y restauración

El runner espera disponibilidad del clúster antes de iniciar la ventana de carga.
Para escenarios dirigidos a roles, resuelve el objetivo a partir del estado
observado. Si no puede identificar un leader único cuando corresponde, el intento
termina con error y no se sustituye silenciosamente el objetivo.

Para un follower se selecciona de forma determinista entre los followers
disponibles. El nodo elegido queda fijado para la restauración aunque cambien
posteriormente los roles.

`caida` termina abruptamente el proceso y vuelve a iniciar el mismo nodo.
`particion` corta su comunicación con los demás nodos y después restaura los
enlaces.

La restauración se intenta también ante error o cancelación. Un error de
restauración queda registrado y evita declarar el experimento exitoso.

#### Aislamiento entre runs

Hay dos modos distintos que no deben confundirse.

El runner manual descrito en [RUNNER.md](RUNNER.md) puede trabajar con un
despliegue existente y conservar sus volúmenes.

La campaña reproducible ejecutada por:

```bash
make experiment
```

usa `scripts/benchmark-failures.sh`. Cada run ejecuta `docker compose down -v`
antes de levantar el siguiente despliegue. Por tanto, los WAL y el estado
persistente comienzan vacíos en cada repetición de la campaña.

Los JSON permanecen fuera de los volúmenes Docker y se conservan en
`experiments/raw/`.

#### Ejecutar una nueva campaña

Por defecto:

```bash
make experiment
```

equivale a:

```text
RUNS=30
CAMPAIGN=final
```

Para una campaña corta independiente:

```bash
make experiment RUNS=2 CAMPAIGN=smoke
```

`make experiment` exige un árbol Git limpio, ejecuta `make validate`, fija la
revisión completa y rechaza sobrescribir una campaña existente.

Los resultados se guardan en:

```text
experiments/raw/<revision-git>/<campana>/
experiments/processed/<revision-git>/<campana>/
```

Las campañas locales nuevas están ignoradas por Git. Una campaña que deba
convertirse en evidencia se versiona explícitamente:

```bash
git add -f experiments/raw/<revision-git>/final
git add -f experiments/processed/<revision-git>/final
```

No se deben editar manualmente los JSON crudos ni sobrescribir campañas
existentes.

#### Campaña final congelada

La campaña final publicada corresponde a la revisión:

```text
8ba7a131c4f452aac014a4c628794062fa1a8c9b
```

Contiene:

```text
4 escenarios
30 runs válidos por escenario
120 runs válidos
```

La evidencia conserva además los intentos inválidos e interrumpidos descritos en
la metodología y en la guía de reproducción.

#### Reproducir y analizar evidencia

`make reproduce` procesa JSON existentes; no vuelve a ejecutar RaftKV.

Sin embargo, la campaña histórica final no debe reproducirse desde el `HEAD` del
release como si ese `HEAD` fuera la revisión experimental. Para la evidencia
congelada siga exactamente:

[ARTIFACT_REPRODUCTION.md](ARTIFACT_REPRODUCTION.md)

La guía documenta la revisión de evidencia y el uso de `git worktree` para la
reproducción histórica de G4.

El análisis estadístico H2 puede verificarse con:

```bash
make analyze \
  REVISION=8ba7a131c4f452aac014a4c628794062fa1a8c9b \
  RUNS=30 \
  CAMPAIGN=final
```

Los resultados derivados se encuentran bajo:

```text
experiments/processed/
  8ba7a131c4f452aac014a4c628794062fa1a8c9b/
    final/
```

#### Integridad

La evidencia utiliza manifests SHA-256 para vincular:

```text
JSON crudos
configuraciones
fuentes del análisis
artefactos estadísticos
figuras
```

Las figuras reproducibles están en `experiments/figures/`.

La guía autoritativa para comprobar checksums y reconstruir artefactos es
[ARTIFACT_REPRODUCTION.md](ARTIFACT_REPRODUCTION.md).

#### Interpretación

Repetir una semilla reproduce el plan y los datos de carga, no elecciones,
latencias ni interleavings idénticos.

La campaña experimental cuantifica comportamiento bajo el protocolo declarado.
No constituye una demostración formal de safety ni de liveness.
