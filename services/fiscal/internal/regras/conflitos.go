package regras

import (
	"fmt"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
)

// Severidade distingue o que quebra do que apenas merece atencao.
type Severidade string

const (
	// Ambigua: mesma especificidade E mesma prioridade, com sobreposicao.
	// Nada no cadastro decide qual vale -- o resultado passa a depender da
	// ordem de leitura. E o unico caso que exige correcao.
	Ambigua Severidade = "AMBIGUA"

	// Sobreposta: mesma especificidade, prioridades diferentes. O sistema
	// resolve pela prioridade, mas vale mostrar: costuma ser sinal de que
	// alguem cadastrou uma excecao sem perceber que ja existia outra.
	Sobreposta Severidade = "SOBREPOSTA"
)

type Conflito struct {
	Severidade Severidade `json:"severidade"`
	RegraA     string     `json:"regraA"`
	RegraB     string     `json:"regraB"`
	Explicacao string     `json:"explicacao"`
}

// Sobrepoe informa se existe algum contexto capaz de casar com as DUAS regras.
//
// Basta um campo em que ambas fixem valores diferentes para que nunca colidam:
// uma regra para SP e outra para RJ jamais disputam a mesma operacao. Onde uma
// delas e curinga, a sobreposicao existe.
func Sobrepoe(a, b dominio.Condicoes) bool {
	textoCompativel := func(x, y *string) bool {
		return x == nil || y == nil || *x == *y
	}
	logicoCompativel := func(x, y *bool) bool {
		return x == nil || y == nil || *x == *y
	}

	return textoCompativel(a.Operacao, b.Operacao) &&
		textoCompativel(a.UFOrigem, b.UFOrigem) &&
		textoCompativel(a.UFDestino, b.UFDestino) &&
		textoCompativel(a.TipoCliente, b.TipoCliente) &&
		logicoCompativel(a.ConsumidorFinal, b.ConsumidorFinal) &&
		logicoCompativel(a.ContribuinteICMS, b.ContribuinteICMS) &&
		textoCompativel(a.RegimeEmpresa, b.RegimeEmpresa) &&
		textoCompativel(a.Finalidade, b.Finalidade)
}

// DetectarConflitos aponta pares de regras ativas que disputam as mesmas
// operacoes sem que a especificidade resolva.
//
// Regras de especificidade DIFERENTE que se sobrepoem nao entram: a mais
// especifica vence deterministicamente, e e para isso que a especificidade
// existe. Denuncia-las seria transformar o funcionamento normal em alarme.
func DetectarConflitos(todas []dominio.RegraTributaria) []Conflito {
	conflitos := []Conflito{}

	for i := 0; i < len(todas); i++ {
		for j := i + 1; j < len(todas); j++ {
			a, b := todas[i], todas[j]

			if !a.Ativa || !b.Ativa {
				continue
			}
			if Especificidade(a.Condicoes) != Especificidade(b.Condicoes) {
				continue
			}
			if !Sobrepoe(a.Condicoes, b.Condicoes) {
				continue
			}

			if a.Prioridade == b.Prioridade {
				conflitos = append(conflitos, Conflito{
					Severidade: Ambigua,
					RegraA:     a.ID,
					RegraB:     b.ID,
					Explicacao: fmt.Sprintf(
						"As regras %q e %q valem para as mesmas operações e têm a mesma prioridade (%d). "+
							"Não há como saber qual deve ser usada: ajuste a prioridade ou torne uma delas mais específica.",
						a.Descricao, b.Descricao, a.Prioridade),
				})
				continue
			}

			vencedora, perdedora := a, b
			if b.Prioridade > a.Prioridade {
				vencedora, perdedora = b, a
			}
			conflitos = append(conflitos, Conflito{
				Severidade: Sobreposta,
				RegraA:     a.ID,
				RegraB:     b.ID,
				Explicacao: fmt.Sprintf(
					"As regras %q e %q valem para as mesmas operações. Prevalece %q, por ter prioridade maior (%d contra %d).",
					a.Descricao, b.Descricao, vencedora.Descricao, vencedora.Prioridade, perdedora.Prioridade),
			})
		}
	}

	return conflitos
}
