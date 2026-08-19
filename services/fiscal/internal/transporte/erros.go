// Package transporte expoe o HTTP do servico fiscal.
//
// Responde erro no MESMO formato RFC 7807 do Estoque e do Faturamento. Nao e
// coincidencia: e o que permite ao Angular tratar os tres servicos com um
// unico interceptor, sem saber em que linguagem cada um foi escrito.
package transporte

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

const baseTipoErro = "https://korp.local/erros/"

var (
	ErrProdutoNaoEncontrado = errors.New("configuracao fiscal nao encontrada para o produto")
	ErrRequisicaoInvalida   = errors.New("requisicao invalida")
)

type Problema struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

// MapearErro e o ponto unico de decisao de status, como no servico de Estoque.
func MapearErro(err error) (status int, tipo, titulo string) {
	switch {
	case errors.Is(err, ErrProdutoNaoEncontrado):
		return http.StatusNotFound, "configuracao-fiscal-nao-encontrada", "Configuração fiscal não encontrada"
	case errors.Is(err, ErrRequisicaoInvalida):
		return http.StatusBadRequest, "requisicao-invalida", "Requisição inválida"
	default:
		return http.StatusInternalServerError, "erro-interno", "Erro interno"
	}
}

func responderErro(w http.ResponseWriter, r *http.Request, err error) {
	status, tipo, titulo := MapearErro(err)

	problema := Problema{
		Type: baseTipoErro + tipo, Title: titulo, Status: status,
		Detail: err.Error(), Instance: r.URL.Path,
	}
	if status == http.StatusInternalServerError {
		slog.Error("erro nao tratado", "erro", err, "rota", r.URL.Path)
		problema.Detail = "Erro interno ao processar a requisição"
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problema)
}

func responderJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

func decodificar(r *http.Request, destino any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return errors.Join(ErrRequisicaoInvalida, err)
	}
	return nil
}
