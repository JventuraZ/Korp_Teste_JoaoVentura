using Faturamento.Api.Dados;
using Faturamento.Api.Dominio;
using Faturamento.Api.Estoque;
using Faturamento.Api.Servicos;
using Faturamento.Api.Web;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace Faturamento.Testes;

public class TestesSagaImpressao
{
    private static ServicoImpressao Impressao(ContextoFaturamento db, EstoqueFalso estoque)
        => new(db, estoque, NullLogger<ServicoImpressao>.Instance);

    private static ServicoNotas Notas(ContextoFaturamento db, EstoqueFalso estoque)
        => new(db, estoque);

    private static async Task<NotaFiscal> NotaComItemAsync(
        ContextoFaturamento db, string codigo = "PRD-001", int quantidade = 2)
    {
        var nota = new NotaFiscal();
        nota.Itens.Add(new ItemNota { Codigo = codigo, Descricao = "Produto " + codigo, Quantidade = quantidade });

        db.Notas.Add(nota);
        await db.SaveChangesAsync();
        return nota;
    }

    [FatoComBanco]
    public async Task Impressao_fecha_a_nota_e_debita_o_estoque()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso();
        var nota = await NotaComItemAsync(db);

        var resultado = await Impressao(db, estoque).ImprimirAsync(nota.Id, default);

        Assert.Equal(nameof(StatusNota.Fechada), resultado.Status);
        Assert.Single(estoque.BaixasAplicadas);
        Assert.Empty(estoque.EstornosSolicitados);

        // A referencia enviada ao Estoque e o numero da nota: e o que liga a
        // trilha de auditoria de um servico ao outro.
        Assert.Equal(nota.NumeroFormatado, estoque.RequisicoesRecebidas[0].Referencia);
    }

    [FatoComBanco]
    public async Task Nota_ja_fechada_nao_pode_ser_impressa_de_novo()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso();
        var nota = await NotaComItemAsync(db);

        await Impressao(db, estoque).ImprimirAsync(nota.Id, default);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        // 409 e conflito de estado da NOTA -- diferente do 422 de saldo.
        Assert.Equal(409, erro.Status);
        Assert.Equal("nota-nao-aberta", erro.Tipo);
        Assert.Single(estoque.BaixasAplicadas);
    }

    [FatoComBanco]
    public async Task Nota_sem_itens_nao_pode_ser_impressa()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso();

        var nota = new NotaFiscal();
        db.Notas.Add(nota);
        await db.SaveChangesAsync();

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        Assert.Equal(422, erro.Status);
        Assert.Empty(estoque.BaixasAplicadas);
    }

    /// <summary>
    /// O 409 do Estoque chega aqui ja traduzido para 422. O ponto do teste e o
    /// efeito colateral: a nota NAO pode fechar.
    /// </summary>
    [FatoComBanco]
    public async Task Saldo_insuficiente_mantem_a_nota_aberta()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso
        {
            ErroNaBaixa = ErroDeNegocio.SaldoInsuficiente(
                "1 item não possui saldo suficiente para a baixa",
                new[] { new { codigo = "PRD-001", quantidadeSolicitada = 5, saldoDisponivel = 1 } })
        };

        var nota = await NotaComItemAsync(db);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        Assert.Equal(422, erro.Status);
        Assert.True(erro.Extensoes.ContainsKey("itensInsuficientes"));

        await using var conferencia = BancoDeTeste.Novo(limpar: false);
        var salva = await conferencia.Notas.FindAsync(nota.Id);
        Assert.Equal(StatusNota.Aberta, salva!.Status);
    }

    [FatoComBanco]
    public async Task Estoque_indisponivel_mantem_a_nota_aberta()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso { ErroNaBaixa = ErroDeNegocio.EstoqueIndisponivel() };
        var nota = await NotaComItemAsync(db);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        Assert.Equal(503, erro.Status);

        await using var conferencia = BancoDeTeste.Novo(limpar: false);
        var salva = await conferencia.Notas.FindAsync(nota.Id);
        Assert.Equal(StatusNota.Aberta, salva!.Status);
    }

    /// <summary>
    /// A chave e gravada ANTES da chamada remota. Se ela so fosse gerada apos a
    /// resposta, uma falha de rede no meio deixaria a nota sem saber qual baixa
    /// foi (ou nao foi) aplicada -- e o retry debitaria de novo.
    /// </summary>
    [FatoComBanco]
    public async Task Chave_e_persistida_antes_da_chamada_e_reaproveitada_no_retry()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso { ErroNaBaixa = ErroDeNegocio.EstoqueIndisponivel() };
        var nota = await NotaComItemAsync(db);

        await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        await using var apos = BancoDeTeste.Novo(limpar: false);
        var chaveGravada = (await apos.Notas.FindAsync(nota.Id))!.ChaveIdempotencia;
        Assert.NotNull(chaveGravada);

        // Segunda tentativa, agora com o Estoque de volta: mesma chave.
        estoque.ErroNaBaixa = null;
        await Impressao(db, estoque).ImprimirAsync(nota.Id, default);

        Assert.Equal(chaveGravada, Assert.Single(estoque.BaixasAplicadas));
    }

    /// <summary>
    /// O unico caminho que a saga nao consegue evitar: o Estoque commitou e o
    /// fechamento local falhou depois. Sem a compensacao, o estoque ficaria
    /// debitado por uma nota que continua aberta.
    /// </summary>
    [FatoComBanco]
    public async Task Falha_ao_fechar_dispara_o_estorno_compensatorio()
    {
        await using var preparo = BancoDeTeste.Novo();
        var nota = await NotaComItemAsync(preparo);

        // Chave pre-gravada para que o unico SaveChanges da saga seja o
        // fechamento -- que e justamente o que este contexto faz falhar.
        nota.ChaveIdempotencia = Guid.NewGuid();
        await preparo.SaveChangesAsync();

        await using var db = new ContextoQueFalhaAoSalvar(BancoDeTeste.Opcoes());
        var estoque = new EstoqueFalso();

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        Assert.Equal("falha-ao-fechar-nota", erro.Tipo);

        // A baixa foi aplicada e depois desfeita pela compensacao.
        Assert.Single(estoque.BaixasAplicadas);
        Assert.Equal(estoque.BaixasAplicadas[0], Assert.Single(estoque.EstornosSolicitados));

        // E a nota voltou a ser imprimivel, sem chave pendente.
        await using var conferencia = BancoDeTeste.Novo(limpar: false);
        var salva = await conferencia.Notas.FindAsync(nota.Id);
        Assert.Equal(StatusNota.Aberta, salva!.Status);
        Assert.Null(salva.ChaveIdempotencia);
    }

    /// <summary>
    /// Editar os itens invalida a chave pendente. Sem isso, uma nota recusada
    /// por saldo ficaria travada: corpo novo com chave velha e conflito.
    /// </summary>
    [FatoComBanco]
    public async Task Editar_itens_invalida_a_chave_pendente()
    {
        await using var db = BancoDeTeste.Novo();
        var estoque = new EstoqueFalso { ErroNaBaixa = ErroDeNegocio.EstoqueIndisponivel() };
        estoque.Cadastrar("PRD-002", "Porca sextavada", 50);

        var nota = await NotaComItemAsync(db);
        await Assert.ThrowsAsync<ErroDeNegocio>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, default));

        Assert.NotNull(nota.ChaveIdempotencia);

        await Notas(db, estoque).AdicionarItemAsync(
            nota.Id, new NovoItem("PRD-002", 3), default);

        await using var conferencia = BancoDeTeste.Novo(limpar: false);
        var salva = await conferencia.Notas.FindAsync(nota.Id);
        Assert.Null(salva!.ChaveIdempotencia);
    }

    /// <summary>
    /// A compensacao NAO pode herdar o cancelamento que causou a falha.
    ///
    /// Cenario real: o usuario clica em Imprimir e fecha a aba. O ASP.NET Core
    /// cancela o token da requisicao. Se isso acontece depois de o Estoque
    /// commitar, a compensacao e a UNICA coisa que ainda precisa rodar -- e
    /// justamente ela seria abortada se reutilizasse o token cancelado,
    /// deixando o estoque debitado por uma nota que continua aberta.
    /// </summary>
    [FatoComBanco]
    public async Task Compensacao_roda_mesmo_com_a_requisicao_cancelada()
    {
        await using var db = BancoDeTeste.Novo();
        var nota = await NotaComItemAsync(db);

        // Chave pre-gravada para que o unico SaveChanges da saga seja o
        // fechamento -- que e o passo que o cancelamento vai atingir.
        nota.ChaveIdempotencia = Guid.NewGuid();
        await db.SaveChangesAsync();

        using var requisicao = new CancellationTokenSource();
        var estoque = new EstoqueFalso
        {
            // O cancelamento acontece EXATAMENTE entre o commit do Estoque e o
            // fechamento da nota -- o usuario fechando a aba no pior instante
            // possivel. Cancelar antes da chamada faria o EF abortar na
            // primeira consulta e a saga nem comecaria.
            AoAplicarBaixa = (chave, req) =>
            {
                requisicao.Cancel();
                return new ResultadoBaixa(
                    chave.ToString(), req.Referencia, DateTimeOffset.UtcNow,
                    req.Itens.Select(item =>
                        new ItemMovimentado(item.Codigo, item.Quantidade, 10, 10 - item.Quantidade)).ToList());
            }
        };

        await Assert.ThrowsAnyAsync<Exception>(
            () => Impressao(db, estoque).ImprimirAsync(nota.Id, requisicao.Token));

        // O estoque foi debitado; a compensacao PRECISA ter rodado apesar do
        // cancelamento, senao sobra saldo debitado por uma nota que segue aberta.
        Assert.Single(estoque.BaixasAplicadas);
        Assert.Equal(estoque.BaixasAplicadas[0], Assert.Single(estoque.EstornosSolicitados));
    }

    /// <summary>Contexto que falha no fechamento, ja com a chave persistida.</summary>
    private sealed class ContextoQueFalhaAoSalvar(DbContextOptions<ContextoFaturamento> opcoes)
        : ContextoFaturamento(opcoes)
    {
        public override Task<int> SaveChangesAsync(CancellationToken ct = default)
            => throw new DbUpdateException("falha simulada ao fechar a nota");

        public override Task<int> SaveChangesAsync(bool aceitarTudo, CancellationToken ct = default)
            => throw new DbUpdateException("falha simulada ao fechar a nota");
    }
}
