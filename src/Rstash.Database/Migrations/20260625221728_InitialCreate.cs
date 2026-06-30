using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Rstash.Database.Migrations
{
    /// <inheritdoc />
    public partial class InitialCreate : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "audit_log",
                columns: table => new
                {
                    Id = table.Column<long>(type: "INTEGER", nullable: false)
                        .Annotation("Sqlite:Autoincrement", true),
                    ActorId = table.Column<long>(type: "INTEGER", nullable: false),
                    Action = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    TargetType = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    TargetId = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    Details = table.Column<string>(type: "TEXT", maxLength: 4096, nullable: false, defaultValue: ""),
                    CreatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_audit_log", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "nodes",
                columns: table => new
                {
                    Id = table.Column<long>(type: "INTEGER", nullable: false)
                        .Annotation("Sqlite:Autoincrement", true),
                    UserId = table.Column<long>(type: "INTEGER", nullable: false),
                    Path = table.Column<string>(type: "TEXT", maxLength: 512, nullable: false),
                    ContentType = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false, defaultValue: ""),
                    ContentLength = table.Column<long>(type: "INTEGER", nullable: false),
                    ETag = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    CreatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false),
                    UpdatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_nodes", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_audit_log_CreatedAt",
                table: "audit_log",
                column: "CreatedAt");

            migrationBuilder.CreateIndex(
                name: "IX_nodes_UserId_Path",
                table: "nodes",
                columns: new[] { "UserId", "Path" },
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "audit_log");

            migrationBuilder.DropTable(
                name: "nodes");
        }
    }
}
