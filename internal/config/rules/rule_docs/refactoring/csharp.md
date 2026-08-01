#### C# Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter object record or builder pattern
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long switch expressions (>7 cases) may benefit from strategy pattern or dictionary dispatch
- Complex LINQ chains spanning many lines should be broken into named intermediate variables

##### Naming
- Use PascalCase for classes, records, interfaces, methods, properties; camelCase for parameters and locals
- Boolean variables and methods should use `is`/`has`/`can`/`should` prefixes
- Async methods should end with `Async` suffix
- Avoid Hungarian notation and type-based prefixes (`strName`, `intCount`)
- Interface names should start with `I` (e.g., `IUserRepository`)

##### Immutability
- Prefer `record` or `record struct` for immutable value-like models
- Use `init`-only properties for read-only configuration objects
- Return `IReadOnlyCollection<T>`, `IReadOnlyList<T>`, or defensive copies from public APIs
- Avoid exposing mutable list/dictionary fields — use `ImmutableArray<T>` or `.AsReadOnly()`
- Copy-on-write for state updates: create new instances rather than mutating fields

##### Null Safety
- Repeated null checks on the same expression should use early return or pattern matching
- Null checks that throw ArgumentNullException should use `ArgumentNullException.ThrowIfNull`
- `Nullable<T>` used where `T?` with nullable reference types would be clearer
- Conditional access chains (`?.`) used excessively — consider extracting to a well-named method

##### Async Patterns
- Overlapping `await` in sequence when `Task.WhenAll` would be concurrent
- Fire-and-forget tasks without exception handling suggest missing error boundary
- `Task.Delay` used as a synchronization mechanism — consider `SemaphoreSlim` or proper coordination

##### Duplication
- Identical code blocks in the same class should be extracted into private methods
- Similar logic across related classes suggests an abstract base or interface
- Repeated null-check-and-throw patterns should use `ArgumentException.ThrowIfNullOrEmpty` or helpers
- Test setup duplicated across test classes should use abstract base or shared fixtures

##### Dead Code
- Unused private methods, fields, and `using` directives should be removed
- Commented-out code blocks should be deleted
- Unreachable code after `return`/`throw`/`break`/`continue`

##### Control Flow
- Nested if-else chains should be flattened with early returns or pattern matching
- Boolean flag parameters suggest splitting into two methods
- Repeated `if (x is T)` patterns across methods suggest missing polymorphic dispatch
- Switch expressions with exhaustive patterns preferred over if-else chains on enums/sealed types

##### Data and State
- Primitive obsession (string for email, int for status) should be wrapped in value objects or enums
- Groups of related data passed together suggest a record struct or class
- Static mutable state should be minimized — passes test isolation and thread safety
- Mutable collections returned from properties without defensive copying

##### Coupling and Dependency
- Direct `new` in business logic prevents testing — use constructor injection
- Static calls to `DateTime.Now`/`File.ReadAllText`/`HttpClient` — inject abstractions
- Circular dependencies between projects should be broken with interfaces or project restructuring
- Service locator (`IServiceProvider.GetService`) in domain logic — inject the specific dependency

##### Testability
- Direct `DateTime.Now` / `DateTimeOffset.Now` — use `TimeProvider` (NET 8+) or injectable clock
- Direct `Random` construction — inject or use `Random.Shared` for non-deterministic needs
- Direct `File.ReadAllText` / `Directory.GetFiles` — inject `IFileSystem` or use `System.IO.Abstractions`
- Direct `HttpClient` construction — use `IHttpClientFactory`

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
