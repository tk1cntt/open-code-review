#### Astro Refactoring Rules

##### Component Design
- `.astro` components with excessive frontmatter logic should extract business logic to separate modules
- Framework components used only for static markup should be replaced with plain Astro markup
- Repeated islands with similar hydration patterns should be consolidated
- Large components with many responsibilities should be split by concern

##### Hydration Optimization
- `client:load` on non-critical UI should be `client:idle`, `client:visible`, or `client:media` where appropriate
- `client:only` without fallback content or framework string should be fixed
- Over-hydration from large or overly numerous islands should be reduced
- Heavy framework components that could be server-rendered Astro should be refactored

##### Data Flow
- Frontmatter data that reaches multiple hydrated islands should be structured to minimize payload
- Props passing through multiple component layers should be consolidated
- `Astro.locals`, cookies, headers, or request-only data exposed to client unintentionally
- Server-only values reaching client bundles should be stripped

##### Duplication
- Repeated layout patterns should be extracted into layout components or base layouts
- Repeated `<script>` or `<style>` blocks suggest missing component extraction
- Similar content fetching logic across pages suggests a shared content loader
- Duplicated SEO/meta tags across pages should use a shared `BaseHead` component

##### Dead Code and Simplification
- Unused island imports and framework components should be removed
- Empty or no-op `<script>`/`<style>` blocks should be cleaned
- Commented-out template sections should be deleted
- Unused slots, props, and content collection references

##### Content and Assets
- Ad-hoc Markdown/MDX loading when content collections would provide better typing and validation
- Plain `<img>` where Astro's `<Image />` component would provide optimization
- Repeated content queries across pages should use shared query functions

##### Template Organization
- Templates with deeply nested conditional rendering should use components or helper functions
- Repeated `set:html` usage patterns should be extracted with consistent sanitization
- `is:global` overuse should be narrowed to scoped styles with targeted `:global()` escapes
- Selectors assuming scoped CSS can pierce child component internals should be fixed

##### Error and Loading States
- Server islands (`server:defer`) without fallback slots should add loading states
- Missing error boundaries for dynamic islands that can fail

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
