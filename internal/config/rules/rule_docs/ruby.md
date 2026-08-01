#### Ruby Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in class names, method names, constant names, or module names at declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Security
- SQL injection via string interpolation in ActiveRecord/Rails queries — use parameterized queries or `?` placeholders
- Hardcoded API keys, tokens, or credentials in source code
- Unsafe deserialization (`Marshal.load`, `YAML.unsafe_load`) on user-controlled data
- Mass assignment without `strong_params` in Rails controllers
- Command injection via `system()`/`` ` `` backticks/`exec()` with unsanitized user input
- XSS via `html_safe` or `raw` on user-controlled strings — prefer `sanitize` or `h()`

#### Ruby-Specific Pitfalls
- `Enumerable#each` used where `map`/`select`/`reject`/`find` expresses intent more clearly
- Mutating a collection while iterating with `each`
- `Time.parse` without specifying expected format — use `Time.strptime` for known formats
- `Float` used for money or exact decimal — use `BigDecimal` or integer cents
- `rescue Exception` catching too broadly — rescue `StandardError` instead
- Thread-safety issues with class variables (`@@var`) and class-level mutable state

#### Rails-Specific (when applicable)
- N+1 queries — use `includes`/`eager_load`/`preload`
- `find_each`/`find_in_batches` not used for large record sets
- Missing model validations on critical fields
- Callback hell (deeply nested `after_save`/`before_create` chains) causing unpredictable side effects
- Missing transaction wrapping around multiple related writes
- Secret keys in `config/` files not using `Rails.application.credentials` or `ENV`

#### Error Handling
- `rescue` with empty body silently swallowing errors
- `raise` with a string instead of a typed exception class
- Missing `ensure` block for resource cleanup (file handles, connections)
- `retry` without a counter limit — infinite loop risk

#### Memory and Performance
- Large files read entirely into memory (`File.read`) when streaming (`File.foreach`) would suffice
- String concatenation in loops — use `String#<<` or array `join`
- `Object#blank?`/`present?` from ActiveSupport used in plain Ruby without requiring `active_support`

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
