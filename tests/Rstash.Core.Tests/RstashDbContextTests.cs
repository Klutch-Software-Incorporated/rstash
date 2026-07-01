using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>
/// Exercises the EF Core SQLite stack end to end (migrate + write + read) on a
/// real file. Also serves as the P1 runtime check that the bumped
/// SQLitePCLRaw 3.0.3 native bundle loads and works.
/// </summary>
public class RstashDbContextTests
{
    [Fact]
    public async Task Migrate_ThenRoundtripNode_Succeeds()
    {
        var path = Path.Combine(Path.GetTempPath(), $"rstash-test-{Guid.NewGuid():N}.sqlite");
        var options = new DbContextOptionsBuilder<RstashDbContext>()
            .UseRstashDatabase($"sqlite:{path}")
            .Options;

        try
        {
            SchemaMigrator.MigrateUp($"sqlite:{path}");

            await using (var ctx = new RstashDbContext(options))
            {
                ctx.Nodes.Add(new Node
                {
                    UserId = 1,
                    Path = "documents/notes.txt",
                    ContentType = "text/plain",
                    ContentLength = 5,
                    ETag = ETag.ForDocument("hello"u8),
                    CreatedAt = DateTimeOffset.UnixEpoch,
                    UpdatedAt = DateTimeOffset.UnixEpoch,
                });
                await ctx.SaveChangesAsync();
            }

            await using (var ctx = new RstashDbContext(options))
            {
                var node = await ctx.Nodes.SingleAsync();

                Assert.Equal(1, node.UserId);
                Assert.Equal("documents/notes.txt", node.Path);
                Assert.Equal(ETag.ForDocument("hello"u8), node.ETag);
                Assert.True(node.Id > 0);
            }
        }
        finally
        {
            SqliteConnection.ClearAllPools();
            TryDelete(path);
            TryDelete(path + "-wal");
            TryDelete(path + "-shm");
        }
    }

    private static void TryDelete(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
        catch (IOException)
        {
            // Best effort — a lingering SQLite handle isn't a test failure.
        }
    }
}
