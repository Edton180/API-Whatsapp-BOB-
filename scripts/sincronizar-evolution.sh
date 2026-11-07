#!/usr/bin/env bash
# =============================================================================
# API WhatsApp BOB - traz as mudanças novas da Evolution Go oficial
#
# Aplica só o que mudou no projeto oficial desde a última sincronização (arquivo UPSTREAM), por
# diff + git apply --3way: as melhorias da BOB continuam, e um conflito aparece marcado no arquivo.
# O painel web da Evolution Go (manager/) e os arquivos de marca (public/) não entram (TRADEMARKS.md).
#
#   bash scripts/sincronizar-evolution.sh            último código do ramo principal oficial
#   bash scripts/sincronizar-evolution.sh <commit>   até um commit ou tag (ex.: 0.7.3)
#
# Saída: "nada-novo", "aplicado <de> <para>" ou "conflito <de> <para>" (código 2).
# Desenvolvido por Edton Braz.
# =============================================================================
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="${EVOLUTION_GO_REPO:-https://github.com/evolution-foundation/evolution-go.git}"
cd "$ROOT"
FROM="$(tr -d '[:space:]' < UPSTREAM)"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

git -C "$TMP" init -q
git -C "$TMP" remote add origin "$REPO"
if [[ -n "${1:-}" ]]; then
  git -C "$TMP" fetch -q --filter=blob:none origin "$1"
else
  git -C "$TMP" fetch -q --filter=blob:none origin HEAD
fi
TO="$(git -C "$TMP" rev-parse FETCH_HEAD)"
if [[ "$TO" == "$FROM" ]]; then
  echo "nada-novo"
  exit 0
fi
git -C "$TMP" fetch -q --filter=blob:none origin "$FROM" 2>/dev/null || git -C "$TMP" fetch -q --filter=blob:none --unshallow origin 2>/dev/null || true

# Só as partes que mudaram, sem o painel e sem a marca da Evolution
git -C "$TMP" diff --binary "$FROM" "$TO" -- . ':(exclude)manager' ':(exclude)public' ':(exclude)evolution-go' ':(exclude)README.md' \
  ':(exclude).github' ':(exclude)Dockerfile' ':(exclude)docker/stack-evocrm.yml' > "$TMP/mudancas.patch"

if [[ ! -s "$TMP/mudancas.patch" ]]; then
  printf '%s\n' "$TO" > UPSTREAM
  echo "aplicado ${FROM:0:7} ${TO:0:7}"
  exit 0
fi

status=0
git apply --3way --whitespace=nowarn "$TMP/mudancas.patch" || status=$?
VERSAO="$(git -C "$TMP" show "$TO:VERSION" 2>/dev/null | tr -d '[:space:]' || true)"
[[ -n "$VERSAO" ]] && printf '%s\n' "$VERSAO" > UPSTREAM_VERSION
printf '%s\n' "$TO" > UPSTREAM
git -C "$TMP" log --format='- %h %s' "$FROM..$TO" > "$TMP/log.txt" 2>/dev/null || true
cp "$TMP/log.txt" .sincronizacao-commits.txt 2>/dev/null || true

if [[ "$status" -ne 0 ]]; then
  echo "conflito ${FROM:0:7} ${TO:0:7}"
  exit 2
fi
echo "aplicado ${FROM:0:7} ${TO:0:7}"
