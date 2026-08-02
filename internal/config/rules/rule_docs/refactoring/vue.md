#### Vue Refactoring Rules (delta)

Overrides common where noted. Complements TS/JS rules for `.vue` SFCs.

##### Complexity
- Components >300 lines → child components or composables
- Templates >100 lines → subcomponents
- Complex computed (>~20 lines) → composable/helper
- >5 props → group into a config object prop when cohesive

##### Naming
- Components: PascalCase in script, kebab-case in template
- Composables: `use` prefix (`useCart`)
- Events: kebab-case (`update:model-value`)

##### Component Architecture
- Extract business logic from `<script setup>` into composables
- Prefer slots/shared components over copy-pasted markup
- Prefer scoped styles / design tokens over repeated inline styles

##### Composables and State
- One composable per concern
- Prefer Pinia/store over deep provide/inject for global state
- Shared fetch logic → `useFetch` or API layer

##### Template Simplification
- Move complex template expressions into computed/methods
- Prefer object `:class`/computed over long ternary chains
- Consider virtualization for huge `v-for` lists
