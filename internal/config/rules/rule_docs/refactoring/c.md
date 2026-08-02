#### C Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- snake_case functions/variables; UPPER_CASE macros/constants
- Boolean prefixes: `is_`/`has_`/`can_`/`should_`
- Prefix module-scope identifiers with module name to avoid collisions
- Enum constants share a common prefix

##### Memory Management
- Every `malloc`/`calloc`/`realloc` has matching `free` on all paths
- Check allocation results for NULL
- NULL out pointers after free
- Guard allocation size math against overflow
- Consider arena/wrappers for repeated alloc-free patterns

##### Buffer and String Safety
- Prefer bounded APIs (`snprintf`, size-aware copies) over `strcpy`/`sprintf`
- Bounds-check array access
- Watch off-by-one loop limits

##### Resource Management
- Close files/sockets on all paths; `goto cleanup` is idiomatic
- Consistent cleanup labels for multi-resource acquisition

##### Error Handling
- Consistent failure convention (NULL, -1, error codes)
- Check `errno` immediately after failing calls
- Do not use `assert` for runtime recoverable errors
