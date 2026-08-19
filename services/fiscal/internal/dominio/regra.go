// Package dominio define a estrutura de uma regra tributaria e do contexto de
// uma operacao.
//
// A premissa que governa este pacote: regra tributaria e DADO, nunca `if`. A
// legislacao brasileira muda com frequencia, e um sistema que codifica aliquota
// em `switch` precisa de deploy a cada alteracao. Aqui a regra e um registro
// que o motor interpreta -- trocar a legislacao e trocar os dados.
package dominio

// Condicoes descreve QUANDO uma regra se aplica.
//
// Todo campo e ponteiro, e nulo significa CURINGA: "vale para qualquer valor".
// Uma regra com UFDestino nulo vale para todos os estados; com UFDestino="SP"
// vale so para Sao Paulo. E o que permite expressar desde a regra geral da
// empresa ate a excecao de um unico destino, sem estruturas diferentes.
type Condicoes struct {
	Operacao         *string `json:"operacao"`
	UFOrigem         *string `json:"ufOrigem"`
	UFDestino        *string `json:"ufDestino"`
	TipoCliente      *string `json:"tipoCliente"`
	ConsumidorFinal  *bool   `json:"consumidorFinal"`
	ContribuinteICMS *bool   `json:"contribuinteIcms"`
	RegimeEmpresa    *string `json:"regimeEmpresa"`
	Finalidade       *string `json:"finalidade"`
}

// CamposCondicao lista os campos na ordem em que aparecem na interface.
// Existe para a trilha de explicacao nomear o que casou sem repetir strings.
var CamposCondicao = []string{
	"operação", "UF de origem", "UF de destino", "tipo de cliente",
	"consumidor final", "contribuinte de ICMS", "regime da empresa", "finalidade",
}

// ResultadoTributario e O QUE a regra determina quando se aplica.
//
// Os valores aqui NAO sao calculados pelo sistema: vem do cadastro feito por
// quem conhece a legislacao. O motor apenas seleciona qual conjunto vale.
type ResultadoTributario struct {
	CFOP    string `json:"cfop"`
	CSTICMS string `json:"cstIcms"`
	CSOSN   string `json:"csosn"`

	AliquotaICMS          *float64 `json:"aliquotaIcms"`
	ReducaoBaseICMS       *float64 `json:"reducaoBaseIcms"`
	AliquotaFCP           *float64 `json:"aliquotaFcp"`
	AliquotaICMSST        *float64 `json:"aliquotaIcmsSt"`
	MVA                   *float64 `json:"mva"`
	AliquotaInterna       *float64 `json:"aliquotaInterna"`
	AliquotaInterestadual *float64 `json:"aliquotaInterestadual"`

	CSTPIS       string   `json:"cstPis"`
	AliquotaPIS  *float64 `json:"aliquotaPis"`
	CSTCOFINS    string   `json:"cstCofins"`
	AliquotaCOFINS *float64 `json:"aliquotaCofins"`

	CSTIPI      string   `json:"cstIpi"`
	AliquotaIPI *float64 `json:"aliquotaIpi"`

	Observacao string `json:"observacao"`
}

// RegraTributaria associa condicoes a um resultado.
type RegraTributaria struct {
	ID        string `json:"id"`
	Descricao string `json:"descricao"`

	// Desempata regras de mesma especificidade. Maior vence.
	Prioridade int  `json:"prioridade"`
	Ativa      bool `json:"ativa"`

	Condicoes Condicoes           `json:"condicoes"`
	Resultado ResultadoTributario `json:"resultado"`
}

// ContextoOperacao e o cenario concreto a avaliar: uma venda especifica, para
// um cliente especifico, com origem e destino conhecidos.
//
// E a segunda metade da equacao do item 11 do escopo: o cadastro do produto
// sozinho nao determina imposto nenhum -- e a combinacao dele com ESTE contexto
// que seleciona a regra.
type ContextoOperacao struct {
	Operacao         string `json:"operacao"`
	UFOrigem         string `json:"ufOrigem"`
	UFDestino        string `json:"ufDestino"`
	TipoCliente      string `json:"tipoCliente"`
	ConsumidorFinal  bool   `json:"consumidorFinal"`
	ContribuinteICMS bool   `json:"contribuinteIcms"`
	RegimeEmpresa    string `json:"regimeEmpresa"`
	Finalidade       string `json:"finalidade"`
}
