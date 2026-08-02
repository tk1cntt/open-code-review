#### Refactoring Rules (Common)

These rules apply to all programming languages. Focus on structural improvements that make code clearer, more maintainable, and less error-prone **without changing observable behavior**.

Language-specific and family sections (when present) **override common on the same topic**. Prefer catalog rule IDs (e.g. `REF-COMPLEX-001`) in finding rule references.

##### Priority and Finding Budget
- **P0 (always prefer)**: silent error ignore, resource leaks, shared mutable / race risk, unreachable wrong cleanup — `REF-BUDGET-001`
- **P1 (default)**: clear dead code, verbatim duplication, guard clauses for deep nesting, parameter object when over threshold, magic numbers with clear domain meaning
- **P2 (only if confidence is VERY_HIGH and P0/P1 are scarce)**: design-pattern rewrites (strategy, full DI redesign), sealed-hierarchy redesign, module boundary refactors
- Prefer at most the profile's max findings per file (default **8**); fewer high-signal findings beat many nitpicks — `REF-BUDGET-001`
- Sort findings by priority (P0 → P1 → P2), then by severity

##### Evidence Requirements
- Report only when evidence is visible in the current file (or explicitly provided context) — `REF-EVIDENCE-001`
- Do **not** claim circular package dependencies, high fan-out, or cross-module coupling without imports/usages shown in this file — `REF-EVIDENCE-001`
- Extract shared helpers only when blocks are **verbatim duplicates** or differ only by literals/types (≥2 occurrences) — `REF-DUP-001`
- Do **not** flag generated code, vendored code, or lockfiles
- Distinguish **local refactor** (safe in-file: dead code, guard clause, extract private helper, rename local) vs **architectural smell** (DI graph, package design) — mark architectural items as lower confidence and avoid large rewrites

##### Complexity
- Functions exceeding the profile max function lines (default **80**) should be considered for splitting by responsibility — `REF-COMPLEX-001`
- Cyclomatic complexity should be kept low — nested conditions deeper than profile max nesting (default **3**) should use guard clauses with early returns — `REF-COMPLEX-003`
- Functions with more parameters than the profile max (default **4**) suggest grouping related parameters into a struct/object — `REF-COMPLEX-002`
- Boolean expressions with >3 terms should be extracted into well-named intermediate variables
- Switch/match statements with more cases than the profile max (default **7**) may benefit from strategy pattern or map/dictionary lookup (**P2** unless trivial map extract)

##### Naming and Readability
- Variable names should convey intent — single-letter names only for short-loop iterators or receivers — `REF-NAME-001`
- Function names should reflect what they do, including side effects
- Boolean variables/functions should use `is`/`has`/`can`/`should` prefixes — `REF-NAME-001`
- Magic numbers and magic strings should be replaced with named constants
- The same concept should use the same term throughout the codebase; one term should not mean multiple concepts

##### Function and Method Design
- Functions should do one thing — query and mutation should be separate
- Boolean flag parameters that switch behavior suggest the function has multiple responsibilities
- Functions should not depend on excessive global/mutable state
- Unused parameters should be removed
- Output parameters should be obvious from the signature — avoid surprising side effects

##### Duplication and Reuse
- Duplicate code blocks (same logic repeated verbatim) should be extracted into a shared function — `REF-DUP-001`
- Near-duplicate code differing only by literal values or types should be parameterized — `REF-DUP-001`
- Repeated validation logic should be centralized
- Repeated error-handling patterns should be extracted into helpers
- Avoid premature abstraction — two similar blocks may not warrant extraction if they serve different purposes

##### Error Handling
- Errors should not be silently ignored; always handle or propagate with context — `REF-ERROR-001`
- Error messages should include context about what operation failed
- Catch blocks should be specific — avoid catching overly broad exception types
- Exceptions should not be used for normal control flow
- Resource cleanup should use try-with-resources / defer / context-manager patterns, not scattered exit paths — `REF-ERROR-002`

##### Dead Code and Simplification
- Unused functions, variables, and imports should be removed — `REF-DEAD-001`
- Unreachable code branches should be eliminated — `REF-DEAD-001`
- Commented-out code should be deleted (version control retains history) — `REF-DEAD-001`
- Redundant assignments, conversions, and conditions should be simplified
- Wrapper functions that add no value should be inlined

##### Control Flow
- Nested `if` statements should be converted to guard clauses with early returns — `REF-COMPLEX-003`
- Repeated condition checks across branches suggest missing polymorphism or strategy pattern (**P2** if it requires new types)
- Loops with multiple control flags can often be decomposed into filter → transform → aggregate pipeline
- Try-catch blocks should cover the minimal necessary scope
- Conditions that are always true/false in context indicate dead or misleading code

##### Immutability (CRITICAL)
- Favor immutable data structures — copy-on-write rather than in-place mutation — `REF-IMMUT-001`
- Return defensive copies from public APIs instead of exposing internal mutable state — `REF-IMMUT-001`
- Use `const`/`final`/`val`/`let` by default; only introduce mutability with explicit justification
- Readonly/immutable collection interfaces (`IReadOnlyList`, `ImmutableArray`, `List.unmodifiable`) over mutable ones
- Shared mutable state across threads/actors is a defect — isolate or make immutable — `REF-IMMUT-001`
- Avoid `static`/`global` mutable fields — they break test isolation and thread safety

##### Sealed Types & Exhaustiveness
- Closed type hierarchies (sealed classes, discriminated unions, sealed traits) enable the compiler to verify exhaustive handling
- Switch/match on non-sealed types should always have a default/else branch
- Adding a new case to a closed hierarchy should cause compile errors at every handling site — this is a feature, not a burden
- Pattern matching with exhaustiveness checking is preferred over chains of `instanceof`/`is`/type checks
- Value-based enums should replace integer/string constants for finite sets of options

##### Input Validation
- Validate data at system boundaries (API entry, user input, file read, message queue consumer) — `REF-VALID-001`
- Fail fast with clear, actionable error messages — don't let invalid data propagate through the system — `REF-VALID-001`
- Guard clauses at function entry for preconditions; dedicated validation layer for domain rules
- Schema-based validation (JSON Schema, protobuf, type providers) over manual check chains
- Sanitize before validation when dealing with user-controlled strings (trim, normalize unicode)
- Distinguish between recoverable validation errors (user input) and unrecoverable preconditions (programmer error)

##### Dependency Injection (DI)
- Constructor injection is the default; property/setter injection only for optional dependencies — `REF-DI-001`
- Inject interfaces/abstractions, not concrete implementations — enables testing with doubles — `REF-DI-001`
- Service locator pattern (`resolve<T>()` in domain code) hides dependencies — inject them explicitly — `REF-DI-001`
- Composition root (single entry point wiring dependencies) keeps the rest of the code free of DI framework pollution
- Circular constructor dependencies indicate a design problem — introduce a third abstraction or merge the types (**architectural; high confidence only**)

##### Data and State
- Primitive types used to represent domain concepts (currency, email, status) should be wrapped in value types
- Groups of data items that always appear together suggest a missing struct/class
- Mutable state should be minimized; prefer immutable data where possible — `REF-IMMUT-001`
- Mutable collections exposed through public APIs are a risk — `REF-IMMUT-001`
- State transitions should be explicit and atomic — avoid scattered state mutation

##### Coupling and Dependency
- High fan-out (one module depending on many others) suggests missing abstractions (**needs evidence in file**) — `REF-EVIDENCE-001`
- Circular dependencies between modules should be broken (**needs evidence in file**) — `REF-EVIDENCE-001`
- Dependencies should flow toward stable abstractions, not concrete implementations
- Domain/business logic should not depend on infrastructure/framework details
- Static/direct constructor dependencies reduce testability — prefer dependency injection — `REF-DI-001`

##### Testability
- Hard-coded dependencies (filesystem, network, clock, random) make code difficult to test — `REF-TEST-001`
- Direct use of system clock should be replaced with injectable time source — `REF-TEST-001`
- Direct use of random generators should be replaced with injectable source — `REF-TEST-001`
- Production code should not exist solely to support tests
- Test fixtures duplicated across tests should be extracted into shared helpers

For each finding, indicate:
- **Severity**: blocker / critical / major / minor / info (how impactful if correct)
- **Confidence**: VERY_HIGH / HIGH / MEDIUM / LOW (how certain you are)
- **Priority**: P0 / P1 / P2 (from Priority and Finding Budget)
- **Rule reference**: catalog ID when applicable (e.g. `REF-COMPLEX-001`) plus category
- **Before/after**: show the key refactored snippet in suggestion_code, original in existing_code
