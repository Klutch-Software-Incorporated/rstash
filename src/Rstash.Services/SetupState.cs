namespace Rstash.Services;

/// <summary>
/// Caches whether first-run setup is complete (i.e. at least one user exists),
/// so the setup guard stops hitting the database once an account is created.
/// </summary>
public sealed class SetupState
{
    private volatile bool _complete;

    public bool IsComplete => _complete;

    public void MarkComplete() => _complete = true;
}
