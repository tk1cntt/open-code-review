#### Prisma Schema Refactoring Rules

##### Schema Organization
- Related models scattered across the file should be grouped logically
- Repeated field patterns across models (timestamps, audit fields) should use a base pattern or documentation
- Long model definitions with many fields should consider grouping related fields with comments
- Inconsistent casing or naming conventions within the schema should be standardized

##### Naming and Clarity
- Model names should be PascalCase and singular (except join tables)
- Field names should be camelCase and self-documenting
- Relation names should clearly describe the relationship direction
- `@map` / `@@map` should be used consistently when database naming differs from model naming

##### Relations and Design
- Ambiguous multiple relations between same models should have explicit relation names
- Repeated relation patterns suggest a reusable design approach
- Missing `onDelete`/`onUpdate` referential actions should be explicitly stated
- Implicit many-to-many relations that need additional fields should use explicit join models

##### Duplication
- Repeated field definitions across models suggest a mixin or base model pattern
- Identical `@@index` or `@@unique` patterns should be consistent
- Repeated `@default` values should use constants or be documented

##### Dead Schema
- Unused models and enums not referenced by any relation or query should be removed
- Unused enum values should be cleaned or deprecated
- Commented-out models and fields should be deleted

##### Index and Performance
- Missing indexes on fields used in application queries, relations, or ordering
- Redundant indexes that are subsets of existing composite indexes
- Index ordering that doesn't match query patterns

##### Type Design
- `String` fields with known limited values should use enums
- `Json` fields with known structure should use typed models where possible
- `Unsupported` type usage should have documented rationale
- Inconsistent use of optional vs required for semantically required fields

##### Security and Data Safety
- Sensitive fields exposed without access control considerations
- Default values that create insecure state
- Missing unique constraints on identity/tenant fields where duplicates matter

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
