using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Faturamento.Api.Dados.Migrations
{
    /// <inheritdoc />
    public partial class CriarNotasFiscais : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateSequence(
                name: "notas_numero_seq");

            migrationBuilder.CreateTable(
                name: "notas_fiscais",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Numero = table.Column<long>(type: "bigint", nullable: false, defaultValueSql: "nextval('notas_numero_seq')"),
                    Status = table.Column<string>(type: "character varying(20)", maxLength: 20, nullable: false),
                    ChaveIdempotencia = table.Column<Guid>(type: "uuid", nullable: true),
                    CriadaEm = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    FechadaEm = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: true),
                    xmin = table.Column<uint>(type: "xid", rowVersion: true, nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_notas_fiscais", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "itens_nota",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    NotaFiscalId = table.Column<Guid>(type: "uuid", nullable: false),
                    Codigo = table.Column<string>(type: "character varying(50)", maxLength: 50, nullable: false),
                    Descricao = table.Column<string>(type: "character varying(200)", maxLength: 200, nullable: false),
                    Quantidade = table.Column<int>(type: "integer", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_itens_nota", x => x.Id);
                    table.CheckConstraint("itens_quantidade_positiva", "\"Quantidade\" > 0");
                    table.ForeignKey(
                        name: "FK_itens_nota_notas_fiscais_NotaFiscalId",
                        column: x => x.NotaFiscalId,
                        principalTable: "notas_fiscais",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_itens_nota_NotaFiscalId",
                table: "itens_nota",
                column: "NotaFiscalId");

            migrationBuilder.CreateIndex(
                name: "IX_notas_fiscais_ChaveIdempotencia",
                table: "notas_fiscais",
                column: "ChaveIdempotencia",
                unique: true,
                filter: "\"ChaveIdempotencia\" IS NOT NULL");

            migrationBuilder.CreateIndex(
                name: "IX_notas_fiscais_Numero",
                table: "notas_fiscais",
                column: "Numero",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "itens_nota");

            migrationBuilder.DropTable(
                name: "notas_fiscais");

            migrationBuilder.DropSequence(
                name: "notas_numero_seq");
        }
    }
}
