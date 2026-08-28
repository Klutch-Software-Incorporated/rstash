using System.Reflection;
using System.Reflection.Emit;
using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

public class BuildInfoTests
{
    [Fact]
    public void VersionOf_ReturnsUnknown_WhenAssemblyIsNull()
    {
        Assert.Equal("unknown", BuildInfo.VersionOf(null));
    }

    [Fact]
    public void VersionOf_ReturnsTheInformationalVersion_WhenStamped()
    {
        var assembly = typeof(BuildInfo).Assembly;
        var expected = assembly
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()!
            .InformationalVersion;

        Assert.Equal(expected, BuildInfo.VersionOf(assembly));
    }

    [Fact]
    public void VersionOf_ReturnsUnknown_WhenTheAttributeIsAbsent()
    {
        // A dynamic assembly carries no InformationalVersion, which is the only way to
        // exercise the fallback: every compiled assembly here is stamped by the SDK.
        AssemblyBuilder bare = AssemblyBuilder.DefineDynamicAssembly(
            new AssemblyName("Rstash.Tests.Unstamped"),
            AssemblyBuilderAccess.Run);

        Assert.Null(bare.GetCustomAttribute<AssemblyInformationalVersionAttribute>());
        Assert.Equal("unknown", BuildInfo.VersionOf(bare));
    }

    [Fact]
    public void Version_IsNeverNullOrWhitespace()
    {
        // Whatever hosts the process, the UI and `rstash version` always have something
        // to print rather than an empty footer or a blank line.
        Assert.False(string.IsNullOrWhiteSpace(BuildInfo.Version));
    }
}
