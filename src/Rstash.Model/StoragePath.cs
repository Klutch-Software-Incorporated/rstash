namespace Rstash.Model;

/// <summary>
/// Validates remoteStorage paths per draft-dejong-remotestorage-26: leading
/// slash, no null bytes, no empty/"."/".." segments, ≤ 512 code points.
/// </summary>
public static class StoragePath
{
    public const int MaxLength = 512;

    public static bool TryValidate(string path, out string? error)
    {
        if (path.Length == 0 || path[0] != '/')
        {
            error = "must start with /";
            return false;
        }

        if (path.Contains('\0'))
        {
            error = "null byte not allowed";
            return false;
        }

        if (path.EnumerateRunes().Count() > MaxLength)
        {
            error = $"path exceeds {MaxLength} characters";
            return false;
        }

        var segments = path.Split('/');
        for (var i = 1; i < segments.Length; i++)
        {
            var segment = segments[i];

            // A trailing slash (folder path) leaves an empty final segment — allowed.
            if (segment.Length == 0 && i == segments.Length - 1)
            {
                continue;
            }

            if (segment.Length == 0)
            {
                error = "empty segment (double slash)";
                return false;
            }

            if (segment is "." or "..")
            {
                error = $"segment \"{segment}\" not allowed";
                return false;
            }
        }

        error = null;
        return true;
    }
}
