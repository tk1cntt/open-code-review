#### C# Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- PascalCase for types/methods/properties; camelCase for locals/parameters
- Async methods end with `Async`
- Interfaces start with `I`
- Avoid Hungarian notation

##### Immutability
- Prefer `record` / `record struct` for value-like models
- Prefer `init`-only properties for configuration
- Return `IReadOnlyCollection<T>` / `IReadOnlyList<T>` or defensive copies
- Prefer `ImmutableArray<T>` / `.AsReadOnly()` over exposing mutable collections

##### Null Safety
- Prefer early return / pattern matching over repeated null checks
- Use `ArgumentNullException.ThrowIfNull` (and related helpers)
- Prefer nullable reference types over ad-hoc null conventions

##### Async Patterns
- Independent awaits → `Task.WhenAll`
- Fire-and-forget tasks need exception handling
- Do not use `Task.Delay` as a synchronization primitive

##### Control Flow
- Prefer switch expressions and exhaustive patterns on enums/sealed types

##### Coupling and Dependency
- Avoid service locator (`IServiceProvider.GetService`) in domain logic
- Prefer constructor injection over static `DateTime`/`File`/`HttpClient` calls

##### Testability
- Prefer `TimeProvider` (.NET 8+) or injectable clock over `DateTime.Now`
- Prefer `IHttpClientFactory` over manual `HttpClient` construction
- Prefer filesystem abstractions over direct `File`/`Directory` APIs
