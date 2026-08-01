#### Dart/Flutter Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in class names, method names, or variable names at their declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Null Safety
- Non-null assertion (`!`) used where a null value is legitimately possible — prefer `?.`, `??`, or early guard
- `late` modifier on fields not guaranteed to be initialized before first access — prefer nullable or constructor init
- Missing `required` on constructor parameters that must always be provided
- `Future<T?>` where the null case is treated as a success — distinguish absence from failure

#### Widget and Build Method
- Side effects (network requests, timers, async operations) inside `build()` method
- Heavy computation inside `build()` that should be cached or moved to initState/controller
- `setState` called after the widget is disposed — check `mounted` before calling
- Building widgets with deeply nested conditionals — extract to helper methods

#### State Management
- `initState` / `dispose` without calling `super.initState()` / `super.dispose()`
- `context` used after an `await` without checking `context.mounted`
- `TextEditingController` / `AnimationController` / `ScrollController` not disposed
- Business logic mixed into widget classes — extract to controller, bloc, or provider

#### Collections and Iterables
- `List.filled` / `List.generate` with a mutable object — every element references the same instance
- Iterable modifications during iteration (`ConcurrentModificationError`)
- `where()` / `map()` re-evaluated on each access — call `.toList()` for lazy iterables
- Missing type parameters on empty collections (`[]` instead of `<String>[]`)

#### Async Patterns
- `await` on a `Future` that will never complete — missing error/timeout handling
- `FutureBuilder` / `StreamBuilder` not handling loading and error states
- Unawaited futures not wrapped in `unawaited()` — signals intentional fire-and-forget
- `Future.wait` with eager error turned off implicitly missing first-error diagnostics

#### File and Network
- File, socket, or stream operations without `try`/`finally` or `using` equivalent
- Hardcoded file paths — use `path_provider` for platform-appropriate directories
- Unsecured HTTP calls on mobile — exceptions needed for cleartext in production
- Large file reads on main isolate — use `compute()` or `Isolate.run` for CPU-intensive work

#### Security
- Hardcoded API keys, tokens, or credentials in source code
- Debug build features enabled in release mode (`debugShowCheckedModeBanner`)
- Local data stored without encryption for sensitive information — use `flutter_secure_storage`
- Deep links / URL schemes without validation of incoming parameters

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
