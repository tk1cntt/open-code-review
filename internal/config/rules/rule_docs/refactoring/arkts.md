#### ArkTS Refactoring Rules (delta)

Overrides common where noted.

##### State Management
- `@State` arrays/objects must be replaced (not mutated in place) to refresh UI
- Props drilling >3 levels → `@Provide`/`@Consume` or context
- Keep `@StorageLink`/`@StorageProp` for true global state only

##### Component Structure
- Keep `build()` free of side effects — use lifecycle hooks
- Extract repeated UI into `@Component`
- Complex `ForEach`/`LazyForEach` item builders → child components

##### Performance
- Large lists → `LazyForEach`
- Avoid object/closure churn inside `build()`; cache with `@Watch` / computed where available

##### Resource Usage
- Prefer `$r('app.string…')` / media resources over hard-coded strings, paths, colors
