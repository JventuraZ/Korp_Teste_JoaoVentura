// Package regras e o motor de selecao de regra tributaria.
//
// Nao calcula imposto e nao conhece legislacao: dado um conjunto de regras
// cadastradas e um contexto de operacao, decide QUAL regra vale e explica por
// que. Toda a logica aqui e deterministica e independente da legislacao, o que
// a torna testavel sem nenhum conhecimento fiscal.
//
// A separacao importa: a legislacao muda o CONTEUDO das regras, nunca este
// algoritmo.
package regras

import (
	"fmt"
	"sort"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
)

// Especificidade conta quantas condicoes a regra fixa.
//
// E o criterio principal de desempate, e a razao e intuitiva: uma regra que
// menciona "venda, para SP, a consumidor final" descreve a situacao com mais
// precisao do que "venda". Quem descreve melhor o caso, vence -- sem precisar
// que alguem ordene as regras manualmente.
func Especificidade(c dominio.Condicoes) int {
	total := 0
	for _, fixado := range []bool{
		c.Operacao != nil, c.UFOrigem != nil, c.UFDestino != nil, c.TipoCliente != nil,
		c.ConsumidorFinal != nil, c.ContribuinteICMS != nil, c.RegimeEmpresa != nil,
		c.Finalidade != nil,
	} {
		if fixado {
			total++
		}
	}
	return total
}

// Casa informa se as condicoes admitem o contexto, e quais condicoes casaram.
//
// A lista devolvida alimenta a trilha de explicacao: o usuario ve "casou por
// operacao e UF de destino", nao um booleano.
func Casa(c dominio.Condicoes, ctx dominio.ContextoOperacao) (bool, []string) {
	var casadas []string

	texto := func(cond *string, valor, rotulo string) bool {
		if cond == nil {
			return true
		}
		if *cond != valor {
			return false
		}
		casadas = append(casadas, fmt.Sprintf("%s = %s", rotulo, valor))
		return true
	}

	logico := func(cond *bool, valor bool, rotulo string) bool {
		if cond == nil {
			return true
		}
		if *cond != valor {
			return false
		}
		casadas = append(casadas, fmt.Sprintf("%s = %s", rotulo, sim(valor)))
		return true
	}

	ok := texto(c.Operacao, ctx.Operacao, "operação") &&
		texto(c.UFOrigem, ctx.UFOrigem, "UF de origem") &&
		texto(c.UFDestino, ctx.UFDestino, "UF de destino") &&
		texto(c.TipoCliente, ctx.TipoCliente, "tipo de cliente") &&
		logico(c.ConsumidorFinal, ctx.ConsumidorFinal, "consumidor final") &&
		logico(c.ContribuinteICMS, ctx.ContribuinteICMS, "contribuinte de ICMS") &&
		texto(c.RegimeEmpresa, ctx.RegimeEmpresa, "regime da empresa") &&
		texto(c.Finalidade, ctx.Finalidade, "finalidade")

	if !ok {
		return false, nil
	}
	return true, casadas
}

// Candidata e uma regra que casou com o contexto.
type Candidata struct {
	Regra            dominio.RegraTributaria `json:"regra"`
	Especificidade   int                     `json:"especificidade"`
	CondicoesCasadas []string                `json:"condicoesCasadas"`
	Escolhida        bool                    `json:"escolhida"`
}

// Avaliacao e o resultado completo, com a explicacao junto.
//
// A trilha e TIPO DE RETORNO, nao log: o item "Como chegamos a este calculo?"
// e requisito de produto, e requisito de produto que depende de vasculhar log
// nao sobrevive ao primeiro refactor.
type Avaliacao struct {
	Encontrou  bool                     `json:"encontrou"`
	Aplicada   *dominio.RegraTributaria `json:"aplicada"`
	Candidatas []Candidata              `json:"candidatas"`
	Trilha     []string                 `json:"trilha"`
}

// Avaliar seleciona a regra que vale para o contexto.
//
// Ordem de decisao: maior especificidade primeiro; empatou, maior prioridade;
// empatou de novo, a primeira em ordem de id -- para o resultado ser estavel
// entre execucoes, ainda que a situacao seja ambigua (e DetectarConflitos
// existe justamente para denunciar esse caso ao usuario).
func Avaliar(todas []dominio.RegraTributaria, ctx dominio.ContextoOperacao) Avaliacao {
	avaliacao := Avaliacao{Candidatas: []Candidata{}, Trilha: []string{}}

	for _, regra := range todas {
		if !regra.Ativa {
			continue
		}
		casou, condicoes := Casa(regra.Condicoes, ctx)
		if !casou {
			continue
		}
		avaliacao.Candidatas = append(avaliacao.Candidatas, Candidata{
			Regra:            regra,
			Especificidade:   Especificidade(regra.Condicoes),
			CondicoesCasadas: condicoes,
		})
	}

	if len(avaliacao.Candidatas) == 0 {
		avaliacao.Trilha = append(avaliacao.Trilha,
			"Nenhuma regra ativa corresponde a esta operação.")
		return avaliacao
	}

	sort.SliceStable(avaliacao.Candidatas, func(i, j int) bool {
		a, b := avaliacao.Candidatas[i], avaliacao.Candidatas[j]
		if a.Especificidade != b.Especificidade {
			return a.Especificidade > b.Especificidade
		}
		if a.Regra.Prioridade != b.Regra.Prioridade {
			return a.Regra.Prioridade > b.Regra.Prioridade
		}
		return a.Regra.ID < b.Regra.ID
	})

	avaliacao.Candidatas[0].Escolhida = true
	escolhida := avaliacao.Candidatas[0]
	avaliacao.Encontrou = true
	avaliacao.Aplicada = &escolhida.Regra

	avaliacao.Trilha = montarTrilha(avaliacao.Candidatas)
	return avaliacao
}

func montarTrilha(candidatas []Candidata) []string {
	escolhida := candidatas[0]

	trilha := []string{
		fmt.Sprintf("%d regra(s) ativa(s) correspondem a esta operação.", len(candidatas)),
		fmt.Sprintf("Aplicada: %q.", escolhida.Regra.Descricao),
	}

	if len(escolhida.CondicoesCasadas) == 0 {
		trilha = append(trilha, "Ela é uma regra geral: vale para qualquer operação.")
	} else {
		trilha = append(trilha, fmt.Sprintf("Corresponde por: %s.",
			juntar(escolhida.CondicoesCasadas)))
	}

	if len(candidatas) > 1 {
		segunda := candidatas[1]
		switch {
		case escolhida.Especificidade > segunda.Especificidade:
			trilha = append(trilha, fmt.Sprintf(
				"Preferida a %q porque descreve a operação com mais precisão (%d condições contra %d).",
				segunda.Regra.Descricao, escolhida.Especificidade, segunda.Especificidade))
		case escolhida.Regra.Prioridade > segunda.Regra.Prioridade:
			trilha = append(trilha, fmt.Sprintf(
				"Empate de especificidade com %q; decidido pela prioridade (%d contra %d).",
				segunda.Regra.Descricao, escolhida.Regra.Prioridade, segunda.Regra.Prioridade))
		default:
			trilha = append(trilha, fmt.Sprintf(
				"ATENÇÃO: empate com %q em especificidade e prioridade. Revise as regras.",
				segunda.Regra.Descricao))
		}
	}

	return trilha
}

func sim(v bool) string {
	if v {
		return "sim"
	}
	return "não"
}

func juntar(itens []string) string {
	if len(itens) == 1 {
		return itens[0]
	}
	texto := ""
	for i, item := range itens {
		switch {
		case i == 0:
			texto = item
		case i == len(itens)-1:
			texto += " e " + item
		default:
			texto += ", " + item
		}
	}
	return texto
}
