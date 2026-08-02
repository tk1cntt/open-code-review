#### Family: Systems (Go / Rust / C / C++)

Shared patterns for systems languages. Language deltas override this family.

##### Errors & Resources
- Prefer explicit error returns / Result over silent ignore
- Every acquisition path needs a matching release path (defer / RAII / free)
- Prefer wrapping errors with context rather than bare return codes alone

##### Ownership & Memory
- Make ownership clear at API boundaries (who frees, who closes, who owns the buffer)
- Prefer stack/RAII/smart pointers over ad-hoc manual lifetime when the language supports it
- Avoid shared mutable globals; pass dependencies explicitly

##### Concurrency
- Goroutines/tasks/threads need a clear lifecycle and cancellation story
- Protect shared mutable state or redesign with message passing
- Document who closes channels / joins handles

##### API Surface
- Prefer small focused functions; prefer concrete types until abstraction is proven
- Avoid storing context/cancellation tokens in long-lived structs when the language idiom forbids it
