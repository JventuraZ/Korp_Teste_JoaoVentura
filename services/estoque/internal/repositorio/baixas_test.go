// Testes de integracao: rodam contra um Postgres real, porque as garantias que
// eles verificam (FOR UPDATE, CHECK de saldo, indice unico de estorno) sao do
// banco, nao do Go. Um dublê em memoria passaria sem provar nada.
//
// Sem TEST_DATABASE_URL os testes se auto-pulam, entao `go test ./...` continua
// verde numa maquina sem banco.
package repositorio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/joaoventura/korp-estoque/internal/dominio"
	migracoes "github.com/joaoventura/korp-estoque/migrations"
)

var migrarUmaVez sync.Once

func repoTeste(t *testing.T) *Repositorio {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL nao definida; teste de integracao pulado")
	}

	migrarUmaVez.Do(func() {
		config, err := pgxpool.ParseConfig(url)
		if err != nil {
			t.Fatalf("interpretar TEST_DATABASE_URL: %v", err)
		}
		banco := stdlib.OpenDB(*config.ConnConfig)
		defer banco.Close()

		goose.SetBaseFS(migracoes.Arquivos)
		goose.SetLogger(goose.NopLogger())
		if err := goose.SetDialect("postgres"); err != nil {
			t.Fatalf("dialeto goose: %v", err)
		}
		if err := goose.Up(banco, "."); err != nil {
			t.Fatalf("migrar banco de teste: %v", err)
		}
	})

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Cada teste comeca do zero: o seed da migration 00004 e removido junto.
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE movimentos_estoque, idempotencia RESTART IDENTITY; DELETE FROM produtos;`); err != nil {
		t.Fatalf("limpar banco: %v", err)
	}

	return Novo(pool)
}

func criarProduto(t *testing.T, repo *Repositorio, codigo string, saldo int) {
	t.Helper()
	if _, err := repo.Criar(context.Background(), codigo, "Produto "+codigo, saldo); err != nil {
		t.Fatalf("criar %s: %v", codigo, err)
	}
}

func saldoDe(t *testing.T, repo *Repositorio, codigo string) int {
	t.Helper()
	produto, err := repo.BuscarPorCodigo(context.Background(), codigo)
	if err != nil {
		t.Fatalf("buscar %s: %v", codigo, err)
	}
	return produto.Saldo
}

func contarMovimentos(t *testing.T, repo *Repositorio, tipo string) int {
	t.Helper()
	var total int
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM movimentos_estoque WHERE tipo = $1`, tipo).Scan(&total); err != nil {
		t.Fatalf("contar movimentos: %v", err)
	}
	return total
}

func baixaDe(itens ...dominio.ItemBaixa) dominio.RequisicaoBaixa {
	return dominio.RequisicaoBaixa{Referencia: "NF-000001", Itens: itens}
}

func TestAplicarBaixaDebitaTodosOsItens(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)
	criarProduto(t, repo, "PRD-002", 4)

	resultado, err := repo.AplicarBaixa(context.Background(), "chave-1", baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 2},
		dominio.ItemBaixa{Codigo: "PRD-002", Quantidade: 1},
	))
	if err != nil {
		t.Fatalf("baixa deveria ter sido aplicada: %v", err)
	}

	if got := saldoDe(t, repo, "PRD-001"); got != 8 {
		t.Errorf("PRD-001: saldo %d, esperado 8", got)
	}
	if got := saldoDe(t, repo, "PRD-002"); got != 3 {
		t.Errorf("PRD-002: saldo %d, esperado 3", got)
	}
	if len(resultado.Itens) != 2 {
		t.Fatalf("resposta com %d itens, esperado 2", len(resultado.Itens))
	}
	if contarMovimentos(t, repo, dominio.MovimentoBaixa) != 2 {
		t.Error("trilha de auditoria deveria ter um movimento por item")
	}
}

// A baixa e atomica: se um item falta, NENHUM produto e debitado.
func TestBaixaComItemFaltanteNaoDebitaNada(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)
	criarProduto(t, repo, "PRD-002", 1)

	_, err := repo.AplicarBaixa(context.Background(), "chave-1", baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 2},
		dominio.ItemBaixa{Codigo: "PRD-002", Quantidade: 5},
	))
	if !errors.Is(err, dominio.ErrSaldoInsuficiente) {
		t.Fatalf("esperava saldo insuficiente, veio: %v", err)
	}

	if got := saldoDe(t, repo, "PRD-001"); got != 10 {
		t.Errorf("PRD-001 foi debitado apesar da falha: saldo %d, esperado 10", got)
	}
	if contarMovimentos(t, repo, dominio.MovimentoBaixa) != 0 {
		t.Error("nenhum movimento deveria ter sido gravado")
	}
}

// O contrato promete listar TODOS os itens sem saldo de uma vez, para o usuario
// corrigir tudo numa passada em vez de descobrir um problema por tentativa.
func TestSaldoInsuficienteListaTodosOsFaltantes(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 1)
	criarProduto(t, repo, "PRD-002", 0)
	criarProduto(t, repo, "PRD-003", 50)

	_, err := repo.AplicarBaixa(context.Background(), "chave-1", baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 5},
		dominio.ItemBaixa{Codigo: "PRD-002", Quantidade: 3},
		dominio.ItemBaixa{Codigo: "PRD-003", Quantidade: 1},
	))

	var insuficiente *dominio.ErroSaldoInsuficiente
	if !errors.As(err, &insuficiente) {
		t.Fatalf("esperava ErroSaldoInsuficiente, veio: %v", err)
	}
	if len(insuficiente.Itens) != 2 {
		t.Fatalf("esperava 2 faltantes, veio %d: %+v", len(insuficiente.Itens), insuficiente.Itens)
	}
}

func TestProdutoInexistenteAbortaBaixa(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)

	_, err := repo.AplicarBaixa(context.Background(), "chave-1", baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 1},
		dominio.ItemBaixa{Codigo: "NAO-EXISTE", Quantidade: 1},
	))
	if !errors.Is(err, dominio.ErrProdutoNaoEncontrado) {
		t.Fatalf("esperava produto nao encontrado, veio: %v", err)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 10 {
		t.Errorf("saldo alterado apesar do aborto: %d", got)
	}
}

// O requisito de retry e o de idempotencia se cruzam exatamente aqui: sem esta
// garantia, um retry apos resposta perdida debitaria o estoque duas vezes.
func TestMesmaChaveNaoDebitaDuasVezes(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)

	req := baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 3})

	primeira, err := repo.AplicarBaixa(context.Background(), "chave-repetida", req)
	if err != nil {
		t.Fatalf("primeira baixa: %v", err)
	}
	segunda, err := repo.AplicarBaixa(context.Background(), "chave-repetida", req)
	if err != nil {
		t.Fatalf("repeticao deveria devolver a resposta original: %v", err)
	}

	if got := saldoDe(t, repo, "PRD-001"); got != 7 {
		t.Errorf("saldo %d, esperado 7 -- a repeticao debitou de novo", got)
	}
	if contarMovimentos(t, repo, dominio.MovimentoBaixa) != 1 {
		t.Error("a repeticao gravou um segundo movimento")
	}
	if !primeira.ProcessadoEm.Equal(segunda.ProcessadoEm) {
		t.Error("a repeticao devolveu um processadoEm diferente do original")
	}
	if primeira.Itens[0].SaldoPosterior != segunda.Itens[0].SaldoPosterior {
		t.Error("a repeticao devolveu saldos diferentes do original")
	}
}

// Devolver a resposta antiga para um corpo diferente seria mentir sobre uma
// operacao que nunca foi executada.
func TestMesmaChaveComCorpoDiferenteConflita(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)

	if _, err := repo.AplicarBaixa(context.Background(), "chave-1",
		baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 2})); err != nil {
		t.Fatalf("primeira baixa: %v", err)
	}

	_, err := repo.AplicarBaixa(context.Background(), "chave-1",
		baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 5}))
	if !errors.Is(err, dominio.ErrChaveIdemConflito) {
		t.Fatalf("esperava conflito de chave, veio: %v", err)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 8 {
		t.Errorf("saldo %d, esperado 8 -- o conflito nao pode debitar", got)
	}
}

// A ordem dos itens e a formatacao do JSON nao mudam a identidade da operacao:
// o mesmo pedido reenviado de outro jeito e retry, nao conflito.
func TestOrdemDosItensNaoAlteraAIdentidadeDaChave(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)
	criarProduto(t, repo, "PRD-002", 10)

	direta := baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 1},
		dominio.ItemBaixa{Codigo: "PRD-002", Quantidade: 2},
	)
	invertida := baixaDe(
		dominio.ItemBaixa{Codigo: "PRD-002", Quantidade: 2},
		dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 1},
	)

	if _, err := repo.AplicarBaixa(context.Background(), "chave-1", direta); err != nil {
		t.Fatalf("primeira baixa: %v", err)
	}
	if _, err := repo.AplicarBaixa(context.Background(), "chave-1", invertida); err != nil {
		t.Fatalf("mesma operacao em outra ordem deveria ser idempotente, veio: %v", err)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 9 {
		t.Errorf("saldo %d, esperado 9", got)
	}
}

func TestEstornoRestauraSaldoEEhIdempotente(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)

	if _, err := repo.AplicarBaixa(context.Background(), "chave-1",
		baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 4})); err != nil {
		t.Fatalf("baixa: %v", err)
	}

	primeiro, err := repo.Estornar(context.Background(), "chave-1")
	if err != nil {
		t.Fatalf("estorno: %v", err)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 10 {
		t.Fatalf("saldo %d apos estorno, esperado 10", got)
	}

	segundo, err := repo.Estornar(context.Background(), "chave-1")
	if err != nil {
		t.Fatalf("estorno repetido deveria ser idempotente: %v", err)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 10 {
		t.Errorf("saldo %d -- o estorno repetido creditou de novo", got)
	}
	if contarMovimentos(t, repo, dominio.MovimentoEstorno) != 1 {
		t.Error("o estorno repetido gravou um segundo movimento")
	}
	if primeiro.Itens[0].SaldoPosterior != segundo.Itens[0].SaldoPosterior {
		t.Error("o estorno repetido devolveu resultado diferente")
	}

	// A baixa original permanece na trilha: estorno compensa, nao apaga.
	if contarMovimentos(t, repo, dominio.MovimentoBaixa) != 1 {
		t.Error("a baixa original sumiu da trilha de auditoria")
	}
}

func TestEstornoDeChaveInexistente(t *testing.T) {
	repo := repoTeste(t)
	if _, err := repo.Estornar(context.Background(), "nunca-aplicada"); !errors.Is(err, dominio.ErrBaixaNaoEncontrada) {
		t.Fatalf("esperava baixa nao encontrada, veio: %v", err)
	}
}

// Requisito opcional (a): duas notas disputando a ultima unidade.
//
// Com FOR UPDATE em ordem estavel, as transacoes se serializam: exatamente uma
// vence e o saldo nunca fica negativo, por mais concorrencia que se jogue.
func TestBaixasConcorrentesNoMesmoProduto(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 1)

	const concorrentes = 8
	var espera sync.WaitGroup
	resultados := make(chan error, concorrentes)

	for i := 0; i < concorrentes; i++ {
		espera.Add(1)
		go func(n int) {
			defer espera.Done()
			_, err := repo.AplicarBaixa(context.Background(), fmt.Sprintf("chave-%d", n),
				baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 1}))
			resultados <- err
		}(i)
	}
	espera.Wait()
	close(resultados)

	var sucessos, semSaldo int
	for err := range resultados {
		switch {
		case err == nil:
			sucessos++
		case errors.Is(err, dominio.ErrSaldoInsuficiente):
			semSaldo++
		default:
			t.Errorf("erro inesperado na concorrencia: %v", err)
		}
	}

	if sucessos != 1 {
		t.Errorf("%d baixas venceram, esperado exatamente 1", sucessos)
	}
	if semSaldo != concorrentes-1 {
		t.Errorf("%d recusas por saldo, esperado %d", semSaldo, concorrentes-1)
	}
	if got := saldoDe(t, repo, "PRD-001"); got != 0 {
		t.Errorf("saldo final %d, esperado 0", got)
	}
}

// Mesma chave chegando em paralelo (o retry do Faturamento disparando duas
// vezes): todas as chamadas devolvem a MESMA resposta e o estoque e debitado
// uma unica vez.
func TestMesmaChaveEmParaleloDebitaUmaVez(t *testing.T) {
	repo := repoTeste(t)
	criarProduto(t, repo, "PRD-001", 10)

	const concorrentes = 6
	req := baixaDe(dominio.ItemBaixa{Codigo: "PRD-001", Quantidade: 2})

	var espera sync.WaitGroup
	respostas := make(chan *dominio.ResultadoBaixa, concorrentes)

	for i := 0; i < concorrentes; i++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			resultado, err := repo.AplicarBaixa(context.Background(), "chave-unica", req)
			if err != nil {
				t.Errorf("chamada concorrente falhou: %v", err)
				return
			}
			respostas <- resultado
		}()
	}
	espera.Wait()
	close(respostas)

	if got := saldoDe(t, repo, "PRD-001"); got != 8 {
		t.Errorf("saldo %d, esperado 8 -- houve debito duplicado", got)
	}
	if got := contarMovimentos(t, repo, dominio.MovimentoBaixa); got != 1 {
		t.Errorf("%d movimentos gravados, esperado 1", got)
	}

	for resposta := range respostas {
		if resposta.Itens[0].SaldoPosterior != 8 {
			t.Errorf("resposta divergente entre chamadas concorrentes: %+v", resposta.Itens[0])
		}
	}
}
