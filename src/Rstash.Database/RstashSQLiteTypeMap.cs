using System.Data;
using FluentMigrator.Runner.Generators.SQLite;

namespace Rstash.Database;

/// <summary>
/// SQLite has no native <c>datetimeoffset</c> type, so FluentMigrator's stock
/// SQLite type map omits it and throws on <c>.AsDateTimeOffset()</c>. EF Core's
/// own SQLite provider persists <see cref="DateTimeOffset"/> as <c>TEXT</c>; we
/// map it the same way so the agnostic <see cref="Migrations.InitialCreate"/>
/// migration renders correctly on SQLite while remaining a single definition for
/// every dialect.
///
/// <see cref="SQLiteTypeMap"/> is sealed, so we compose one and delegate every
/// other type to it. Registered in place of the default
/// <see cref="ISQLiteTypeMap"/>.
/// </summary>
internal sealed class RstashSQLiteTypeMap : ISQLiteTypeMap
{
    private readonly SQLiteTypeMap _inner = new(useStrictTables: false);

    public string GetTypeMap(DbType type, int? size, int? precision) =>
        type == DbType.DateTimeOffset
            ? "TEXT"
            : _inner.GetTypeMap(type, size, precision);
}
