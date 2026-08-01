#### C# Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in class names, method names, property names, or namespace declarations; do not report at reference sites
- Typos in log messages, exception messages, or user-facing strings that affect readability

#### Null Reference Safety
- Dereference of a nullable reference without null checks when nullable reference types are enabled
- Use of the null-forgiving operator `!` when the value is not provably non-null
- `ArgumentNullException.ThrowIfNull` should be preferred over manual null-check-and-throw patterns
- Properties or fields returning nullable references that should be non-null by domain contract

#### Async and Task Handling
- `async void` outside of event handlers — exceptions cannot be caught and crash the process
- `.Result` or `.Wait()` blocking on tasks — risks deadlock; prefer `await`
- Missing `CancellationToken` propagation through public async APIs
- `Task.Run` wrapping inherently async work — just call the async method directly
- Fire-and-forget tasks (`_ = DoAsync()`) without error observation or exception handling
- `ConfigureAwait(false)` missing in library code where synchronization context capture is unnecessary

#### Resource Management
- `IDisposable` / `IAsyncDisposable` objects not wrapped in `using` / `await using`
- Resources opened in constructors but not disposed deterministically — implement `IDisposable`
- Stream, HttpClient, or DbConnection instances created but not disposed
- HttpClient as a singleton or static field without `IHttpClientFactory` — socket exhaustion risk

#### LINQ and Collection Usage
- `IEnumerable<T>` enumerated multiple times — causes re-execution; call `.ToList()` or `.ToArray()` first
- `.Count()` (extension method) on arrays or `List<T>` — use `.Length` or `.Count` property
- Chained `.Where()` calls that could be combined into a single predicate
- LINQ queries with side effects — should not mutate state inside `Select`/`Where` lambdas

#### Exception Handling
- `catch (Exception)` or `catch { }` that silently swallows all errors at non-boundary code
- Exception re-thrown with `throw ex` instead of `throw` — loses original stack trace
- `try-catch` blocks covering large scopes where only specific operations can fail
- Domain/business exceptions that should extend a project-specific base, not generic `Exception`

#### Performance
- Boxing of value types through non-generic collections or interfaces
- String concatenation in loops — use `StringBuilder`
- Missing `IEquatable<T>` on frequently-compared value types
- Large object allocations in hot paths without pooling or reuse strategy

#### Security
- Hardcoded connection strings, API keys, or credentials in source code
- SQL or command strings built with user-controlled string interpolation — use parameterized queries
- XSS from unencoded user input rendered in views without proper encoding
- Path traversal from unsanitized file paths built from user input
- `[AllowAnonymous]` on sensitive endpoints or missing authorization attributes

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
