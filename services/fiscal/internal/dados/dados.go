// Package dados carrega as tabelas de referencia e os exemplos embutidos no
// binario.
//
// Sao arquivos JSON e nao constantes Go de proposito: a legislacao muda, e
// trocar uma tabela nao pode exigir recompilar o servico. Num ambiente real
// estes arquivos viriam de um banco ou de um servico de tabelas fiscais -- o
// formato ja e o mesmo.
package dados

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
)

//go:embed referencias.json
var brutoReferencias []byte

//go:embed campos-por-cst.json
var brutoCampos []byte

//go:embed exemplos.json
var brutoExemplos []byte

type Item struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
}

type Referencias struct {
	Aviso              string   `json:"_aviso"`
	OrigensMercadoria  []Item   `json:"origensMercadoria"`
	CSTIcms            []Item   `json:"cstIcms"`
	CSOSN              []Item   `json:"csosn"`
	CSTPisCofins       []Item   `json:"cstPisCofins"`
	CSTIpi             []Item   `json:"cstIpi"`
	TiposOperacao      []Item   `json:"tiposOperacao"`
	Finalidades        []Item   `json:"finalidades"`
	RegimesTributarios []Item   `json:"regimesTributarios"`
	TiposCliente       []Item   `json:"tiposCliente"`
	UFs                []string `json:"ufs"`
}

type CamposPorCST struct {
	Aviso   string              `json:"_aviso"`
	ICMS    map[string][]string `json:"icms"`
	CSOSN   map[string][]string `json:"csosn"`
	Rotulos map[string]string   `json:"rotulos"`
}

type Exemplos struct {
	Aviso         string                                   `json:"_aviso"`
	NCM           []Item                                   `json:"ncm"`
	CFOP          []Item                                   `json:"cfop"`
	Configuracoes map[string]dominio.ConfiguracaoFiscal    `json:"configuracoes"`
	Regras        map[string][]dominio.RegraTributaria     `json:"regras"`
}

type Catalogo struct {
	Referencias Referencias
	Campos      CamposPorCST
	Exemplos    Exemplos
}

// Carregar le os tres arquivos embutidos. Falha na subida, nao no primeiro
// request: dado de referencia corrompido e problema de implantacao.
func Carregar() (*Catalogo, error) {
	catalogo := &Catalogo{}

	if err := json.Unmarshal(brutoReferencias, &catalogo.Referencias); err != nil {
		return nil, fmt.Errorf("ler referencias.json: %w", err)
	}
	if err := json.Unmarshal(brutoCampos, &catalogo.Campos); err != nil {
		return nil, fmt.Errorf("ler campos-por-cst.json: %w", err)
	}
	if err := json.Unmarshal(brutoExemplos, &catalogo.Exemplos); err != nil {
		return nil, fmt.Errorf("ler exemplos.json: %w", err)
	}
	return catalogo, nil
}
