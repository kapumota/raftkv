#!/usr/bin/env bash
set -euo pipefail

# Simula una particion de red real desconectando un contenedor de la red
# Docker del cluster, y lo reconecta despues de un tiempo dado. A diferencia
# de una simulacion en memoria, esto rompe la conectividad TCP real entre
# los nodos Raft.
#
# Uso: ./scripts/partition.sh <nombre-contenedor> <segundos-particionado> [red]
# Ejemplo: ./scripts/partition.sh raft-node-3 20 raftkv_raftnet

NODE="${1:?Falta el nombre del contenedor a particionar}"
DURATION="${2:?Falta la duracion en segundos}"
NETWORK="${3:-raftkv_raftnet}"

echo "[partition-tool] desconectando ${NODE} de ${NETWORK}"
docker network disconnect "${NETWORK}" "${NODE}"

echo "[partition-tool] ${NODE} aislado por ${DURATION}s"
sleep "${DURATION}"

echo "[partition-tool] reconectando ${NODE} a ${NETWORK}"
docker network connect "${NETWORK}" "${NODE}"

echo "[partition-tool] ${NODE} reincorporado al cluster"
