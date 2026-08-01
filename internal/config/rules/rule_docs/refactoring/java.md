#### Java Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter object or builder pattern
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Switch statements with >7 cases may benefit from strategy pattern or Map<Enum, Handler>
- Long if-else chains testing the same variable should be replaced with switch or polymorphism

##### Naming
- Variable and method names should clearly communicate intent without abbreviations
- Boolean variables and methods should use `is`/`has`/`can`/`should` prefixes
- Package-private classes and methods should be named for clarity, not brevity
- Avoid stutter: `UserValidator.validateUser()` → `UserValidator.validate()`

##### Null Safety
- Repeated null checks on the same expression suggest missing early return or Optional
- Overuse of `Objects.requireNonNull` suggests unclear contracts — document or restructure
- `Optional` used as a field or parameter is an anti-pattern; prefer it for return types only
- Null checks that throw generic NPE should use `@Nullable` + tooling instead of manual guards

##### Error Handling
- Catch blocks that only log and rethrow add noise — remove or add meaningful recovery
- Catch of `Exception` or `Throwable` is too broad; catch specific exception types
- Resources opened without try-with-resources risk leaks
- Checked exceptions wrapped in RuntimeException lose context — include original as cause
- Empty catch blocks hide failures; at minimum log with context

##### Duplication
- Identical code blocks in the same class should be extracted into private methods
- Similar logic across related classes suggests an abstract base class or interface
- Repeated null-check-then-throw patterns should be centralized in utility methods
- Test setup duplicated across test classes should use abstract test base or @BeforeEach helpers

##### Dead Code
- Unused private methods and fields should be removed
- Unused imports should be cleaned (IDE warnings)
- Commented-out code blocks should be deleted
- Never-read variables and parameters should be eliminated

##### Control Flow
- Nested if-else chains should be flattened with early returns
- Boolean flag parameters (`boolean dryRun`) suggest split into two methods
- Loops with break/continue as primary control flow may be clearer as streams or iterators
- Repeated condition checks across methods suggest missing polymorphic dispatch

##### Data and State
- Mutable fields accessible outside the class break encapsulation — use defensive copies
- Primitive obsession (string for email, int for status) should be wrapped in value types
- Collections returned from getters should be unmodifiable or defensive copies
- Static mutable state is a testability and thread-safety risk — prefer instance state

##### Coupling and Dependency
- Direct constructor calls (`new Service()`) prevent testing — use dependency injection
- Circular dependencies between packages should be broken with interfaces
- Static calls to utility methods with side effects (DB, HTTP) hinder testability
- `context.Context` equivalent (RequestContext) should flow as parameter, not stored in fields

##### Testability
- Direct `System.currentTimeMillis()` calls should use `Clock` interface for testing
- Direct file I/O (`Files.readString`, `new FileInputStream`) — consider injectable Path or Resource abstraction
- Static method calls that hit external systems should be wrapped in injectable services
- `new` inside methods for non-value objects prevents mocking — inject via constructor

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
