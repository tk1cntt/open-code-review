#### Python Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be split by responsibility
- Functions with >4 parameters should use keyword arguments with defaults or a dataclass
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Complex list/dict comprehensions spanning multiple lines should be rewritten as loops or generator functions
- Long chains of string operations should be broken into named steps

##### Naming
- Snake_case for functions and variables; PascalCase for classes
- Boolean variables and functions should use `is`/`has`/`can`/`should` prefixes
- Avoid single-letter names except in comprehensions (`[x for x in ...]`) and short loops
- Private helpers should use leading underscore (`_helper`) convention consistently

##### Type Safety
- Functions lacking type hints reduce readability — add them, especially on public APIs
- `Any` type should be avoided; use `object`, `Protocol`, `TypeVar`, or generics
- `Optional[X]` interpreted as `X | None` — a union that callers must handle
- `# type: ignore` without a comment explaining why should be removed or fixed

##### Async Handling
- `asyncio.create_task` with no reference risks silent failure — capture and handle
- Blocking calls (`time.sleep`, `requests.get`) inside `async def` stall the event loop
- `await` inside loops where `asyncio.gather` would be concurrent
- Coroutine functions called without `await` (accidental missing await) produce unused coroutine warnings

##### Error Handling
- Bare `except:` catches `KeyboardInterrupt` and `SystemExit` — use `except Exception` as minimum
- `except Exception: pass` silently swallows all errors — at minimum log
- `raise ... from err` preserves the traceback chain; plain `raise` loses context
- `assert` statements used for runtime validation are stripped under `-O` — use explicit `if/raise`
- Broad `try` blocks catch errors from unrelated code — narrow the scope

##### Duplication
- Repeated validation patterns should be centralized in a validator function or decorator
- Similar logic in multiple views/endpoints suggests a shared service layer
- Repeated test fixtures should use pytest fixtures or `conftest.py`
- Similar string formatting across the codebase suggests template functions or constants

##### Dead Code
- Unused imports (reported by `flake8`/`ruff`) should be removed
- Functions and classes with no callers in the codebase should be deleted
- Commented-out code blocks should be removed (git history has them)
- Unreachable code after `return`/`raise`/`break`/`continue` should be eliminated

##### Control Flow
- Deeply nested `if/for/try` should be flattened with early returns
- Long `if-elif` chains (>5 branches) may benefit from dict dispatch or match/case (Python 3.10+)
- Boolean parameters that switch behavior suggest two separate functions
- `for item in list:` combined with index tracking (`enumerate` exists for a reason)

##### Data and State
- Mutable default arguments (`def f(x=[])`) create shared state across calls — use `None` default
- Class-level mutable attributes (`class Foo: items = []`) are shared across instances
- Module-level mutable globals that are mutated across requests/threads break isolation
- Closures capturing loop variables by reference (`lambda: i`) — use default argument binding

##### Testability
- Direct `open()` / `os.path` calls — consider `pathlib.Path` and injectable filesystem
- Direct `time.time()` / `datetime.now()` calls — use injectable clock for deterministic tests
- Direct `requests.get` / `urllib` calls — inject an HTTP client for mocking
- Global state in modules makes tests order-dependent — minimize or reset in fixtures

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
