using Faturamento.Api.Dados;
using Microsoft.EntityFrameworkCore;

namespace Faturamento.Testes;

/// <summary>
/// Infra dos testes de integracao: Postgres de verdade, porque as garantias
/// verificadas (sequence da numeracao, xmin como token de concorrencia,
/// cascade dos itens) sao do banco. Um provedor em memoria passaria sem
/// provar nada disso.
/// </summary>
public static class BancoDeTeste
{
    private static readonly Lock Trava = new();
    private static bool _migrado;

    public static string? Conexao =>
        Environment.GetEnvironmentVariable("TEST_FATURAMENTO_CONEXAO");

    public static DbContextOptions<ContextoFaturamento> Opcoes()
        => new DbContextOptionsBuilder<ContextoFaturamento>().UseNpgsql(Conexao).Options;

    /// <summary>Contexto novo, com o schema migrado e as tabelas limpas.</summary>
    public static ContextoFaturamento Novo(bool limpar = true)
    {
        var contexto = new ContextoFaturamento(Opcoes());
        Preparar(contexto, limpar);
        return contexto;
    }

    public static void Preparar(ContextoFaturamento contexto, bool limpar = true)
    {
        lock (Trava)
        {
            if (!_migrado)
            {
                contexto.Database.Migrate();
                _migrado = true;
            }
        }

        if (limpar)
        {
            // Os itens saem por cascade, mas o TRUNCATE explicito deixa o teste
            // independente da configuracao de FK.
            contexto.Database.ExecuteSqlRaw(
                "TRUNCATE itens_nota, notas_fiscais RESTART IDENTITY CASCADE;");
        }
    }
}

/// <summary>
/// Fato que se auto-pula quando nao ha banco configurado, para que
/// `dotnet test` continue verde numa maquina sem Postgres.
/// </summary>
public sealed class FatoComBancoAttribute : FactAttribute
{
    public FatoComBancoAttribute()
    {
        if (string.IsNullOrWhiteSpace(BancoDeTeste.Conexao))
        {
            Skip = "TEST_FATURAMENTO_CONEXAO nao definida; teste de integracao pulado";
        }
    }
}
