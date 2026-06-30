using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Rstash.Database.Migrations
{
    /// <inheritdoc />
    public partial class AddOAuthTokens : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "oauth_tokens",
                columns: table => new
                {
                    Token = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    UserId = table.Column<long>(type: "INTEGER", nullable: false),
                    ClientId = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    Scopes = table.Column<string>(type: "TEXT", maxLength: 1024, nullable: false),
                    CreatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false),
                    ExpiresAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_oauth_tokens", x => x.Token);
                });

            migrationBuilder.CreateIndex(
                name: "IX_oauth_tokens_UserId",
                table: "oauth_tokens",
                column: "UserId");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "oauth_tokens");
        }
    }
}
