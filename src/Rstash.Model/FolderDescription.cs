using System.Text.Json.Serialization;

namespace Rstash.Model;

/// <summary>
/// A single entry in a remoteStorage folder listing. Field names follow the
/// spec's folder-description vocabulary verbatim (capitalised, hyphenated).
/// </summary>
public sealed record FolderItem
{
    [JsonPropertyName("ETag")]
    public required string ETag { get; init; }

    [JsonPropertyName("Content-Type")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? ContentType { get; init; }

    [JsonPropertyName("Content-Length")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public long? ContentLength { get; init; }

    [JsonPropertyName("Last-Modified")]
    [JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? LastModified { get; init; }
}

/// <summary>
/// The JSON-LD folder listing returned for a folder GET, per
/// draft-dejong-remotestorage-26.
/// </summary>
public sealed record FolderDescription
{
    public const string FolderDescriptionContext =
        "http://remotestorage.io/spec/folder-description";

    [JsonPropertyName("@context")]
    public string Context { get; init; } = FolderDescriptionContext;

    [JsonPropertyName("items")]
    public Dictionary<string, FolderItem> Items { get; init; } = [];
}
