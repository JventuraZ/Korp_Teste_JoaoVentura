package repositorio

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/joaoventura/korp-estoque/internal/analise"
	"github.com/joaoventura/korp-estoque/internal/dominio"
)

// Limites da janela de historico aceita pelos endpoints.
const (
	janelaPadrao = 90
	janelaMinima = 7
	janelaMaxima = 365
)

// escoreMaximo existe porque JSON nao representa infinito. O analise devolve
// +Inf quando o MAD e zero e a quantidade destoa mesmo assim; aqui isso vira um
// teto finito, que e o suficiente para ordenar e exibir.
const escoreMaximo = 999.0

// PreverRupturas projeta, para cada produto ativo, em quantos dias o saldo
// chega a zero mantido o ritmo de consumo recente.
//
// Divisao de responsabilidades: este metodo busca os dados e monta a resposta;
// toda a matematica vive em internal/analise, sem saber que existe banco.
func (r *Repositorio) PreverRupturas(ctx context.Context, dias int) (*dominio.PainelPrevisao, error) {
	dias = ajustarJanela(dias)

	produtos, err := r.produtosAtivos(ctx)
	if err != nil {
		return nil, err
	}

	consumoPorProduto, err := r.consumoDiario(ctx, dias)
	if err != nil {
		return nil, err
	}

	agoraUTC := agora()
	inicio := agoraUTC.AddDate(0, 0, -dias)

	painel := &dominio.PainelPrevisao{
		Itens:      make([]dominio.PrevisaoProduto, 0, len(produtos)),
		JanelaDias: dias,
		GeradoEm:   agoraUTC,
		Resumo:     map[string]int{},
	}

	for _, produto := range produtos {
		serie := analise.PreencherDias(consumoPorProduto[produto.Codigo], inicio, agoraUTC)

		taxa := analise.TaxaConsumoDiaria(serie, analise.AlfaPadrao)
		diasRuptura := analise.DiasAteRuptura(produto.Saldo, taxa)
		risco := analise.ClassificarRisco(diasRuptura, len(serie))

		previsao := dominio.PrevisaoProduto{
			Codigo:        produto.Codigo,
			Descricao:     produto.Descricao,
			Saldo:         produto.Saldo,
			ConsumoDiario: arredondar(taxa, 2),
			Risco:         string(risco),
			Amostras:      len(serie),
		}

		if !math.IsInf(diasRuptura, 1) {
			arredondado := arredondar(diasRuptura, 1)
			data := agoraUTC.Add(time.Duration(diasRuptura * float64(24*time.Hour)))

			previsao.DiasAteRuptura = &arredondado
			previsao.DataRupturaEstimada = &data
		}

		painel.Itens = append(painel.Itens, previsao)
		painel.Resumo[previsao.Risco]++
	}

	ordenarPorUrgencia(painel.Itens)
	return painel, nil
}

// Anomalias aponta baixas cuja quantidade destoa do padrao do proprio produto.
func (r *Repositorio) Anomalias(ctx context.Context, dias int) (*dominio.PainelAnomalias, error) {
	dias = ajustarJanela(dias)

	linhas, err := r.pool.Query(ctx,
		`SELECT p.codigo, p.descricao, m.quantidade, m.criado_em, coalesce(m.referencia, '')
		   FROM movimentos_estoque m
		   JOIN produtos p ON p.id = m.produto_id
		  WHERE m.tipo = $1
		    AND p.ativo
		    AND m.criado_em >= now() - make_interval(days => $2)
		  ORDER BY m.criado_em`, dominio.MovimentoBaixa, dias)
	if err != nil {
		return nil, fmt.Errorf("consultar movimentos para anomalias: %w", err)
	}
	defer linhas.Close()

	movimentos := make([]analise.Movimento, 0, 256)
	descricoes := map[string]string{}

	for linhas.Next() {
		var codigo, descricao, referencia string
		var quantidade int
		var momento time.Time

		if err := linhas.Scan(&codigo, &descricao, &quantidade, &momento, &referencia); err != nil {
			return nil, fmt.Errorf("ler movimento: %w", err)
		}

		descricoes[codigo] = descricao
		movimentos = append(movimentos, analise.Movimento{
			Codigo:     codigo,
			Quantidade: float64(quantidade),
			Momento:    momento,
			Referencia: referencia,
		})
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("consultar movimentos para anomalias: %w", err)
	}

	detectadas := analise.DetectarAnomalias(movimentos, analise.LimiarAnomalia, analise.MinimoAmostrasAnomalia)

	painel := &dominio.PainelAnomalias{
		Itens:      make([]dominio.AnomaliaDetectada, 0, len(detectadas)),
		JanelaDias: dias,
		GeradoEm:   agora(),
	}

	for _, anomalia := range detectadas {
		escore := anomalia.Escore
		if math.IsInf(escore, 1) {
			escore = escoreMaximo
		}

		painel.Itens = append(painel.Itens, dominio.AnomaliaDetectada{
			Codigo:     anomalia.Codigo,
			Descricao:  descricoes[anomalia.Codigo],
			Quantidade: int(anomalia.Quantidade),
			Mediana:    arredondar(anomalia.Mediana, 1),
			Escore:     arredondar(escore, 1),
			Referencia: anomalia.Referencia,
			Momento:    anomalia.Momento,
		})
	}

	return painel, nil
}

// consumoDiario agrega o consumo LIQUIDO por produto e por dia.
//
// O estorno entra com sinal negativo, abatendo a baixa que ele desfez. Contar
// so as baixas inflaria a previsao e faria o sistema pedir reposicao de
// mercadoria que nunca chegou a sair.
func (r *Repositorio) consumoDiario(ctx context.Context, dias int) (map[string][]analise.ConsumoDia, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT p.codigo,
		        (m.criado_em AT TIME ZONE 'UTC')::date AS dia,
		        sum(CASE WHEN m.tipo = $1 THEN m.quantidade ELSE -m.quantidade END)::float8
		   FROM movimentos_estoque m
		   JOIN produtos p ON p.id = m.produto_id
		  WHERE m.tipo IN ($1, $2)
		    AND p.ativo
		    AND m.criado_em >= now() - make_interval(days => $3)
		  GROUP BY p.codigo, dia
		  ORDER BY p.codigo, dia`,
		dominio.MovimentoBaixa, dominio.MovimentoEstorno, dias)
	if err != nil {
		return nil, fmt.Errorf("agregar consumo diario: %w", err)
	}
	defer linhas.Close()

	consumo := make(map[string][]analise.ConsumoDia)
	for linhas.Next() {
		var codigo string
		var dia time.Time
		var quantidade float64

		if err := linhas.Scan(&codigo, &dia, &quantidade); err != nil {
			return nil, fmt.Errorf("ler consumo diario: %w", err)
		}
		consumo[codigo] = append(consumo[codigo], analise.ConsumoDia{Dia: dia, Consumo: quantidade})
	}

	return consumo, linhas.Err()
}

func (r *Repositorio) produtosAtivos(ctx context.Context) ([]dominio.Produto, error) {
	linhas, err := r.pool.Query(ctx,
		`SELECT codigo, descricao, saldo FROM produtos WHERE ativo ORDER BY codigo`)
	if err != nil {
		return nil, fmt.Errorf("listar produtos ativos: %w", err)
	}
	defer linhas.Close()

	produtos := make([]dominio.Produto, 0, 64)
	for linhas.Next() {
		var produto dominio.Produto
		if err := linhas.Scan(&produto.Codigo, &produto.Descricao, &produto.Saldo); err != nil {
			return nil, fmt.Errorf("ler produto: %w", err)
		}
		produtos = append(produtos, produto)
	}

	return produtos, linhas.Err()
}

// ordenarPorUrgencia coloca primeiro o que precisa de decisao: menor prazo de
// ruptura no topo, produtos sem consumo no fim.
func ordenarPorUrgencia(itens []dominio.PrevisaoProduto) {
	prazo := func(item dominio.PrevisaoProduto) float64 {
		if item.DiasAteRuptura == nil {
			return math.Inf(1)
		}
		return *item.DiasAteRuptura
	}

	for i := 1; i < len(itens); i++ {
		atual := itens[i]
		j := i - 1
		for j >= 0 && prazo(itens[j]) > prazo(atual) {
			itens[j+1] = itens[j]
			j--
		}
		itens[j+1] = atual
	}
}

func ajustarJanela(dias int) int {
	switch {
	case dias < janelaMinima:
		return janelaPadrao
	case dias > janelaMaxima:
		return janelaMaxima
	default:
		return dias
	}
}

func arredondar(valor float64, casas int) float64 {
	fator := math.Pow(10, float64(casas))
	return math.Round(valor*fator) / fator
}
