#### Family: .NET (C# / F#)

Shared patterns for .NET languages. Language deltas override this family.

##### Nullability & Types
- Prefer nullable reference types / Option over ad-hoc null conventions
- Prefer records / immutable models for value-like data
- Prefer exhaustive pattern matching on closed sets

##### Async
- Prefer `Task`/`async` end-to-end; avoid fire-and-forget without observation
- Independent awaits → concurrent combinators (`WhenAll` / parallel CE)
- Async method names should reflect asynchrony (C# `Async` suffix)

##### DI & Testability
- Prefer constructor injection over service locator in domain code
- Prefer `TimeProvider` / abstractions over static `DateTime` and direct IO
