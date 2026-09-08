### RaftKV

RaftKV es un clúster experimental de cinco nodos escrito en Go. Implementa
Raft desde cero, sin `hashicorp/raft` ni otra biblioteca de consenso, y se
orquesta con Docker Compose.

El proyecto está orientado al estudio de sistemas distribuidos y forma parte
de una ruta de aprendizaje aplicada a infraestructura fintech. No pretende
ser una base de datos lista para producción.

#### Estado del proyecto

Estado actual: desarrollo experimental.

La implementación base incluye elección de líder, replicación del log,
persistencia parcial mediante WAL, aplicación de comandos comprometidos y un
generador de carga. Antes de considerar una versión estable todavía se deben
completar pruebas de fallas, recuperación completa del estado, confirmación de
escrituras después del commit y lecturas linealizables.

#### Qué implementa

- Elección de líder con timeouts aleatorios y votación por mayoría.
- Replicación del log mediante `AppendEntries`, con retroceso de `nextIndex`
  cuando un follower rechaza una entrada.
- Avance de `commitIndex` por mayoría, respetando la regla de Raft de
  comprometer directamente solo entradas del término actual del líder.
- Write-ahead log persistente en disco, con un volumen Docker por nodo.
- Recuperación de `currentTerm`, `votedFor` y log al reiniciar el proceso.
- Máquina de estados KV con operaciones `SET` y `GET`.
- Particiones de red reales mediante desconexión temporal de un contenedor de
  la red Docker.
- Cliente de carga que intenta localizar al líder mediante reintentos.

#### Simplificaciones deliberadas

- No hay snapshots ni compactación del log. El WAL puede crecer sin límite.
- No hay cambios dinámicos de membresía. El clúster está fijado en cinco nodos.
- `GET` no es linealizable. Lee el estado local del nodo consultado y puede
  observar un valor atrasado respecto del líder.
- `POST /kv/set` confirma que el líder aceptó la propuesta, pero todavía no
  espera a que la entrada quede comprometida por mayoría.
- La recuperación después de reiniciar restaura el estado persistente de Raft,
  pero todavía no reconstruye completamente la máquina de estados KV.
- El cliente descubre al líder por prueba y error. Cuando recibe
  `no_es_lider`, intenta otro nodo.

Estas limitaciones están documentadas para que la evolución del proyecto sea
verificable y no se presenten garantías que la implementación todavía no
ofrece.

#### Convenciones del repositorio

- Identificadores, tipos, funciones y métodos de Go se mantienen en inglés.
- Comentarios y mensajes dirigidos a personas se escriben en español.
- Los términos propios del protocolo, como `RequestVote`, `AppendEntries`,
  `leader`, `follower`, `commitIndex` y `nextIndex`, se conservan cuando forman
  parte del vocabulario técnico o de identificadores del código.
- La documentación evita símbolos tipográficos innecesarios. Para relaciones
  textuales se usa `->` cuando corresponde.
- Los títulos Markdown usan `###` y los subtítulos usan `####`.

#### Estructura

```text
raftkv/
|-- .github/workflows/ci.yml
|-- cmd/
|   |-- client-driver/
|   `-- raftnode/
|-- docker/
|-- internal/
|   |-- kv/
|   `-- raft/
|-- scripts/
|-- docker-compose.yml
|-- go.mod
|-- Makefile
`-- README.md
```

#### Validación local

```bash
make validate
```

El comando ejecuta formato, análisis estático, pruebas con detector de carreras
y compilación completa.

También se pueden ejecutar los pasos por separado:

```bash
gofmt -w ./cmd ./internal
go vet ./...
go test -race ./...
go build ./...
```

#### Levantar el clúster

```bash
docker compose up --build
```

Docker Compose construye las imágenes de `raft-node` y `client-driver`, levanta
los cinco nodos con volúmenes WAL independientes y arranca el generador de
carga. La elección del líder ocurre automáticamente.

#### Ver el estado de un nodo

Desde el propio contenedor:

```bash
docker compose exec raft-node-1 curl -s http://localhost:8080/status
```

La respuesta contiene el identificador del nodo, estado Raft, término actual,
voto persistido e índice de commit.

#### Escribir y leer directamente

```bash
docker compose exec raft-node-1 curl -s -X POST \
  http://localhost:8080/kv/set \
  -H 'Content-Type: application/json' \
  -d '{"Key":"nombre","Value":"kapumota"}'

docker compose exec raft-node-1 curl -s \
  'http://localhost:8080/kv/get?key=nombre'
```

Si el nodo consultado no es líder, `/kv/set` devuelve HTTP 421 con:

```json
{"error":"no_es_lider"}
```

#### Provocar una partición real

```bash
chmod +x scripts/partition.sh
./scripts/partition.sh raft-node-3 20
```

El script desconecta temporalmente `raft-node-3` de la red Docker y después lo
reconecta. Si el nodo aislado era líder, el resto del clúster debe elegir un
nuevo líder siempre que conserve mayoría.

Para comprobar el nombre de la red:

```bash
docker network ls | grep raftnet
```

#### Qué observar durante una partición

- Tiempo entre la partición del líder y la elección de uno nuevo.
- Cambio del líder anterior a follower cuando recibe un término superior.
- Convergencia de los logs después de recuperar la conectividad.
- Evolución de escrituras, fallos y latencia reportada por `client-driver`.
- Ausencia de divergencias en comandos ya comprometidos por mayoría.

#### Trabajo pendiente

La siguiente iteración debe priorizar corrección y validación antes de ampliar
funcionalidades:

1. Esperar el commit por mayoría antes de confirmar una escritura.
2. Reconstruir la máquina de estados después de un reinicio.
3. Implementar lecturas linealizables.
4. Añadir identificadores de solicitud y supresión de duplicados.
5. Ampliar pruebas de elección, partición, reinicio y convergencia.
6. Añadir benchmarks reproducibles de throughput, latencia y failover.
7. Evaluar snapshots solo después de cerrar las garantías anteriores.
