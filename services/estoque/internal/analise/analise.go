// Package analise concentra a parte preditiva do Estoque: estimar quando um
// produto vai acabar e apontar movimentos fora do padrao.
//
// Tudo aqui e funcao pura sobre series numericas -- nao ha acesso a banco nem a
// HTTP. Isso mantem a matematica testavel sem subir Postgres e deixa claro onde
// termina a modelagem e comeca a infraestrutura.
//
// NAO e um `WHERE saldo <= limite`. Um limite fixo nao distingue um produto com
// 10 unidades que gira 5 por dia (acaba depois de amanha) de outro com 10 que
// gira uma por mes. O que interessa e a TAXA, e ela e estimada a partir do
// historico de consumo.
package analise

import (
	"math"
	"sort"
	"time"
)

// Alfa da suavizacao exponencial.
//
// Peso de uma observacao de k dias atras: alfa*(1-alfa)^k. Com 0.10 a meia-vida
// fica em ~6,6 dias: o que aconteceu na ultima semana domina a estimativa, mas
// um unico dia atipico nao sequestra o resultado.
//
// Valor mais alto reage mais rapido e oscila mais; mais baixo suaviza e demora
// a perceber mudanca de patamar.
const AlfaPadrao = 0.10

// Limiares de dias ate a ruptura.
const (
	DiasCritico = 7.0
	DiasAtencao = 21.0
)

// MinimoAmostras e o numero de dias de historico abaixo do qual nao opinamos.
// Prever a partir de dois pontos e chute com aparencia de numero.
const MinimoAmostras = 7

type Risco string

const (
	RiscoCritico  Risco = "CRITICO"
	RiscoAtencao  Risco = "ATENCAO"
	RiscoOk       Risco = "OK"
	RiscoSemDados Risco = "SEM_DADOS"
)

// ConsumoDia e o consumo liquido de um produto num dia: baixas menos estornos.
type ConsumoDia struct {
	Dia     time.Time
	Consumo float64
}

// TaxaConsumoDiaria estima quantas unidades por dia o produto consome, dando
// mais peso ao passado recente (media movel exponencial).
//
// A serie precisa vir COMPLETA, com os dias sem movimento explicitos em zero --
// use PreencherDias. Somar apenas os dias que tiveram venda superestima a taxa:
// um produto que vendeu uma vez em sessenta dias pareceria vender todo dia.
func TaxaConsumoDiaria(serie []ConsumoDia, alfa float64) float64 {
	if len(serie) == 0 {
		return 0
	}
	if alfa <= 0 || alfa > 1 {
		alfa = AlfaPadrao
	}

	ordenada := make([]ConsumoDia, len(serie))
	copy(ordenada, serie)
	sort.Slice(ordenada, func(i, j int) bool { return ordenada[i].Dia.Before(ordenada[j].Dia) })

	media := ordenada[0].Consumo
	for _, dia := range ordenada[1:] {
		media = alfa*dia.Consumo + (1-alfa)*media
	}

	if media < 0 {
		return 0
	}
	return media
}

// PreencherDias completa a serie com zeros nos dias sem movimento, entre inicio
// e fim inclusive. E o passo que torna a taxa honesta.
func PreencherDias(serie []ConsumoDia, inicio, fim time.Time) []ConsumoDia {
	porDia := make(map[string]float64, len(serie))
	for _, ponto := range serie {
		porDia[ponto.Dia.UTC().Format(time.DateOnly)] += ponto.Consumo
	}

	inicio = truncarDia(inicio)
	fim = truncarDia(fim)

	completa := make([]ConsumoDia, 0, int(fim.Sub(inicio).Hours()/24)+1)
	for dia := inicio; !dia.After(fim); dia = dia.AddDate(0, 0, 1) {
		completa = append(completa, ConsumoDia{
			Dia:     dia,
			Consumo: porDia[dia.Format(time.DateOnly)],
		})
	}
	return completa
}

// DiasAteRuptura projeta em quantos dias o saldo chega a zero no ritmo atual.
//
// Taxa zero devolve +Inf: "nao consome" e diferente de "vai acabar agora", e a
// diferenca importa -- um produto parado nao pode aparecer como urgente.
func DiasAteRuptura(saldo int, taxaDiaria float64) float64 {
	if saldo <= 0 {
		return 0
	}
	if taxaDiaria <= 0 {
		return math.Inf(1)
	}
	return float64(saldo) / taxaDiaria
}

// ClassificarRisco traduz a projecao numa faixa acionavel.
func ClassificarRisco(dias float64, amostras int) Risco {
	if amostras < MinimoAmostras {
		return RiscoSemDados
	}
	switch {
	case dias < DiasCritico:
		return RiscoCritico
	case dias < DiasAtencao:
		return RiscoAtencao
	default:
		return RiscoOk
	}
}

// Movimento e uma baixa observada, para a deteccao de anomalias.
type Movimento struct {
	Codigo     string
	Quantidade float64
	Momento    time.Time
	Referencia string
}

// Anomalia e um movimento cuja quantidade destoa do padrao do proprio produto.
type Anomalia struct {
	Movimento
	Mediana float64 `json:"mediana"`
	Escore  float64 `json:"escore"`
}

// LimiarAnomalia e o escore-z modificado acima do qual um ponto e considerado
// atipico. 3.5 e o valor proposto por Iglewicz e Hoaglin.
const LimiarAnomalia = 3.5

// MinimoAmostrasAnomalia: com poucas baixas, qualquer variacao parece outlier.
const MinimoAmostrasAnomalia = 8

// FatorMaterialidade separa significancia ESTATISTICA de significancia PRATICA.
//
// Num produto cuja baixa tipica e 1 unidade, uma baixa de 2 e estatisticamente
// extrema -- dobra a mediana -- mas nao interessa a ninguem. Sem este filtro o
// painel se enche de ruido justamente nos itens de giro pequeno, e o operador
// para de olhar.
//
// Exigimos as duas coisas: escore alto E pelo menos 3x a mediana.
const FatorMaterialidade = 3.0

// DetectarAnomalias aponta baixas muito acima do padrao do produto.
//
// Usa mediana e MAD (desvio absoluto mediano), nao media e desvio-padrao. A
// razao e direta: media e desvio sao deslocados pelos proprios outliers que
// procuramos -- uma baixa de 500 unidades levanta a media e infla o desvio, e o
// metodo acaba escondendo justamente o ponto que deveria denunciar. A mediana
// e insensivel a isso.
//
// So aponta desvios PARA CIMA: uma baixa menor que o normal nao e problema.
func DetectarAnomalias(movimentos []Movimento, limiar float64, minAmostras int) []Anomalia {
	if limiar <= 0 {
		limiar = LimiarAnomalia
	}
	if minAmostras <= 0 {
		minAmostras = MinimoAmostrasAnomalia
	}

	porProduto := make(map[string][]Movimento)
	for _, mov := range movimentos {
		porProduto[mov.Codigo] = append(porProduto[mov.Codigo], mov)
	}

	var anomalias []Anomalia
	for _, movs := range porProduto {
		if len(movs) < minAmostras {
			continue
		}

		quantidades := make([]float64, len(movs))
		for i, mov := range movs {
			quantidades[i] = mov.Quantidade
		}

		mediana := medianaDe(quantidades)

		desvios := make([]float64, len(quantidades))
		for i, valor := range quantidades {
			desvios[i] = math.Abs(valor - mediana)
		}
		mad := medianaDe(desvios)

		desvioMedio := mediaDe(desvios)

		for _, mov := range movs {
			if mov.Quantidade <= mediana {
				continue
			}

			if mediana > 0 && mov.Quantidade < FatorMaterialidade*mediana {
				continue
			}

			escore := escoreModificado(mov.Quantidade, mediana, mad, desvioMedio)
			if escore >= limiar {
				anomalias = append(anomalias, Anomalia{
					Movimento: mov,
					Mediana:   mediana,
					Escore:    escore,
				})
			}
		}
	}

	sort.Slice(anomalias, func(i, j int) bool { return anomalias[i].Escore > anomalias[j].Escore })
	return anomalias
}

// escoreModificado e o z-score robusto de Iglewicz-Hoaglin.
//
// 0.6745 e o quantil 0.75 da normal padrao: converte o MAD para a mesma escala
// de um desvio-padrao, de modo que o limiar 3.5 signifique "3,5 desvios".
//
// Quando o MAD e zero -- comum em estoque, onde mais da metade das baixas tem
// exatamente o mesmo tamanho por causa de pedidos padronizados -- os proprios
// autores preveem a alternativa: trocar o MAD pelo desvio absoluto MEDIO, com
// a constante 1.253314. Isso mantem o escore finito e na mesma escala, em vez
// de exigir uma regra improvisada.
func escoreModificado(valor, mediana, mad, desvioMedio float64) float64 {
	if mad > 0 {
		return 0.6745 * (valor - mediana) / mad
	}
	if desvioMedio > 0 {
		return (valor - mediana) / (1.253314 * desvioMedio)
	}

	return 0
}

func mediaDe(valores []float64) float64 {
	if len(valores) == 0 {
		return 0
	}

	var soma float64
	for _, valor := range valores {
		soma += valor
	}
	return soma / float64(len(valores))
}

func medianaDe(valores []float64) float64 {
	if len(valores) == 0 {
		return 0
	}

	ordenados := make([]float64, len(valores))
	copy(ordenados, valores)
	sort.Float64s(ordenados)

	meio := len(ordenados) / 2
	if len(ordenados)%2 == 1 {
		return ordenados[meio]
	}
	return (ordenados[meio-1] + ordenados[meio]) / 2
}

func truncarDia(momento time.Time) time.Time {
	momento = momento.UTC()
	return time.Date(momento.Year(), momento.Month(), momento.Day(), 0, 0, 0, 0, time.UTC)
}
