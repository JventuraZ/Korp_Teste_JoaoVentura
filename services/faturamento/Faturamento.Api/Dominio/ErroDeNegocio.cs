using System.Net;

namespace Faturamento.Api.Dominio;

/// <summary>
/// Erro previsto de negocio, ja carregando o que a resposta RFC 7807 precisa.
///
/// Excecao aqui e o contrario de fluxo de controle escondido: cada fabrica
/// estatica abaixo e uma linha da tabela de erros do contrato. O middleware de
/// erro so escreve o que a excecao ja traz -- ele nao decide status nenhum.
/// </summary>
public class ErroDeNegocio : Exception
{
    public string Tipo { get; }
    public string Titulo { get; }
    public int Status { get; }
    public Dictionary<string, object?> Extensoes { get; } = [];

    public ErroDeNegocio(string tipo, string titulo, int status, string detalhe, Exception? interna = null)
        : base(detalhe, interna)
    {
        Tipo = tipo;
        Titulo = titulo;
        Status = status;
    }

    public static ErroDeNegocio NotaNaoEncontrada(Guid id) => new(
        "nota-nao-encontrada", "Nota fiscal não encontrada", (int)HttpStatusCode.NotFound,
        $"Nenhuma nota fiscal com o identificador {id}");

    /// <summary>
    /// 409 e conflito de estado da NOTA -- diferente do 422 de saldo. Para o
    /// usuario significa "recarregue a tela", nao "ajuste as quantidades".
    /// </summary>
    public static ErroDeNegocio NotaNaoAberta(string numero) => new(
        "nota-nao-aberta", "Nota fiscal não está aberta", (int)HttpStatusCode.Conflict,
        $"A nota {numero} já foi impressa e não pode ser impressa novamente");

    public static ErroDeNegocio NotaSemItens(string numero) => new(
        "nota-sem-itens", "Nota fiscal sem itens", (int)HttpStatusCode.UnprocessableContent,
        $"A nota {numero} não possui itens para faturar");

    /// <summary>
    /// O Estoque devolve 409 (conflito de estado do PRODUTO); aqui vira 422,
    /// porque a nota em si esta valida -- apenas nao e processavel agora.
    /// </summary>
    public static ErroDeNegocio SaldoInsuficiente(string detalhe, object? itensInsuficientes)
    {
        var erro = new ErroDeNegocio(
            "saldo-insuficiente", "Saldo insuficiente", (int)HttpStatusCode.UnprocessableContent, detalhe);

        if (itensInsuficientes is not null)
        {
            erro.Extensoes["itensInsuficientes"] = itensInsuficientes;
        }
        return erro;
    }

    public static ErroDeNegocio ProdutoNaoEncontrado(string codigo) => new(
        "produto-nao-encontrado", "Produto não encontrado", (int)HttpStatusCode.NotFound,
        $"O produto {codigo} não existe no estoque");

    /// <summary>
    /// Retries esgotados ou circuito aberto. E o feedback do requisito de falha:
    /// a nota continua Aberta e intacta, entao basta o usuario tentar de novo.
    /// </summary>
    public static ErroDeNegocio EstoqueIndisponivel(Exception? interna = null) => new(
        "estoque-indisponivel", "Serviço de estoque indisponível", (int)HttpStatusCode.ServiceUnavailable,
        "O serviço de estoque não respondeu. A nota permanece aberta — tente novamente em instantes.",
        interna);

    public static ErroDeNegocio ChaveIdempotenciaConflito() => new(
        "chave-idempotencia-conflito", "Chave de idempotência reutilizada", (int)HttpStatusCode.Conflict,
        "A chave de idempotência desta nota já foi usada com outro conteúdo");

    public static ErroDeNegocio RequisicaoInvalida(string detalhe) => new(
        "requisicao-invalida", "Requisição inválida", (int)HttpStatusCode.BadRequest, detalhe);

    /// <summary>
    /// O unico caminho que a saga nao consegue evitar: o Estoque commitou e o
    /// fechamento da nota falhou depois. A compensacao ja rodou quando este erro
    /// e lancado, entao o estoque foi devolvido.
    /// </summary>
    public static ErroDeNegocio FalhaAoFecharNota(string numero, Exception interna) => new(
        "falha-ao-fechar-nota", "Falha ao fechar a nota", (int)HttpStatusCode.InternalServerError,
        $"A baixa da nota {numero} foi estornada porque o fechamento falhou. Tente imprimir novamente.",
        interna);
}
