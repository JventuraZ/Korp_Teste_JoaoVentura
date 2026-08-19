#!/usr/bin/env bash
set -u
cd "$(dirname "$0")/.."
source scripts/comum.sh

exigir_servicos

CONCORRENTES="${CONCORRENTES:-6}"

CODIGO="PRD-DISPUTA-$$"

limpar() {
  psql_estoque "UPDATE produtos SET ativo = false WHERE codigo = '$CODIGO'" > /dev/null 2>&1
}
trap limpar EXIT

titulo "Preparacao"
curl -sS -X POST "$EST/api/produtos" -H 'Content-Type: application/json' \
  -d "{\"codigo\":\"$CODIGO\",\"descricao\":\"Ultima unidade em disputa\",\"saldo\":1}" > /dev/null
nota_de "produto $CODIGO criado com saldo 1"

passo "montando $CONCORRENTES notas, cada uma pedindo 1 unidade"
ids=()
for _ in $(seq 1 "$CONCORRENTES"); do
  ids+=("$(nota_com_item "$CODIGO" 1)")
done
ok "${#ids[@]} notas prontas, todas Abertas"

titulo "Disparando as impressoes simultaneamente"
temporario="$(mktemp -d)"
for i in "${!ids[@]}"; do
  (
    status="$(curl -sS -o "$temporario/corpo-$i" -w '%{http_code}' \
      -X POST "$FAT/api/notas/${ids[$i]}/impressao")"
    echo "$status" > "$temporario/status-$i"
  ) &
done
wait
ok "todas as respostas chegaram"

titulo "Resultado"
vencedoras=0
recusadas=0
outros=0
for i in "${!ids[@]}"; do
  status="$(cat "$temporario/status-$i")"
  case "$status" in
    200) vencedoras=$((vencedoras+1)); printf '   nota %d: \033[1;32m200 impressa\033[0m\n' "$((i+1))" ;;
    422) recusadas=$((recusadas+1));  printf '   nota %d: \033[1;33m422 saldo insuficiente\033[0m\n' "$((i+1))" ;;
    *)   outros=$((outros+1));        printf '   nota %d: \033[1;31m%s\033[0m %s\n' "$((i+1))" "$status" "$(cat "$temporario/corpo-$i")" ;;
  esac
done
rm -rf "$temporario"

echo
[ "$vencedoras" = "1" ] \
  && ok "exatamente 1 nota foi impressa" \
  || falha "$vencedoras notas impressas -- esperado exatamente 1"

[ "$recusadas" = "$((CONCORRENTES-1))" ] \
  && ok "as outras $recusadas receberam 422 com mensagem clara" \
  || falha "$recusadas recusas, esperado $((CONCORRENTES-1))"

[ "$outros" = "0" ] || falha "$outros resposta(s) inesperada(s)"

saldo_final="$(saldo_de "$CODIGO")"
nota_de "saldo final de $CODIGO: $saldo_final"
[ "$saldo_final" = "0" ] && ok "saldo zerado, nunca negativo" || falha "saldo final $saldo_final"

movimentos="$(psql_estoque "SELECT count(*) FROM movimentos_estoque m
                              JOIN produtos p ON p.id = m.produto_id
                             WHERE p.codigo = '$CODIGO'")"
nota_de "movimentos gravados: $movimentos (uma unica baixa)"

titulo "Conclusao"
nota_de "Sem o FOR UPDATE em ordem estavel, as $CONCORRENTES transacoes leriam saldo 1 ao mesmo"
nota_de "tempo e todas debitariam -- o classico lost update. O lock serializa;"
nota_de "o CHECK do banco existe como segunda linha de defesa."
