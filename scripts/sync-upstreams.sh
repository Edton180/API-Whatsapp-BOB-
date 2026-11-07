#!/usr/bin/env bash
# Atalho mantido por compatibilidade: a sincronização oficial é incremental
# (só aplica o que mudou na Evolution Go desde o commit registrado em UPSTREAM).
exec "$(dirname "$0")/sincronizar-evolution.sh" "$@"
