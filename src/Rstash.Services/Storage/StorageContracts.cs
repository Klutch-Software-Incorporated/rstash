namespace Rstash.Services.Storage;

/// <summary>Parsed If-Match / If-None-Match header values (unquoted ETags).</summary>
public sealed record StorageConditions
{
    public string? IfMatch { get; init; }

    public IReadOnlyList<string> IfNoneMatch { get; init; } = [];

    public bool IfNoneMatchStar => IfNoneMatch is ["*"];

    public bool IfNoneMatchContains(string etag) => IfNoneMatch.Contains(etag, StringComparer.Ordinal);
}

public sealed record PutResult(string ETag, bool IsNew);

public sealed record GetResult(Stream Content, string ContentType, long ContentLength, string ETag);

public sealed record HeadResult(string ContentType, long ContentLength, string ETag);

public sealed record DeleteResult(string ETag);
