using FluentMigrator;

namespace Rstash.Database.Migrations;

/// <summary>
/// The ASP.NET Core Data Protection key ring.
///
/// These keys encrypt the authentication cookie and antiforgery tokens. Left on the
/// default provider they live in the container filesystem (or, with no suitable
/// location, in memory), so every restart generates a fresh ring and silently signs
/// out every user. Persisting them to the database makes sessions survive restarts
/// and lets a multi-instance deployment share one ring.
///
/// The shape matches <c>DataProtectionKey</c> from
/// <c>Microsoft.AspNetCore.DataProtection.EntityFrameworkCore</c>: an int identity
/// key plus two unbounded nullable strings. <c>Xml</c> holds the serialized key
/// element, which is why it is not length-capped.
/// </summary>
[Migration(202607280001, "Data Protection key ring (persists auth cookie + antiforgery keys)")]
public sealed class AddDataProtectionKeys : Migration
{
    public override void Up()
    {
        Create.Table("DataProtectionKeys")
            .WithColumn("Id").AsInt32().NotNullable().PrimaryKey().Identity()
            .WithColumn("FriendlyName").AsString(int.MaxValue).Nullable()
            .WithColumn("Xml").AsString(int.MaxValue).Nullable();
    }

    public override void Down()
    {
        Delete.Table("DataProtectionKeys");
    }
}
