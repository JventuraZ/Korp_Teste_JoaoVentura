package dominio

import "time"

// PrevisaoProduto e a projecao de ruptura de um produto.
type PrevisaoProduto struct {
	Codigo    string `json:"codigo"`
	Descricao string `json:"descricao"`
	Saldo     int    `json:"saldo"`

	// Unidades por dia, estimadas por media movel exponencial sobre o consumo
	// liquido (baixas menos estornos).
	ConsumoDiario float64 `json:"consumoDiario"`

	// Nulo quando o produto nao tem consumo na janela: o saldo dura
	// indefinidamente, e "infinito" nao existe em JSON.
	DiasAteRuptura      *float64   `json:"diasAteRuptura"`
	DataRupturaEstimada *time.Time `json:"dataRupturaEstimada"`

	Risco string `json:"risco"`

	// Dias de historico considerados. Serve para o cliente saber o quanto
	// confiar na projecao -- e por que um produto pode vir como SEM_DADOS.
	Amostras int `json:"amostras"`
}

// PainelPrevisao e a resposta de GET /api/estoque/previsao.
type PainelPrevisao struct {
	Itens      []PrevisaoProduto `json:"itens"`
	JanelaDias int               `json:"janelaDias"`
	GeradoEm   time.Time         `json:"geradoEm"`

	// Contagem por faixa de risco, para o frontend montar o selo do cabecalho
	// sem precisar percorrer a lista.
	Resumo map[string]int `json:"resumo"`
}

// AnomaliaDetectada e uma baixa fora do padrao historico do proprio produto.
type AnomaliaDetectada struct {
	Codigo     string    `json:"codigo"`
	Descricao  string    `json:"descricao"`
	Quantidade int       `json:"quantidade"`
	Mediana    float64   `json:"medianaDoProduto"`
	Escore     float64   `json:"escore"`
	Referencia string    `json:"referencia"`
	Momento    time.Time `json:"momento"`
}

// PainelAnomalias e a resposta de GET /api/estoque/anomalias.
type PainelAnomalias struct {
	Itens      []AnomaliaDetectada `json:"itens"`
	JanelaDias int                 `json:"janelaDias"`
	GeradoEm   time.Time           `json:"geradoEm"`
}
