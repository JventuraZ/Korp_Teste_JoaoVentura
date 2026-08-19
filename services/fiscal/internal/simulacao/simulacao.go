// Package simulacao monta a previa de tributacao de uma operacao.
//
// LIMITE IMPORTANTE, e ele aparece na resposta e na tela: esta previa aplica
// aritmetica simples sobre as aliquotas QUE O USUARIO CADASTROU na regra
// selecionada. Ela nao implementa a legislacao.
//
// Fica de fora, entre outras coisas: composicao da base do ICMS-ST com MVA,
// DIFAL e partilha entre estados, exclusao do ICMS da base de PIS/COFINS,
// regimes monofasicos, beneficios regionais e o IPI na base do ICMS-ST.
//
// Um motor tributario de producao trata tudo isso. O objetivo aqui e mostrar
// COMO a regra escolhida se traduz em numeros, e sobretudo POR QUE aquela
// regra foi escolhida -- que e a pergunta que o operador do ERP faz.
package simulacao

import (
	"github.com/joaoventura/korp-fiscal/internal/dominio"
	"github.com/joaoventura/korp-fiscal/internal/regras"
)

const AvisoSimulacao = "Prévia simplificada: aplica as alíquotas cadastradas na regra, " +
	"sem DIFAL, composição de base de ST por MVA, exclusões de base ou regimes especiais. " +
	"Não utilize para apuração ou emissão de documento fiscal."

type Pedido struct {
	Produto  string                    `json:"produto"`
	Contexto dominio.ContextoOperacao  `json:"contexto"`
	Valor    float64                   `json:"valor"`
	Quantidade float64                 `json:"quantidade"`
}

type Tributo struct {
	Nome        string   `json:"nome"`
	Situacao    string   `json:"situacao"`
	BaseCalculo *float64 `json:"baseCalculo"`
	Aliquota    *float64 `json:"aliquota"`
	Valor       *float64 `json:"valor"`
	Observacao  string   `json:"observacao"`
}

type Resultado struct {
	Encontrou     bool             `json:"encontrou"`
	Aviso         string           `json:"aviso"`
	CFOP          string           `json:"cfop"`
	CSTouCSOSN    string           `json:"cstOuCsosn"`
	ValorOperacao float64          `json:"valorOperacao"`
	Tributos      []Tributo        `json:"tributos"`
	TotalTributos float64          `json:"totalTributos"`
	Trilha        []string         `json:"trilha"`
	Candidatas    []regras.Candidata `json:"candidatas"`
}

func Simular(lista []dominio.RegraTributaria, pedido Pedido) Resultado {
	avaliacao := regras.Avaliar(lista, pedido.Contexto)

	resultado := Resultado{
		Aviso:         AvisoSimulacao,
		ValorOperacao: pedido.Valor,
		Trilha:        avaliacao.Trilha,
		Candidatas:    avaliacao.Candidatas,
		Tributos:      []Tributo{},
	}

	if !avaliacao.Encontrou {
		return resultado
	}

	resultado.Encontrou = true
	regra := avaliacao.Aplicada.Resultado
	resultado.CFOP = regra.CFOP

	resultado.CSTouCSOSN = regra.CSTICMS
	if resultado.CSTouCSOSN == "" {
		resultado.CSTouCSOSN = regra.CSOSN
	}

	base := pedido.Valor

	adicionar := func(nome, situacao string, aliquota *float64, obs string) {
		tributo := Tributo{Nome: nome, Situacao: situacao, Observacao: obs}
		if aliquota != nil {
			valor := arredondar(base * *aliquota / 100)
			baseCopia := base
			tributo.BaseCalculo = &baseCopia
			tributo.Aliquota = aliquota
			tributo.Valor = &valor
			resultado.TotalTributos += valor
		}
		resultado.Tributos = append(resultado.Tributos, tributo)
	}

	// A base do ICMS considera a reducao cadastrada, quando houver. E o unico
	// ajuste de base aplicado: os demais dependem de legislacao.
	baseIcms := base
	observacaoIcms := ""
	if regra.ReducaoBaseICMS != nil && *regra.ReducaoBaseICMS > 0 {
		baseIcms = arredondar(base * (1 - *regra.ReducaoBaseICMS/100))
		observacaoIcms = "Base reduzida conforme percentual cadastrado na regra."
	}

	if regra.AliquotaICMS != nil {
		valor := arredondar(baseIcms * *regra.AliquotaICMS / 100)
		baseCopia := baseIcms
		resultado.Tributos = append(resultado.Tributos, Tributo{
			Nome: "ICMS", Situacao: resultado.CSTouCSOSN,
			BaseCalculo: &baseCopia, Aliquota: regra.AliquotaICMS, Valor: &valor,
			Observacao: observacaoIcms,
		})
		resultado.TotalTributos += valor
	} else {
		resultado.Tributos = append(resultado.Tributos, Tributo{
			Nome: "ICMS", Situacao: resultado.CSTouCSOSN,
			Observacao: "A regra não define alíquota — pode ser operação não tributada ou com ICMS já retido.",
		})
	}

	adicionar("FCP", "", regra.AliquotaFCP, "")
	adicionar("ICMS-ST", "", regra.AliquotaICMSST,
		"Cálculo simplificado: não compõe a base por MVA.")
	adicionar("PIS", regra.CSTPIS, regra.AliquotaPIS, "")
	adicionar("COFINS", regra.CSTCOFINS, regra.AliquotaCOFINS, "")
	adicionar("IPI", regra.CSTIPI, regra.AliquotaIPI, "")

	resultado.TotalTributos = arredondar(resultado.TotalTributos)
	return resultado
}

func arredondar(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
