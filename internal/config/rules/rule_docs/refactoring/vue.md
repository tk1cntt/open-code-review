#### Vue Refactoring Rules

##### Complexity
- Component files exceeding 300 lines — extract child components or composables
- Components with >5 props — consider grouping into a configuration object prop
- Template exceeding 100 lines — extract sub-components or render-less components
- Complex computed properties spanning >20 lines — extract to composable or helper

##### Naming
- Component names: PascalCase in `<script>`, kebab-case in `<template>` tags
- Props: camelCase in script, kebab-case in template
- Composables: camelCase with `use` prefix (`useUser`, `useCart`)
- Boolean props should use `is`/`has`/`can` prefixes
- Event names: kebab-case (`update:model-value`, `item-selected`)

##### Component Architecture
- Giant components with multiple responsibilities — decompose by feature or UI region
- Repeated markup patterns across components — extract to shared component with slots
- Business logic in `<script setup>` — extract to composables for reusability and testability
- Inline styles repeated across components — use CSS modules, scoped styles, or design tokens

##### Composables and State
- Composable doing too many things — split by concern (one composable per domain)
- Global state via `provide/inject` when Pinia/store would scale better
- Local `ref()` where `useLocalStorage` or `useSessionStorage` would persist state
- Repeated fetch logic across components — extract to `useFetch` composable or API layer

##### Template Simplification
- Deeply nested `v-if`/`v-else-if`/`v-else` chains — use computed with object mapping or `<component :is>`
- Complex template expressions — move to computed properties or methods
- Repeated slot patterns — use named slots with defaults
- Long `v-for` without pagination or virtual scrolling — consider `vue-virtual-scroller`

##### Duplication
- Identical composable logic in the same file — extract to shared helper
- Similar component structures across views — create a generic component with slots/props
- Repeated form validation logic — use `vee-validate` or custom validation composable
- API call patterns duplicated across composables — extract to shared API layer

##### Dead Code and Simplification
- Unused imports, components, composables, and reactive variables should be removed
- Commented-out template blocks and script code should be deleted
- Empty lifecycle hooks (`onMounted(() => {})`) — remove
- Props/emits declared but never used

##### Control Flow
- Boolean flag props used for conditional rendering — consider `<slot>` with named slots
- Multiple `v-if` on the same condition in a template — extract to wrapper or computed
- Repeated `:class` / `:style` ternary chains — use computed or object syntax

##### Data and State
- Primitive obsession — wrap related primitives in a typed object/reactive class
- Props drilling through many component layers — use `provide/inject` or store
- Multiple top-level `ref()` calls that are always updated together — group in `reactive()`

##### Coupling and Dependency
- Direct `fetch()`/`axios` calls in components — inject API service or use composable
- Direct `useRouter()`/`useRoute()` in deeply nested components — pass params as props or inject
- Direct `localStorage`/`sessionStorage` access — use `useStorage` from VueUse or similar

##### Testability
- Direct `Date.now()` in components/composables — inject time provider
- Direct API calls in components — mock at composable or service boundary
- Timer-dependent logic (`setTimeout`/`setInterval`) — wrap in composable with cleanup
- Direct DOM manipulation — use template refs and Vue's reactivity system

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
