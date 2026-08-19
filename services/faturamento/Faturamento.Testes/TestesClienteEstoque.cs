using System.Net;
using System.Text;
using Faturamento.Api.Dominio;
using Faturamento.Api.Estoque;
using Microsoft.Extensions.Logging.Abstractions;

namespace Faturamento.Testes;

/// <summary>
/// Traducao da fronteira entre os dois microsservicos. Nao precisa de banco:
/// o que esta sob teste e como a resposta HTTP do Go vira erro do C#.
/// </summary>
public class TestesClienteEstoque
{
    private static (ClienteEstoqueHttp cliente, ManipuladorFalso handler) Montar(
        HttpStatusCode status, string corpo)
    {
        var handler = new ManipuladorFalso(status, corpo);
        var http = new HttpClient(handler) { BaseAddress = new Uri("http://estoque:8081") };
        return (new ClienteEstoqueHttp(http, NullLogger<ClienteEstoqueHttp>.Instance), handler);
    }

    /// <summary>
    /// A traducao mais importante do sistema: para o Estoque e conflito de
    /// estado do produto (409); para o Faturamento a nota esta valida, apenas
    /// nao e processavel agora (422).
    /// </summary>
    [Fact]
    public async Task Saldo_insuficiente_vira_422_com_a_lista_de_faltantes()
    {
        var (cliente, _) = Montar(HttpStatusCode.Conflict, """
        {
          "type": "https://korp.local/erros/saldo-insuficiente",
          "title": "Saldo insuficiente",
          "status": 409,
          "detail": "1 item não possui saldo suficiente para a baixa",
          "itensInsuficientes": [
            { "codigo": "PRD-001", "quantidadeSolicitada": 5, "saldoDisponivel": 1 }
          ]
        }
        """);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(() => cliente.AplicarBaixaAsync(
            Guid.NewGuid(), new RequisicaoBaixa("NF-000001", [new ItemBaixa("PRD-001", 5)]), default));

        Assert.Equal(422, erro.Status);
        Assert.Equal("saldo-insuficiente", erro.Tipo);

        var itens = Assert.IsType<List<ItemInsuficiente>>(erro.Extensoes["itensInsuficientes"]);
        Assert.Equal("PRD-001", Assert.Single(itens).Codigo);
    }

    [Fact]
    public async Task Erro_5xx_do_estoque_vira_503()
    {
        var (cliente, _) = Montar(HttpStatusCode.InternalServerError, """
        {"type":"https://korp.local/erros/erro-interno","title":"Erro interno","status":500,"detail":"x"}
        """);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(() => cliente.AplicarBaixaAsync(
            Guid.NewGuid(), new RequisicaoBaixa("NF-000001", [new ItemBaixa("PRD-001", 1)]), default));

        Assert.Equal(503, erro.Status);
        Assert.Equal("estoque-indisponivel", erro.Tipo);
    }

    [Fact]
    public async Task Produto_inexistente_vira_404()
    {
        var (cliente, _) = Montar(HttpStatusCode.NotFound, """
        {"type":"https://korp.local/erros/produto-nao-encontrado","title":"Produto não encontrado","status":404,"detail":"x"}
        """);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(
            () => cliente.BuscarProdutoAsync("NAO-EXISTE", default));

        Assert.Equal(404, erro.Status);
        Assert.Contains("NAO-EXISTE", erro.Message);
    }

    /// <summary>
    /// Estoque inalcancavel (conexao recusada, DNS, timeout) precisa virar 503
    /// com mensagem util, nao uma excecao crua vazando para o usuario.
    /// </summary>
    [Fact]
    public async Task Falha_de_conexao_vira_503()
    {
        var handler = new ManipuladorFalso(HttpStatusCode.OK, "")
        {
            ErroAoEnviar = new HttpRequestException("conexão recusada")
        };
        var http = new HttpClient(handler) { BaseAddress = new Uri("http://estoque:8081") };
        var cliente = new ClienteEstoqueHttp(http, NullLogger<ClienteEstoqueHttp>.Instance);

        var erro = await Assert.ThrowsAsync<ErroDeNegocio>(() => cliente.AplicarBaixaAsync(
            Guid.NewGuid(), new RequisicaoBaixa("NF-000001", [new ItemBaixa("PRD-001", 1)]), default));

        Assert.Equal(503, erro.Status);
    }

    /// <summary>Sem este header, todo retry debitaria o estoque de novo.</summary>
    [Fact]
    public async Task Baixa_envia_o_header_de_idempotencia()
    {
        var (cliente, handler) = Montar(HttpStatusCode.OK, """
        {"chaveIdempotencia":"x","referencia":"NF-000001","processadoEm":"2026-08-17T12:00:00Z","itens":[]}
        """);

        var chave = Guid.NewGuid();
        await cliente.AplicarBaixaAsync(
            chave, new RequisicaoBaixa("NF-000001", [new ItemBaixa("PRD-001", 1)]), default);

        Assert.Equal(chave.ToString(),
            Assert.Single(handler.UltimaRequisicao!.Headers.GetValues("Idempotency-Key")));
    }

    private sealed class ManipuladorFalso(HttpStatusCode status, string corpo) : HttpMessageHandler
    {
        public HttpRequestMessage? UltimaRequisicao { get; private set; }
        public Exception? ErroAoEnviar { get; init; }

        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage requisicao, CancellationToken ct)
        {
            UltimaRequisicao = requisicao;

            if (ErroAoEnviar is not null)
            {
                throw ErroAoEnviar;
            }

            return Task.FromResult(new HttpResponseMessage(status)
            {
                Content = new StringContent(corpo, Encoding.UTF8, "application/problem+json")
            });
        }
    }
}
