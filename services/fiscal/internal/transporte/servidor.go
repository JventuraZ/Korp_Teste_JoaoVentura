package transporte

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/joaoventura/korp-fiscal/internal/dados"
	"github.com/joaoventura/korp-fiscal/internal/dominio"
	"github.com/joaoventura/korp-fiscal/internal/regras"
	"github.com/joaoventura/korp-fiscal/internal/simulacao"
	"github.com/joaoventura/korp-fiscal/internal/validacao"
)

// servidor guarda o catalogo em memoria.
//
// Sem banco nesta fase: o escopo acordado e prototipo de interface. As escritas
// sao validadas e devolvidas, mas nao persistidas -- e a tela avisa o usuario.
type servidor struct {
	catalogo *dados.Catalogo
}

func NovoRoteador(catalogo *dados.Catalogo) http.Handler {
	s := &servidor{catalogo: catalogo}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.saude)

	r.Route("/api/fiscal", func(r chi.Router) {
		r.Get("/referencias", s.referencias)
		r.Get("/campos-por-cst", s.camposPorCST)
		r.Get("/ncm", s.buscarNCM)
		r.Get("/cfop", s.buscarCFOP)

		r.Get("/produtos/{codigo}", s.obterConfiguracao)
		r.Put("/produtos/{codigo}", s.salvarConfiguracao)
		r.Get("/produtos/{codigo}/regras", s.listarRegras)
		r.Post("/produtos/{codigo}/validacao", s.validar)

		r.Post("/simulacao", s.simular)
	})

	return r
}

func (s *servidor) saude(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "referencias": "carregadas",
	})
}

func (s *servidor) referencias(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, s.catalogo.Referencias)
}

func (s *servidor) camposPorCST(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, s.catalogo.Campos)
}

func (s *servidor) buscarNCM(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, filtrar(s.catalogo.Exemplos.NCM, r.URL.Query().Get("busca")))
}

func (s *servidor) buscarCFOP(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, filtrar(s.catalogo.Exemplos.CFOP, r.URL.Query().Get("busca")))
}

// filtrar alimenta os autocompletes por codigo ou descricao.
func filtrar(itens []dados.Item, busca string) []dados.Item {
	busca = strings.ToLower(strings.TrimSpace(busca))
	if busca == "" {
		return itens
	}
	encontrados := make([]dados.Item, 0, len(itens))
	for _, item := range itens {
		if strings.Contains(strings.ToLower(item.Codigo), busca) ||
			strings.Contains(strings.ToLower(item.Descricao), busca) {
			encontrados = append(encontrados, item)
		}
	}
	return encontrados
}

// configuracaoDe devolve a configuracao do produto, ou uma vazia.
//
// Produto sem configuracao nao e erro: e um produto que ainda nao foi
// configurado, e a tela precisa abrir para que ele possa ser.
func (s *servidor) configuracaoDe(codigo string) dominio.ConfiguracaoFiscal {
	if cfg, ok := s.catalogo.Exemplos.Configuracoes[codigo]; ok {
		return cfg
	}
	return dominio.ConfiguracaoFiscal{Codigo: codigo}
}

func (s *servidor) regrasDe(codigo string) []dominio.RegraTributaria {
	if lista, ok := s.catalogo.Exemplos.Regras[codigo]; ok {
		return lista
	}
	return []dominio.RegraTributaria{}
}

type respostaConfiguracao struct {
	Configuracao dominio.ConfiguracaoFiscal `json:"configuracao"`
	Resumo       dominio.ResumoFiscal       `json:"resumo"`
	Aviso        string                     `json:"aviso"`
}

func (s *servidor) obterConfiguracao(w http.ResponseWriter, r *http.Request) {
	codigo := chi.URLParam(r, "codigo")
	cfg := s.configuracaoDe(codigo)
	lista := s.regrasDe(codigo)

	responderJSON(w, http.StatusOK, respostaConfiguracao{
		Configuracao: cfg,
		Resumo:       resumir(cfg, lista),
		Aviso:        s.catalogo.Exemplos.Aviso,
	})
}

// salvarConfiguracao valida e devolve, SEM persistir. O prototipo mantem o
// estado na sessao do navegador, e a tela deixa isso explicito.
func (s *servidor) salvarConfiguracao(w http.ResponseWriter, r *http.Request) {
	var cfg dominio.ConfiguracaoFiscal
	if err := decodificar(r, &cfg); err != nil {
		responderErro(w, r, err)
		return
	}
	cfg.Codigo = chi.URLParam(r, "codigo")
	lista := s.regrasDe(cfg.Codigo)

	responderJSON(w, http.StatusOK, respostaConfiguracao{
		Configuracao: cfg,
		Resumo:       resumir(cfg, lista),
		Aviso:        "Protótipo: a configuração não é persistida em banco.",
	})
}

type respostaRegras struct {
	Itens     []dominio.RegraTributaria `json:"itens"`
	Conflitos []regras.Conflito         `json:"conflitos"`
	Aviso     string                    `json:"aviso"`
}

func (s *servidor) listarRegras(w http.ResponseWriter, r *http.Request) {
	lista := s.regrasDe(chi.URLParam(r, "codigo"))

	responderJSON(w, http.StatusOK, respostaRegras{
		Itens:     lista,
		Conflitos: regras.DetectarConflitos(lista),
		Aviso:     s.catalogo.Exemplos.Aviso,
	})
}

// validar aceita a configuracao e as regras no corpo, porque no prototipo o
// estado autoritativo esta na tela, nao no servidor.
type pedidoValidacao struct {
	Configuracao dominio.ConfiguracaoFiscal `json:"configuracao"`
	Regras       []dominio.RegraTributaria  `json:"regras"`
}

func (s *servidor) validar(w http.ResponseWriter, r *http.Request) {
	var pedido pedidoValidacao
	if err := decodificar(r, &pedido); err != nil {
		responderErro(w, r, err)
		return
	}
	responderJSON(w, http.StatusOK, validacao.Validar(pedido.Configuracao, pedido.Regras))
}

type pedidoSimulacao struct {
	Pedido simulacao.Pedido          `json:"pedido"`
	Regras []dominio.RegraTributaria `json:"regras"`
}

func (s *servidor) simular(w http.ResponseWriter, r *http.Request) {
	var corpo pedidoSimulacao
	if err := decodificar(r, &corpo); err != nil {
		responderErro(w, r, err)
		return
	}

	lista := corpo.Regras
	if len(lista) == 0 {
		lista = s.regrasDe(corpo.Pedido.Produto)
	}
	responderJSON(w, http.StatusOK, simulacao.Simular(lista, corpo.Pedido))
}
