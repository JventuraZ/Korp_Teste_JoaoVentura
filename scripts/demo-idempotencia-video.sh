#!/usr/bin/env bash
#
# Versao do demo-idempotencia.sh pensada para GRAVACAO.
#
# Duas diferencas em relacao ao script original:
#
#   1. pausa entre os passos, esperando Enter, para o narrador falar sem
#      correr atras da saida que ja passou;
#   2. mostra a tabela `idempotencia` -- o MECANISMO. O script original prova
#      o efeito (o saldo nao mudou); aqui da para ver de onde a segunda
#      resposta saiu.
#
# Uso:  bash scripts/demo-idempotencia-video.sh
#       PAUSAR=0 bash scripts/demo-idempotencia-video.sh   # sem pausas, para ensaiar

set -u
cd "$(dirname "$0")/.."
source scripts/comum.sh

exigir_servicos

PAUSAR="${PAUSAR:-1}"

pausa() {
  [ "$PAUSAR" = "1" ] || return 0
  [ -t 0 ] || return 0
  printf '\n\033[2m   ── Enter para continuar ──\033[0m'
  read -r
  printf '\n'
}

# Query formatada com cabecalho, para a tabela ficar legivel na tela.
psql_tabela() { "$MOTOR" exec "$CONTAINER_POSTGRES" psql -U korp -d db_estoque -c "$1"; }

CHAVE="$(uuid)"
CORPO='{"referencia":"NF-VIDEO","itens":[{"codigo":"PRD-001","quantidade":2}]}'

clear 2>/dev/null || true

titulo "O cenario"
nota_de "A resposta se perde no caminho de volta e o cliente repete a chamada."
nota_de "Sem idempotencia, o estoque seria debitado duas vezes."
antes="$(saldo_de PRD-001)"
nota_de ""
nota_de "saldo de PRD-001 ............ $antes"
nota_de "Idempotency-Key ............. $CHAVE"
pausa

titulo "1. Primeira chamada"
passo "POST /api/estoque/baixas  (Idempotency-Key: ${CHAVE:0:8}...)"
primeira="$(curl -sS -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE" -d "$CORPO")"
nota_de "$primeira"
depois_primeira="$(saldo_de PRD-001)"
ok "saldo: $antes -> $depois_primeira"
pausa

titulo "2. Mesma chave, mesmo corpo -- o retry"
passo "exatamente a mesma requisicao, de novo"
segunda="$(curl -sS -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE" -d "$CORPO")"
depois_segunda="$(saldo_de PRD-001)"

if [ "$primeira" = "$segunda" ]; then
  ok "resposta byte a byte identica a primeira"
else
  falha "a resposta mudou entre as chamadas"
  nota_de "$segunda"
fi

if [ "$depois_segunda" = "$depois_primeira" ]; then
  ok "saldo intacto em $depois_segunda -- a repeticao NAO debitou de novo"
else
  falha "o saldo mudou para $depois_segunda"
fi
pausa

titulo "3. De onde veio a segunda resposta"
nota_de "Nao foi reexecutada: foi LIDA da tabela de idempotencia."
psql_tabela "SELECT chave, endpoint, left(hash_requisicao, 12) AS hash_corpo,
                    status_http
               FROM idempotencia WHERE chave = '$CHAVE'"
nota_de "O corpo da resposta original, gravado em JSONB na mesma transacao da baixa:"
psql_tabela "SELECT jsonb_pretty(corpo_resposta) AS corpo_resposta_gravado
               FROM idempotencia WHERE chave = '$CHAVE'"
pausa

titulo "4. A prova na trilha de auditoria"
nota_de "Duas requisicoes chegaram. Quantos movimentos de estoque existem?"
psql_tabela "SELECT p.codigo, m.tipo, m.quantidade,
                    m.saldo_anterior || ' -> ' || m.saldo_posterior AS saldo
               FROM movimentos_estoque m JOIN produtos p ON p.id = m.produto_id
              WHERE m.chave_idem = '$CHAVE'"
movimentos="$(psql_estoque "SELECT count(*) FROM movimentos_estoque WHERE chave_idem = '$CHAVE'")"
[ "$movimentos" = "1" ] \
  && ok "duas requisicoes, UM movimento" \
  || falha "esperava 1 movimento, encontrei $movimentos"
pausa

titulo "5. Mesma chave, corpo DIFERENTE"
nota_de "Devolver a resposta antiga aqui seria mentir sobre uma operacao"
nota_de "que nunca foi executada. O contrato manda recusar."
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
pausa

titulo "6. A mesma identidade, escrita de outro jeito"
nota_de "A identidade e o SHA-256 do corpo CANONICO -- itens agregados e"
nota_de "ordenados. O mesmo pedido com outra formatacao continua sendo retry."
CHAVE2="$(uuid)"
curl -sS -X POST "$EST/api/estoque/baixas" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $CHAVE2" \
  -d '{"referencia":"NF-CANON","itens":[{"codigo":"PRD-001","quantidade":1},{"codigo":"PRD-001","quantidade":1}]}' > /dev/null
saldo_canon="$(saldo_de PRD-001)"
reenvio="$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$EST/api/estoque/baixas" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: $CHAVE2" \
  -d '{  "itens" : [ { "quantidade" : 2, "codigo" : "PRD-001" } ], "referencia" : "NF-CANON" }')"
[ "$reenvio" = "200" ] && [ "$(saldo_de PRD-001)" = "$saldo_canon" ] \
  && ok "duas linhas de 1 un. == uma linha de 2 un., campos fora de ordem: 200, sem debitar" \
  || falha "esperava 200 sem debito, veio $reenvio"
pausa

titulo "7. Limpeza -- e o estorno tambem e idempotente"
curl -sS -X POST "$EST/api/estoque/baixas/$CHAVE/estorno" > /dev/null
curl -sS -X POST "$EST/api/estoque/baixas/$CHAVE/estorno" > /dev/null
curl -sS -X POST "$EST/api/estoque/baixas/$CHAVE2/estorno" > /dev/null
final="$(saldo_de PRD-001)"
estornos="$(psql_estoque "SELECT count(*) FROM movimentos_estoque WHERE chave_idem = '$CHAVE' AND tipo = 'ESTORNO'")"

nota_de "estorno chamado 2x para a mesma chave -> $estornos movimento(s) de ESTORNO"
[ "$final" = "$antes" ] \
  && ok "saldo de volta ao original: $final" \
  || falha "saldo terminou em $final, esperava $antes"
nota_de "A baixa original CONTINUA na trilha: o estorno compensa, nao apaga."
