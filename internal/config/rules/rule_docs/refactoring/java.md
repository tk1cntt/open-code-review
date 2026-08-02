#### Java Refactoring Rules (delta)

Overrides common where noted. Keep only Java idioms.

##### Naming
- Avoid stutter: `UserValidator.validateUser()` → `UserValidator.validate()`
- Package-private types should be named for clarity, not brevity

##### Null Safety
- Repeated null checks on the same expression suggest early return or `Optional`
- Overuse of `Objects.requireNonNull` suggests unclear contracts — document or restructure
- `Optional` as a field or parameter is an anti-pattern; prefer it for return types only
- Prefer `@Nullable` / tooling over ad-hoc NPE throws for contracts

##### Error Handling
- Catch blocks that only log and rethrow add noise — remove or add meaningful recovery
- Catch of `Exception` or `Throwable` is too broad
- Resources must use try-with-resources
- Checked exceptions wrapped in `RuntimeException` must include the original as cause

##### Control Flow
- Long if-else chains testing the same variable → `switch` or polymorphism
- Loops with break/continue as primary control may be clearer as streams

##### Coupling and Dependency
- Direct `new Service()` in domain code prevents testing — inject
- Request/context objects should flow as parameters, not stored in long-lived fields

##### Testability
- `System.currentTimeMillis()` → injectable `Clock`
- Direct file I/O → injectable `Path` / resource abstraction
- Static calls that hit external systems → injectable services
