#### Kotlin Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a data class parameter object or builder
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long `when` blocks (>7 branches) that share logic should consider sealed class + polymorphic dispatch
- Complex expression bodies wrapping across multiple lines should be broken into named intermediate values

##### Naming and Style
- Functions use camelCase; classes use PascalCase; constants use UPPER_SNAKE_CASE
- Boolean variables and functions should use `is`/`has`/`can`/`should` prefixes
- Single-letter names only for short-loop iterators and lambda parameters (`it` for single-param lambdas)
- Avoid stutter: `UserValidator.validateUser()` → `UserValidator.validate()`
- Private helpers, extensions, and top-level declarations should use clear names over brevity

##### Function and Expression Conciseness
- Single-expression functions should use `=` syntax (`fun sum(a: Int, b: Int) = a + b`)
- Redundant `return` statements when expression results would suffice (especially in lambdas)
- Long `if-else if` chains testing the same value should use `when` expression
- Functions that take a lambda as the last parameter should use trailing lambda syntax for readability

##### Null Safety
- Overuse of `!!` (non-null assertion) — prefer safe calls `?.`, Elvis `?:`, or early return with `?: return`
- Repeated null checks on the same expression should use `?.let { ... }` or early guard
- Nullable properties without reasonable defaults should be checked at boundaries
- `lateinit var` used when a `by lazy {}` or default value would be safer

##### Collection Operation Optimization
- Multiple chained `map`/`filter` operations on large collections should use `asSequence()` for lazy evaluation
- Manual loops implementing what standard library functions provide (`groupBy`, `associate`, `partition`, `any`, `all`)
- Redundant intermediate `toList()`/`toSet()` calls within a chain
- Avoid creating objects inside loops (e.g., `Regex` instances, temporary collections)

##### Coroutine Correctness
- `GlobalScope` should be replaced with structured concurrency (`viewModelScope`, `coroutineScope`, `lifecycleScope`)
- `async` without `await` or proper error handling risks silent failure
- Blocking calls (`Thread.sleep`, `runBlocking`) inside coroutine context should use `delay` or `withContext(Dispatchers.IO)`
- Missing `try/catch` or `CoroutineExceptionHandler` where coroutine failure matters
- Long-running suspend functions should use `yield()` to allow cancellation and fairness

##### Class and Object Design
- Classes with only data and no behavior should be `data class` (auto-generates equals/hashCode/copy)
- Restricted type hierarchies should use `sealed class`/`sealed interface` for exhaustive `when` checking
- Property delegation (`by lazy`, `observable`, `vetoable`) should replace manual getter/setter logic
- Class delegation (`by`) for decorator pattern instead of manual forwarding methods
- Top-level extension functions for utilities that don't need class state

##### Resource Management
- `Closeable` resources should use `.use { }` extension (auto-close on block exit)
- File/stream operations without `use` block risk resource leaks on exception paths
- Nested `use` blocks should be flattened with explicit resource management where possible

##### Error Handling
- `try-catch` blocks that only log and rethrow add noise — remove or add meaningful recovery
- Catch of `Exception` is too broad; catch specific exception types
- Empty catch blocks hide failures — at minimum log with context
- `runCatching` with ignored result should handle or propagate the failure

##### Duplication
- Identical code blocks in the same file should be extracted into private functions or extensions
- Similar logic across related classes suggests an abstract base class, interface, or sealed hierarchy
- Repeated validation patterns should be centralized
- Test setup duplicated across test classes should use shared fixtures or base test classes

##### Dead Code
- Unused imports and private declarations should be removed
- Commented-out code blocks should be deleted
- Never-read variables and parameters should be eliminated
- Unused extension functions should be removed

##### Control Flow
- Nested if-else chains should be flattened with early returns, `when`, or `?.let`
- Boolean flag parameters suggest splitting the function in two
- Loops with break/continue as primary flow — may be clearer as sequence operations
- `require`/`check` should replace manual `if-throw` at function entry points

##### Data and State
- Prefer `val` over `var` — mutable state introduces subtle coupling
- Mutable collections exposed through public APIs should return read-only views (`List` not `MutableList`)
- Primitive obsession (string for email, int for status) should be wrapped in value/inline classes
- Companion object mutable state is shared across instances and thread-unsafe — use judiciously
- String concatenation in loops should use `buildString { }` for efficiency

##### Coupling and Dependency
- Direct constructor calls (`Service()`) prevent testing — use constructor injection or factory
- Circular dependencies between packages should be broken with interfaces
- Object declarations (`object`) with side effects act as singletons — ensure testability
- Extension functions with external dependencies should be injectable or testable

##### Testability
- Direct `System.currentTimeMillis()` / `Instant.now()` calls should use injectable `Clock`
- Direct file operations (`File.readText()`) — consider injectable path or filesystem abstraction
- Direct network calls (`URL.readText()`, `HttpClient`) — inject client for mocking
- Object/companion singletons with state make tests order-dependent

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
