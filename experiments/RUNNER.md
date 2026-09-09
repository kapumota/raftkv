# Ejecución de experimentos

El runner F2 funciona en Linux con Go y Docker local. Usa los contenedores
`raft-node-1` a `raft-node-5` del Compose del proyecto. No requiere Docker dentro
de un contenedor ni monta su socket. El usuario debe poder ejecutar Docker.

## Preparación y validación

Desde la raíz del repositorio:

```bash
go mod tidy
make fmt
go test -race ./internal/experiment -count=1 -v
make validate
go build -o /tmp/raftkv-experiment ./cmd/experiment-runner
/tmp/raftkv-experiment -scenario experiments/leader_partition.yaml -check
```

`-check` solo valida. El parser exige todos los campos, tipos exactos, ausencia
de duplicados y de campos desconocidos, un único documento y un máximo de 64 KiB.
Para acotar recursos se admiten duraciones de 3 a 3600 segundos, de 1 a 128
clientes, de 1 a 1000 operaciones por segundo y como máximo un millón de
operaciones programadas por ejecución.

## Clúster local

No ejecute los scripts manuales de partición ni otro generador de carga durante
el experimento. Detenga el cliente anterior y arranque únicamente los nodos:

```bash
docker compose stop client-driver
docker compose -f docker-compose.yml -f docker-compose.experiments.yml up -d --build \
  raft-node-1 raft-node-2 raft-node-3 raft-node-4 raft-node-5
```

El archivo complementario publica los puertos 18081 a 18085 exclusivamente en
`127.0.0.1`. Compose puede recrear contenedores al añadir los puertos; conserva
los volúmenes WAL. No se elimina ningún volumen ni se reinicia el estado lógico.

El runner espera cinco nodos accesibles y un líder único antes de generar carga.
Inspecciona la red efectiva y exige una única red compartida. Registra las imágenes
Docker y los hashes iniciales del log y del estado persistente. Son observaciones
secuenciales antes de la carga, no una instantánea atómica del clúster.

## Ejecución

Después de confirmar los cambios en Git:

```bash
/tmp/raftkv-experiment -scenario experiments/leader_partition.yaml \
  -output /tmp/raftkv-particion-01.json
```

El archivo de salida debe ser nuevo. Use nombres distintos para otras ejecuciones.
La salida incluye revisión y cambios de Git, configuración, WAL iniciales, eventos
reales y resultados por operación. Las duraciones por operación están en nanosegundos.

Las escrituras se distribuyen cíclicamente entre clientes sin cola de espera ni
reintentos automáticos. Se registran como `confirmada`, `rechazada`, `incierta`,
`omitida_saturacion` u `omitida_sin_lider`. Una respuesta distinta de confirmación
puede ser incierta, incluso cuando el nodo perdió liderazgo. Un timeout nunca se
interpreta como prueba de que la escritura no se aplicó.

El escenario determina la carga global. El backend consulta el líder periódicamente;
si observa varios líderes o ninguno, omite envíos hasta recuperar una vista única.
Los tiempos efectivos pueden desviarse por HTTP, disco y Docker. Las operaciones
vencidas no se acumulan para formar una ráfaga posterior.

Para `caida` se ejecuta `docker kill --signal=KILL` y después `docker start` sobre
el mismo contenedor. Para `particion` se desconecta el nodo de la red y se restaura
su dirección IP y sus alias originales. El objetivo queda fijado al aplicar la falla.

Ctrl+C y SIGTERM cancelan la carga e intentan restaurar con un contexto independiente.
Los errores de restauración se conservan en el JSON y producen salida distinta de cero.
SIGKILL, la caída del host o la terminación del daemon Docker no permiten garantizar
esa limpieza. Los eventos del resultado identifican el nodo afectado.

El bloqueo local impide ejecutar dos comandos del runner simultáneamente. No protege
contra comandos Docker externos. La API `RunExperiment` requiere que el llamador
coordine el acceso exclusivo al despliegue.

## Alcance de los resultados

Un experimento finalizado indica que terminó la planificación y se ejecutó la
restauración; no constituye una prueba de linealizabilidad ni de convergencia final.
Las pruebas de la fase E siguen cubriendo esas propiedades dentro de su alcance.
El runner tampoco verifica automáticamente el estado final de cada clave.

Los escenarios reutilizan claves determinadas por nombre, semilla y secuencia.
Compare ejecuciones únicamente con condiciones iniciales equivalentes y conserve
los hashes registrados. La semilla no reproduce exactamente las elecciones ni
el reparto de las operaciones omitidas bajo carga.

Para detener el clúster sin eliminar sus volúmenes:

```bash
docker compose -f docker-compose.yml -f docker-compose.experiments.yml stop
```
