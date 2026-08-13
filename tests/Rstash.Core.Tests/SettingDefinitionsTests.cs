using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

public class SettingDefinitionsTests
{
    [Fact]
    public void All_HasUniqueKeys()
    {
        var keys = SettingDefinitions.All.Select(d => d.Key).ToList();
        Assert.Equal(keys.Count, keys.Distinct(StringComparer.Ordinal).Count());
    }

    [Fact]
    public void ByKey_ReturnsExpectedDefinition()
    {
        var def = SettingDefinitions.ByKey("registration_mode");

        Assert.NotNull(def);
        Assert.Equal("closed", def.Default);
        Assert.Equal(SettingInputType.Select, def.InputType);
        Assert.Equal(["open", "approval", "closed"], def.ValidValues);
        Assert.True(def.RuntimeEditable);
    }

    [Fact]
    public void ByKey_UnknownReturnsNull()
    {
        Assert.Null(SettingDefinitions.ByKey("does_not_exist"));
    }

    [Fact]
    public void RuntimeEditable_ExcludesEnvOnlyBootSettings()
    {
        Assert.DoesNotContain(SettingDefinitions.RuntimeEditable, d => d.Key == "addr");
        Assert.DoesNotContain(SettingDefinitions.RuntimeEditable, d => d.Key == "database_dsn");
        Assert.Contains(SettingDefinitions.RuntimeEditable, d => d.Key == "site_name");
    }

    [Fact]
    public void ByteSizeSetting_IsTypedAndRuntimeEditable()
    {
        var def = SettingDefinitions.ByKey("max_upload_size");

        Assert.NotNull(def);
        Assert.Equal(SettingInputType.ByteSize, def.InputType);
        Assert.Equal("50MB", def.Default);
        Assert.True(def.RuntimeEditable);
    }

    [Fact]
    public void EnvBackedSettings_CarryEnvVarNames()
    {
        Assert.Equal(EnvVars.Addr, SettingDefinitions.ByKey("addr")!.EnvVar);
        Assert.Equal(EnvVars.Database, SettingDefinitions.ByKey("database_dsn")!.EnvVar);
        Assert.Null(SettingDefinitions.ByKey("site_name")!.EnvVar);
    }
}
