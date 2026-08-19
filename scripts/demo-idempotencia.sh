#!/usr/bin/env bash
set -u
cd "$(dirname "$0")/.."
source scripts/comum.sh

exigir_servicos

CHAVE="$(uuid)"
CORPO='{"referencia":"NF-DEMO","itens":[{"codigo":"PRD-001","quantidade":2}]}'

titulo "Preparacao"
antes="$(saldo_de PRD-001)"
nota_de "saldo de PRD-001: $antes"
nota_de "Idempotency-Key: $CHAVE"

titulo "1. Primeira chamada"
primeira="$(curl -sS -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE" -d "$CORPO")"
nota_de "$primeira"
depois_primeira="$(saldo_de PRD-001)"
ok "saldo: $antes -> $depois_primeira"

titulo "2. Mesma chave, mesmo corpo (o retry)"
segunda="$(curl -sS -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE" -d "$CORPO")"
depois_segunda="$(saldo_de PRD-001)"

if [ "$primeira" = "$segunda" ]; then
  ok "resposta byte a byte identica a original"
else
  falha "a resposta mudou entre as chamadas"
  nota_de "$segunda"
fi

if [ "$depois_segunda" = "$depois_primeira" ]; then
  ok "saldo intacto em $depois_segunda -- a repeticao NAO debitou de novo"
else
  falha "o saldo mudou para $depois_segunda"
fi

titulo "3. A prova na trilha de auditoria"
movimentos="$(psql_estoque "SELECT count(*) FROM movimentos_estoque WHERE chave_idem = '$CHAVE'")"
nota_de "movimentos gravados para esta chave: $movimentos"
[ "$movimentos" = "1" ] && ok "duas requisicoes, um unico movimento" || falha "esperava 1 movimento"

psql_estoque "SELECT p.codigo || '  ' || m.tipo || '  qtd=' || m.quantidade ||
                     '  ' || m.saldo_anterior || ' -> ' || m.saldo_posterior
                FROM movimentos_estoque m JOIN produtos p ON p.id = m.produto_id
               WHERE m.chave_idem = '$CHAVE'" | sed 's/^/   /'

titulo "4. Mesma chave, corpo DIFERENTE"
nota_de "devolver a resposta antiga seria mentir sobre uma operacao nunca executada"
conflito="$(curl -sS -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE" \
  -d '{"referencia":"NF-OUTRA","itens":[{"codigo":"PRD-001","quantidade":99}]}' \
  -w '\n%{http_code}')"

if [ "$(tail -1 <<<"$conflito")" = "409" ]; then
  ok "409 chave-idempotencia-conflito"
else
  falha "esperava 409, veio $(tail -1 <<<"$conflito")"
fi
nota_de "$(head -1 <<<"$conflito")"

titulo "5. Estorno tambem e idempotente"
curl -sS -X POST "$EST/api/estoque/baixas/$CHAVE/estorno" > /dev/null
apos_estorno="$(saldo_de PRD-001)"
curl -sS -X POST "$EST/api/estoque/baixas/$CHAVE/estorno" > /dev/null
apos_segundo="$(saldo_de PRD-001)"

nota_de "saldo apos 1o estorno: $apos_estorno | apos o 2o: $apos_segundo"
[ "$apos_estorno" = "$apos_segundo" ] && [ "$apos_estorno" = "$antes" ] \
  && ok "estornar duas vezes credita uma so; saldo de volta ao original" \
  || falha "o estorno repetido alterou o saldo"

estornos="$(psql_estoque "SELECT count(*) FROM movimentos_estoque WHERE chave_idem = '$CHAVE' AND tipo = 'ESTORNO'")"
nota_de "movimentos de estorno: $estornos (garantido pelo indice unico do banco)"

titulo "Conclusao"
nota_de "A baixa original continua na trilha: o estorno compensa, nao apaga."
