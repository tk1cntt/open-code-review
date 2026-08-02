#### Python Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- Snake_case for functions and variables; PascalCase for classes
- Private helpers use leading underscore (`_helper`) consistently
- Single-letter names OK in comprehensions and short loops

##### Type Safety
- Add type hints on public APIs
- Avoid `Any` — use `object`, `Protocol`, `TypeVar`, or generics
- Treat `Optional[X]` / `X | None` as a union callers must handle
- `# type: ignore` without a justifying comment should be fixed or explained

##### Async Handling
- `asyncio.create_task` without a retained reference risks silent failure
- Blocking calls (`time.sleep`, `requests.get`) inside `async def` stall the event loop
- Prefer `asyncio.gather` over sequential `await` when independent
- Coroutines called without `await` are bugs

##### Error Handling
- Bare `except:` is forbidden — use `except Exception` at minimum
- Prefer `raise ... from err` to preserve chains
- Do not use `assert` for runtime validation (stripped under `-O`)

##### Control Flow
- Long `if-elif` chains may use dict dispatch or `match`/`case` (3.10+)
- Prefer `enumerate` over manual index tracking

##### Data and State
- Never use mutable default arguments (`def f(x=[])`) — use `None` + init
- Class-level mutable attributes are shared across instances
- Closures capturing loop variables — bind with default args (`lambda i=i: ...`)

##### Testability
- Prefer `pathlib.Path` and injectable filesystem over hard-coded `open()`
- Injectable clock for `time.time` / `datetime.now`
- Inject HTTP client instead of hard-coded `requests` / `urllib`
- Reset module globals in fixtures when unavoidable
