namespace Rstash.Storage;

/// <summary>A blob row in the database backend, keyed by (UserId, Path).</summary>
public class Blob
{
    public long UserId { get; set; }

    public string Path { get; set; } = string.Empty;

    public byte[] Data { get; set; } = [];
}
