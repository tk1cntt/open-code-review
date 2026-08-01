#### Refactoring Rules (Common)

These rules apply to all programming languages. Focus on structural improvements that make code clearer, more maintainable, and less error-prone without changing observable behavior.

##### Complexity
- Functions exceeding 80 lines should be considered for splitting by responsibility
- Cyclomatic complexity should be kept low — nested conditions should use guard clauses with early returns
- Functions with >4 parameters suggest grouping related parameters into a struct/object
- Boolean expressions with >3 terms should be extracted into well-named intermediate variables
- Switch/match statements with >7 cases may benefit from strategy pattern or map/dictionary lookup

##### Naming and Readability
- Variable names should convey intent — single-letter names only for short-loop iterators or receivers
- Function names should reflect what they do, including side effects
- Boolean variables/functions should use `is`/`has`/`can`/`should` prefixes
- Magic numbers and magic strings should be replaced with named constants
- The same concept should use the same term throughout the codebase; one term should not mean multiple concepts

##### Function and Method Design
- Functions should do one thing — query and mutation should be separate
- Boolean flag parameters that switch behavior suggest the function has multiple responsibilities
- Functions should not depend on excessive global/mutable state
- Unused parameters should be removed
- Output parameters should be obvious from the signature — avoid surprising side effects

##### Duplication and Reuse
- Duplicate code blocks (same logic repeated verbatim) should be extracted into a shared function
- Near-duplicate code differing only by literal values or types should be parameterized
- Repeated validation logic should be centralized
- Repeated error-handling patterns should be extracted into helpers
- Avoid premature abstraction — two similar blocks may not warrant extraction if they serve different purposes

##### Error Handling
- Errors should not be silently ignored; always handle or propagate with context
- Error messages should include context about what operation failed
- Catch blocks should be specific — avoid catching overly broad exception types
- Exceptions should not be used for normal control flow
- Resource cleanup should use try-with-resources / defer / context-manager patterns, not scattered exit paths

##### Dead Code and Simplification
- Unused functions, variables, and imports should be removed
- Unreachable code branches should be eliminated
- Commented-out code should be deleted (version control retains history)
- Redundant assignments, conversions, and conditions should be simplified
- Wrapper functions that add no value should be inlined

##### Control Flow
- Nested `if` statements should be converted to guard clauses with early returns
- Repeated condition checks across branches suggest missing polymorphism or strategy pattern
- Loops with multiple control flags can often be decomposed into filter → transform → aggregate pipeline
- Try-catch blocks should cover the minimal necessary scope
- Conditions that are always true/false in context indicate dead or misleading code

##### Immutability (CRITICAL)
- Favor immutable data structures — copy-on-write rather than in-place mutation
- Return defensive copies from public APIs instead of exposing internal mutable state
- Use `const`/`final`/`val`/`let` by default; only introduce mutability with explicit justification
- Readonly/immutable collection interfaces (`IReadOnlyList`, `ImmutableArray`, `List.unmodifiable`) over mutable ones
- Shared mutable state across threads/actors is a defect — isolate or make immutable
- Avoid `static`/`global` mutable fields — they break test isolation and thread safety

##### Sealed Types & Exhaustiveness
- Closed type hierarchies (sealed classes, discriminated unions, sealed traits) enable the compiler to verify exhaustive handling
- Switch/match on non-sealed types should always have a default/else branch
- Adding a new case to a closed hierarchy should cause compile errors at every handling site — this is a feature, not a burden
- Pattern matching with exhaustiveness checking is preferred over chains of `instanceof`/`is`/type checks
- Value-based enums should replace integer/string constants for finite sets of options

##### Input Validation
- Validate data at system boundaries (API entry, user input, file read, message queue consumer)
- Fail fast with clear, actionable error messages — don't let invalid data propagate through the system
- Guard clauses at function entry for preconditions; dedicated validation layer for domain rules
- Schema-based validation (JSON Schema, protobuf, type providers) over manual check chains
- Sanitize before validation when dealing with user-controlled strings (trim, normalize unicode)
- Distinguish between recoverable validation errors (user input) and unrecoverable preconditions (programmer error)

##### Dependency Injection (DI)
- Constructor injection is the default; property/setter injection only for optional dependencies
- Inject interfaces/abstractions, not concrete implementations — enables testing with doubles
- Service locator pattern (`resolve<T>()` in domain code) hides dependencies — inject them explicitly
- Composition root (single entry point wiring dependencies) keeps the rest of the code free of DI framework pollution
- Circular constructor dependencies indicate a design problem — introduce a third abstraction or merge the types

##### Data and State
- Primitive types used to represent domain concepts (currency, email, status) should be wrapped in value types
- Groups of data items that always appear together suggest a missing struct/class
- Mutable state should be minimized; prefer immutable data where possible
- Mutable collections exposed through public APIs are a risk
- State transitions should be explicit and atomic — avoid scattered state mutation

##### Coupling and Dependency
- High fan-out (one module depending on many others) suggests missing abstractions
- Circular dependencies between modules should be broken
- Dependencies should flow toward stable abstractions, not concrete implementations
- Domain/business logic should not depend on infrastructure/framework details
- Static/direct constructor dependencies reduce testability — prefer dependency injection

##### Testability
- Hard-coded dependencies (filesystem, network, clock, random) make code difficult to test
- Direct use of system clock should be replaced with injectable time source
- Direct use of random generators should be replaced with injectable source
- Production code should not exist solely to support tests
- Test fixtures duplicated across tests should be extracted into shared helpers

For each finding, indicate:
- **Severity**: blocker / critical / major / minor / info (how impactful if correct)
- **Confidence**: VERY_HIGH / HIGH / MEDIUM / LOW (how certain you are)
- **Rule reference**: which category and rule triggered the finding
- **Before/after**: show the key refactored snippet in suggestion_code, original in existing_code
