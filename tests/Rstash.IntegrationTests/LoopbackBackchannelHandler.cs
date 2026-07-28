using Microsoft.AspNetCore.Hosting.Server;
using Microsoft.AspNetCore.TestHost;
using Microsoft.Extensions.DependencyInjection;

namespace Rstash.IntegrationTests;

/// <summary>
/// Routes the OpenID Connect handler's back-channel calls back into the test server.
/// </summary>
/// <remarks>
/// <para>
/// As a relying party against itself, rstash fetches its own discovery document and
/// POSTs to its own token endpoint over real HTTP. Under <c>TestServer</c> there is no
/// socket to reach, so those calls fail and every challenge becomes a 500.
/// </para>
/// <para>
/// The server cannot be resolved when options are configured — it does not exist until
/// the host is built — so the inner handler is resolved on first use instead.
/// </para>
/// </remarks>
internal sealed class LoopbackBackchannelHandler(IServiceProvider services) : DelegatingHandler
{
    private readonly Lock _gate = new();
    private HttpMessageHandler? _inner;

    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        InnerHandler ??= Resolve();
        return base.SendAsync(request, cancellationToken);
    }

    private HttpMessageHandler Resolve()
    {
        lock (_gate)
        {
            return _inner ??= ((TestServer)services.GetRequiredService<IServer>()).CreateHandler();
        }
    }
}
