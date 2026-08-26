namespace Rstash.Core.Tests;

/// <summary>
/// A <see cref="FactAttribute"/> that self-skips on Windows, for behaviour that only exists
/// on the platforms rstash is actually deployed to. Creating a symbolic link on Windows
/// needs elevation or Developer Mode, so a test that builds one would fail on a developer
/// machine for reasons that say nothing about the code — but it still has to run on CI,
/// which is <c>ubuntu-latest</c>.
/// </summary>
public sealed class UnixFactAttribute : FactAttribute
{
    public UnixFactAttribute()
    {
        if (OperatingSystem.IsWindows())
        {
            Skip = "Unix-only: creating symbolic links on Windows requires elevation or Developer Mode.";
        }
    }
}
