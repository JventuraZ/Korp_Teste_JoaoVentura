// Package validacao confere a CONSISTENCIA ESTRUTURAL da configuracao fiscal.
//
// Limite deliberado: aqui se verifica o que da para verificar sem conhecer a
// legislacao -- campo obrigatorio ausente, regra sem CFOP, formato invalido,
// regras ambiguas. NAO se verifica se uma aliquota esta correta para um NCM,
// porque isso exigiria a legislacao e qualquer resposta seria invencao.
//
// As mensagens sao escritas para quem opera o ERP, nao para quem escreveu o
// codigo: "A regra de venda interestadual está sem CFOP", nunca "CFOP_NULL".
package validacao

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
	"github.com/joaoventura/korp-fiscal/internal/regras"
)

type Gravidade string

const (
	Erro  Gravidade = "ERRO"
	Aviso Gravidade = "AVISO"
)

type Problema struct {
	Gravidade Gravidade `json:"gravidade"`
	Area      string    `json:"area"`
	Mensagem  string    `json:"mensagem"`
	ComoResolver string `json:"comoResolver"`
}

type Resultado struct {
	Valida    bool       `json:"valida"`
	Problemas []Problema `json:"problemas"`
}

var apenasDigitos = regexp.MustCompile(`^\d+$`)

// cstQueExigemAliquota: CSTs de PIS/COFINS que descrevem operacao tributada e
// portanto precisam de aliquota informada. Conferir CST contra a presenca de
// aliquota e estrutural; qual aliquota usar e que seria legislacao.
var cstPisCofinsTributados = map[string]bool{"01": true, "02": true}

func Validar(cfg dominio.ConfiguracaoFiscal, listaRegras []dominio.RegraTributaria) Resultado {
	problemas := []Problema{}

	problemas = append(problemas, validarClassificacao(cfg)...)
	problemas = append(problemas, validarPisCofins(cfg)...)
	problemas = append(problemas, validarIpi(cfg)...)
	problemas = append(problemas, validarRegras(listaRegras)...)

	temErro := false
	for _, p := range problemas {
		if p.Gravidade == Erro {
			temErro = true
			break
		}
	}
	return Resultado{Valida: !temErro, Problemas: problemas}
}

func validarClassificacao(cfg dominio.ConfiguracaoFiscal) []Problema {
	var problemas []Problema
	area := "Classificação fiscal"

	ncm := strings.TrimSpace(cfg.NCM)
	switch {
	case ncm == "":
		problemas = append(problemas, Problema{Erro, area,
			"O NCM não foi informado.",
			"Informe o NCM do produto — sem ele a NF-e é rejeitada pela SEFAZ."})
	case len(ncm) != 8 || !apenasDigitos.MatchString(ncm):
		problemas = append(problemas, Problema{Erro, area,
			fmt.Sprintf("O NCM %q não tem o formato esperado.", ncm),
			"O NCM deve ter exatamente 8 dígitos."})
	}

	if cest := strings.TrimSpace(cfg.CEST); cest != "" {
		if len(cest) != 7 || !apenasDigitos.MatchString(cest) {
			problemas = append(problemas, Problema{Erro, area,
				fmt.Sprintf("O CEST %q não tem o formato esperado.", cest),
				"O CEST deve ter exatamente 7 dígitos."})
		}
		if ncm == "" {
			problemas = append(problemas, Problema{Erro, area,
				"O CEST foi informado, mas o NCM está vazio.",
				"O CEST só faz sentido acompanhado do NCM correspondente."})
		}
	}

	if strings.TrimSpace(cfg.OrigemMercadoria) == "" {
		problemas = append(problemas, Problema{Erro, area,
			"A origem da mercadoria não foi informada.",
			"Selecione a origem — ela define se o produto é nacional ou importado."})
	}

	if strings.TrimSpace(cfg.UnidadeTributavel) == "" {
		problemas = append(problemas, Problema{Aviso, area,
			"A unidade tributável não foi informada.",
			"Informe a unidade usada na tributação, quando diferente da unidade de venda."})
	}

	return problemas
}

func validarPisCofins(cfg dominio.ConfiguracaoFiscal) []Problema {
	var problemas []Problema
	area := "PIS/COFINS"

	conferir := func(rotulo, cst string, aliquota *float64) {
		if strings.TrimSpace(cst) == "" {
			problemas = append(problemas, Problema{Erro, area,
				fmt.Sprintf("O CST de %s não foi informado.", rotulo),
				fmt.Sprintf("Selecione a situação tributária de %s.", rotulo)})
			return
		}
		if cstPisCofinsTributados[cst] && aliquota == nil {
			problemas = append(problemas, Problema{Erro, area,
				fmt.Sprintf("O CST %s de %s indica operação tributada, mas a alíquota está vazia.", cst, rotulo),
				fmt.Sprintf("Informe a alíquota de %s ou escolha um CST de operação não tributada.", rotulo)})
		}
	}

	conferir("PIS", cfg.PisCofins.CSTPIS, cfg.PisCofins.AliquotaPIS)
	conferir("COFINS", cfg.PisCofins.CSTCOFINS, cfg.PisCofins.AliquotaCOFINS)
	return problemas
}

func validarIpi(cfg dominio.ConfiguracaoFiscal) []Problema {
	var problemas []Problema
	area := "IPI"

	cst := strings.TrimSpace(cfg.IPI.CSTIPI)
	if cst == "" {
		return []Problema{{Aviso, area,
			"O CST de IPI não foi informado.",
			"Se o produto não sofre incidência de IPI, selecione o CST correspondente para deixar isso explícito."}}
	}

	// CST 50 e "saida tributada" -- conferir que ha aliquota OU valor por
	// unidade e estrutural, nao interpretacao da legislacao.
	if cst == "50" && cfg.IPI.AliquotaIPI == nil && cfg.IPI.ValorPorUnidade == nil {
		problemas = append(problemas, Problema{Erro, area,
			"O CST 50 indica saída tributada, mas não há alíquota nem valor por unidade.",
			"Informe a alíquota de IPI ou o valor por unidade."})
	}
	return problemas
}

func validarRegras(lista []dominio.RegraTributaria) []Problema {
	problemas := []Problema{}
	area := "Regras por operação"

	ativas := 0
	for _, regra := range lista {
		if regra.Ativa {
			ativas++
		}
	}

	if ativas == 0 {
		return append(problemas, Problema{Erro, area,
			"Não há nenhuma regra tributária ativa para este produto.",
			"Cadastre ao menos uma regra — sem ela o sistema não sabe qual CFOP e CST usar na emissão."})
	}

	for _, regra := range lista {
		if !regra.Ativa {
			continue
		}
		nome := regra.Descricao
		if nome == "" {
			nome = regra.ID
		}

		if strings.TrimSpace(regra.Resultado.CFOP) == "" {
			problemas = append(problemas, Problema{Erro, area,
				fmt.Sprintf("A regra %q está sem CFOP.", nome),
				"Todo documento fiscal exige CFOP. Informe o código correspondente à operação."})
		}
		if strings.TrimSpace(regra.Resultado.CSTICMS) == "" && strings.TrimSpace(regra.Resultado.CSOSN) == "" {
			problemas = append(problemas, Problema{Erro, area,
				fmt.Sprintf("A regra %q não tem CST nem CSOSN de ICMS.", nome),
				"Informe o CST (regime normal) ou o CSOSN (Simples Nacional)."})
		}
		if strings.TrimSpace(regra.Resultado.CSTICMS) != "" && strings.TrimSpace(regra.Resultado.CSOSN) != "" {
			problemas = append(problemas, Problema{Aviso, area,
				fmt.Sprintf("A regra %q tem CST e CSOSN preenchidos ao mesmo tempo.", nome),
				"Cada regime usa um dos dois. Deixe preenchido apenas o que corresponde ao regime da empresa."})
		}
	}

	for _, conflito := range regras.DetectarConflitos(lista) {
		gravidade := Aviso
		if conflito.Severidade == regras.Ambigua {
			gravidade = Erro
		}
		problemas = append(problemas, Problema{gravidade, area, conflito.Explicacao,
			"Ajuste a prioridade de uma delas ou acrescente uma condição que as diferencie."})
	}

	return problemas
}
