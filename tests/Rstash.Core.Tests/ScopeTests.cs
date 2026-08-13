using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>Ported from the Go implementation's internal/api/scope_test.go.</summary>
public class ScopeTests
{
    [Theory]
    [InlineData(new[] { "contacts:r" }, "/contacts/card.vcf", false, true)]
    [InlineData(new[] { "contacts:rw" }, "/contacts/card.vcf", true, true)]
    [InlineData(new[] { "contacts:r" }, "/contacts/card.vcf", true, false)]   // read denies write
    [InlineData(new[] { "contacts:rw" }, "/calendar/event.ics", true, false)] // wrong module
    [InlineData(new[] { "*:r" }, "/anything/file.txt", false, true)]
    [InlineData(new[] { "*:rw" }, "/anything/file.txt", true, true)]
    [InlineData(new[] { "*:r" }, "/anything/file.txt", true, false)]
    [InlineData(new[] { "photos:r" }, "/public/photos/pic.jpg", false, true)]   // public → module
    [InlineData(new[] { "photos:rw" }, "/public/photos/pic.jpg", true, true)]
    [InlineData(new[] { "contacts:r" }, "/public/photos/pic.jpg", false, false)]
    [InlineData(new[] { "photos:r" }, "/public/photos/", false, true)]
    [InlineData(new[] { "*:rw" }, "/public/photos/pic.jpg", true, true)]
    [InlineData(new[] { "photos:rw", "contacts:r" }, "/photos/pic.jpg", true, true)]
    [InlineData(new[] { "photos:rw", "contacts:r" }, "/contacts/card.vcf", false, true)]
    [InlineData(new[] { "*:r" }, "/", false, true)]
    public void Grants_MatchesReference(string[] scopes, string path, bool write, bool expected)
    {
        Assert.Equal(expected, Scope.Grants(scopes, path, write));
    }

    [Theory]
    [InlineData("contacts:r", true, 1)]
    [InlineData("contacts:rw", true, 1)]
    [InlineData("*:r", true, 1)]
    [InlineData("*:rw", true, 1)]
    [InlineData("photos:r contacts:rw", true, 2)]
    [InlineData("", false, 0)]
    [InlineData("invalid", false, 0)]
    [InlineData("contacts:x", false, 0)]
    [InlineData("contacts:", false, 0)]
    [InlineData("public:r", false, 0)]   // reserved word
    [InlineData("public:rw", false, 0)]
    public void TryParse_MatchesReference(string input, bool ok, int count)
    {
        var parsed = Scope.TryParse(input, out var scopes);

        Assert.Equal(ok, parsed);
        if (ok)
        {
            Assert.Equal(count, scopes.Count);
        }
    }
}
