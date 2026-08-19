#!/usr/bin/env bash

EST="${EST:-http://localhost:8081}"
FAT="${FAT:-http://localhost:8082}"
CONTAINER_ESTOQUE="${CONTAINER_ESTOQUE:-korp-estoque}"
CONTAINER_POSTGRES="${CONTAINER_POSTGRES:-korp-postgres}"
# Detecta o motor de containers disponivel -- esta maquina so tem docker,
# mas o projeto foi construido originalmente com podman.
MOTOR="${MOTOR:-$(command -v podman > /dev/null 2>&1 && echo podman || echo docker)}"

titulo()  { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
passo()   { printf '\033[1;33m-> %s\033[0m\n' "$*"; }
ok()      { printf '   \033[1;32mok\033[0m %s\n' "$*"; }
falha()   { printf '   \033[1;31mFALHA\033[0m %s\n' "$*"; }
nota_de() { printf '   %s\n' "$*"; }

campo() { grep -o "\"$2\":\"[^\"]*\"" <<<"$1" | head -1 | cut -d'"' -f4; }

numero() { grep -o "\"$2\":[0-9]*" <<<"$1" | head -1 | cut -d: -f2; }

saldo_de() { curl -sS "$EST/api/produtos/$1" | grep -o '"saldo":[0-9]*' | head -1 | cut -d: -f2; }

uuid() {
  # /proc/sys/kernel/random/uuid nao existe no Git Bash do Windows -- gera
  # em bash puro (nao precisa ser criptografico, so unico o bastante pro demo).
  if [ -r /proc/sys/kernel/random/uuid ]; then
    cat /proc/sys/kernel/random/uuid
  else
    printf '%04x%04x-%04x-4%03x-%04x-%04x%04x%04x\n' \
      "$RANDOM" "$RANDOM" "$RANDOM" $((RANDOM & 0x0fff)) \
      $(((RANDOM & 0x3fff) | 0x8000)) "$RANDOM" "$RANDOM" "$RANDOM"
  fi
}

nota_com_item() {
  local nota id
  nota="$(curl -sS -X POST "$FAT/api/notas")"
  id="$(campo "$nota" id)"
  curl -sS -X POST "$FAT/api/notas/$id/itens" \
    -H 'Content-Type: application/json' \
    -d "{\"codigo\":\"$1\",\"quantidade\":$2}" > /dev/null
  printf '%s' "$id"
}

psql_estoque() { "$MOTOR" exec "$CONTAINER_POSTGRES" psql -U korp -d db_estoque -tAc "$1"; }

esperar_estoque() {
  for _ in $(seq 1 30); do
    if [ "$(curl -sS -o /dev/null -w '%{http_code}' "$EST/health" 2>/dev/null)" = "200" ]; then
      return 0
    fi
    sleep 1
  done
  return 1
}

exigir_servicos() {
  if [ "$(curl -sS -o /dev/null -w '%{http_code}' "$FAT/health" 2>/dev/null)" != "200" ]; then
    falha "Faturamento nao responde em $FAT -- suba a stack antes ($MOTOR compose up -d)"
    exit 1
  fi
}
