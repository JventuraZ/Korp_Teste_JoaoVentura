using Faturamento.Api.Dominio;
using Faturamento.Api.Estoque;

namespace Faturamento.Testes;

/// <summary>
/// Dublê do servico de Estoque.
///
/// A saga precisa ser testada nos caminhos que sao dificeis de produzir com o
/// servico real: falha de rede no meio da operacao, e fechamento que quebra
/// DEPOIS de o estoque ja ter commitado. Aqui esses cenarios sao uma linha.
/// </summary>
public class EstoqueFalso : IClienteEstoque
{
    public Func<Guid, RequisicaoBaixa, ResultadoBaixa>? AoAplicarBaixa { get; set; }
    public Exception? ErroNaBaixa { get; set; }

    public List<Guid> BaixasAplicadas { get; } = [];
    public List<Guid> EstornosSolicitados { get; } = [];
    public List<RequisicaoBaixa> RequisicoesRecebidas { get; } = [];

    public Dictionary<string, ProdutoEstoque> Produtos { get; } = [];

    public Task<ProdutoEstoque> BuscarProdutoAsync(string codigo, CancellationToken ct)
        => Produtos.TryGetValue(codigo, out var produto)
            ? Task.FromResult(produto)
            : throw ErroDeNegocio.ProdutoNaoEncontrado(codigo);

    public Task<ResultadoBaixa> AplicarBaixaAsync(Guid chave, RequisicaoBaixa requisicao, CancellationToken ct)
    {
        RequisicoesRecebidas.Add(requisicao);

        if (ErroNaBaixa is not null)
        {
            throw ErroNaBaixa;
        }

        BaixasAplicadas.Add(chave);

        var resultado = AoAplicarBaixa?.Invoke(chave, requisicao) ?? new ResultadoBaixa(
            chave.ToString(),
            requisicao.Referencia,
            DateTimeOffset.UtcNow,
            requisicao.Itens
                .Select(item => new ItemMovimentado(item.Codigo, item.Quantidade, 10, 10 - item.Quantidade))
                .ToList());

        return Task.FromResult(resultado);
    }

    public Task<ResultadoEstorno> EstornarAsync(Guid chave, CancellationToken ct)
    {
        // Honrar o token e essencial para o teste da compensacao ter valor: um
        // duble que ignora o cancelamento registraria o estorno mesmo quando o
        // cliente real jamais seria chamado.
        ct.ThrowIfCancellationRequested();

        EstornosSolicitados.Add(chave);
        return Task.FromResult(new ResultadoEstorno(chave.ToString(), DateTimeOffset.UtcNow, []));
    }

    public Task<bool> EstaSaudavelAsync(CancellationToken ct) => Task.FromResult(ErroNaBaixa is null);

    public void Cadastrar(string codigo, string descricao, int saldo)
        => Produtos[codigo] = new ProdutoEstoque(Guid.NewGuid(), codigo, descricao, saldo, 1);
}
