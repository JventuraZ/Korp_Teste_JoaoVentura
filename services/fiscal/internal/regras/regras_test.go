package regras

import (
	"strings"
	"testing"

	"github.com/joaoventura/korp-fiscal/internal/dominio"
)

func txt(v string) *string { return &v }
func log(v bool) *bool     { return &v }

func venda() dominio.ContextoOperacao {
	return dominio.ContextoOperacao{
		Operacao:         "VENDA",
		UFOrigem:         "SP",
		UFDestino:        "RJ",
		TipoCliente:      "PJ",
		ConsumidorFinal:  false,
		ContribuinteICMS: true,
		RegimeEmpresa:    "SIMPLES",
		Finalidade:       "NORMAL",
	}
}

func regra(id, descricao string, prioridade int, cond dominio.Condicoes) dominio.RegraTributaria {
	return dominio.RegraTributaria{
		ID: id, Descricao: descricao, Prioridade: prioridade, Ativa: true, Condicoes: cond,
	}
}

func TestEspecificidadeContaCondicoesFixadas(t *testing.T) {
	casos := []struct {
		nome     string
		cond     dominio.Condicoes
		esperado int
	}{
		{"regra geral", dominio.Condicoes{}, 0},
		{"so operacao", dominio.Condicoes{Operacao: txt("VENDA")}, 1},
		{"operacao e destino", dominio.Condicoes{Operacao: txt("VENDA"), UFDestino: txt("RJ")}, 2},
		{"com booleano", dominio.Condicoes{Operacao: txt("VENDA"), ConsumidorFinal: log(true)}, 2},
	}
	for _, caso := range casos {
		if got := Especificidade(caso.cond); got != caso.esperado {
			t.Errorf("%s: veio %d, esperado %d", caso.nome, got, caso.esperado)
		}
	}
}

// Curinga e o mecanismo que permite escrever a regra geral da empresa sem
// enumerar 27 estados.
func TestCondicaoNulaCasaComQualquerValor(t *testing.T) {
	if casou, _ := Casa(dominio.Condicoes{}, venda()); !casou {
		t.Fatal("regra sem condicoes deveria casar com qualquer operacao")
	}
}

func TestCondicaoFixadaSoCasaComOValorIgual(t *testing.T) {
	if casou, _ := Casa(dominio.Condicoes{UFDestino: txt("MG")}, venda()); casou {
		t.Fatal("regra para MG nao pode casar com operacao destinada ao RJ")
	}
	casou, condicoes := Casa(dominio.Condicoes{UFDestino: txt("RJ")}, venda())
	if !casou {
		t.Fatal("regra para RJ deveria casar")
	}
	if len(condicoes) != 1 || !strings.Contains(condicoes[0], "RJ") {
		t.Errorf("trilha deveria citar a condicao casada, veio %v", condicoes)
	}
}

// Booleano false e diferente de ausente: "nao e consumidor final" e uma
// condicao, nao a falta de uma.
func TestBooleanoFalsoNaoEhCuringa(t *testing.T) {
	cond := dominio.Condicoes{ConsumidorFinal: log(true)}
	if casou, _ := Casa(cond, venda()); casou {
		t.Fatal("regra para consumidor final nao pode casar com operacao que nao e")
	}
}

// O criterio principal: quem descreve melhor o caso vence, sem ninguem
// precisar ordenar regras a mao.
func TestRegraMaisEspecificaVence(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("geral", "Regra geral de venda", 10, dominio.Condicoes{Operacao: txt("VENDA")}),
		regra("rj", "Venda para o RJ", 1, dominio.Condicoes{
			Operacao: txt("VENDA"), UFDestino: txt("RJ")}),
	}

	avaliacao := Avaliar(todas, venda())

	if !avaliacao.Encontrou {
		t.Fatal("deveria ter encontrado regra")
	}
	if avaliacao.Aplicada.ID != "rj" {
		t.Fatalf("aplicou %q; a regra mais especifica deveria vencer mesmo com prioridade menor",
			avaliacao.Aplicada.ID)
	}
}

func TestPrioridadeDesempataMesmaEspecificidade(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("a", "Alternativa A", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
		regra("b", "Alternativa B", 9, dominio.Condicoes{TipoCliente: txt("PJ")}),
	}

	if aplicada := Avaliar(todas, venda()).Aplicada; aplicada.ID != "b" {
		t.Fatalf("aplicou %q, esperado a de maior prioridade", aplicada.ID)
	}
}

func TestRegraInativaEhIgnorada(t *testing.T) {
	inativa := regra("rj", "Venda para o RJ", 99, dominio.Condicoes{UFDestino: txt("RJ")})
	inativa.Ativa = false

	todas := []dominio.RegraTributaria{
		regra("geral", "Regra geral", 1, dominio.Condicoes{}),
		inativa,
	}

	if aplicada := Avaliar(todas, venda()).Aplicada; aplicada.ID != "geral" {
		t.Fatalf("aplicou %q; regra inativa nao pode participar", aplicada.ID)
	}
}

func TestSemRegraCorrespondenteExplicaOMotivo(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("mg", "Venda para MG", 1, dominio.Condicoes{UFDestino: txt("MG")}),
	}

	avaliacao := Avaliar(todas, venda())

	if avaliacao.Encontrou || avaliacao.Aplicada != nil {
		t.Fatal("nao deveria ter encontrado regra")
	}
	if len(avaliacao.Trilha) == 0 {
		t.Fatal("a ausencia de regra tambem precisa ser explicada ao usuario")
	}
}

// A trilha e requisito de produto ("Como chegamos a este calculo?"), entao
// precisa dizer POR QUE a regra venceu, nao apenas qual venceu.
func TestTrilhaExplicaAEscolha(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("geral", "Regra geral de venda", 10, dominio.Condicoes{Operacao: txt("VENDA")}),
		regra("rj", "Venda para o RJ", 1, dominio.Condicoes{
			Operacao: txt("VENDA"), UFDestino: txt("RJ")}),
	}

	trilha := strings.Join(Avaliar(todas, venda()).Trilha, " ")

	if !strings.Contains(trilha, "Venda para o RJ") {
		t.Error("a trilha deveria nomear a regra aplicada")
	}
	if !strings.Contains(trilha, "precisão") {
		t.Errorf("a trilha deveria explicar que venceu por especificidade: %s", trilha)
	}
}

func TestCandidatasMarcamAEscolhida(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("geral", "Regra geral", 1, dominio.Condicoes{}),
		regra("rj", "Venda para o RJ", 1, dominio.Condicoes{UFDestino: txt("RJ")}),
	}

	avaliacao := Avaliar(todas, venda())

	if len(avaliacao.Candidatas) != 2 {
		t.Fatalf("as duas regras casam; vieram %d candidatas", len(avaliacao.Candidatas))
	}
	if !avaliacao.Candidatas[0].Escolhida || avaliacao.Candidatas[1].Escolhida {
		t.Error("exatamente a primeira candidata deveria estar marcada como escolhida")
	}
}

// ── conflitos ───────────────────────────────────────────────────────────

func TestRegrasComValoresDiferentesNaoSobrepoem(t *testing.T) {
	sp := dominio.Condicoes{UFDestino: txt("SP")}
	rj := dominio.Condicoes{UFDestino: txt("RJ")}

	if Sobrepoe(sp, rj) {
		t.Fatal("regras para estados diferentes nunca disputam a mesma operacao")
	}
}

func TestCuringaSobrepoeQualquerRegra(t *testing.T) {
	if !Sobrepoe(dominio.Condicoes{}, dominio.Condicoes{UFDestino: txt("SP")}) {
		t.Fatal("regra geral se sobrepoe a qualquer regra especifica")
	}
}

func TestMesmaEspecificidadeEPrioridadeEhAmbiguo(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("a", "Venda para o RJ", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
		regra("b", "Venda para PJ", 5, dominio.Condicoes{TipoCliente: txt("PJ")}),
	}

	conflitos := DetectarConflitos(todas)

	if len(conflitos) != 1 {
		t.Fatalf("esperava 1 conflito, veio %d", len(conflitos))
	}
	if conflitos[0].Severidade != Ambigua {
		t.Errorf("severidade %s, esperado %s", conflitos[0].Severidade, Ambigua)
	}
}

func TestPrioridadeDiferenteEhApenasSobreposicao(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("a", "Venda para o RJ", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
		regra("b", "Venda para PJ", 8, dominio.Condicoes{TipoCliente: txt("PJ")}),
	}

	conflitos := DetectarConflitos(todas)

	if len(conflitos) != 1 || conflitos[0].Severidade != Sobreposta {
		t.Fatalf("esperava uma sobreposicao resolvida por prioridade, veio %+v", conflitos)
	}
}

// Especificidades diferentes NAO sao conflito: e o funcionamento normal do
// motor. Denunciar isso encheria o painel de alarme falso.
func TestEspecificidadeDiferenteNaoEhConflito(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("geral", "Regra geral", 5, dominio.Condicoes{}),
		regra("rj", "Venda para o RJ", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
	}

	if conflitos := DetectarConflitos(todas); len(conflitos) != 0 {
		t.Fatalf("nao deveria acusar conflito, veio %+v", conflitos)
	}
}

func TestRegraInativaNaoGeraConflito(t *testing.T) {
	inativa := regra("b", "Venda para PJ", 5, dominio.Condicoes{TipoCliente: txt("PJ")})
	inativa.Ativa = false

	todas := []dominio.RegraTributaria{
		regra("a", "Venda para o RJ", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
		inativa,
	}

	if conflitos := DetectarConflitos(todas); len(conflitos) != 0 {
		t.Fatalf("regra inativa nao disputa nada, veio %+v", conflitos)
	}
}

func TestConflitoExplicaEmLinguagemSimples(t *testing.T) {
	todas := []dominio.RegraTributaria{
		regra("a", "Venda para o RJ", 5, dominio.Condicoes{UFDestino: txt("RJ")}),
		regra("b", "Venda para PJ", 5, dominio.Condicoes{TipoCliente: txt("PJ")}),
	}

	explicacao := DetectarConflitos(todas)[0].Explicacao

	if !strings.Contains(explicacao, "Venda para o RJ") || !strings.Contains(explicacao, "prioridade") {
		t.Errorf("a explicacao precisa nomear as regras e dizer o que fazer: %s", explicacao)
	}
}
