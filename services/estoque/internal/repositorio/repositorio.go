// Package repositorio concentra todo o acesso ao Postgres via pgx.
//
// SQL e escrito a mao de proposito: as garantias que sustentam este servico
// (FOR UPDATE com ordem estavel, CHECK de saldo, idempotencia transacional)
// dependem de controle explicito sobre a query e sobre os limites da transacao.
package repositorio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joaoventura/korp-estoque/internal/dominio"
)

// SQLSTATE do Postgres que precisamos distinguir.
const (
	violacaoUnique = "23505"
	violacaoCheck  = "23514"
)

type Repositorio struct {
	pool *pgxpool.Pool
}

func Novo(pool *pgxpool.Pool) *Repositorio { return &Repositorio{pool: pool} }

// emTransacao executa fn dentro de uma transacao, revertendo em qualquer erro.
// O Rollback adiado e no-op apos um Commit bem-sucedido.
func (r *Repositorio) emTransacao(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transacao: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ehViolacao informa se o erro do Postgres corresponde ao SQLSTATE dado.
func ehViolacao(err error, sqlstate string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == sqlstate
}

const colunasProduto = `id, codigo, descricao, saldo, versao, ativo, criado_em, atualizado_em`

func lerProduto(linha pgx.Row) (*dominio.Produto, error) {
	var p dominio.Produto
	err := linha.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Saldo, &p.Versao,
		&p.Ativo, &p.CriadoEm, &p.AtualizadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, dominio.ErrProdutoNaoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("ler produto: %w", err)
	}
	return &p, nil
}

func (r *Repositorio) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

// Listar devolve produtos ativos paginados. O filtro `busca` alimenta o
// autocomplete do frontend e usa os indices trigram criados na migration.
func (r *Repositorio) Listar(ctx context.Context, busca string, pagina, tamanho int) ([]dominio.Produto, int, error) {
	if pagina < 1 {
		pagina = 1
	}
	if tamanho < 1 || tamanho > 100 {
		tamanho = 20
	}
	deslocamento := (pagina - 1) * tamanho

	const filtro = `
		WHERE ativo
		  AND ($1 = '' OR codigo ILIKE '%' || $1 || '%' OR descricao ILIKE '%' || $1 || '%')`

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM produtos`+filtro, busca).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar produtos: %w", err)
	}

	linhas, err := r.pool.Query(ctx,
		`SELECT `+colunasProduto+` FROM produtos`+filtro+`
		 ORDER BY codigo LIMIT $2 OFFSET $3`, busca, tamanho, deslocamento)
	if err != nil {
		return nil, 0, fmt.Errorf("listar produtos: %w", err)
	}
	defer linhas.Close()

	produtos := make([]dominio.Produto, 0, tamanho)
	for linhas.Next() {
		var p dominio.Produto
		if err := linhas.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Saldo, &p.Versao,
			&p.Ativo, &p.CriadoEm, &p.AtualizadoEm); err != nil {
			return nil, 0, fmt.Errorf("ler linha de produto: %w", err)
		}
		produtos = append(produtos, p)
	}
	return produtos, total, linhas.Err()
}

func (r *Repositorio) BuscarPorCodigo(ctx context.Context, codigo string) (*dominio.Produto, error) {
	return lerProduto(r.pool.QueryRow(ctx,
		`SELECT `+colunasProduto+` FROM produtos WHERE codigo = $1 AND ativo`, codigo))
}

func (r *Repositorio) Criar(ctx context.Context, codigo, descricao string, saldo int) (*dominio.Produto, error) {
	p, err := lerProduto(r.pool.QueryRow(ctx,
		`INSERT INTO produtos (id, codigo, descricao, saldo)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+colunasProduto,
		uuid.New(), codigo, descricao, saldo))

	if err != nil && ehViolacao(err, violacaoUnique) {
		return nil, dominio.ErrCodigoDuplicado
	}
	return p, err
}

// Atualizar altera descricao e, opcionalmente, faz ajuste manual de saldo.
//
// Ajuste de cadastro e coisa diferente de baixa por nota fiscal: grava
// movimento do tipo AJUSTE e nao passa pelo caminho da saga.
func (r *Repositorio) Atualizar(ctx context.Context, codigo, descricao string, novoSaldo *int) (*dominio.Produto, error) {
	var atualizado *dominio.Produto

	err := r.emTransacao(ctx, func(tx pgx.Tx) error {
		anterior, err := lerProduto(tx.QueryRow(ctx,
			`SELECT `+colunasProduto+` FROM produtos WHERE codigo = $1 AND ativo FOR UPDATE`, codigo))
		if err != nil {
			return err
		}

		saldoFinal := anterior.Saldo
		if novoSaldo != nil {
			saldoFinal = *novoSaldo
		}

		atualizado, err = lerProduto(tx.QueryRow(ctx,
			`UPDATE produtos
			    SET descricao = $2, saldo = $3, versao = versao + 1, atualizado_em = now()
			  WHERE id = $1
			  RETURNING `+colunasProduto,
			anterior.ID, descricao, saldoFinal))
		if err != nil {
			if ehViolacao(err, violacaoCheck) {
				return fmt.Errorf("%w: saldo nao pode ser negativo", dominio.ErrRequisicaoInvalida)
			}
			return err
		}

		if novoSaldo != nil && *novoSaldo != anterior.Saldo {
			delta := *novoSaldo - anterior.Saldo
			if delta < 0 {
				delta = -delta
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO movimentos_estoque
				   (produto_id, tipo, quantidade, saldo_anterior, saldo_posterior, referencia)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				anterior.ID, dominio.MovimentoAjuste, delta,
				anterior.Saldo, *novoSaldo, "ajuste de cadastro"); err != nil {
				return fmt.Errorf("gravar movimento de ajuste: %w", err)
			}
		}
		return nil
	})

	return atualizado, err
}

// agora existe para manter o timestamp consistente dentro de uma mesma operacao.
func agora() time.Time { return time.Now().UTC() }
