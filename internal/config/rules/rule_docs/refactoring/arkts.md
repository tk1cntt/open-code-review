#### ArkTS Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter object or interface
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Complex UI building methods should extract sub-components for readability

##### Naming
- Use camelCase for functions and variables; PascalCase for components and classes
- Boolean variables and functions should use `is`/`has`/`can`/`should` prefixes
- Component names should clearly describe their purpose
- Private members should use meaningful names, not abbreviations

##### State Management
- Arrays/objects decorated with `@State` must be replaced (not mutated in place) to trigger UI refresh
- Repeated `@State` + pass-through patterns suggest using `@Provide`/`@Consume`
- Props drilling beyond 3 levels should use `@Provide`/`@Consume` or context
- `@StorageLink`/`@StorageProp` used for non-global state should be local state instead

##### Component Structure
- Giant components with many responsibilities should be split into smaller components
- Side effects in `build()` method should be moved to lifecycle hooks
- Repeated UI patterns across components should be extracted into reusable `@Component`
- Long `ForEach`/`LazyForEach` with complex item builders should extract to child components

##### Duplication
- Repeated styles across components should be extracted to shared style sheets or `@Extend`
- Repeated validation logic should be centralized in utility functions
- Similar component logic suggests a base component or composition pattern
- Repeated resource references suggest shared constants

##### Dead Code
- Unused `@State`/`@Prop`/`@Link` decorated properties should be removed
- Unused component imports and declarations should be removed
- Commented-out code blocks should be deleted
- Unreachable UI branches in conditional rendering

##### Performance
- `ForEach` on large lists (>20 items) should use `LazyForEach`
- Object/closure creation in `build()` should be moved outside or cached with `@Watch`
- Repeated computations across renders should use `@Computed` or memoization
- Image resources without caching strategies in frequently-rendered components

##### Resource Usage
- Hardcoded strings should use `$r('app.string.key')` for i18n
- Hardcoded image paths should use `$r('app.media.xxx')` or `$rawfile()`
- Hardcoded colors/dimensions should use resource references for theme support

##### Error Handling
- Async functions without try-catch should add error handling
- Null checks missing when accessing nullable values should use optional chaining
- Callback-based async deeply nested should use async/await

##### Testability
- Direct system time calls should be injectable
- Direct file/network operations should be behind injected services
- Global state accessed directly in components should be passed via props or context

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
