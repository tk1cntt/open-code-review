#### Ruby Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use keyword arguments or a parameter object
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long case/when statements (>7 branches) may benefit from strategy pattern or hash lookup
- Complex blocks spanning >10 lines should be extracted to named methods

##### Naming
- Use snake_case for methods, variables, file names; PascalCase for classes and modules
- Boolean methods should end with `?` (e.g., `valid?`, `done?`)
- Destructive/mutating methods should end with `!` when a safe counterpart exists
- Constants should use SCREAMING_SNAKE_CASE
- Avoid single-letter variables except for well-known idioms (`i`, `e`, `_`)

##### Immutability
- Prefer `freeze` on constants and shared strings
- Mutating methods (`gsub!`, `reject!`) when the non-mutating version would suffice
- Shared mutable default values in method signatures (`def foo(x = [])`) — use `nil` + initialize pattern
- Favor `Enumerable` methods that return new collections over in-place mutation

##### Object Design
- "God objects" with too many responsibilities — extract service objects or concerns
- Modules used as namespaces when a class with composition would be clearer
- `method_missing` used when `define_method` or explicit methods would be maintainable
- Overuse of `send`/`public_send` bypassing encapsulation
- Missing `to_s`/`inspect` on value objects used in logging

##### Duplication
- Identical code blocks in the same class should be extracted into private methods
- Similar logic across classes suggests a module or base class
- Repeated validation patterns should be centralized in validators or concerns
- Controller boilerplate across actions should use `before_action` or concerns

##### Dead Code and Simplification
- Unused methods, classes, modules; commented-out code should be removed
- Empty `rescue` blocks and `ensure` blocks that only return nil
- Redundant `self.` prefixes where not needed for disambiguation
- `if bool == true` / `if bool == false` — use `if bool` / `unless bool`

##### Control Flow
- Nested if/else chains should use guard clauses or case/when
- Boolean flag parameters suggest splitting the method
- `begin/rescue/end` blocks that span entire methods — use method-level rescue
- Repeated type checks (`is_a?`) across methods suggest missing polymorphic dispatch

##### Data and State
- Primitive obsession (string for email, int for status) — wrap in value objects
- Groups of related data passed as positional arguments suggest a struct or class
- Mutable instance variables shared across methods without clear ownership
- Global state via `$variables` or class variables (`@@var`)

##### Coupling and Dependency
- Direct class instantiation in business logic prevents testing — inject via constructor
- Direct `File.read`/`Net::HTTP.get`/`DateTime.now` in domain logic — inject abstractions
- Tight coupling through `Rails.cache`/`Redis.current` — inject cache/Redis client

##### Testability
- Direct `Time.now`/`Date.today` — inject clock or use timecop/test helpers
- Direct `Net::HTTP` usage — inject HTTP client for test doubles
- Direct filesystem access (`File`, `Dir`) — inject or use tempfile helpers
- `sleep` in tests — use time travel or deterministic waiting

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
