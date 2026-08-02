#### C++ Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- snake_case functions/variables; PascalCase classes/structs; UPPER_CASE macros
- Boolean prefixes: `is_`/`has_`/`can_`/`should_`
- Avoid namespace stutter: `audio::AudioMixer` → `audio::Mixer`
- Private members: trailing `_` or `m_` consistently

##### Smart Pointer and Ownership
- Prefer `std::make_unique` / `std::make_shared` over raw `new`/`delete`
- Owning raw pointers → `unique_ptr` / `shared_ptr`
- Pass `shared_ptr` by const ref when sharing, not by value unnecessarily
- Break `shared_ptr` cycles with `weak_ptr`
- Prefer RAII for all resources

##### STL and Algorithm Usage
- Prefer `std::array` / `std::vector` over raw C arrays
- Prefer standard algorithms over hand-rolled loops
- Call `reserve()` when size is known
- Prefer range-based for when appropriate

##### const Correctness
- Mark non-mutating methods and unchanging locals/parameters `const`
- Prefer `const&` parameters for non-owned inputs

##### Error Handling
- Avoid catch-all `...` without rethrow strategy
- Destructors must not throw
- Do not mark `noexcept` functions that can throw
- Prefer a consistent error strategy (exceptions vs codes) per API surface

##### Modern C++
- Prefer `using` over `typedef`; `nullptr` over `NULL`/`0`
- Prefer `enum class` over unscoped `enum`
- Prefer `std::optional` over sentinel values / out-params for optional results
- Prefer `if constexpr` / concepts to simplify heavy templates
