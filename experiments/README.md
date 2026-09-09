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

## G4 - Resultados reproducibles

G4 separa la ejecución del experimento de la reproducción de sus resultados.

```text
experiments/
|-- configs/
|-- raw/
|-- processed/
`-- README.md
```

`configs/` contiene los escenarios versionados. `raw/` contiene los JSON producidos
por el runner. `processed/` contiene únicamente artefactos derivados de esos JSON.

Cada ejecución se identifica por la revisión Git y por `CAMPAIGN`. Esto permite
conservar, por ejemplo, un smoke test y la campaña final sin sobrescribir evidencia.

### Ejecutar la campaña

La campaña final por defecto ejecuta 30 repeticiones de cada uno de los cuatro
escenarios G3, es decir, 120 ejecuciones.

```bash
make experiment
```

Equivale a `RUNS=30 CAMPAIGN=final`.

Para una prueba corta independiente:

```bash
make experiment RUNS=2 CAMPAIGN=smoke
```

`make experiment` exige un árbol Git limpio, valida el proyecto, obtiene la revisión
Git completa y guarda los resultados en:

```text
experiments/raw/<revision-git>/<campana>/
```

Los JSON de `raw/` están ignorados mientras se ejecuta la campaña. Esto es
deliberado: el runner G3 exige que cada ejecución observe la misma revisión y un
árbol limpio. Una vez terminada y validada la campaña, los datos crudos pueden
versionarse explícitamente:

```bash
git add -f experiments/raw/<revision-git>/final
```

No elimine ni sobrescriba una campaña existente. Use otro valor de `CAMPAIGN` si
necesita conservar otra ejecución de la misma revisión.

### Reproducir resultados procesados

`make reproduce` no inicia contenedores ni vuelve a ejecutar RaftKV. Procesa
exclusivamente los JSON existentes:

```bash
make reproduce
```

Para una campaña específica:

```bash
make reproduce REVISION=<revision-git> RUNS=30 CAMPAIGN=final
```

La revisión solicitada debe existir y ser ancestro de `HEAD`. Se permiten commits
posteriores únicamente cuando sus cambios están confinados a `experiments/raw/`
y `experiments/processed/`. Si cambió código o configuración, la reproducción se
rechaza para evitar procesar evidencia histórica con una implementación distinta.

La salida se guarda en:

```text
experiments/processed/<revision-git>/<campana>/
|-- summary.txt
|-- raw-manifest.sha256
|-- config-manifest.sha256
`-- metadata.txt
```

`summary.txt` se genera con `cmd/benchmark-failures`, por lo que conserva las
validaciones G3: número exacto de ejecuciones, misma revisión Git, árbol limpio,
configuración comparable y restauración completada en los escenarios con falla.

`raw-manifest.sha256` fija criptográficamente cada JSON crudo.
`config-manifest.sha256` fija los cuatro escenarios YAML utilizados.
`metadata.txt` registra revisión, campaña y tamaño esperado.

### Comprobar reproducción

Después de versionar una campaña y sus resultados procesados en un commit de
evidencia:

```bash
make reproduce REVISION=<revision-experimental> RUNS=30 CAMPAIGN=final
git diff --exit-code -- experiments/processed/<revision-experimental>/final
```

Si el segundo comando no produce diferencias, los artefactos procesados se
reconstruyeron de forma idéntica desde los datos crudos.

### Versionar la evidencia final

Después de revisar la campaña de 30 repeticiones:

```bash
git add -f experiments/raw/<revision-git>/final
git add experiments/processed/<revision-git>/final
git diff --check
```

Los datos crudos deben conservarse sin edición manual. Si se necesita corregir el
pipeline experimental, debe generarse una nueva campaña desde una nueva revisión.
