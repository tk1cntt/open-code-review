#### Swift Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- Follow Apple API Design Guidelines: clarity at point of use
- Types UpperCamelCase; vars/functions lowerCamelCase
- Factory methods: `make…` or `init` conventions

##### Optional and Error Design
- Long `guard let` / `if let` chains → extract validation helper
- Prefer `do/catch` over `try?` + nil-coalesce when diagnostics matter
- Prefer `Result` when failure modes must be distinguished
- Flatten nested optionals (`T??`)

##### Value vs Reference Types
- Prefer structs/value semantics by default; use classes when identity/sharing is required
- Prefer `let` + COW over many mutable `var` properties on structs

##### Concurrency
- Prefer structured concurrency / task groups over nested unstructured `Task {}`
- Keep UI work on MainActor; avoid scattered `MainActor.run`
- Mark types crossing actors as `Sendable` when required
