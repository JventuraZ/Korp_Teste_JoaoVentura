package transporte

import (
	"strings"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
	"github.com/joaoventura/korp-fiscal/internal/validacao"
)

// resumir monta o semaforo do cabecalho da aba fiscal.
//
// O status e sempre DERIVADO da configuracao e da validacao -- nunca um campo
// que alguem marca a mao. Status editavel envelhece: fica verde enquanto a
// configuracao apodrece embaixo.
func resumir(cfg dominio.ConfiguracaoFiscal, regras []dominio.RegraTributaria) dominio.ResumoFiscal {
	resultado := validacao.Validar(cfg, regras)

	// Agrupa a gravidade mais alta encontrada em cada area.
	pior := map[string]dominio.Situacao{}
	for _, problema := range resultado.Problemas {
		atual := pior[problema.Area]
		if problema.Gravidade == validacao.Erro {
			pior[problema.Area] = dominio.ComErro
		} else if atual != dominio.ComErro {
			pior[problema.Area] = dominio.Incompleto
		}
	}

	situacaoDe := func(area string) dominio.Situacao {
		if s, ok := pior[area]; ok {
			return s
		}
		return dominio.Configurado
	}

	ativas := 0
	for _, regra := range regras {
		if regra.Ativa {
			ativas++
		}
	}

	areas := map[string]string{
		"Classificação fiscal": string(situacaoDe("Classificação fiscal")),
		"PIS/COFINS":           string(situacaoDe("PIS/COFINS")),
		"Regras por operação":  string(situacaoDe("Regras por operação")),
	}

	// IPI sem CST nao e erro: ha produtos fora do campo de incidencia. A tela
	// mostra "não aplicável" em vez de acusar pendencia falsa.
	if strings.TrimSpace(cfg.IPI.CSTIPI) == "" {
		areas["IPI"] = string(dominio.NaoAplicavel)
	} else {
		areas["IPI"] = string(situacaoDe("IPI"))
	}

	geral := dominio.Configurado
	for _, situacao := range areas {
		switch dominio.Situacao(situacao) {
		case dominio.ComErro:
			geral = dominio.ComErro
		case dominio.Incompleto:
			if geral != dominio.ComErro {
				geral = dominio.Incompleto
			}
		}
	}

	return dominio.ResumoFiscal{
		Situacao:     geral,
		Areas:        areas,
		TotalRegras:  len(regras),
		RegrasAtivas: ativas,
	}
}
