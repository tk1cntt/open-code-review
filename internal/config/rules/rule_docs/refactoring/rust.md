#### Rust Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- snake_case functions/modules; CamelCase types/traits/enums; SCREAMING_SNAKE_CASE consts
- Boolean prefixes: `is_`/`has_`/`can_`/`should_`
- Avoid stutter: `user::UserService` → `user::Service`
- Prefer meaningful type params (`Item`, `Key`) over bare `T` when domain is known

##### Type and API Design
- Domain primitives → newtype wrappers
- Prefer builder/constructor over public mutable fields for public structs
- Prefer `impl Iterator`, slices, or associated types over exposing concrete collections in public APIs
- Model state machines with enums, not optional-field soup

##### Error Handling
- Avoid `unwrap()` / `expect()` on recoverable production paths — use `Result`
- Document or handle discarded errors (`let _ = ...`)
- Prefer `context` / `thiserror` / shared helpers over repeated `map_err`
- Preserve error context instead of bare `.ok()?`

##### Ownership and Borrowing
- Reduce needless `.clone()` — prefer borrow, `Cow`, or ownership redesign
- Take references when ownership is not required
- Prefer simpler ownership over habitual `Rc<RefCell<T>>`
- Break cycles with `Weak` where appropriate

##### Concurrency
- Observe `JoinHandle` completion; do not drop blindly
- Avoid holding `Mutex`/`RwLock` across `.await`
- Prefer channels or `RwLock` when they match access patterns better than `Arc<Mutex<T>>`
- Document safety for `unsafe impl Send/Sync`

##### Control Flow
- Flatten nested `if let` / `match` with `let else`, guards, or combinators
- Prefer iterator combinators (`enumerate`, `zip`, `filter_map`) over manual indexing

##### Testability
- Injectable filesystem / clock / HTTP / RNG instead of hard globals
