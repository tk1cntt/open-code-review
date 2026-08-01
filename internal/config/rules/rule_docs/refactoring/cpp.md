#### C++ Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter struct or builder pattern
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long switch statements (>7 cases) may benefit from strategy pattern or `std::map<Enum, Handler>`
- Complex template metaprogramming should be simplified with `if constexpr` (C++17) or concepts (C++20)

##### Naming
- Use snake_case for functions and variables; PascalCase for classes and structs; UPPER_CASE for macros
- Boolean variables and functions should use `is_`/`has_`/`can_`/`should_` prefixes
- Single-letter names only for short-loop iterators; avoid in any wider scope
- Namespace-qualified names should avoid redundancy: `audio::AudioMixer` → `audio::Mixer`
- Private members should use trailing underscore (`member_`) or `m_` consistently within a codebase

##### Smart Pointer and Ownership
- `new`/`delete` pairs that can be replaced with `std::make_unique`/`std::make_shared`
- Raw owning pointers should be converted to `std::unique_ptr` or `std::shared_ptr`
- `std::shared_ptr` passed by value causes unnecessary reference count churn — pass by const ref
- Circular references with `std::shared_ptr` should use `std::weak_ptr` to break cycles
- Resource acquisition should happen in constructors, release in destructors (RAII)

##### STL and Algorithm Usage
- Raw C arrays should be replaced with `std::array` or `std::vector`
- Hand-written loops that replicate standard algorithms (`std::transform`, `std::copy_if`, `std::find_if`, `std::accumulate`)
- Inappropriate container choice (`std::list` for random access, `std::vector` for frequent front insertion)
- Range-based for loops should be preferred over index-based loops when possible
- Missing `reserve()` on vectors when element count is known in advance

##### const Correctness
- Member functions that don't mutate should be marked `const`
- Parameters that are not mutated should be passed by `const&` (or `const T*`)
- Local variables that never change should be marked `const`
- Return types that should not be mutated should be `const&` or `const` value

##### Error Handling
- Catch of `...` is too broad; catch specific exception types
- Empty catch blocks hide failures — at minimum log with context
- Functions marked `noexcept` that can actually throw (surfaces as `std::terminate`)
- Exceptions thrown from destructors risk `std::terminate` — catch and handle in destructor
- Return codes mixed with exceptions in the same API suggests unclear error strategy

##### Duplication
- Identical code blocks in the same translation unit should be extracted into functions
- Similar logic across classes differing only by type suggests template extraction
- Repeated `try-catch` patterns should be centralized with RAII wrappers
- Test setup duplicated across test files should use fixtures and `SetUp`/`TearDown`

##### Dead Code
- Unused functions, variables, and includes should be removed
- Commented-out code blocks should be deleted
- Unreachable code after `return`/`throw`/`break`/`continue`
- Preprocessor blocks guarded by always-false conditions (`#if 0`, stale feature flags)

##### Control Flow
- Nested `if-else` chains should be flattened with early returns, `switch`, or map lookup
- Boolean flag parameters suggest splitting into two separate functions
- Deeply nested loops should consider extraction to named functions
- `goto` used for anything other than cleanup in C-style code should be restructured

##### Data and State
- Primitive types representing domain concepts (email, currency, id) should be strong typedefs or value classes
- Mutable global/static variables should be minimized — prefer dependency injection
- Groups of data consistently passed together suggest a struct/class
- Raw `enum` used when `enum class` (scoped, type-safe) would prevent implicit conversions

##### Coupling and Dependency
- Direct `new` inside business logic prevents testing — inject factories or smart pointer arguments
- `#include` of concrete headers when forward declaration would suffice — breaks compilation isolation
- Circular includes between headers should be broken with forward declarations
- Heavy headers included in widely-used headers increase build times unnecessarily

##### Modern C++ Adoption (version-appropriate)
- Manual memory management where smart pointers would express ownership
- `typedef` should be `using` alias (clearer syntax, supports templates)
- `NULL` or `0` should be `nullptr`
- Functions taking iterators/ranges should be templates, not container-specific
- `std::optional` for functions that may not return a value instead of sentinel values or output parameters

##### Testability
- Direct `std::chrono::system_clock::now()` calls — inject a `Clock` interface
- Direct file I/O (`std::fstream`, `std::filesystem`) — consider injectable path or stream abstraction
- Direct network/HTTP calls — inject client interface for mocking
- Global singletons (Meyer's singleton, static local) complicate test isolation

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
