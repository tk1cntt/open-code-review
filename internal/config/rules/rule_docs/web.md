#### Web (CSS/HTML/Svelte) Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in CSS class names, custom property names, HTML `id`/`class` attributes, or `data-*` attributes
- Typos in user-facing text content or `aria-label` values

#### HTML Semantics and Accessibility
- `<div>`/`<span>` used for interactive controls — use `<button>`, `<a>`, `<input>`, `<select>` instead
- Missing `alt` attribute on informative `<img>` elements
- Form inputs without associated `<label>` — use `for`/`id` or wrapping label
- Heading levels skipped (`<h1>` → `<h3>`) — maintain sequential hierarchy
- `tabindex` values > 0 — prefer natural DOM order
- Missing `lang` attribute on `<html>` element

#### CSS Specificity and Architecture
- `!important` used to override specificity wars — refactor selector chain instead
- ID selectors (`#id`) in CSS — prefer class selectors for reusable styles
- Deeply nested selectors (>3 levels) — flatter selectors prevent specificity issues
- Universal selector (`*`) with broad scope — narrow to specific elements
- Inline styles in HTML when a CSS class would enable reuse and caching

#### Responsive and Layout
- Fixed pixel widths preventing responsive layout — use `rem`/`%`/`vw`/`clamp()`/`min()`
- Missing `viewport` meta tag (`<meta name="viewport" content="width=device-width, initial-scale=1">`)
- Media queries using device-specific breakpoints instead of content-based — use `em` not `px`
- `overflow: hidden` on body — accessibility/scroll issue

#### Performance
- Render-blocking CSS in `<head>` without `media` attribute — use `media="print" onload="this.media='all'"`
- Animations on properties that trigger layout (`width`, `height`, `top`, `left`) — use `transform` and `opacity`
- Unused CSS loaded on page — consider code-splitting or `@import` with media queries
- Large DOM trees from unnecessary wrapper elements

#### Svelte-Specific
- Reactive statements (`$:`) with side effects that should be in `onMount`/`onDestroy`
- Store subscriptions not unsubscribed in `onDestroy`
- Missing `bind:this` cleanup when referencing DOM elements
- Mutating props from child component — use events or two-way binding

#### Security
- Inline event handlers with user-controlled data (`onclick="eval(...)"`)
- `innerHTML` / `{@html}` with unsanitized user content — XSS
- Third-party script embeds without `integrity` and `crossorigin` attributes
- User-controlled data in `href` (`javascript:` protocol) or `src` attributes

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
