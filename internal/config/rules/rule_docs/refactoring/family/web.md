#### Family: Web UI (TS/JS / Vue / Astro / ArkTS)

Shared patterns for web/UI frontends. Language deltas override this family.

##### Component Size
- Split components/files that mix data-fetch, business rules, and large presentational trees
- Prefer extracting repeated markup into components/composables/helpers

##### State & Effects
- Prefer deriving state over storing copies that can drift
- Effects/hooks with many dependencies often do too much — split by concern
- Avoid prop drilling beyond ~3 levels; use context/store/provide when justified

##### Async UI
- Always handle loading and error states for remote calls
- Prefer concurrent independent requests over sequential awaits when order is free

##### Performance Hygiene
- Avoid unstable object/array identities that force needless re-renders
- Memoize only when measurement or clear cost justifies it
