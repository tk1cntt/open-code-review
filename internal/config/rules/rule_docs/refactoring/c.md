#### C Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be split by responsibility
- Functions with >4 parameters should consider grouping into a struct parameter
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Long switch statements (>7 cases) may benefit from function pointer table or lookup array
- Macros with complex multi-line bodies should be replaced with inline functions where possible

##### Naming
- Use snake_case for functions and variables; UPPER_CASE for macros and constants
- Boolean-like variables and functions should use `is_`/`has_`/`can_`/`should_` prefixes
- Single-letter names only for short-loop iterators (`for (int i = 0; i < n; i++)`)
- Prefix module-scope identifiers with module name to avoid collisions (`arena_alloc`, not `alloc`)
- Enum constants should have a common prefix to prevent namespace collisions

##### Memory Management
- Every `malloc`/`calloc`/`realloc` must have a corresponding `free` on all exit paths
- Allocated pointer should be checked for NULL after allocation
- Pointer set to NULL after `free()` to prevent use-after-free and double-free
- Allocation size calculations should guard against integer overflow
- Repeated alloc-free patterns suggest extracting allocation wrappers or arena management

##### Buffer and String Safety
- `strcpy`/`strcat`/`sprintf` should be replaced with `strncpy`/`strncat`/`snprintf` with explicit size bounds
- Array access should have bounds checking before indexing
- Loop boundary conditions should be verified for off-by-one errors
- String literals concatenated with user input should use format strings, not direct concatenation

##### Resource Management
- File handles (`fopen`), sockets, and other resources must be closed/released on all exit paths
- Error paths should clean up previously acquired resources (goto cleanup pattern is idiomatic)
- Repeated open-use-close patterns suggest a wrapper function or structured cleanup
- Multiple resource acquisitions should use a consistent cleanup label approach (`goto cleanup`)

##### Error Handling
- Functions that can fail should have a consistent return convention (NULL, -1, error code)
- `errno` should be checked immediately after a call that sets it
- Callers ignoring return values from functions that can fail
- Inconsistent error reporting (mixing `perror`, `fprintf(stderr, ...)`, return codes, `assert`)
- `assert` used for runtime errors (stripped in release builds) — use explicit `if` checks

##### Duplication
- Identical code blocks in the same file should be extracted into static helper functions
- Repeated error-checking patterns should be centralized in helper macros or functions
- Repeated setup/teardown in test code should use test fixtures
- Similar struct initialization patterns suggest a factory/init function

##### Dead Code
- Unused functions (especially `static` declarations) and variables should be removed
- Commented-out code blocks should be deleted
- Unreachable code after `return`/`break`/`continue`/`goto`
- Preprocessor blocks guarded by `#if 0` should be cleaned up
- Unused struct fields, enums, and typedefs

##### Control Flow
- `goto` used for anything other than cleanup on error (the idiomatic C pattern) should be restructured
- Nested `if-else` chains should be flattened with guard clauses and early returns
- Boolean flag parameters suggest splitting the function
- Repeated condition checks across functions suggest missing dispatch table or callback

##### Data and State
- Global mutable variables should be minimized — pass state explicitly via struct pointers
- Groups of related variables passed to multiple functions suggest a missing struct
- Magic numbers should be replaced with named `#define` or `enum` constants
- Opaque pointer types (`typedef struct Foo* FooHandle`) should define clear ownership and lifecycle
- Bit-field layout is implementation-defined — avoid for serialization or cross-platform data

##### Preprocessor and Macros
- Function-like macros with side effects (multiple evaluation of arguments) should be inline functions
- Macro arguments without parentheses in the expansion cause precedence bugs
- Long macros should be broken into `do { ... } while(0)` blocks for safe multi-statement use
- Feature flags managed through `#ifdef` scattered across the code; consider consolidating

##### Testability
- Direct `time()` / `clock()` calls — pass a time provider function pointer
- Direct `fopen`/`read`/`write` — pass `FILE*` handles or I/O function pointers for test injection
- Global state that makes tests order-dependent should be reset in fixtures
- Functions with side effects on hardware/network should separate computation from I/O

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
