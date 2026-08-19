package dominio

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Limites identicos aos CHECK da migration 00001. A validacao aqui existe para
// devolver 400 com mensagem util; o banco continua sendo a garantia final.
const (
	tamanhoMaxCodigo    = 50
	tamanhoMaxDescricao = 200
)

// RequisicaoCriarProduto e o corpo de POST /api/produtos.
type RequisicaoCriarProduto struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Saldo     int    `json:"saldo"`
}

func (r RequisicaoCriarProduto) Validar() error {
	if err := validarTexto("codigo", r.Codigo, tamanhoMaxCodigo); err != nil {
		return err
	}
	if err := validarTexto("descricao", r.Descricao, tamanhoMaxDescricao); err != nil {
		return err
	}
	if r.Saldo < 0 {
		return fmt.Errorf("%w: saldo nao pode ser negativo", ErrRequisicaoInvalida)
	}
	return nil
}

// RequisicaoAtualizarProduto e o corpo de PUT /api/produtos/{codigo}.
//
// Saldo e ponteiro de proposito: ausente significa "nao mexa no saldo", que e
// diferente de "zere o saldo". Sem isso, editar so a descricao zeraria o estoque.
type RequisicaoAtualizarProduto struct {
	Descricao string `json:"descricao"`
	Saldo     *int   `json:"saldo"`
}

func (r RequisicaoAtualizarProduto) Validar() error {
	if err := validarTexto("descricao", r.Descricao, tamanhoMaxDescricao); err != nil {
		return err
	}
	if r.Saldo != nil && *r.Saldo < 0 {
		return fmt.Errorf("%w: saldo nao pode ser negativo", ErrRequisicaoInvalida)
	}
	return nil
}

// PaginaProdutos e a resposta paginada de GET /api/produtos.
type PaginaProdutos struct {
	Itens   []Produto `json:"itens"`
	Pagina  int       `json:"pagina"`
	Tamanho int       `json:"tamanho"`
	Total   int       `json:"total"`
}

// validarTexto usa contagem de runas, nao de bytes: "Parafuso ação" tem 13
// caracteres, nao 15. O CHECK do Postgres tambem conta caracteres.
func validarTexto(campo, valor string, maximo int) error {
	limpo := strings.TrimSpace(valor)
	if limpo == "" {
		return fmt.Errorf("%w: %s e obrigatorio", ErrRequisicaoInvalida, campo)
	}
	if utf8.RuneCountInString(limpo) > maximo {
		return fmt.Errorf("%w: %s excede %d caracteres", ErrRequisicaoInvalida, campo, maximo)
	}
	return nil
}
