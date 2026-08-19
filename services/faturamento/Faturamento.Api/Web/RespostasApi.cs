using Faturamento.Api.Dominio;

namespace Faturamento.Api.Web;

public record ItemNotaResposta(Guid Id, string Codigo, string Descricao, int Quantidade);

public record NotaResumo(
    Guid Id,
    long Numero,
    string NumeroFormatado,
    string Status,
    DateTimeOffset CriadaEm,
    DateTimeOffset? FechadaEm,
    int TotalItens,
    int TotalUnidades);

public record NotaDetalhe(
    Guid Id,
    long Numero,
    string NumeroFormatado,
    string Status,
    DateTimeOffset CriadaEm,
    DateTimeOffset? FechadaEm,
    bool PodeImprimir,
    IReadOnlyList<ItemNotaResposta> Itens);

public record PaginaNotas(IReadOnlyList<NotaResumo> Itens, int Pagina, int Tamanho, int Total);

public record NovoItem(string Codigo, int Quantidade);

/// <summary>
/// Projecoes de dominio para resposta.
///
/// Um metodo unico por forma evita o risco classico de dois endpoints
/// devolverem a "mesma" nota com campos diferentes.
/// </summary>
public static class Projecoes
{
    public static NotaResumo ParaResumo(this NotaFiscal nota) => new(
        nota.Id,
        nota.Numero,
        nota.NumeroFormatado,
        nota.Status.ToString(),
        nota.CriadaEm,
        nota.FechadaEm,
        nota.Itens.Count,
        nota.Itens.Sum(item => item.Quantidade));

    public static NotaDetalhe ParaDetalhe(this NotaFiscal nota) => new(
        nota.Id,
        nota.Numero,
        nota.NumeroFormatado,
        nota.Status.ToString(),
        nota.CriadaEm,
        nota.FechadaEm,
        nota.PodeImprimir,
        nota.Itens
            .OrderBy(item => item.Codigo)
            .Select(item => new ItemNotaResposta(item.Id, item.Codigo, item.Descricao, item.Quantidade))
            .ToList());
}
