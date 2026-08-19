#!/usr/bin/env bash
set -u
cd "$(dirname "$0")/.."
source scripts/comum.sh

exigir_servicos

titulo "Preparacao"
saldo_inicial="$(saldo_de PRD-003)"
nota_de "saldo de PRD-003: $saldo_inicial"
id="$(nota_com_item PRD-003 3)"
nota_de "nota criada com 3 unidades de PRD-003"

titulo "1. Derrubando o servico de Estoque"
passo "$MOTOR stop $CONTAINER_ESTOQUE"
"$MOTOR" stop "$CONTAINER_ESTOQUE" > /dev/null
sleep 1

if [ "$(curl -sS -o /dev/null -w '%{http_code}' "$EST/health" 2>/dev/null)" = "200" ]; then
  falha "o Estoque ainda responde"
else
  ok "Estoque fora do ar"
fi

titulo "2. O Faturamento sobreviveu?"
saude="$(curl -sS -w '\n%{http_code}' "$FAT/health")"
if [ "$(tail -1 <<<"$saude")" = "200" ]; then
  ok "Faturamento responde 200 mesmo com a dependencia caida"
  nota_de "$(head -1 <<<"$saude")"
else
  falha "o Faturamento caiu junto"
fi

passo "listar notas (nao depende do Estoque)"
codigo="$(curl -sS -o /dev/null -w '%{http_code}' "$FAT/api/notas?tamanho=1")"
[ "$codigo" = "200" ] && ok "listagem responde 200" || falha "listagem respondeu $codigo"

titulo "3. Tentando imprimir com o Estoque fora do ar"
resposta="$(curl -sS -X POST "$FAT/api/notas/$id/impressao" -w '\n%{http_code}')"
status="$(tail -1 <<<"$resposta")"
corpo="$(head -1 <<<"$resposta")"

if [ "$status" = "503" ]; then
  ok "503 Service Unavailable -- o Polly esgotou os retries ou o disjuntor abriu"
else
  falha "esperava 503, veio $status"
fi
nota_de "$corpo"

passo "a nota sofreu algum efeito colateral?"
estado="$(campo "$(curl -sS "$FAT/api/notas/$id")" status)"
[ "$estado" = "Aberta" ] && ok "nota segue Aberta -- nada foi perdido" || falha "nota ficou $estado"

titulo "4. Religando o Estoque"
passo "$MOTOR start $CONTAINER_ESTOQUE"
"$MOTOR" start "$CONTAINER_ESTOQUE" > /dev/null
esperar_estoque && ok "Estoque de volta" || falha "Estoque nao voltou a tempo"

passo "aguardando o disjuntor fechar (BreakDuration = 10s)"
sleep 11

titulo "5. Imprimindo a MESMA nota"
resposta="$(curl -sS -X POST "$FAT/api/notas/$id/impressao" -w '\n%{http_code}')"
status="$(tail -1 <<<"$resposta")"

if [ "$status" = "200" ]; then
  ok "impressao concluida"
  nota_de "status da nota: $(campo "$(head -1 <<<"$resposta")" status)"
else
  falha "esperava 200, veio $status"
  nota_de "$(head -1 <<<"$resposta")"
fi

saldo_final="$(saldo_de PRD-003)"
nota_de "saldo de PRD-003: $saldo_inicial -> $saldo_final"
[ "$saldo_final" = "$((saldo_inicial - 3))" ] \
  && ok "debito aplicado exatamente uma vez" \
  || falha "saldo inesperado"

titulo "Conclusao"
nota_de "O sistema se recuperou sozinho. A falha nao corrompeu estado nem exigiu"
nota_de "intervencao: bastou o Estoque voltar para a operacao concluir."
