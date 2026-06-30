using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Rstash.Database.Migrations
{
    /// <inheritdoc />
    public partial class AddEgressUsage : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "egress_usage",
                columns: table => new
                {
                    Id = table.Column<long>(type: "INTEGER", nullable: false)
                        .Annotation("Sqlite:Autoincrement", true),
                    UserId = table.Column<long>(type: "INTEGER", nullable: false),
                    Period = table.Column<string>(type: "TEXT", maxLength: 7, nullable: false),
                    BytesOut = table.Column<long>(type: "INTEGER", nullable: false),
                    UpdatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_egress_usage", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_egress_usage_UserId_Period",
                table: "egress_usage",
                columns: new[] { "UserId", "Period" },
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "egress_usage");
        }
    }
}
