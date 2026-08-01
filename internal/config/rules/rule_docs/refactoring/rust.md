#### Rust Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should consider grouping into a config struct
- Deeply nested conditions (>3 levels) should use guard clauses with early returns (`return`, `continue`, `break`)
- Complex match arms spanning many lines should consider extraction into functions or sub-patterns
- Long iterator chains obscuring error handling or side effects should be broken into named steps

##### Naming
- Use snake_case for functions, variables, modules; CamelCase for types, traits, enums; SCREAMING_SNAKE_CASE for consts
- Boolean variables should use `is_`/`has_`/`can_`/`should_` prefixes
- Single-letter names only for short-loop iterators (`|x|`, `for i in 0..n`); avoid in public APIs
- Avoid redundant prefix repetition: `user::UserService` → `user::Service`
- Type parameter names should be meaningful: `T` only when truly arbitrary, otherwise `Item`, `Key`, etc.

##### Type and API Design
- Plain strings, integers, or booleans representing domain concepts (email, status, currency) should be newtype wrappers
- Boolean parameters that switch behavior signal two separate functions
- Public struct fields expose implementation — consider builder pattern or constructor for forward compatibility
- Avoid exposing concrete collection types in public APIs; prefer `impl Iterator`, slices, or associated types
- Model state machines with enums rather than runtime checks on optional fields

##### Error Handling
- `unwrap()` / `expect()` in production/library paths where failure is recoverable should propagate with `Result`
- Errors discarded via `let _ = ...` without comment should either handle or document why it's safe
- Repeated error-wrapping patterns (`map_err(|e| ...)`) should use `anyhow::Context` / `thiserror` or extract helpers
- Error messages should include context about what operation failed
- `.ok()?` chains that discard error details should preserve context with `.context("...")?` or mapping

##### Duplication
- Identical code blocks in same file should be extracted into helper functions
- Similar logic differing only by type suggests generics or trait abstraction
- Repeated `#[cfg(test)]` setup should use test helper functions or macros
- Repeated conversion/validation patterns should be centralized in `From`/`TryFrom` impls

##### Ownership and Borrowing
- Excessive `.clone()` that exists only to satisfy the borrow checker — consider borrowing, `Cow`, or restructuring ownership
- Functions that take owned values when only a reference is needed reduce flexibility
- `Rc<RefCell<T>>` where a simpler ownership model would suffice
- Unnecessary reference cycles — `Weak` should break parent→child cycles where children outlive their parent

##### Concurrency
- Spawned tasks whose `JoinHandle` is dropped before observing completion
- Holding `Mutex`/`RwLock` guards across `.await` or slow operations
- `Arc<Mutex<T>>` where `Arc<RwLock<T>>` or channels would better express the access pattern
- Unprotected `unsafe impl Send/Sync` should have a documented safety rationale

##### Dead Code and Simplification
- Unused functions, struct fields, and imports should be removed
- Commented-out code blocks should be deleted
- Redundant `if let` → `match` nesting should be flattened
- Empty `impl` blocks and unused trait bounds should be cleaned
- `format!("{x}")` where `x.to_string()` or `x` directly in `println!` is simpler

##### Control Flow
- Nested `if let` / `match` should be flattened with `let else`, guard clauses, or combinator chains
- Repeated condition checks across methods suggest missing trait or enum dispatch
- Loops with manual indexing when iterator combinators (`enumerate`, `zip`, `filter_map`) are clearer
- Boolean flags controlling flow suggest splitting into separate functions

##### Data and State
- Mutable module-level state (`static mut`, `lazy_static!` with interior mutability) should be minimized
- Groups of data traveling together suggest a struct rather than separate parameters
- Collections returned from getters should allow mutation control; prefer iterators or `&[T]` over `&Vec<T>`
- Primitive obsession (string ID, int status) should be newtype wrappers with validation

##### Testability
- Direct `std::fs::read_to_string` / `File::open` calls — consider injectable filesystem via `&Path` or trait alias
- Direct `std::time::Instant::now()` / `SystemTime::now()` — pass a clock reference or use `Instant` as parameter
- Direct `reqwest::get` / `ureq::get` — inject HTTP client trait for mocking
- Global random (`rand::random()`) — pass `&mut impl RngCore` for determinism

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
