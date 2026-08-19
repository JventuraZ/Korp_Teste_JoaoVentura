// Package dominio concentra as entidades e a taxonomia de erros do Estoque.
//
// Nao ha excecao nem fluxo de controle escondido: erro e valor de retorno.
// Toda a traducao de erro para HTTP acontece num unico ponto
// (transporte.MapearErro), o que mantem os handlers livres de logica de status.
package dominio

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Erros sentinela do servico. Comparados com errors.Is; os que carregam
// detalhe adicional sao recuperados com errors.As (ver ErroSaldoInsuficiente).
var (
	ErrProdutoNaoEncontrado = errors.New("produto nao encontrado")
	ErrCodigoDuplicado      = errors.New("codigo ja cadastrado")
	ErrSaldoInsuficiente    = errors.New("saldo insuficiente")
	ErrChaveIdemConflito    = errors.New("chave de idempotencia reutilizada com corpo diferente")
	ErrBaixaNaoEncontrada   = errors.New("baixa nao encontrada para a chave informada")
	ErrRequisicaoInvalida   = errors.New("requisicao invalida")
)

// Produto e o agregado do Estoque. O saldo so muda por AplicarBaixa,
// Estornar ou ajuste explicito de cadastro -- nunca por escrita direta.
type Produto struct {
	ID           uuid.UUID `json:"id"`
	Codigo       string    `json:"codigo"`
	Descricao    string    `json:"descricao"`
	Saldo        int       `json:"saldo"`
	Versao       int64     `json:"versao"`
	Ativo        bool      `json:"-"`
	CriadoEm     time.Time `json:"criadoEm"`
	AtualizadoEm time.Time `json:"atualizadoEm"`
}

// ItemBaixa e um item solicitado numa baixa de estoque.
type ItemBaixa struct {
	Codigo     string `json:"codigo"`
	Quantidade int    `json:"quantidade"`
}

// RequisicaoBaixa e o corpo de POST /api/estoque/baixas.
type RequisicaoBaixa struct {
	Referencia string      `json:"referencia"`
	Itens      []ItemBaixa `json:"itens"`
}

// Validar checa o corpo antes de qualquer acesso ao banco.
func (r RequisicaoBaixa) Validar() error {
	if len(r.Itens) == 0 {
		return fmt.Errorf("%w: informe ao menos um item", ErrRequisicaoInvalida)
	}
	for i, item := range r.Itens {
		if item.Codigo == "" {
			return fmt.Errorf("%w: item %d sem codigo", ErrRequisicaoInvalida, i)
		}
		if item.Quantidade <= 0 {
			return fmt.Errorf("%w: item %q com quantidade %d (deve ser > 0)",
				ErrRequisicaoInvalida, item.Codigo, item.Quantidade)
		}
	}
	return nil
}

// ItemInsuficiente detalha por que um item nao pode ser baixado.
type ItemInsuficiente struct {
	Codigo               string `json:"codigo"`
	QuantidadeSolicitada int    `json:"quantidadeSolicitada"`
	SaldoDisponivel      int    `json:"saldoDisponivel"`
}

// ErroSaldoInsuficiente carrega TODOS os itens problematicos, nao apenas o
// primeiro: o usuario corrige tudo numa passada em vez de descobrir um a um.
//
// Implementa Unwrap para que errors.Is(err, ErrSaldoInsuficiente) funcione,
// e e recuperado com errors.As quando o transporte precisa dos detalhes.
type ErroSaldoInsuficiente struct {
	Itens []ItemInsuficiente
}

func (e *ErroSaldoInsuficiente) Error() string {
	return fmt.Sprintf("%v: %d item(ns) sem saldo", ErrSaldoInsuficiente, len(e.Itens))
}

func (e *ErroSaldoInsuficiente) Unwrap() error { return ErrSaldoInsuficiente }

// ItemMovimentado e o efeito de uma baixa ou estorno sobre um produto.
type ItemMovimentado struct {
	Codigo         string `json:"codigo"`
	Quantidade     int    `json:"quantidade"`
	SaldoAnterior  int    `json:"saldoAnterior"`
	SaldoPosterior int    `json:"saldoPosterior"`
}

// ResultadoBaixa e a resposta de POST /api/estoque/baixas. E gravada na
// tabela de idempotencia e devolvida identica quando a chave se repete.
type ResultadoBaixa struct {
	ChaveIdempotencia string            `json:"chaveIdempotencia"`
	Referencia        string            `json:"referencia"`
	ProcessadoEm      time.Time         `json:"processadoEm"`
	Itens             []ItemMovimentado `json:"itens"`
}

// ResultadoEstorno e a resposta da compensacao da saga.
type ResultadoEstorno struct {
	ChaveIdempotencia string            `json:"chaveIdempotencia"`
	EstornadoEm       time.Time         `json:"estornadoEm"`
	Itens             []ItemMovimentado `json:"itens"`
}

// Tipos de movimento gravados na trilha de auditoria.
const (
	MovimentoBaixa   = "BAIXA"
	MovimentoEstorno = "ESTORNO"
	MovimentoAjuste  = "AJUSTE"
)
