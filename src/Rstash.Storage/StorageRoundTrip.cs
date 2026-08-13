namespace Rstash.Storage;

/// <summary>
/// Shared <see cref="IStorageProbe"/> body for backends whose health is best
/// shown by using them: write a scratch blob, read it back, delete it.
/// </summary>
/// <remarks>
/// A connect-only check passes against a store that cannot take a single write —
/// a missing table, a read-only mount, a credential that can list but not put.
/// Those all surface on a user's first upload otherwise.
/// </remarks>
internal static class StorageRoundTrip
{
    /// <summary>Identity ids start at 1, so a probe under 0 can never collide with real data.</summary>
    private const long ProbeUserId = 0;

    internal static async Task RunAsync(IStorage storage, CancellationToken cancellationToken)
    {
        var path = $".rstash-probe-{Guid.NewGuid():N}";
        byte[] payload = [0x72, 0x73];

        await storage.PutAsync(ProbeUserId, path, payload, cancellationToken);
        try
        {
            await using var stream = await storage.GetAsync(ProbeUserId, path, cancellationToken);
            using var buffer = new MemoryStream();
            await stream.CopyToAsync(buffer, cancellationToken);

            if (!buffer.ToArray().AsSpan().SequenceEqual(payload))
            {
                throw new InvalidOperationException(
                    "blob store returned different bytes than were written to it");
            }
        }
        finally
        {
            await storage.DeleteAsync(ProbeUserId, path, cancellationToken);
        }
    }
}
