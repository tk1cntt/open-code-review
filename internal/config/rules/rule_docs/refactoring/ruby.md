#### Ruby Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- snake_case methods/vars; PascalCase classes/modules; SCREAMING_SNAKE_CASE constants
- Boolean methods end with `?`
- Destructive methods end with `!` when a safe counterpart exists

##### Immutability
- Prefer `freeze` on constants/shared strings
- Prefer non-mutating Enumerable methods over bang mutators when either works
- Avoid mutable default args (`def foo(x = [])`)

##### Object Design
- Extract service objects/concerns from god objects
- Prefer explicit methods over heavy `method_missing` / `send` for maintainability
- Provide meaningful `to_s`/`inspect` on value objects used in logs

##### Control Flow
- Prefer method-level `rescue` over wrapping entire methods in `begin/end`
- Prefer polymorphism over repeated `is_a?` checks

##### Framework
- Controller boilerplate → `before_action` / concerns when applicable
