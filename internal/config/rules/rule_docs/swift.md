#### Swift Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in type names, function names, property names, or enum case names at declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Optional Handling
- Force unwrap (`!`) on optionals that may legitimately be nil — prefer `if let`, `guard let`, or `??`
- Implicitly unwrapped optionals (`T!`) used when a regular optional or non-optional would be safer
- Optional chaining used where a guard/if-let early exit would provide clearer error context
- `try!` on throwing functions — handle the error or propagate with `try`

#### Memory Management
- Strong reference cycles between classes — use `weak` or `unowned` on one side
- Closure capture lists missing where `self` is captured strongly in escaping closures
- `unowned` used where the referenced object may be deallocated — prefer `weak`
- Large data held in memory without explicit lifecycle management

#### Concurrency (Swift 6+)
- Unstructured `Task {}` without error handling or cancellation observation
- `@MainActor` missing on UI-updating code called from background tasks
- Sending non-`Sendable` types across actor/isolation boundaries
- Blocking calls on `@MainActor` — offload to background actor
- `Task.sleep` without cancellation handling

#### Error Handling
- `try?` discarding error details when the caller needs to distinguish failure modes
- Empty `catch` blocks silently swallowing errors
- `fatalError` / `preconditionFailure` in production paths when a typed error would be recoverable
- Functions that `throw` in some paths and `return nil` in others — pick one error strategy

#### Protocol and Type Design
- Protocol conformance with incomplete or incorrect required methods
- Force casting (`as!`) when conditional cast (`as?`) would be safer
- Raw value enums used when associated-value enums would express the domain model better
- Mirroring `Equatable` / `Hashable` conformance missing where value comparison is needed

#### Resource Management
- File handles, network connections, or database handles not closed deterministically
- NotificationCenter observers not removed in `deinit`
- Large resources loaded synchronously on the main thread

#### Security
- Hardcoded API keys, tokens, or credentials in source code
- User-controlled data passed unvalidated to system APIs or shell commands
- Sensitive data logged or written to unencrypted storage
- URL paths constructed from user input without validation

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
