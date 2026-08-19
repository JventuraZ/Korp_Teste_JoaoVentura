package analise

import (
	"math"
	"testing"
	"time"
)

func dia(offset int) time.Time {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, offset)
}

func serieConstante(dias int, valor float64) []ConsumoDia {
	serie := make([]ConsumoDia, dias)
	for i := range serie {
		serie[i] = ConsumoDia{Dia: dia(i), Consumo: valor}
	}
	return serie
}

func TestTaxaConstanteConvergeParaOValor(t *testing.T) {
	taxa := TaxaConsumoDiaria(serieConstante(60, 3), AlfaPadrao)

	if math.Abs(taxa-3) > 0.05 {
		t.Fatalf("taxa %.3f, esperado ~3", taxa)
	}
}

// O ponto inteiro de usar media exponencial em vez de media simples: quem
// dobrou o consumo esta semana precisa aparecer como mais urgente, nao ter a
// mudanca diluida em tres meses de calmaria.
func TestTaxaDaMaisPesoAoRecente(t *testing.T) {
	// 50 dias consumindo 1/dia, depois 10 dias consumindo 10/dia.
	serie := serieConstante(50, 1)
	for i := 50; i < 60; i++ {
		serie = append(serie, ConsumoDia{Dia: dia(i), Consumo: 10})
	}

	exponencial := TaxaConsumoDiaria(serie, AlfaPadrao)

	var soma float64
	for _, ponto := range serie {
		soma += ponto.Consumo
	}
	simples := soma / float64(len(serie))

	if exponencial <= simples {
		t.Fatalf("exponencial %.2f deveria superar a media simples %.2f", exponencial, simples)
	}
}

// Um produto que vendeu uma unica vez em 60 dias nao consome 1/dia. Sem
// preencher os dias vazios, a taxa sairia 60x maior que a real.
func TestPreencherDiasEvitaSuperestimarATaxa(t *testing.T) {
	esparsa := []ConsumoDia{{Dia: dia(0), Consumo: 1}}

	semPreencher := TaxaConsumoDiaria(esparsa, AlfaPadrao)
	preenchida := TaxaConsumoDiaria(PreencherDias(esparsa, dia(0), dia(59)), AlfaPadrao)

	if semPreencher != 1 {
		t.Fatalf("serie esparsa deveria dar taxa 1, deu %.3f", semPreencher)
	}
	if preenchida >= 0.1 {
		t.Fatalf("taxa preenchida %.4f deveria ser proxima de zero", preenchida)
	}
}

func TestPreencherDiasCriaUmPontoPorDia(t *testing.T) {
	completa := PreencherDias([]ConsumoDia{{Dia: dia(3), Consumo: 5}}, dia(0), dia(6))

	if len(completa) != 7 {
		t.Fatalf("esperava 7 dias, veio %d", len(completa))
	}
	if completa[3].Consumo != 5 {
		t.Errorf("o dia com movimento perdeu o valor: %+v", completa[3])
	}
	if completa[0].Consumo != 0 {
		t.Errorf("dia sem movimento deveria ser zero, veio %.2f", completa[0].Consumo)
	}
}

// Estorno abate consumo. Se uma nota estornada continuasse contando, o sistema
// pediria reposicao de um produto que nunca chegou a sair.
func TestEstornoReduzOConsumoLiquido(t *testing.T) {
	comEstorno := []ConsumoDia{
		{Dia: dia(0), Consumo: 10},
		{Dia: dia(1), Consumo: -10}, // baixa desfeita
	}
	completa := PreencherDias(comEstorno, dia(0), dia(29))

	if taxa := TaxaConsumoDiaria(completa, AlfaPadrao); taxa > 0.5 {
		t.Fatalf("taxa %.3f alta demais para consumo que foi estornado", taxa)
	}
}

func TestTaxaNuncaFicaNegativa(t *testing.T) {
	so_estornos := []ConsumoDia{{Dia: dia(0), Consumo: -5}, {Dia: dia(1), Consumo: -3}}

	if taxa := TaxaConsumoDiaria(so_estornos, AlfaPadrao); taxa < 0 {
		t.Fatalf("taxa negativa (%.2f) nao faz sentido", taxa)
	}
}

func TestDiasAteRuptura(t *testing.T) {
	casos := []struct {
		nome     string
		saldo    int
		taxa     float64
		esperado float64
	}{
		{"consumo estavel", 10, 2, 5},
		{"saldo zerado", 0, 5, 0},
		{"sem consumo", 100, 0, math.Inf(1)},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := DiasAteRuptura(caso.saldo, caso.taxa); got != caso.esperado {
				t.Fatalf("veio %v, esperado %v", got, caso.esperado)
			}
		})
	}
}

// Produto parado nao pode ser urgente: sem consumo, o saldo dura para sempre.
func TestProdutoSemConsumoNaoEhCritico(t *testing.T) {
	dias := DiasAteRuptura(3, TaxaConsumoDiaria(PreencherDias(nil, dia(0), dia(59)), AlfaPadrao))

	if risco := ClassificarRisco(dias, 60); risco != RiscoOk {
		t.Fatalf("risco %s, esperado %s", risco, RiscoOk)
	}
}

func TestClassificarRisco(t *testing.T) {
	casos := []struct {
		dias     float64
		amostras int
		esperado Risco
	}{
		{2, 60, RiscoCritico},
		{6.9, 60, RiscoCritico},
		{7, 60, RiscoAtencao},
		{20, 60, RiscoAtencao},
		{21, 60, RiscoOk},
		{math.Inf(1), 60, RiscoOk},
		// Histórico curto demais: o honesto e nao opinar.
		{1, 3, RiscoSemDados},
	}

	for _, caso := range casos {
		if got := ClassificarRisco(caso.dias, caso.amostras); got != caso.esperado {
			t.Errorf("dias=%v amostras=%d: veio %s, esperado %s",
				caso.dias, caso.amostras, got, caso.esperado)
		}
	}
}

func movimentosDe(codigo string, quantidades ...float64) []Movimento {
	movs := make([]Movimento, len(quantidades))
	for i, qtd := range quantidades {
		movs[i] = Movimento{Codigo: codigo, Quantidade: qtd, Momento: dia(i), Referencia: "NF-TESTE"}
	}
	return movs
}

func TestAnomaliaDetectaBaixaMuitoAcimaDoPadrao(t *testing.T) {
	// Padrao entre 2 e 4 unidades, com uma baixa de 60 no meio.
	movs := movimentosDe("PRD-001", 3, 2, 4, 3, 3, 2, 4, 3, 60, 2, 3, 4)

	anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia)

	if len(anomalias) != 1 {
		t.Fatalf("esperava 1 anomalia, veio %d: %+v", len(anomalias), anomalias)
	}
	if anomalias[0].Quantidade != 60 {
		t.Errorf("apontou a quantidade errada: %.0f", anomalias[0].Quantidade)
	}
}

func TestVariacaoNormalNaoViraAnomalia(t *testing.T) {
	movs := movimentosDe("PRD-001", 3, 2, 4, 3, 5, 2, 4, 3, 2, 5, 3, 4)

	if anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia); len(anomalias) != 0 {
		t.Fatalf("nao deveria apontar nada, veio %+v", anomalias)
	}
}

// Com poucas baixas qualquer variacao parece outlier. Preferimos calar.
func TestPoucasAmostrasNaoGeramAnomalia(t *testing.T) {
	movs := movimentosDe("PRD-001", 3, 2, 100)

	if anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia); len(anomalias) != 0 {
		t.Fatalf("amostra pequena demais para opinar, veio %+v", anomalias)
	}
}

// Baixa MENOR que o normal nao e problema de estoque.
func TestDesvioParaBaixoNaoEhAnomalia(t *testing.T) {
	movs := movimentosDe("PRD-001", 50, 50, 50, 50, 1, 50, 50, 50, 50, 50)

	for _, anomalia := range DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia) {
		if anomalia.Quantidade < anomalia.Mediana {
			t.Fatalf("apontou desvio para baixo: %+v", anomalia)
		}
	}
}

// Pedidos padronizados fazem o MAD zerar. O metodo cai no desvio absoluto
// medio em vez de explodir ou passar a apontar tudo.
func TestMadZeroCaiNoDesvioMedio(t *testing.T) {
	movs := movimentosDe("PRD-001", 5, 5, 5, 5, 5, 5, 5, 5, 5, 40)

	anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia)

	if len(anomalias) != 1 || anomalias[0].Quantidade != 40 {
		t.Fatalf("esperava apontar so a de 40, veio %+v", anomalias)
	}
	if math.IsInf(anomalias[0].Escore, 1) {
		t.Error("escore infinito nao sobrevive a serializacao JSON")
	}
}

// REGRESSAO: encontrado com dados reais. Num produto cuja baixa tipica e 1
// unidade, o MAD zera e QUALQUER baixa de 2 virava anomalia -- o painel abria
// com 15 alertas, sendo 12 deles ruido de itens de giro pequeno.
//
// Materialidade e significancia estatistica sao coisas diferentes: dobrar a
// mediana e estatisticamente extremo e praticamente irrelevante.
func TestBaixaPoucoAcimaDaMedianaNaoEhAnomalia(t *testing.T) {
	movs := movimentosDe("PRD-005", 1, 1, 2, 1, 1, 2, 1, 1, 1, 2, 1, 1)

	if anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia); len(anomalias) != 0 {
		t.Fatalf("baixa de 2 com mediana 1 nao e anomalia, veio %+v", anomalias)
	}
}

// O outro lado do filtro: o desvio grande continua sendo apontado no MESMO
// produto de giro pequeno.
func TestDesvioGrandeEmProdutoDeGiroPequenoEhAnomalia(t *testing.T) {
	movs := movimentosDe("PRD-005", 1, 1, 2, 1, 1, 2, 1, 1, 1, 2, 1, 30)

	anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia)

	if len(anomalias) != 1 || anomalias[0].Quantidade != 30 {
		t.Fatalf("esperava apontar so a de 30, veio %+v", anomalias)
	}
}

func TestAnomaliasSaemOrdenadasPelaMaisAtipica(t *testing.T) {
	movs := movimentosDe("PRD-001", 3, 2, 4, 3, 3, 2, 4, 3, 30, 2, 3, 90)

	anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia)

	if len(anomalias) < 2 {
		t.Fatalf("esperava ao menos 2 anomalias, veio %d", len(anomalias))
	}
	if anomalias[0].Escore < anomalias[1].Escore {
		t.Errorf("fora de ordem: %.1f antes de %.1f", anomalias[0].Escore, anomalias[1].Escore)
	}
}

// Cada produto tem seu proprio padrao: 50 unidades e rotina para um item de
// alto giro e absurdo para outro.
func TestPadraoEhAvaliadoPorProduto(t *testing.T) {
	movs := append(
		movimentosDe("PRD-ALTO", 100, 95, 105, 98, 102, 99, 101, 97, 103, 100),
		movimentosDe("PRD-BAIXO", 2, 3, 2, 3, 2, 3, 2, 3, 2, 100)...,
	)

	anomalias := DetectarAnomalias(movs, LimiarAnomalia, MinimoAmostrasAnomalia)

	if len(anomalias) != 1 {
		t.Fatalf("esperava 1 anomalia, veio %d: %+v", len(anomalias), anomalias)
	}
	if anomalias[0].Codigo != "PRD-BAIXO" {
		t.Errorf("apontou o produto errado: %s", anomalias[0].Codigo)
	}
}
