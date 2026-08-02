#### Family: Mobile / App UI (Swift / Dart)

Shared patterns for mobile UI toolkits. Language deltas override this family.

##### View / Widget Size
- Large `build` / view bodies should extract child views or helpers
- Keep business logic out of pure view trees (controllers, notifiers, view models)

##### Value vs Reference
- Prefer value semantics by default; use reference types when identity/sharing is required
- Prefer immutable UI state updates (`copyWith`, COW, `let`)

##### Concurrency & Main Thread
- Keep UI mutations on the main actor / UI thread
- Prefer structured concurrency over nested fire-and-forget tasks
