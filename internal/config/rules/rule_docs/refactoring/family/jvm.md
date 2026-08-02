#### Family: JVM (Java / Kotlin)

Shared patterns for JVM languages. Language deltas override this family.

##### Null Safety
- Prefer explicit null contracts (`Optional`, nullable types, annotations) over scattered NPE-prone chains
- Repeated null checks on the same expression → early return / safe-call idioms
- Prefer return-type optionality over optional fields/parameters when modeling absence

##### Type Design
- Prefer sealed hierarchies for closed sets of variants with exhaustive `switch`/`when`
- Prefer value types / data classes for pure data carriers
- Avoid primitive obsession for domain identifiers and statuses

##### Concurrency & State
- Prefer immutable or thread-confined state; document shared mutability
- Avoid static mutable caches without clear lifecycle and synchronization

##### Dependency Style
- Prefer constructor injection over service locator and hard-coded `new` of collaborators
- Keep request/context objects as method parameters, not long-lived fields
