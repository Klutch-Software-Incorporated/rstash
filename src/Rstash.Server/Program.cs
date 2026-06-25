var builder = WebApplication.CreateBuilder(args);

builder.Services.AddHealthChecks();

var app = builder.Build();

// Liveness probe. The Blazor Web App host, storage endpoints, and auth are
// layered on in later phases (P2–P5); this keeps P0 to a runnable shell.
app.MapHealthChecks("/healthz");

app.Run();
