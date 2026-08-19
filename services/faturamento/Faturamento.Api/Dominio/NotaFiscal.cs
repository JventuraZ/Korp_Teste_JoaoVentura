namespace Faturamento.Api.Dominio;

public enum StatusNota
{
    Aberta,
    Fechada
}

/// <summary>
/// Nota fiscal. O saldo dos produtos NAO vive aqui: pertence ao servico de
/// Estoque, que e a unica autoridade sobre ele. Esta nota guarda apenas o que
/// foi pedido e um retrato da descricao no momento da inclusao.
/// </summary>
public class NotaFiscal
{
    public Guid Id { get; set; }

    /// <summary>
    /// Numeracao sequencial gerada por uma sequence do Postgres.
    /// Contar no C# (max + 1) daria numero duplicado sob concorrencia.
    /// </summary>
    public long Numero { get; set; }

    public StatusNota Status { get; set; } = StatusNota.Aberta;

    /// <summary>
    /// Chave enviada ao Estoque na baixa. E gravada ANTES da chamada: se o
    /// processo morrer no meio da impressao, o retry reaproveita a mesma chave
    /// e o Estoque devolve a resposta original em vez de debitar de novo.
    /// </summary>
    public Guid? ChaveIdempotencia { get; set; }

    public DateTimeOffset CriadaEm { get; set; } = DateTimeOffset.UtcNow;
    public DateTimeOffset? FechadaEm { get; set; }

    public List<ItemNota> Itens { get; set; } = [];

    /// <summary>Formato exibido ao usuario e enviado como referencia ao Estoque.</summary>
    public string NumeroFormatado => $"NF-{Numero:D6}";

    public bool PodeImprimir => Status == StatusNota.Aberta && Itens.Count > 0;
}

public class ItemNota
{
    public Guid Id { get; set; }
    public Guid NotaFiscalId { get; set; }

    public string Codigo { get; set; } = string.Empty;

    /// <summary>
    /// Retrato da descricao no momento da inclusao. Se o produto for renomeado
    /// depois, a nota continua mostrando o que foi de fato faturado -- e o
    /// Faturamento nao precisa consultar o Estoque para exibir a nota.
    /// </summary>
    public string Descricao { get; set; } = string.Empty;

    public int Quantidade { get; set; }
}
