#### F# Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in type names, function names, module names, or union case names at declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Pattern Matching
- Non-exhaustive `match` expressions — add wildcard or missing cases to make exhaustive
- `match` on `Option`/`Result` where `Option.map`/`Option.bind`/`Result.map` would be clearer
- Deeply nested pattern matches that can be flattened with active patterns or helper functions
- `if x.IsSome then x.Value` used instead of pattern matching or `Option.defaultValue`

#### Option and Result Handling
- `Option.get` / `Value` property called on `None` — use pattern match or `defaultValue`/`defaultWith`
- `Result.Error` accessed without checking `Result.IsError` first
- Functions returning `'a option` when `Result<'a, string>` or a custom error type would give better diagnostics
- `null` used in F# code instead of `Option<T>` — except for interop boundaries

#### Immutability and State
- `mutable` keyword used when a purely functional approach with `let` rebinding or fold would work
- Reference cells (`ref`) used when `mutable` or an accumulator pattern would be simpler
- `lock` / `Monitor` usage when immutable data structures would eliminate shared mutable state

#### Async and Tasks
- `Async.RunSynchronously` on UI/main thread — use `Async.StartImmediate` or `task {}` CE
- `async {}` mixed with `task {}` — prefer `task {}` for .NET interop, `async {}` for pure F#
- Fire-and-forget `Async.Start` without exception handling — use `Async.StartWithContinuations`
- Blocking `Task.Wait()`/`Task.Result` in F# async code — deadlock risk like C#

#### Type Design
- Discriminated unions used when a record would be simpler, or vice versa
- Missing `[<RequireQualifiedAccess>]` on discriminated unions with commonly-named cases
- Recursive types without `and` keyword causing compilation errors
- Single-case discriminated unions missing for type-safety on primitives

#### Interop and Resource Management
- `use` not used for `IDisposable` resources — deterministic cleanup is idiomatic
- `ComputationExpression` not calling `Dispose` on enumerators/enumerable resources
- P/Invoke calls missing safety attributes or error checking

#### Security
- Hardcoded API keys, tokens, or credentials in source code
- User-controlled strings passed to `System.Diagnostics.Process.Start`
- SQL queries built with string interpolation instead of parameterized queries

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
