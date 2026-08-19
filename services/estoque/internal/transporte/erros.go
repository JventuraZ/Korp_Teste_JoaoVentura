// Package transporte expoe o HTTP do servico: roteamento, decodificacao e a
// traducao de erro de dominio para RFC 7807.
//
// Toda a decisao de status HTTP acontece em MapearErro. Os handlers nunca
// escolhem status: eles devolvem erro de dominio e deixam a traducao num ponto
// so. E o que mantem a resposta consistente entre os sete endpoints.
package transporte

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/joaoventura/korp-estoque/internal/dominio"
)

const baseTipoErro = "https://korp.local/erros/"

// Problema e a representacao RFC 7807 devolvida em qualquer erro.
//
// O Angular tem um unico interceptor que le esta forma, independente de o erro
// ter vindo do Go ou do C#. E por isso que o Faturamento responde no mesmo formato.
type Problema struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`

	// Extensao especifica de saldo-insuficiente: lista TODOS os itens
	// problematicos, nao apenas o primeiro.
	ItensInsuficientes []dominio.ItemInsuficiente `json:"itensInsuficientes,omitempty"`
}

// MapearErro traduz um erro de dominio em status, tipo e titulo.
//
// Usa errors.Is (nao comparacao direta) para que erros embrulhados com %w
// mantenham a classificacao, e errors.As para recuperar o detalhe estruturado
// de ErroSaldoInsuficiente.
func MapearErro(err error) (status int, tipo, titulo string) {
	switch {
	case errors.Is(err, dominio.ErrProdutoNaoEncontrado):
		return http.StatusNotFound, "produto-nao-encontrado", "Produto não encontrado"
	case errors.Is(err, dominio.ErrBaixaNaoEncontrada):
		return http.StatusNotFound, "baixa-nao-encontrada", "Baixa não encontrada"
	case errors.Is(err, dominio.ErrCodigoDuplicado):
		return http.StatusConflict, "codigo-duplicado", "Código já cadastrado"
	case errors.Is(err, dominio.ErrSaldoInsuficiente):
		return http.StatusConflict, "saldo-insuficiente", "Saldo insuficiente"
	case errors.Is(err, dominio.ErrChaveIdemConflito):
		return http.StatusConflict, "chave-idempotencia-conflito", "Chave de idempotência reutilizada"
	case errors.Is(err, dominio.ErrRequisicaoInvalida):
		return http.StatusBadRequest, "requisicao-invalida", "Requisição inválida"
	default:
		return http.StatusInternalServerError, "erro-interno", "Erro interno"
	}
}

// responderErro escreve o problem+json correspondente ao erro.
func responderErro(w http.ResponseWriter, r *http.Request, err error) {
	status, tipo, titulo := MapearErro(err)

	problema := Problema{
		Type:     baseTipoErro + tipo,
		Title:    titulo,
		Status:   status,
		Detail:   err.Error(),
		Instance: r.URL.Path,
	}

	if status == http.StatusInternalServerError {
		slog.Error("erro nao tratado", "erro", err, "rota", r.URL.Path)
		problema.Detail = "Erro interno ao processar a requisição"
	}

	var insuficiente *dominio.ErroSaldoInsuficiente
	if errors.As(err, &insuficiente) {
		problema.ItensInsuficientes = insuficiente.Itens
		problema.Detail = detalharInsuficientes(len(insuficiente.Itens))
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problema)
}

func detalharInsuficientes(quantidade int) string {
	if quantidade == 1 {
		return "1 item não possui saldo suficiente para a baixa"
	}
	return fmt.Sprintf("%d itens não possuem saldo suficiente para a baixa", quantidade)
}

// responderJSON escreve uma resposta de sucesso.
func responderJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

// decodificar le o corpo JSON recusando campos desconhecidos: um typo em
// "quantiade" vira 400 explicito em vez de uma baixa silenciosa de zero.
func decodificar(r *http.Request, destino any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return fmt.Errorf("%w: corpo JSON malformado (%v)", dominio.ErrRequisicaoInvalida, err)
	}
	return nil
}
