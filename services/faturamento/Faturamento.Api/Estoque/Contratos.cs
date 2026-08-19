namespace Faturamento.Api.Estoque;

/// <summary>
/// Espelho tipado do docs/contrato-estoque.md.
///
/// Estes tipos foram escritos a partir do documento, nao do codigo Go: e o que
/// mantem os dois servicos desacoplados de verdade.
/// </summary>
public record ProdutoEstoque(Guid Id, string Codigo, string Descricao, int Saldo, long Versao);

public record ItemBaixa(string Codigo, int Quantidade);

public record RequisicaoBaixa(string Referencia, IReadOnlyList<ItemBaixa> Itens);

public record ItemMovimentado(string Codigo, int Quantidade, int SaldoAnterior, int SaldoPosterior);

public record ResultadoBaixa(
    string ChaveIdempotencia,
    string Referencia,
    DateTimeOffset ProcessadoEm,
    IReadOnlyList<ItemMovimentado> Itens);

public record ResultadoEstorno(
    string ChaveIdempotencia,
    DateTimeOffset EstornadoEm,
    IReadOnlyList<ItemMovimentado> Itens);

public record ItemInsuficiente(string Codigo, int QuantidadeSolicitada, int SaldoDisponivel);

/// <summary>
/// RFC 7807 como o Estoque devolve, incluindo a extensao itensInsuficientes.
/// Ler o campo `type` (e nao so o status) e o que permite traduzir cada erro
/// para o significado correto no Faturamento.
/// </summary>
public record ProblemaEstoque(
    string? Type,
    string? Title,
    int Status,
    string? Detail,
    List<ItemInsuficiente>? ItensInsuficientes);

public interface IClienteEstoque
{
    Task<ProdutoEstoque> BuscarProdutoAsync(string codigo, CancellationToken ct);
    Task<ResultadoBaixa> AplicarBaixaAsync(Guid chave, RequisicaoBaixa requisicao, CancellationToken ct);
    Task<ResultadoEstorno> EstornarAsync(Guid chave, CancellationToken ct);
    Task<bool> EstaSaudavelAsync(CancellationToken ct);
}
