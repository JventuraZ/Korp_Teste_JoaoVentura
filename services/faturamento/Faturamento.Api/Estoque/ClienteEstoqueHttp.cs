using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using Faturamento.Api.Dominio;
using Polly.CircuitBreaker;
using Polly.Timeout;

namespace Faturamento.Api.Estoque;

/// <summary>
/// Cliente HTTP do servico de Estoque.
///
/// Concentra a traducao de fronteira: o que chega como problem+json do Go sai
/// daqui como ErroDeNegocio do Faturamento, ja com o status que faz sentido
/// nesta camada (o 409 de saldo vira 422). Nenhum outro ponto do codigo precisa
/// saber que existe HTTP do outro lado.
/// </summary>
public class ClienteEstoqueHttp(HttpClient http, ILogger<ClienteEstoqueHttp> log) : IClienteEstoque
{
    private static readonly JsonSerializerOptions OpcoesJson = new(JsonSerializerDefaults.Web);

    public async Task<ProdutoEstoque> BuscarProdutoAsync(string codigo, CancellationToken ct)
    {
        var requisicao = new HttpRequestMessage(HttpMethod.Get, $"/api/produtos/{Uri.EscapeDataString(codigo)}");
        var resposta = await EnviarAsync(requisicao, ct);

        await GarantirSucessoAsync(resposta, codigo, ct);
        return await LerCorpoAsync<ProdutoEstoque>(resposta, ct);
    }

    public async Task<ResultadoBaixa> AplicarBaixaAsync(Guid chave, RequisicaoBaixa requisicaoBaixa, CancellationToken ct)
    {
        var requisicao = new HttpRequestMessage(HttpMethod.Post, "/api/estoque/baixas")
        {
            Content = JsonContent.Create(requisicaoBaixa, options: OpcoesJson)
        };

        requisicao.Headers.Add("Idempotency-Key", chave.ToString());

        var resposta = await EnviarAsync(requisicao, ct);
        await GarantirSucessoAsync(resposta, requisicaoBaixa.Referencia, ct);
        return await LerCorpoAsync<ResultadoBaixa>(resposta, ct);
    }

    public async Task<ResultadoEstorno> EstornarAsync(Guid chave, CancellationToken ct)
    {
        var requisicao = new HttpRequestMessage(HttpMethod.Post, $"/api/estoque/baixas/{chave}/estorno");
        var resposta = await EnviarAsync(requisicao, ct);

        await GarantirSucessoAsync(resposta, chave.ToString(), ct);
        return await LerCorpoAsync<ResultadoEstorno>(resposta, ct);
    }

    public async Task<bool> EstaSaudavelAsync(CancellationToken ct)
    {
        try
        {
            var resposta = await http.GetAsync("/health", ct);
            return resposta.IsSuccessStatusCode;
        }
        catch (Exception erro) when (EhFalhaDeComunicacao(erro))
        {
            return false;
        }
    }

    /// <summary>
    /// Converte falha de comunicacao em 503. E aqui que o requisito de
    /// tratamento de falhas se materializa: retries esgotados, timeout ou
    /// circuito aberto viram uma resposta compreensivel, nao uma stack trace.
    /// </summary>
    private async Task<HttpResponseMessage> EnviarAsync(HttpRequestMessage requisicao, CancellationToken ct)
    {
        try
        {
            return await http.SendAsync(requisicao, ct);
        }
        catch (BrokenCircuitException erro)
        {
            log.LogWarning(erro, "circuito aberto para o estoque; falhando rapido");
            throw ErroDeNegocio.EstoqueIndisponivel(erro);
        }
        catch (Exception erro) when (EhFalhaDeComunicacao(erro))
        {
            log.LogWarning(erro, "estoque inacessivel apos as tentativas");
            throw ErroDeNegocio.EstoqueIndisponivel(erro);
        }
    }

    private static bool EhFalhaDeComunicacao(Exception erro) =>
        erro is HttpRequestException or TimeoutRejectedException or TaskCanceledException or BrokenCircuitException;

    /// <summary>
    /// Traduz o problem+json do Estoque pelo campo `type`, nao pelo status:
    /// o significado do erro e que decide o status desta camada.
    /// </summary>
    private async Task GarantirSucessoAsync(HttpResponseMessage resposta, string contexto, CancellationToken ct)
    {
        if (resposta.IsSuccessStatusCode)
        {
            return;
        }

        var problema = await LerProblemaAsync(resposta, ct);
        var tipo = problema?.Type?.Split('/').LastOrDefault() ?? string.Empty;

        throw tipo switch
        {
            "saldo-insuficiente" => ErroDeNegocio.SaldoInsuficiente(
                problema?.Detail ?? "Um ou mais itens não possuem saldo suficiente",
                problema?.ItensInsuficientes),

            "produto-nao-encontrado" => ErroDeNegocio.ProdutoNaoEncontrado(contexto),

            "chave-idempotencia-conflito" => ErroDeNegocio.ChaveIdempotenciaConflito(),

            "baixa-nao-encontrada" => new ErroDeNegocio(
                "baixa-nao-encontrada", "Baixa não encontrada", (int)HttpStatusCode.NotFound,
                $"Não há baixa aplicada para {contexto}"),

            _ when (int)resposta.StatusCode >= 500 => ErroDeNegocio.EstoqueIndisponivel(),

            _ => new ErroDeNegocio(
                "erro-do-estoque", "Erro no serviço de estoque", (int)HttpStatusCode.BadGateway,
                problema?.Detail ?? $"O estoque respondeu {(int)resposta.StatusCode}")
        };
    }

    private async Task<ProblemaEstoque?> LerProblemaAsync(HttpResponseMessage resposta, CancellationToken ct)
    {
        try
        {
            return await resposta.Content.ReadFromJsonAsync<ProblemaEstoque>(OpcoesJson, ct);
        }
        catch (Exception erro)
        {
            log.LogWarning(erro, "resposta de erro do estoque nao era problem+json");
            return null;
        }
    }

    private static async Task<T> LerCorpoAsync<T>(HttpResponseMessage resposta, CancellationToken ct)
        => await resposta.Content.ReadFromJsonAsync<T>(OpcoesJson, ct)
           ?? throw ErroDeNegocio.EstoqueIndisponivel();
}
