using System.Reflection;

namespace Rstash.Services.Configuration;

/// <summary>
/// The running build's identity. Lives here rather than in the host so the web UI
/// can show it too — an operator should be able to tell which version a server is
/// running without shell access to it.
/// </summary>
public static class BuildInfo
{
    /// <summary>
    /// "v0.5.0" from a release build, "v0.5.0+dev" from anything else. Read once:
    /// the entry assembly cannot change while the process is alive.
    /// </summary>
    public static string Version { get; } = VersionOf(Assembly.GetEntryAssembly());

    /// <summary>
    /// The informational version stamped on <paramref name="assembly"/>, or
    /// "unknown" when there isn't one. Separate from <see cref="Version"/> so the
    /// fallback is testable without a process whose entry assembly lacks the attribute.
    /// </summary>
    public static string VersionOf(Assembly? assembly) =>
        assembly?
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()?
            .InformationalVersion
        ?? "unknown";
}
