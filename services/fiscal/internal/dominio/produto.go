package dominio

// PisCofins agrupa a configuracao das duas contribuicoes.
//
// Ficam juntas porque compartilham estrutura e CST, mas sao configuradas
// separadamente -- a legislacao permite tratamento diferente para cada uma.
type PisCofins struct {
	CSTPIS            string   `json:"cstPis"`
	AliquotaPIS       *float64 `json:"aliquotaPis"`
	BaseCalculoPIS    string   `json:"baseCalculoPis"`
	CSTCOFINS         string   `json:"cstCofins"`
	AliquotaCOFINS    *float64 `json:"aliquotaCofins"`
	BaseCalculoCOFINS string   `json:"baseCalculoCofins"`
}

type IPI struct {
	CSTIPI               string   `json:"cstIpi"`
	EnquadramentoLegal   string   `json:"enquadramentoLegal"`
	CodigoEnquadramento  string   `json:"codigoEnquadramento"`
	AliquotaIPI          *float64 `json:"aliquotaIpi"`
	UnidadeTributavel    string   `json:"unidadeTributavel"`
	QuantidadeTributavel *float64 `json:"quantidadeTributavel"`
	ValorPorUnidade      *float64 `json:"valorPorUnidade"`
}

// ConfiguracaoFiscal e a parte fiscal do cadastro do produto.
//
// Repare no que NAO esta aqui: CFOP e CST de ICMS. Eles dependem da operacao
// (venda ou devolucao, dentro ou fora do estado, para contribuinte ou nao) e
// por isso vivem nas regras. Fixa-los no produto seria o erro que o item 6 do
// escopo pede explicitamente para evitar.
type ConfiguracaoFiscal struct {
	Codigo string `json:"codigo"`

	NCM                   string `json:"ncm"`
	CEST                  string `json:"cest"`
	CodigoBeneficioFiscal string `json:"codigoBeneficioFiscal"`
	OrigemMercadoria      string `json:"origemMercadoria"`
	ExTIPI                string `json:"exTipi"`

	UnidadeTributavel    string   `json:"unidadeTributavel"`
	QuantidadeTributavel *float64 `json:"quantidadeTributavel"`
	CodigoANP            string   `json:"codigoAnp"`
	ProducaoPropria      bool     `json:"producaoPropria"`

	PisCofins PisCofins `json:"pisCofins"`
	IPI       IPI       `json:"ipi"`
}

// Situacao resume o estado de configuracao de uma area para o semaforo da tela.
type Situacao string

const (
	Configurado  Situacao = "CONFIGURADO"
	Incompleto   Situacao = "INCOMPLETO"
	ComErro      Situacao = "ERRO"
	NaoAplicavel Situacao = "NAO_APLICAVEL"
)

// ResumoFiscal alimenta o cabecalho do item 10.
//
// E sempre DERIVADO da configuracao, nunca um campo digitado: status que o
// usuario pode marcar a mao mente com o tempo.
type ResumoFiscal struct {
	Situacao   Situacao          `json:"situacao"`
	Areas      map[string]string `json:"areas"`
	TotalRegras int              `json:"totalRegras"`
	RegrasAtivas int             `json:"regrasAtivas"`
}
