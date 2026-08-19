using Faturamento.Api.Dominio;
using Microsoft.AspNetCore.Diagnostics;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using Npgsql;

namespace Faturamento.Api.Web;

/// <summary>
/// Ponto unico de traducao de excecao para RFC 7807 -- o equivalente ao
/// MapearErro do servico Go.
///
/// Nenhum endpoint escreve status de erro: eles lancam ErroDeNegocio e este
/// manipulador decide. Isso e o que garante que os dois microsservicos, em
/// linguagens diferentes, respondam erro exatamente no mesmo formato, e que o
/// Angular precise de um unico interceptor.
/// </summary>
public class ManipuladorDeErros(ILogger<ManipuladorDeErros> log) : IExceptionHandler
{
    private const string BaseTipo = "https://korp.local/erros/";

    public async ValueTask<bool> TryHandleAsync(
        HttpContext contexto, Exception erro, CancellationToken ct)
    {
        var problema = Traduzir(erro, contexto);

        contexto.Response.StatusCode = problema.Status ?? StatusCodes.Status500InternalServerError;
        contexto.Response.ContentType = "application/problem+json";

        await contexto.Response.WriteAsJsonAsync(problema, ct);
        return true;
    }

    private ProblemDetails Traduzir(Exception erro, HttpContext contexto)
    {
        var problema = new ProblemDetails { Instance = contexto.Request.Path };

        switch (erro)
        {
            case ErroDeNegocio negocio:
                problema.Type = BaseTipo + negocio.Tipo;
                problema.Title = negocio.Titulo;
                problema.Status = negocio.Status;
                problema.Detail = negocio.Message;

                foreach (var (chave, valor) in negocio.Extensoes)
                {
                    problema.Extensions[chave] = valor;
                }

                if (negocio.Status >= 500)
                {
                    log.LogError(erro, "erro de negocio com status {Status}", negocio.Status);
                }
                break;

            case DbUpdateConcurrencyException:
                problema.Type = BaseTipo + "nota-em-alteracao";
                problema.Title = "Nota alterada por outra operação";
                problema.Status = StatusCodes.Status409Conflict;
                problema.Detail = "Esta nota foi alterada em outra janela. Recarregue a tela e tente novamente.";
                break;

            case DbUpdateException or NpgsqlException:
                log.LogError(erro, "falha de acesso ao banco");
                problema.Type = BaseTipo + "banco-indisponivel";
                problema.Title = "Banco de dados indisponível";
                problema.Status = StatusCodes.Status503ServiceUnavailable;
                problema.Detail = "Não foi possível acessar o banco de dados. Tente novamente em instantes.";
                break;

            default:
                log.LogError(erro, "erro nao tratado em {Rota}", contexto.Request.Path);
                problema.Type = BaseTipo + "erro-interno";
                problema.Title = "Erro interno";
                problema.Status = StatusCodes.Status500InternalServerError;
                problema.Detail = "Erro interno ao processar a requisição";
                break;
        }

        return problema;
    }
}
