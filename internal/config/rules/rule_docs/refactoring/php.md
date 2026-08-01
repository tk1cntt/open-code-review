#### PHP Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter object class or named arguments (PHP 8.0+)
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long `if-elseif` chains (>5 branches) should use `match` expression (PHP 8.0+) or lookup array
- Complex closures spanning many lines should be extracted into named functions or invokable classes

##### Naming
- Use camelCase for methods and variables; PascalCase for classes and interfaces; UPPER_SNAKE_CASE for constants
- Boolean variables and methods should use `is`/`has`/`can`/`should` prefixes
- Single-letter variable names only in short closures (`fn($x) => $x * 2`) or short loops
- Avoid redundant namespace repetition: `App\Service\UserService` → use for clarity in context
- Private methods should use meaningful names over brevity; protected methods signal extension points

##### Type Safety and Declarations
- Functions without return type declarations reduce readability — add `: Type` on public methods
- Parameters without type hints (`int`, `string`, `?User`, `array`) should be typed where the contract is known
- `mixed` type should be narrowed with PHPDoc generics or replaced with union types (PHP 8.0+)
- Array shapes should be documented with PHPDoc `@param array{key: Type, ...}` or replaced with DTO classes
- Property type hints should be used (PHP 7.4+) when the type is stable

##### Error Handling
- Catch of `\Exception` or `\Throwable` without specific types catches too much
- Empty catch blocks silently suppress failures — at minimum log
- Exceptions swallowed and replaced with `null`/`false` default lose diagnostic information
- Inconsistent error strategy (mixing exceptions, return false, error_log) within the same module
- `@` error suppression on operations where failure matters — use explicit check

##### Duplication
- Identical code blocks in the same class should be extracted into private methods
- Similar logic across related classes suggests an abstract base class, trait, or interface
- Repeated validation rules should be centralized in a validator service or value object
- Test data/assertions duplicated across test methods should use data providers or setUp

##### Dead Code
- Unused use-statements, private methods, and properties should be removed
- Commented-out code blocks should be deleted
- Unreachable code after `return`/`throw`/`die`/`exit`
- Empty method overrides (just calling `parent::method()`) add no value

##### Control Flow
- Nested `if` statements should be converted to guard clauses with early returns
- Boolean flag parameters (`bool $force`) suggest splitting the method
- Repeated `if ($x instanceof Type)` checks suggest missing interface or abstract method
- Loops with `break`/`continue` as primary control — may be clearer as array functions (`array_filter`, `array_map`)

##### Data and State
- Primitive obsession (string for email, int for status) should be wrapped in value objects or enums (PHP 8.1+)
- Magic strings scattered across the code should be enum cases or class constants
- Array shapes with known keys passed between methods suggest a DTO class
- Mutable static properties are shared across requests in long-running processes — prefer instance state
- Global/`$GLOBALS` access should be replaced with dependency injection

##### Enums and Modern PHP Features (PHP 8.1+)
- String/int constants used as enumerations should be migrated to native `enum`
- `enum` cases with primitive backing values that carry behavior should use rich enums with methods
- Repeated `match` on enum cases across the codebase suggests moving logic into the enum itself
- Constructor property promotion (`__construct(private string $name)`) should replace manual property+assignment

##### Coupling and Dependency
- Direct `new` inside business logic prevents testing — use constructor injection or factory
- Concrete class dependencies should be behind interfaces for testability
- Static method calls to classes with external side effects (DB, HTTP, filesystem) hinder testing
- Service locator / container access in domain logic couples to the framework

##### Testability
- Direct `new DateTime()` or `time()` calls should use injectable `ClockInterface` or `Carbon` test helpers
- Direct `file_get_contents`/`fopen` — inject filesystem abstraction or use stream wrappers
- Direct `new PDO` or `DB::connection()` static calls — inject repository or connection
- Functions that `echo`/`print` directly — return string or use response object
- Global `$_GET`, `$_POST`, `$_SERVER` access — use request abstraction or parameter passing

##### Performance Considerations
- N+1 queries in loops where eager loading or batch fetching preserves behavior
- Array merge/spread in hot loops causing repeated allocation — consider `array_push` or pre-allocation
- `in_array()` on large arrays — convert to `array_flip` + `isset` or use appropriate data structure
- Repeated regex compilation in loops — extract `preg_match` with pattern to outside loop
- Large dataset materialization (`->all()`, `fetchAll()`) when pagination or generators (`yield`) would suffice

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
