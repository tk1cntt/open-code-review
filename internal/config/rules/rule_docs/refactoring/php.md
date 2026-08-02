#### PHP Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- camelCase methods/vars; PascalCase classes; UPPER_SNAKE_CASE constants
- Avoid redundant namespace stutter in local usage

##### Type Safety and Declarations
- Add return types and parameter types on public APIs
- Narrow `mixed`; prefer union types / DTOs over untyped arrays
- Document array shapes with PHPDoc or replace with value objects
- Use property types when stable (7.4+)

##### Error Handling
- Avoid catching bare `\Exception`/`\Throwable` without specificity
- Do not use `@` error suppression where failure matters
- Prefer consistent error strategy per module (exceptions vs false/null)

##### Control Flow
- Prefer `match` (8.0+) or lookup arrays over long `if-elseif` chains
- Prefer array functions (`array_filter`, `array_map`) when clearer than manual loops

##### Data and State
- Prefer enums (8.1+) / value objects over primitive obsession
