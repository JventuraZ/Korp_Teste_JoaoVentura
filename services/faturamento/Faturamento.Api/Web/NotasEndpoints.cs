using Faturamento.Api.Servicos;

namespace Faturamento.Api.Web;

/// <summary>
/// Rotas do Faturamento. Os handlers nao tratam erro: qualquer ErroDeNegocio
/// sobe para o ManipuladorDeErros, que responde em problem+json.
/// </summary>
public static class NotasEndpoints
{
    public static void MapearNotas(this WebApplication app)
    {
        var notas = app.MapGroup("/api/notas");

        notas.MapGet("/", async (
            ServicoNotas servico, CancellationToken ct, int pagina = 1, int tamanho = 20)
            => Results.Ok(await servico.ListarAsync(pagina, tamanho, ct)));

        notas.MapGet("/{id:guid}", async (Guid id, ServicoNotas servico, CancellationToken ct)
            => Results.Ok(await servico.ObterAsync(id, ct)));

        notas.MapPost("/", async (ServicoNotas servico, CancellationToken ct) =>
        {
            var nota = await servico.CriarAsync(ct);
            return Results.Created($"/api/notas/{nota.Id}", nota);
        });

        notas.MapPost("/{id:guid}/itens", async (
            Guid id, NovoItem item, ServicoNotas servico, CancellationToken ct)
            => Results.Ok(await servico.AdicionarItemAsync(id, item, ct)));

        notas.MapDelete("/{id:guid}/itens/{itemId:guid}", async (
            Guid id, Guid itemId, ServicoNotas servico, CancellationToken ct)
            => Results.Ok(await servico.RemoverItemAsync(id, itemId, ct)));

        notas.MapPost("/{id:guid}/impressao", async (
            Guid id, ServicoImpressao servico, CancellationToken ct)
            => Results.Ok(await servico.ImprimirAsync(id, ct)));
    }
}
