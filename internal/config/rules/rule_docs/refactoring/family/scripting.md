#### Family: Scripting (Python / Ruby / PHP / Perl / Julia)

Shared patterns for dynamic/scripting languages. Language deltas override this family.

##### Typing & Contracts
- Prefer explicit type hints/annotations on public APIs when the ecosystem supports them
- Avoid unconstrained `Any` / untyped params on hot public surfaces
- Document or encode array/dict shapes that act as de-facto DTOs

##### Mutability Traps
- Never use mutable default arguments / shared class-level mutables unless intentional and documented
- Module-level mutable globals break test isolation — prefer injection or explicit reset

##### Errors
- Prefer specific exception types; never bare catch-all that swallows everything
- Preserve exception chains / context when re-raising
- Do not use assert/debug-only checks for production validation

##### Async / IO
- Do not mix blocking IO into async event loops
- Always await / observe spawned async work; avoid fire-and-forget without error handling
