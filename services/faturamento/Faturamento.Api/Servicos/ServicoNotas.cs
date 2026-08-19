using Faturamento.Api.Dados;
using Faturamento.Api.Dominio;
using Faturamento.Api.Estoque;
using Faturamento.Api.Web;
using Microsoft.EntityFrameworkCore;

namespace Faturamento.Api.Servicos;

/// <summary>
/// Cadastro de notas fiscais. Nao mexe em saldo: incluir um item na nota nao
/// reserva estoque nenhum. O debito acontece uma unica vez, na impressao.
/// </summary>
public class ServicoNotas(ContextoFaturamento db, IClienteEstoque estoque)
{
    public async Task<PaginaNotas> ListarAsync(int pagina, int tamanho, CancellationToken ct)
    {
        pagina = pagina < 1 ? 1 : pagina;
        tamanho = tamanho is < 1 or > 100 ? 20 : tamanho;

        var total = await db.Notas.CountAsync(ct);

        var notas = await db.Notas
            .AsNoTracking()
            .Include(nota => nota.Itens)
            .OrderByDescending(nota => nota.Numero)
            .Skip((pagina - 1) * tamanho)
            .Take(tamanho)
            .ToListAsync(ct);

        return new PaginaNotas(
            notas.Select(nota => nota.ParaResumo()).ToList(),
            pagina, tamanho, total);
    }

    public async Task<NotaDetalhe> ObterAsync(Guid id, CancellationToken ct)
        => (await CarregarAsync(id, ct, rastrear: false)).ParaDetalhe();

    /// <summary>
    /// Cria a nota vazia, ja com numeracao sequencial e status Aberta.
    /// </summary>
    public async Task<NotaDetalhe> CriarAsync(CancellationToken ct)
    {
        var nota = new NotaFiscal();
        db.Notas.Add(nota);

        await db.SaveChangesAsync(ct);
        return nota.ParaDetalhe();
    }

    public async Task<NotaDetalhe> AdicionarItemAsync(Guid notaId, NovoItem novo, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(novo.Codigo))
        {
            throw ErroDeNegocio.RequisicaoInvalida("O código do produto é obrigatório");
        }
        if (novo.Quantidade <= 0)
        {
            throw ErroDeNegocio.RequisicaoInvalida("A quantidade deve ser maior que zero");
        }

        var nota = await CarregarAsync(notaId, ct);
        GarantirAberta(nota);

        var produto = await estoque.BuscarProdutoAsync(novo.Codigo.Trim(), ct);

        var existente = nota.Itens.FirstOrDefault(item => item.Codigo == produto.Codigo);
        if (existente is not null)
        {
            existente.Quantidade += novo.Quantidade;
        }
        else
        {
            nota.Itens.Add(new ItemNota
            {
                NotaFiscalId = nota.Id,
                Codigo = produto.Codigo,
                Descricao = produto.Descricao,
                Quantidade = novo.Quantidade
            });
        }

        InvalidarChave(nota);
        await db.SaveChangesAsync(ct);
        return nota.ParaDetalhe();
    }

    public async Task<NotaDetalhe> RemoverItemAsync(Guid notaId, Guid itemId, CancellationToken ct)
    {
        var nota = await CarregarAsync(notaId, ct);
        GarantirAberta(nota);

        var item = nota.Itens.FirstOrDefault(item => item.Id == itemId)
            ?? throw ErroDeNegocio.RequisicaoInvalida($"O item {itemId} não pertence a esta nota");

        nota.Itens.Remove(item);
        InvalidarChave(nota);
        await db.SaveChangesAsync(ct);
        return nota.ParaDetalhe();
    }

    /// <summary>
    /// Descarta a chave de idempotencia quando os itens mudam.
    ///
    /// Sem isto, uma nota recusada por saldo insuficiente ficaria travada para
    /// sempre: o usuario corrige a quantidade, manda imprimir de novo, e o
    /// Estoque recusa com chave-idempotencia-conflito -- mesma chave, corpo
    /// diferente. A chave descreve UM conteudo especifico; se o conteudo muda,
    /// ela deixa de valer.
    ///
    /// Retentar sem editar nada continua reaproveitando a chave, que e
    /// exatamente o caso em que a idempotencia precisa agir.
    /// </summary>
    private static void InvalidarChave(NotaFiscal nota) => nota.ChaveIdempotencia = null;

    private static void GarantirAberta(NotaFiscal nota)
    {
        if (nota.Status != StatusNota.Aberta)
        {
            throw ErroDeNegocio.NotaNaoAberta(nota.NumeroFormatado);
        }
    }

    private async Task<NotaFiscal> CarregarAsync(Guid id, CancellationToken ct, bool rastrear = true)
    {
        var consulta = db.Notas.Include(nota => nota.Itens).AsQueryable();
        if (!rastrear)
        {
            consulta = consulta.AsNoTracking();
        }

        return await consulta.FirstOrDefaultAsync(nota => nota.Id == id, ct)
            ?? throw ErroDeNegocio.NotaNaoEncontrada(id);
    }
}
