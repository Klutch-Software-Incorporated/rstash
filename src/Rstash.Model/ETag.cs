using System.Security.Cryptography;
using System.Text;

namespace Rstash.Model;

/// <summary>
/// remoteStorage ETag computation. Ports the Go reference exactly: the ETag is
/// the first 8 bytes of a SHA-256 digest rendered as lowercase hex (16 chars).
/// </summary>
public static class ETag
{
    /// <summary>ETag for a document: first 8 bytes of SHA-256(content), hex.</summary>
    public static string ForDocument(ReadOnlySpan<byte> content)
    {
        Span<byte> hash = stackalloc byte[SHA256.HashSizeInBytes];
        SHA256.HashData(content, hash);
        return Convert.ToHexStringLower(hash[..8]);
    }

    /// <summary>
    /// ETag for a folder, derived from its direct children. Hashes the
    /// ordinal-sorted concatenation of each child's name followed by its ETag,
    /// so the result is stable regardless of enumeration order.
    /// </summary>
    public static string ForFolder(IReadOnlyDictionary<string, string> children)
    {
        var builder = new StringBuilder();
        foreach (var name in children.Keys.OrderBy(static n => n, StringComparer.Ordinal))
        {
            builder.Append(name);
            builder.Append(children[name]);
        }

        Span<byte> hash = stackalloc byte[SHA256.HashSizeInBytes];
        SHA256.HashData(Encoding.UTF8.GetBytes(builder.ToString()), hash);
        return Convert.ToHexStringLower(hash[..8]);
    }

    /// <summary>Wraps an ETag value in double quotes for HTTP headers.</summary>
    public static string Quote(string etag) => $"\"{etag}\"";

    /// <summary>Strips surrounding double quotes from an ETag header value.</summary>
    public static string Unquote(string value) =>
        value.Length >= 2 && value[0] == '"' && value[^1] == '"'
            ? value[1..^1]
            : value;
}
