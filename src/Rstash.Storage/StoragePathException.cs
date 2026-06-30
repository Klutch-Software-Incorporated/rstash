namespace Rstash.Storage;

/// <summary>Thrown when a resolved blob path would escape its storage root.</summary>
public sealed class StoragePathException(string message) : Exception(message);
