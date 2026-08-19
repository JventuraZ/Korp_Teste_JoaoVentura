package repositorio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/joaoventura/korp-estoque/internal/dominio"
)

const endpointBaixas = "POST /api/estoque/baixas"

// errChaveEmCorrida e interno: sinaliza que outra transacao gravou a mesma chave
// de idempotencia primeiro. Nao vaza para o transporte -- e tratado devolvendo a
// resposta que a transacao vencedora gravou.
var errChaveEmCorrida = errors.New("chave gravada por transacao concorrente")

// produtoTravado e o estado de um produto ja sob lock na transacao corrente.
type produtoTravado struct {
	id    uuid.UUID
	saldo int
}

// agregarItens soma as quantidades por codigo e devolve a lista ORDENADA.
//
// A ordenacao aqui nao e cosmetica: e ela que garante que duas requisicoes com
// os mesmos produtos em ordem inversa travem as linhas na mesma sequencia. Sem
// isso, duas notas concorrentes entram em deadlock no Postgres.
func agregarItens(itens []dominio.ItemBaixa) []dominio.ItemBaixa {
	somados := make(map[string]int, len(itens))
	for _, item := range itens {
		somados[item.Codigo] += item.Quantidade
	}

	agregados := make([]dominio.ItemBaixa, 0, len(somados))
	for codigo, quantidade := range somados {
		agregados = append(agregados, dominio.ItemBaixa{Codigo: codigo, Quantidade: quantidade})
	}
	sort.Slice(agregados, func(i, j int) bool { return agregados[i].Codigo < agregados[j].Codigo })
	return agregados
}

// hashCanonico identifica o conteudo da requisicao independentemente de
// formatacao ou da ordem dos itens: reenviar o mesmo pedido com o JSON
// indentado de outro jeito e a mesma operacao, nao um conflito.
func hashCanonico(req dominio.RequisicaoBaixa) string {
	canonico := struct {
		Referencia string              `json:"referencia"`
		Itens      []dominio.ItemBaixa `json:"itens"`
	}{req.Referencia, agregarItens(req.Itens)}

	dados, _ := json.Marshal(canonico)
	soma := sha256.Sum256(dados)
	return hex.EncodeToString(soma[:])
}

// baixaGravada devolve a resposta ja registrada para a chave, se houver.
//
// (nil, nil) significa "chave inedita". Hash diferente para a mesma chave e
// conflito: devolver a resposta antiga seria mentir sobre uma operacao que
// nunca foi executada.
func (r *Repositorio) baixaGravada(ctx context.Context, chave, hash string) (*dominio.ResultadoBaixa, error) {
	var hashGravado string
	var corpo []byte

	err := r.pool.QueryRow(ctx,
		`SELECT hash_requisicao, corpo_resposta FROM idempotencia WHERE chave = $1`,
		chave).Scan(&hashGravado, &corpo)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consultar idempotencia: %w", err)
	}
	if hashGravado != hash {
		return nil, dominio.ErrChaveIdemConflito
	}

	var resultado dominio.ResultadoBaixa
	if err := json.Unmarshal(corpo, &resultado); err != nil {
		return nil, fmt.Errorf("decodificar resposta gravada: %w", err)
	}
	return &resultado, nil
}

// AplicarBaixa debita saldo de varios produtos atomicamente.
//
// E o endpoint mais importante do servico e concentra as cinco garantias do
// contrato: atomicidade, validacao antes da escrita, idempotencia, ordem
// estavel de lock e o CHECK do banco como rede de seguranca.
//
// COMO CHEGA AQUI:
//
//	Angular  ->  POST /api/notas/{id}/impressao   (Faturamento, C#)
//	         ->  ServicoImpressao.ImprimirAsync   passo 1 da saga
//	         ->  POST /api/estoque/baixas         (Estoque, Go)
//	         ->  transporte.aplicarBaixa          le o header Idempotency-Key
//	         ->  AQUI
//
// ROTEIRO DESTA FUNCAO:
//
//	A. calcula a identidade do pedido (hash do corpo canonico);
//	B. se a chave ja foi usada, devolve a resposta gravada e NAO abre transacao;
//	C. senao, executa a baixa dentro de uma transacao (ver aplicarBaixaTx);
//	D. se perdeu uma corrida pela mesma chave, devolve a resposta da vencedora.
func (r *Repositorio) AplicarBaixa(ctx context.Context, chave string, req dominio.RequisicaoBaixa) (*dominio.ResultadoBaixa, error) {
	hash := hashCanonico(req)

	if gravada, err := r.baixaGravada(ctx, chave, hash); err != nil || gravada != nil {
		return gravada, err
	}

	var resultado *dominio.ResultadoBaixa
	err := r.emTransacao(ctx, func(tx pgx.Tx) error {
		var err error
		resultado, err = aplicarBaixaTx(ctx, tx, chave, hash, req)
		return err
	})

	if errors.Is(err, errChaveEmCorrida) {
		gravada, errLeitura := r.baixaGravada(ctx, chave, hash)
		if errLeitura != nil {
			return nil, errLeitura
		}
		if gravada == nil {
			return nil, fmt.Errorf("chave %q em corrida mas sem resposta gravada", chave)
		}
		return gravada, nil
	}
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

// aplicarBaixaTx e o corpo da transacao. TUDO aqui dentro acontece entre um
// BEGIN e um COMMIT: se qualquer passo devolver erro, o Postgres desfaz os
// anteriores e nada aconteceu.
//
// A ordem dos cinco passos nao e arbitraria -- e ela que produz as garantias:
//
//  1. agrega e ORDENA os itens
//  2. trava as linhas dos produtos            <- ponto de serializacao
//  3. valida TODOS os saldos                  <- antes de qualquer escrita
//  4. debita e grava a trilha de auditoria
//  5. grava a resposta na tabela de idempotencia
func aplicarBaixaTx(ctx context.Context, tx pgx.Tx, chave, hash string, req dominio.RequisicaoBaixa) (*dominio.ResultadoBaixa, error) {
	itens := agregarItens(req.Itens)

	codigos := make([]string, 0, len(itens))
	for _, item := range itens {
		codigos = append(codigos, item.Codigo)
	}

	travados, err := travarProdutos(ctx, tx, codigos)
	if err != nil {
		return nil, err
	}

	var faltantes []dominio.ItemInsuficiente
	for _, item := range itens {
		if travados[item.Codigo].saldo < item.Quantidade {
			faltantes = append(faltantes, dominio.ItemInsuficiente{
				Codigo:               item.Codigo,
				QuantidadeSolicitada: item.Quantidade,
				SaldoDisponivel:      travados[item.Codigo].saldo,
			})
		}
	}
	if len(faltantes) > 0 {
		return nil, &dominio.ErroSaldoInsuficiente{Itens: faltantes}
	}

	movimentados := make([]dominio.ItemMovimentado, 0, len(itens))
	for _, item := range itens {
		produto := travados[item.Codigo]
		posterior := produto.saldo - item.Quantidade

		if _, err := tx.Exec(ctx,
			`UPDATE produtos SET saldo = $2, versao = versao + 1, atualizado_em = now()
			  WHERE id = $1`, produto.id, posterior); err != nil {
			if ehViolacao(err, violacaoCheck) {
				return nil, &dominio.ErroSaldoInsuficiente{Itens: []dominio.ItemInsuficiente{{
					Codigo:               item.Codigo,
					QuantidadeSolicitada: item.Quantidade,
					SaldoDisponivel:      produto.saldo,
				}}}
			}
			return nil, fmt.Errorf("debitar saldo de %q: %w", item.Codigo, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO movimentos_estoque
			   (produto_id, tipo, quantidade, saldo_anterior, saldo_posterior, referencia, chave_idem)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			produto.id, dominio.MovimentoBaixa, item.Quantidade,
			produto.saldo, posterior, req.Referencia, chave); err != nil {
			return nil, fmt.Errorf("gravar movimento de baixa: %w", err)
		}

		movimentados = append(movimentados, dominio.ItemMovimentado{
			Codigo:         item.Codigo,
			Quantidade:     item.Quantidade,
			SaldoAnterior:  produto.saldo,
			SaldoPosterior: posterior,
		})
	}

	resultado := &dominio.ResultadoBaixa{
		ChaveIdempotencia: chave,
		Referencia:        req.Referencia,
		ProcessadoEm:      agora(),
		Itens:             movimentados,
	}

	corpo, err := json.Marshal(resultado)
	if err != nil {
		return nil, fmt.Errorf("serializar resposta: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO idempotencia (chave, endpoint, hash_requisicao, status_http, corpo_resposta)
		 VALUES ($1, $2, $3, $4, $5)`,
		chave, endpointBaixas, hash, http.StatusOK, corpo); err != nil {
		if ehViolacao(err, violacaoUnique) {
			return nil, errChaveEmCorrida
		}
		return nil, fmt.Errorf("gravar idempotencia: %w", err)
	}

	return resultado, nil
}

// travarProdutos bloqueia as linhas dos codigos informados em ordem estavel.
// Codigo inexistente ou inativo aborta a operacao inteira -- baixa e tudo ou nada.
func travarProdutos(ctx context.Context, tx pgx.Tx, codigos []string) (map[string]produtoTravado, error) {
	linhas, err := tx.Query(ctx,
		`SELECT id, codigo, saldo FROM produtos
		  WHERE codigo = ANY($1) AND ativo
		  ORDER BY codigo
		    FOR UPDATE`, codigos)
	if err != nil {
		return nil, fmt.Errorf("travar produtos: %w", err)
	}
	defer linhas.Close()

	travados := make(map[string]produtoTravado, len(codigos))
	for linhas.Next() {
		var id uuid.UUID
		var codigo string
		var saldo int
		if err := linhas.Scan(&id, &codigo, &saldo); err != nil {
			return nil, fmt.Errorf("ler produto travado: %w", err)
		}
		travados[codigo] = produtoTravado{id: id, saldo: saldo}
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("travar produtos: %w", err)
	}

	for _, codigo := range codigos {
		if _, ok := travados[codigo]; !ok {
			return nil, fmt.Errorf("%w: %s", dominio.ErrProdutoNaoEncontrado, codigo)
		}
	}
	return travados, nil
}

// Estornar desfaz uma baixa ja aplicada -- a compensacao da saga.
//
// A baixa original NAO e apagada: o estorno grava movimentos novos do tipo
// ESTORNO, preservando a trilha de auditoria.
func (r *Repositorio) Estornar(ctx context.Context, chave string) (*dominio.ResultadoEstorno, error) {
	var resultado *dominio.ResultadoEstorno

	err := r.emTransacao(ctx, func(tx pgx.Tx) error {
		gravado, err := estornoGravado(ctx, tx, chave)
		if err != nil {
			return err
		}
		if gravado != nil {
			resultado = gravado
			return nil
		}

		resultado, err = estornarTx(ctx, tx, chave)
		return err
	})

	if errors.Is(err, errChaveEmCorrida) {
		return r.estornoAplicado(ctx, chave)
	}
	if err != nil {
		return nil, err
	}
	return resultado, nil
}

func estornarTx(ctx context.Context, tx pgx.Tx, chave string) (*dominio.ResultadoEstorno, error) {
	linhas, err := tx.Query(ctx,
		`SELECT p.codigo, m.produto_id, m.quantidade
		   FROM movimentos_estoque m
		   JOIN produtos p ON p.id = m.produto_id
		  WHERE m.chave_idem = $1 AND m.tipo = $2
		  ORDER BY p.codigo`, chave, dominio.MovimentoBaixa)
	if err != nil {
		return nil, fmt.Errorf("buscar baixa original: %w", err)
	}

	type creditar struct {
		codigo     string
		produtoID  uuid.UUID
		quantidade int
	}
	var pendentes []creditar
	for linhas.Next() {
		var c creditar
		if err := linhas.Scan(&c.codigo, &c.produtoID, &c.quantidade); err != nil {
			linhas.Close()
			return nil, fmt.Errorf("ler baixa original: %w", err)
		}
		pendentes = append(pendentes, c)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("buscar baixa original: %w", err)
	}
	if len(pendentes) == 0 {
		return nil, dominio.ErrBaixaNaoEncontrada
	}

	movimentados := make([]dominio.ItemMovimentado, 0, len(pendentes))
	for _, item := range pendentes {
		var anterior, posterior int
		if err := tx.QueryRow(ctx,
			`UPDATE produtos
			    SET saldo = saldo + $2, versao = versao + 1, atualizado_em = now()
			  WHERE id = $1
			  RETURNING saldo - $2, saldo`,
			item.produtoID, item.quantidade).Scan(&anterior, &posterior); err != nil {
			return nil, fmt.Errorf("creditar saldo de %q: %w", item.codigo, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO movimentos_estoque
			   (produto_id, tipo, quantidade, saldo_anterior, saldo_posterior, referencia, chave_idem)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			item.produtoID, dominio.MovimentoEstorno, item.quantidade,
			anterior, posterior, "estorno da saga", chave); err != nil {
			if ehViolacao(err, violacaoUnique) {
				return nil, errChaveEmCorrida
			}
			return nil, fmt.Errorf("gravar movimento de estorno: %w", err)
		}

		movimentados = append(movimentados, dominio.ItemMovimentado{
			Codigo:         item.codigo,
			Quantidade:     item.quantidade,
			SaldoAnterior:  anterior,
			SaldoPosterior: posterior,
		})
	}

	return &dominio.ResultadoEstorno{
		ChaveIdempotencia: chave,
		EstornadoEm:       agora(),
		Itens:             movimentados,
	}, nil
}

// estornoGravado reconstroi o resultado a partir dos movimentos ESTORNO ja
// existentes. A trilha de auditoria e a propria fonte da idempotencia aqui --
// nao precisa de tabela auxiliar.
func estornoGravado(ctx context.Context, tx pgx.Tx, chave string) (*dominio.ResultadoEstorno, error) {
	linhas, err := tx.Query(ctx,
		`SELECT p.codigo, m.quantidade, m.saldo_anterior, m.saldo_posterior, m.criado_em
		   FROM movimentos_estoque m
		   JOIN produtos p ON p.id = m.produto_id
		  WHERE m.chave_idem = $1 AND m.tipo = $2
		  ORDER BY p.codigo`, chave, dominio.MovimentoEstorno)
	if err != nil {
		return nil, fmt.Errorf("consultar estorno: %w", err)
	}
	defer linhas.Close()

	resultado := &dominio.ResultadoEstorno{ChaveIdempotencia: chave}
	for linhas.Next() {
		var item dominio.ItemMovimentado
		var criadoEm time.Time
		if err := linhas.Scan(&item.Codigo, &item.Quantidade,
			&item.SaldoAnterior, &item.SaldoPosterior, &criadoEm); err != nil {
			return nil, fmt.Errorf("ler estorno: %w", err)
		}
		if resultado.EstornadoEm.IsZero() {
			resultado.EstornadoEm = criadoEm
		}
		resultado.Itens = append(resultado.Itens, item)
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("consultar estorno: %w", err)
	}
	if len(resultado.Itens) == 0 {
		return nil, nil
	}
	return resultado, nil
}

// estornoAplicado le o estorno ja commitado fora de transacao (caminho de corrida).
func (r *Repositorio) estornoAplicado(ctx context.Context, chave string) (*dominio.ResultadoEstorno, error) {
	var resultado *dominio.ResultadoEstorno
	err := r.emTransacao(ctx, func(tx pgx.Tx) error {
		var err error
		resultado, err = estornoGravado(ctx, tx, chave)
		return err
	})
	if err != nil {
		return nil, err
	}
	if resultado == nil {
		return nil, dominio.ErrBaixaNaoEncontrada
	}
	return resultado, nil
}
