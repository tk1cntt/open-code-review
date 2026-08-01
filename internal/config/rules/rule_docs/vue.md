#### Vue Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in component names, prop names, emit event names, or exposed names at declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Reactivity
- `ref()` / `reactive()` value mutated without `.value` accessor (Composition API)
- Destructuring `reactive()` object — loses reactivity; use `toRefs()` instead
- Large reactive object recreated on every render — use `computed()` or `shallowRef()`
- Mutating props directly — emit events or use `v-model` for two-way binding
- `watchEffect()` with side effects that should be in `watch()` with explicit dependencies

#### Computed vs Watch
- Side effects inside `computed()` — use `watch()` for effects
- `watch()` on a value when `computed()` would express the derived state more clearly
- Missing `immediate: true` on `watch()` when the initial value needs processing

#### Template Safety
- `v-if` and `v-for` on the same element — extract to computed property filtering
- `v-html` with unescaped user input — XSS vulnerability, use `{{ }}` interpolation or sanitize
- `:key` missing or using index in `v-for` with dynamic list — use unique stable IDs
- Event handlers with `async` without error handling — wrap with try/catch

#### Component Design
- Props without type validation (`type`, `required`, `validator`)
- `emit` events not declared in `defineEmits` (Vue 3) or `emits` option
- Large components with multiple responsibilities — extract child components
- Inline template logic mixing presentation and business logic — use composables

#### Composables
- Composable with side effects in setup that should be lazy — return functions instead
- Composable creating new state on every call when shared state is intended — use file-level state
- Missing `onUnmounted` cleanup for event listeners, timers, or subscriptions in composables

#### Lifecycle and SSR
- DOM access in `setup()` without `onMounted()` — element may not exist yet
- `window`/`document` access without guard in SSR context — check `typeof window !== 'undefined'`
- Missing `onServerPrefetch` for async data that should be server-rendered (Nuxt/SSR)
- Memory leaks from event listeners, observers, or timers not cleaned up in `onUnmounted`

#### Security
- Hardcoded API keys, tokens, or credentials in source code
- `v-html` with user-controlled content — use `{{ }}` or DOMPurify
- Dynamic component loading with user-controlled component name (`<component :is="userInput">`)
- URL redirects from user-controlled route params without validation

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
