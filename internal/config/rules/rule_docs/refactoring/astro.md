#### Astro Refactoring Rules (delta)

Overrides common where noted.

##### Component Design
- Keep frontmatter thin — extract business logic to modules
- Prefer plain Astro markup when hydration is unnecessary
- Split large multi-responsibility components

##### Hydration Optimization
- Prefer `client:idle` / `client:visible` / `client:media` over blanket `client:load`
- Fix `client:only` missing fallback/framework string
- Reduce over-hydration of islands

##### Data Flow
- Minimize payload to hydrated islands
- Do not leak server-only values (`Astro.locals`, cookies, secrets) to client bundles

##### Content and Assets
- Prefer content collections over ad-hoc MD/MDX loading
- Prefer Astro `<Image />` over plain `<img>` when optimization matters
- Share SEO/meta via base head/layout components

##### Error and Loading States
- `server:defer` islands should provide fallback slots
