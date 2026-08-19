using System.Net;
using Faturamento.Api.Dados;
using Faturamento.Api.Estoque;
using Faturamento.Api.Servicos;
using Faturamento.Api.Web;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Http.Resilience;
using Polly;
using Polly.Timeout;

var construtor = WebApplication.CreateBuilder(args);

var conexao = construtor.Configuration.GetConnectionString("Postgres")
    ?? throw new InvalidOperationException("ConnectionStrings__Postgres nao configurada");

var urlEstoque = construtor.Configuration["Estoque:BaseUrl"]
    ?? throw new InvalidOperationException("Estoque__BaseUrl nao configurada");

construtor.Services.AddDbContext<ContextoFaturamento>(opcoes => opcoes.UseNpgsql(conexao));

var falhaTransitoria = new PredicateBuilder<HttpResponseMessage>()
    .Handle<HttpRequestException>()
    .Handle<TimeoutRejectedException>()
    .HandleResult(resposta =>
        (int)resposta.StatusCode >= 500 || resposta.StatusCode == HttpStatusCode.RequestTimeout);

construtor.Services
    .AddHttpClient<IClienteEstoque, ClienteEstoqueHttp>(cliente =>
    {
        cliente.BaseAddress = new Uri(urlEstoque);
    })
    .AddResilienceHandler("estoque", pipeline =>
    {
        pipeline.AddRetry(new HttpRetryStrategyOptions
        {
            Name = "retry-estoque",
            MaxRetryAttempts = 3,
            Delay = TimeSpan.FromMilliseconds(300),
            BackoffType = DelayBackoffType.Exponential,
            UseJitter = true,
            ShouldHandle = falhaTransitoria
        });

        pipeline.AddCircuitBreaker(new HttpCircuitBreakerStrategyOptions
        {
            Name = "breaker-estoque",
            FailureRatio = 0.5,
            MinimumThroughput = 4,
            SamplingDuration = TimeSpan.FromSeconds(15),
            BreakDuration = TimeSpan.FromSeconds(10),
            ShouldHandle = falhaTransitoria
        });

        pipeline.AddTimeout(TimeSpan.FromSeconds(5));
    });

construtor.Services.AddScoped<ServicoNotas>();
construtor.Services.AddScoped<ServicoImpressao>();

construtor.Services.AddExceptionHandler<ManipuladorDeErros>();
construtor.Services.AddProblemDetails();

var app = construtor.Build();

app.UseExceptionHandler();

await AplicarMigrationsAsync(app);

app.MapearNotas();

app.MapGet("/health/vivo", async (ContextoFaturamento db, CancellationToken ct) =>
{
    var bancoOk = await db.Database.CanConnectAsync(ct);
    var corpo = new { status = bancoOk ? "ok" : "degradado", banco = bancoOk ? "ok" : "indisponivel" };

    return bancoOk ? Results.Ok(corpo) : Results.Json(corpo, statusCode: 503);
});

app.MapGet("/health", async (ContextoFaturamento db, IClienteEstoque estoque, CancellationToken ct) =>
{
    var bancoOk = await db.Database.CanConnectAsync(ct);
    var estoqueOk = await estoque.EstaSaudavelAsync(ct);

    var corpo = new
    {
        status = bancoOk ? "ok" : "degradado",
        banco = bancoOk ? "ok" : "indisponivel",
        estoque = estoqueOk ? "ok" : "indisponivel"
    };

    return bancoOk ? Results.Ok(corpo) : Results.Json(corpo, statusCode: 503);
});

app.Run();

static async Task AplicarMigrationsAsync(WebApplication app)
{
    using var escopo = app.Services.CreateScope();
    var contexto = escopo.ServiceProvider.GetRequiredService<ContextoFaturamento>();

    for (var tentativa = 1; tentativa <= 10; tentativa++)
    {
        try
        {
            await contexto.Database.MigrateAsync();
            return;
        }
        catch (Exception erro) when (tentativa < 10)
        {
            app.Logger.LogWarning(erro,
                "banco indisponivel (tentativa {Tentativa}/10); tentando de novo em 2s", tentativa);
            await Task.Delay(TimeSpan.FromSeconds(2));
        }
    }
}

public partial class Program;
