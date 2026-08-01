#### Properties File Refactoring Rules

##### Organization
- Large `.properties` files should be organized by functional area with comment headers
- Related keys should be grouped together rather than alphabetically scattered
- Repeated key prefixes suggest a namespace convention that should be applied consistently
- Multi-line values should use proper `\` continuation consistently

##### Naming
- Key names should follow consistent conventions (dot-separated, camelCase, or snake_case)
- Key prefixes should reflect the component or feature namespace
- Boolean-like keys should use `enabled`/`disabled` consistently
- Avoid duplicate keys within the same file (causes silent overrides)

##### Duplication
- Repeated default values suggest extraction to a shared defaults section
- Repeated comment blocks explaining the same concept
- Near-duplicate keys with slight naming variations suggest a refactoring opportunity

##### Dead Entries
- Unused keys not referenced by any application code should be removed
- Commented-out key-value pairs should be deleted
- Stale keys from removed features or components
- Duplicate key definitions where only one value is actually used

##### Value Consistency
- Placeholder syntax (`{}`, `%s`, `${var}`) should be consistent across related keys
- Date/time/currency format strings should follow project standards
- URL paths and endpoints should use consistent formatting

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
