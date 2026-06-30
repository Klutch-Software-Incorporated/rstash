using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Rstash.Database.Migrations
{
    /// <inheritdoc />
    public partial class AddAuthorizationCodes : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "authorization_codes",
                columns: table => new
                {
                    Code = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    UserId = table.Column<long>(type: "INTEGER", nullable: false),
                    ClientId = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    RedirectUri = table.Column<string>(type: "TEXT", maxLength: 2048, nullable: false),
                    Scopes = table.Column<string>(type: "TEXT", maxLength: 1024, nullable: false),
                    CodeChallenge = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    CodeChallengeMethod = table.Column<string>(type: "TEXT", maxLength: 32, nullable: false),
                    CreatedAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false),
                    ExpiresAt = table.Column<DateTimeOffset>(type: "TEXT", nullable: false),
                    Used = table.Column<bool>(type: "INTEGER", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_authorization_codes", x => x.Code);
                });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "authorization_codes");
        }
    }
}
