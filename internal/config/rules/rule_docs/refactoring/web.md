#### Web (CSS/HTML/Svelte) Refactoring Rules

##### Complexity
- Stylesheets exceeding 500 lines — split by component, page, or concern
- Complex selectors spanning >3 combinators — break into separate rules or use BEM/utility
- HTML files exceeding 300 lines — extract partials or components
- Complex CSS animations — extract to dedicated animation classes or keyframes file

##### Naming
- CSS class names should use BEM (`.block__element--modifier`), utility-first, or CSS Modules convention
- CSS custom properties: kebab-case with namespace prefix (`--app-color-primary`, `--btn-radius`)
- HTML `id` attributes: kebab-case, used sparingly (prefer `class` or `data-*`)
- Keyframe names: kebab-case describing the motion (`fade-in`, `slide-from-right`)

##### Design Tokens
- Repeated color values, font sizes, spacing — define as CSS custom properties in a theme layer
- Magic numbers in layout — use design-token-based spacing scale
- Inconsistent breakpoints — define standard named breakpoints as custom properties or SCSS variables
- Repeated `font-family` declarations — set at `:root` or `body` level

##### CSS Organization
- Mixed concerns in one file (layout, typography, animations, theming) — use layered architecture
- `@import` scattered across files — consolidate or use a build tool
- Inline `style` attributes in HTML — extract to CSS class or `style` tag for critical-path CSS only
- Duplicating vendor-prefixed properties — use `autoprefixer` or PostCSS plugin

##### Duplication
- Repeated CSS declarations across components — extract to shared utility or mixin
- Repeated HTML structure across pages — extract to partial/component/layout
- Repeated media query blocks — consolidate into component-level or use container queries
- Identical `@keyframes` defined in multiple files — centralize animation definitions

##### Dead Code
- Unused CSS classes, keyframes, custom properties, and `@font-face` declarations should be removed
- Commented-out styles should be deleted
- Empty CSS rulesets (`selector { }`) — remove
- Vendor prefixes for features with >98% native support — let autoprefixer handle

##### Selector and Specificity Refactoring
- Nested selectors exceeding 3 levels — flatten or use BEM naming
- `!important` flags — refactor source of specificity conflict
- Qualifying selectors unnecessarily (`div.header`, `ul.list`) — remove element qualifier
- Overly broad selectors catching unintended children — use `>` child combinator or BEM

##### Responsive Design
- Desktop-first or mobile-first inconsistency — standardize on one approach
- Inline media queries — use `@media` at end of component or separate breakpoint files
- Duplicate media query blocks — use mixins or PostCSS custom-media
- Magic breakpoint numbers — standardize to a scale (375, 768, 1024, 1280)

##### Svelte Architecture
- Large Svelte files with HTML+CSS+JS exceeding 300 lines — split into child components
- Business logic in `.svelte` files — extract to `.svelte.js` or `.svelte.ts` modules
- Repeated store patterns — extract to reusable store factory
- Missing `onDestroy` cleanup for subscriptions, listeners, timers

##### Performance
- Large CSS bundles — remove unused CSS, lazy-load route-specific styles
- `@import` in CSS files creating request chains — use bundler or inline
- Heavy `box-shadow` and `filter` on animated elements — use `will-change` or simplify
- Missing `loading="lazy"` / `decoding="async"` on off-screen images

##### Coupling and Dependency
- CSS styles tightly coupled to specific DOM structure — use BEM or component-scoped styles
- HTML referencing CSS from unrelated components — component styles should be co-located
- Inline `<script>` and `<style>` in HTML — extract to files with `defer`/`media` where appropriate

##### Testability
- Visual regression testing baseline — use screenshot comparison or Playwright for critical pages
- Interactive elements without `data-testid` or accessible selectors
- CSS-only states (hidden, disabled) without testable attributes — add `aria-*` attributes

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
