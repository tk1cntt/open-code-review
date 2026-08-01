#### Swift Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a configuration struct or builder
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long switch statements (>7 cases) may benefit from strategy pattern or dictionary lookup
- Complex trailing closures should be extracted to named functions

##### Naming
- Follow Apple API Design Guidelines: clarity at the point of use, omit needless words
- Types: UpperCamelCase; variables/functions: lowerCamelCase; protocols use noun/adjective naming
- Boolean variables should use `is`/`has`/`can`/`should` prefixes
- Avoid abbreviations in public API names unless domain-standard
- Factory methods should start with `make` (managed) or use `init` conventions

##### Optional and Error Design
- Repeated `guard let` / `if let` unwrapping chains — consider extracting to a dedicated validation method
- `try?` followed by nil-coalescing when `do/catch` would give better diagnostics
- Functions returning optional when a `Result<Success, Error>` would distinguish failure modes
- Nested optionals (`T??`) — flatten to a single optional level

##### Value vs Reference Types
- Classes used when a struct would suffice — prefer value semantics by default
- Struct types with many mutable `var` properties — consider `let` + copy-on-write
- Reference types passed where value semantics would avoid unintended sharing
- `class` with `Equatable` / `Hashable` manually implemented — consider struct or auto-synthesis

##### Concurrency
- Nested unstructured `Task {}` — prefer task groups or structured concurrency
- Repeated `await MainActor.run` in background code — restructure so UI work is on MainActor
- Actor methods that become sequential bottlenecks — decompose by data ownership
- Missing `Sendable` conformance on types crossing actor boundaries

##### Duplication
- Identical code blocks in the same file should be extracted into private functions
- Similar logic across types suggests a protocol with default implementation
- Repeated formatting/validation patterns should be centralized
- UI boilerplate repeated across views should use view modifiers or custom views

##### Dead Code and Simplification
- Unused extensions, protocol conformances, and private functions should be removed
- Commented-out code blocks should be deleted
- Unused enum cases and raw values
- Empty `willSet`/`didSet` observers — remove if they add no logic

##### Control Flow
- Nested if-else chains should use guard-let early exits or switch
- Boolean flag parameters suggest splitting the function
- Repeated conditional checks across methods suggest protocol or enum dispatch

##### Data and State
- Primitive obsession (string for email, int for status) — wrap in struct with validation
- Mutable state in reference types shared across subsystems — prefer isolated state
- Direct UserDefaults access scattered across code — centralize in a settings service
- Groups of related data passed as parameters suggest a struct

##### Coupling and Dependency
- Direct instantiation of dependencies in init — inject via protocol
- Static methods with side effects (network, file I/O) that prevent testing
- Tight coupling through shared singletons — prefer dependency injection

##### Testability
- Direct `Date()` / `Date.now` — inject `Date` or use a clock protocol
- Direct `URLSession.shared` — inject `URLSession` or networking protocol
- Direct `UserDefaults.standard` — inject settings service
- Direct `FileManager.default` — inject or use protocol for test doubles

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
