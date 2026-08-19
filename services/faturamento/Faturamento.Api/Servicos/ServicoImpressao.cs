using Faturamento.Api.Dados;
using Faturamento.Api.Dominio;
using Faturamento.Api.Estoque;
using Faturamento.Api.Web;
using Microsoft.EntityFrameworkCore;

namespace Faturamento.Api.Servicos;

/// <summary>
/// A saga de impressao da nota fiscal.
///
/// Dois bancos separados significam que nao existe transacao distribuida: o
/// debito do Estoque commita num banco e o fechamento da nota noutro. A saga
/// resolve isso com ordem deliberada e uma compensacao:
///
///   0. grava a chave de idempotencia ANTES de qualquer chamada externa;
///   1. debita o Estoque (passo remoto, idempotente por causa da chave);
///   2. fecha a nota localmente;
///   3. se o passo 2 falhar, estorna o passo 1.
///
/// A ordem importa. Se a nota fosse fechada primeiro e o Estoque falhasse,
/// existiria nota fechada sem baixa -- e nada a compensar do outro lado.
/// </summary>
public class ServicoImpressao(
    ContextoFaturamento db,
    IClienteEstoque estoque,
    ILogger<ServicoImpressao> log)
{
    /// <summary>
    /// COMO CHEGA AQUI: clique em "Imprimir nota" na tela
    ///   -> detalhe-nota.ts::imprimir()
    ///   -> POST /api/notas/{id}/impressao   (NotasEndpoints)
    ///   -> AQUI
    ///
    /// PARA ONDE VAI: o passo 1 abaixo chama o servico de Estoque (Go), que
    /// executa Repositorio.AplicarBaixa -- e la que o saldo muda de fato.
    ///
    /// Qualquer ErroDeNegocio lancado aqui sobe ate o ManipuladorDeErros, que o
    /// converte em problem+json. Este metodo nunca escolhe status HTTP.
    /// </summary>
    public async Task<NotaDetalhe> ImprimirAsync(Guid notaId, CancellationToken ct)
    {
        var nota = await db.Notas
            .Include(nota => nota.Itens)
            .FirstOrDefaultAsync(nota => nota.Id == notaId, ct)
            ?? throw ErroDeNegocio.NotaNaoEncontrada(notaId);

        if (nota.Status != StatusNota.Aberta)
        {
            throw ErroDeNegocio.NotaNaoAberta(nota.NumeroFormatado);
        }
        if (nota.Itens.Count == 0)
        {
            throw ErroDeNegocio.NotaSemItens(nota.NumeroFormatado);
        }

        var chave = await GarantirChaveAsync(nota, ct);

        var itens = nota.Itens
            .GroupBy(item => item.Codigo)
            .Select(grupo => new ItemBaixa(grupo.Key, grupo.Sum(item => item.Quantidade)))
            .OrderBy(item => item.Codigo)
            .ToList();

        var baixa = await estoque.AplicarBaixaAsync(
            chave, new RequisicaoBaixa(nota.NumeroFormatado, itens), ct);

        log.LogInformation("baixa aplicada para {Nota} com a chave {Chave}",
            nota.NumeroFormatado, chave);

        try
        {
            nota.Status = StatusNota.Fechada;
            nota.FechadaEm = DateTimeOffset.UtcNow;

            await db.SaveChangesAsync(ct);
        }
        catch (Exception erro)
        {
            log.LogError(erro, "fechamento da nota {Nota} falhou; compensando a baixa {Chave}",
                nota.NumeroFormatado, chave);

            using var prazoCompensacao = new CancellationTokenSource(TimeSpan.FromSeconds(15));

            await CompensarAsync(nota, chave, prazoCompensacao.Token);
            throw ErroDeNegocio.FalhaAoFecharNota(nota.NumeroFormatado, erro);
        }

        log.LogInformation("nota {Nota} fechada; {Itens} item(ns) baixados",
            nota.NumeroFormatado, baixa.Itens.Count);

        return nota.ParaDetalhe();
    }

    /// <summary>
    /// A chave e persistida antes da chamada remota e reaproveitada em toda
    /// nova tentativa desta nota. E o que faz o retry ser seguro: o Estoque
    /// reconhece a repeticao e devolve a resposta original em vez de debitar
    /// duas vezes.
    /// </summary>
    private async Task<Guid> GarantirChaveAsync(NotaFiscal nota, CancellationToken ct)
    {
        if (nota.ChaveIdempotencia is Guid existente)
        {
            return existente;
        }

        nota.ChaveIdempotencia = Guid.NewGuid();
        await db.SaveChangesAsync(ct);
        return nota.ChaveIdempotencia.Value;
    }

    /// <summary>
    /// Compensacao best-effort. Se ela tambem falhar, o erro vai para o log com
    /// a chave: o estorno e idempotente, entao reprocessar depois e seguro.
    /// </summary>
    private async Task CompensarAsync(NotaFiscal nota, Guid chave, CancellationToken ct)
    {
        try
        {
            await estoque.EstornarAsync(chave, ct);
            log.LogInformation("baixa {Chave} estornada com sucesso", chave);

            db.Entry(nota).State = EntityState.Detached;
            await db.Notas
                .Where(atual => atual.Id == nota.Id)
                .ExecuteUpdateAsync(atualizacao => atualizacao
                    .SetProperty(atual => atual.ChaveIdempotencia, (Guid?)null)
                    .SetProperty(atual => atual.Status, StatusNota.Aberta)
                    .SetProperty(atual => atual.FechadaEm, (DateTimeOffset?)null), ct);
        }
        catch (Exception erro)
        {
            log.LogCritical(erro,
                "COMPENSACAO FALHOU para a nota {Nota}: o estoque segue debitado pela chave {Chave}",
                nota.NumeroFormatado, chave);
        }
    }
}
