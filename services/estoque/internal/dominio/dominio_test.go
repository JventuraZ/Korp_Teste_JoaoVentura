package dominio

import (
	"errors"
	"testing"
)

func TestValidarRequisicaoBaixa(t *testing.T) {
	casos := []struct {
		nome  string
		req   RequisicaoBaixa
		valida bool
	}{
		{"itens vazios", RequisicaoBaixa{Referencia: "NF-1"}, false},
		{"item sem codigo", RequisicaoBaixa{Itens: []ItemBaixa{{Quantidade: 1}}}, false},
		{"quantidade zero", RequisicaoBaixa{Itens: []ItemBaixa{{Codigo: "PRD-1", Quantidade: 0}}}, false},
		{"quantidade negativa", RequisicaoBaixa{Itens: []ItemBaixa{{Codigo: "PRD-1", Quantidade: -3}}}, false},
		{"valida", RequisicaoBaixa{Itens: []ItemBaixa{{Codigo: "PRD-1", Quantidade: 2}}}, true},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			err := caso.req.Validar()
			if caso.valida && err != nil {
				t.Fatalf("esperava requisicao valida, veio erro: %v", err)
			}
			if !caso.valida {
				if err == nil {
					t.Fatal("esperava erro de validacao")
				}
				// Todo erro de validacao precisa ser classificavel como
				// requisicao invalida, senao o transporte devolve 500 em vez de 400.
				if !errors.Is(err, ErrRequisicaoInvalida) {
					t.Fatalf("erro nao classificado como ErrRequisicaoInvalida: %v", err)
				}
			}
		})
	}
}

// ErroSaldoInsuficiente precisa satisfazer errors.Is (para o mapeamento de
// status) e errors.As (para o transporte recuperar a lista de faltantes).
// Se o Unwrap sumir, o 409 vira 500 sem nenhum teste quebrar em outro lugar.
func TestErroSaldoInsuficienteEhComparavelERecuperavel(t *testing.T) {
	original := &ErroSaldoInsuficiente{Itens: []ItemInsuficiente{
		{Codigo: "PRD-001", QuantidadeSolicitada: 5, SaldoDisponivel: 1},
	}}

	var embrulhado error = original
	if !errors.Is(embrulhado, ErrSaldoInsuficiente) {
		t.Fatal("errors.Is falhou: o mapeamento de status devolveria 500")
	}

	var recuperado *ErroSaldoInsuficiente
	if !errors.As(embrulhado, &recuperado) {
		t.Fatal("errors.As falhou: itensInsuficientes nao chegaria na resposta")
	}
	if len(recuperado.Itens) != 1 || recuperado.Itens[0].Codigo != "PRD-001" {
		t.Fatalf("detalhe perdido: %+v", recuperado.Itens)
	}
}

func TestValidarProduto(t *testing.T) {
	casos := []struct {
		nome   string
		req    RequisicaoCriarProduto
		valida bool
	}{
		{"codigo vazio", RequisicaoCriarProduto{Descricao: "d", Saldo: 1}, false},
		{"codigo so espacos", RequisicaoCriarProduto{Codigo: "   ", Descricao: "d"}, false},
		{"descricao vazia", RequisicaoCriarProduto{Codigo: "PRD-1"}, false},
		{"saldo negativo", RequisicaoCriarProduto{Codigo: "PRD-1", Descricao: "d", Saldo: -1}, false},
		{"saldo zero e valido", RequisicaoCriarProduto{Codigo: "PRD-1", Descricao: "d", Saldo: 0}, true},
		{"valida", RequisicaoCriarProduto{Codigo: "PRD-1", Descricao: "Parafuso", Saldo: 10}, true},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			err := caso.req.Validar()
			if caso.valida != (err == nil) {
				t.Fatalf("validade esperada %v, erro %v", caso.valida, err)
			}
		})
	}
}

// Saldo ausente no PUT tem de ser distinguivel de saldo zero: sem o ponteiro,
// editar apenas a descricao zeraria o estoque do produto.
func TestAtualizarDistingueSaldoAusenteDeZero(t *testing.T) {
	semSaldo := RequisicaoAtualizarProduto{Descricao: "Nova descricao"}
	if semSaldo.Saldo != nil {
		t.Fatal("saldo ausente deveria ser nil")
	}
	if err := semSaldo.Validar(); err != nil {
		t.Fatalf("requisicao sem saldo deveria ser valida: %v", err)
	}

	zero := 0
	comZero := RequisicaoAtualizarProduto{Descricao: "d", Saldo: &zero}
	if comZero.Saldo == nil || *comZero.Saldo != 0 {
		t.Fatal("saldo zero explicito deveria chegar como ponteiro para 0")
	}
}
