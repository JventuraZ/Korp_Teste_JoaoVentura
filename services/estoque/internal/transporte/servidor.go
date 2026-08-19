package transporte

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/joaoventura/korp-estoque/internal/dominio"
)

// Repositorio e o contrato que o transporte precisa do armazenamento.
//
// Interface declarada aqui, no consumidor, e nao no pacote que a implementa:
// permite testar os handlers com um dublê sem subir Postgres.
type Repositorio interface {
	Ping(ctx context.Context) error
	Listar(ctx context.Context, busca string, pagina, tamanho int) ([]dominio.Produto, int, error)
	BuscarPorCodigo(ctx context.Context, codigo string) (*dominio.Produto, error)
	Criar(ctx context.Context, codigo, descricao string, saldo int) (*dominio.Produto, error)
	Atualizar(ctx context.Context, codigo, descricao string, novoSaldo *int) (*dominio.Produto, error)
	AplicarBaixa(ctx context.Context, chave string, req dominio.RequisicaoBaixa) (*dominio.ResultadoBaixa, error)
	Estornar(ctx context.Context, chave string) (*dominio.ResultadoEstorno, error)
	PreverRupturas(ctx context.Context, dias int) (*dominio.PainelPrevisao, error)
	Anomalias(ctx context.Context, dias int) (*dominio.PainelAnomalias, error)
}

type servidor struct {
	repo Repositorio
}

// NovoRoteador monta as sete rotas do contrato.
func NovoRoteador(repo Repositorio) http.Handler {
	s := &servidor{repo: repo}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.saude)

	r.Route("/api/produtos", func(r chi.Router) {
		r.Get("/", s.listarProdutos)
		r.Post("/", s.criarProduto)
		r.Get("/{codigo}", s.buscarProduto)
		r.Put("/{codigo}", s.atualizarProduto)
	})

	r.Route("/api/estoque/baixas", func(r chi.Router) {
		r.Post("/", s.aplicarBaixa)
		r.Post("/{chave}/estorno", s.estornar)
	})

	r.Get("/api/estoque/previsao", s.preverRupturas)
	r.Get("/api/estoque/anomalias", s.anomalias)

	return r
}

// saude e o healthcheck do compose e o sinal que o circuit breaker do
// Faturamento observa. 503 quando o banco nao responde.
func (s *servidor) saude(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.Ping(r.Context()); err != nil {
		responderJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degradado", "banco": "indisponivel",
		})
		return
	}
	responderJSON(w, http.StatusOK, map[string]string{"status": "ok", "banco": "ok"})
}

func (s *servidor) listarProdutos(w http.ResponseWriter, r *http.Request) {
	pagina := inteiroOuPadrao(r.URL.Query().Get("pagina"), 1)
	tamanho := inteiroOuPadrao(r.URL.Query().Get("tamanho"), 20)
	busca := r.URL.Query().Get("busca")

	produtos, total, err := s.repo.Listar(r.Context(), busca, pagina, tamanho)
	if err != nil {
		responderErro(w, r, err)
		return
	}

	responderJSON(w, http.StatusOK, dominio.PaginaProdutos{
		Itens: produtos, Pagina: pagina, Tamanho: tamanho, Total: total,
	})
}

func (s *servidor) buscarProduto(w http.ResponseWriter, r *http.Request) {
	produto, err := s.repo.BuscarPorCodigo(r.Context(), chi.URLParam(r, "codigo"))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, produto)
}

func (s *servidor) criarProduto(w http.ResponseWriter, r *http.Request) {
	var req dominio.RequisicaoCriarProduto
	if err := decodificar(r, &req); err != nil {
		responderErro(w, r, err)
		return
	}
	if err := req.Validar(); err != nil {
		responderErro(w, r, err)
		return
	}

	produto, err := s.repo.Criar(r.Context(), req.Codigo, req.Descricao, req.Saldo)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusCreated, produto)
}

func (s *servidor) atualizarProduto(w http.ResponseWriter, r *http.Request) {
	var req dominio.RequisicaoAtualizarProduto
	if err := decodificar(r, &req); err != nil {
		responderErro(w, r, err)
		return
	}
	if err := req.Validar(); err != nil {
		responderErro(w, r, err)
		return
	}

	produto, err := s.repo.Atualizar(r.Context(), chi.URLParam(r, "codigo"), req.Descricao, req.Saldo)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, produto)
}

func (s *servidor) aplicarBaixa(w http.ResponseWriter, r *http.Request) {
	chave := r.Header.Get("Idempotency-Key")
	if chave == "" {
		responderErro(w, r, fmt.Errorf(
			"%w: header Idempotency-Key e obrigatorio", dominio.ErrRequisicaoInvalida))
		return
	}

	var req dominio.RequisicaoBaixa
	if err := decodificar(r, &req); err != nil {
		responderErro(w, r, err)
		return
	}
	if err := req.Validar(); err != nil {
		responderErro(w, r, err)
		return
	}

	resultado, err := s.repo.AplicarBaixa(r.Context(), chave, req)
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, resultado)
}

func (s *servidor) estornar(w http.ResponseWriter, r *http.Request) {
	resultado, err := s.repo.Estornar(r.Context(), chi.URLParam(r, "chave"))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, resultado)
}

// preverRupturas responde quanto tempo o saldo de cada produto ainda dura no
// ritmo de consumo recente. O calculo vive em internal/analise.
func (s *servidor) preverRupturas(w http.ResponseWriter, r *http.Request) {
	painel, err := s.repo.PreverRupturas(r.Context(), inteiroOuPadrao(r.URL.Query().Get("dias"), 90))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, painel)
}

// anomalias lista baixas fora do padrao historico do proprio produto.
//
// Janela padrao de 90 dias, e nao 30: a deteccao exige um minimo de baixas por
// produto para nao apontar ruido, e um item que gira a cada cinco dias reune
// apenas seis amostras num mes -- abaixo do minimo, ele seria simplesmente
// ignorado, e a ausencia de alerta pareceria ausencia de problema.
func (s *servidor) anomalias(w http.ResponseWriter, r *http.Request) {
	painel, err := s.repo.Anomalias(r.Context(), inteiroOuPadrao(r.URL.Query().Get("dias"), 90))
	if err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, painel)
}

// inteiroOuPadrao converte parametros de query tolerando lixo: paginacao
// invalida cai no padrao em vez de derrubar a listagem com 400.
func inteiroOuPadrao(valor string, padrao int) int {
	convertido, err := strconv.Atoi(valor)
	if err != nil || convertido < 1 {
		return padrao
	}
	return convertido
}
