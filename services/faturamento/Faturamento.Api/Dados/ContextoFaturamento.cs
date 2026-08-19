using Faturamento.Api.Dominio;
using Microsoft.EntityFrameworkCore;

namespace Faturamento.Api.Dados;

/// <summary>
/// Contexto do banco db_faturamento.
///
/// Database-per-service: nao existe FK nem JOIN para o banco do Estoque. Os
/// codigos de produto sao referencias logicas, resolvidas por HTTP. E o que
/// torna a separacao dos microsservicos real e nao apenas cosmetica.
/// </summary>
public class ContextoFaturamento(DbContextOptions<ContextoFaturamento> opcoes) : DbContext(opcoes)
{
    public DbSet<NotaFiscal> Notas => Set<NotaFiscal>();
    public DbSet<ItemNota> Itens => Set<ItemNota>();

    protected override void OnModelCreating(ModelBuilder modelo)
    {
        modelo.HasSequence<long>("notas_numero_seq").StartsAt(1).IncrementsBy(1);

        modelo.Entity<NotaFiscal>(nota =>
        {
            nota.ToTable("notas_fiscais");
            nota.HasKey(n => n.Id);

            nota.Property(n => n.Numero)
                .HasDefaultValueSql("nextval('notas_numero_seq')")
                .ValueGeneratedOnAdd();
            nota.HasIndex(n => n.Numero).IsUnique();

            nota.Property(n => n.Status)
                .HasConversion<string>()
                .HasMaxLength(20);

            nota.Property(n => n.ChaveIdempotencia);
            nota.HasIndex(n => n.ChaveIdempotencia).IsUnique()
                .HasFilter("\"ChaveIdempotencia\" IS NOT NULL");

            nota.Property<uint>("xmin")
                .HasColumnName("xmin")
                .HasColumnType("xid")
                .ValueGeneratedOnAddOrUpdate()
                .IsConcurrencyToken();

            nota.HasMany(n => n.Itens)
                .WithOne()
                .HasForeignKey(i => i.NotaFiscalId)
                .OnDelete(DeleteBehavior.Cascade);
        });

        modelo.Entity<ItemNota>(item =>
        {
            item.ToTable("itens_nota");
            item.HasKey(i => i.Id);
            item.Property(i => i.Codigo).HasMaxLength(50).IsRequired();
            item.Property(i => i.Descricao).HasMaxLength(200).IsRequired();
            item.HasIndex(i => i.NotaFiscalId);

            item.ToTable(t => t.HasCheckConstraint(
                "itens_quantidade_positiva", "\"Quantidade\" > 0"));
        });
    }
}
