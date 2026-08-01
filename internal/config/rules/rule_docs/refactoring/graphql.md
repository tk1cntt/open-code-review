#### GraphQL Schema Refactoring Rules

##### Schema Design and Simplification
- Types unreachable from root fields (`Query`/`Mutation`/`Subscription`) should be removed or connected
- Repeated field patterns across types suggest an interface or shared fragment
- Redundant wrapper types that add no semantic value should be inlined
- Duplicate or near-duplicate input types should be consolidated

##### Naming Conventions
- Types must be PascalCase; fields/arguments camelCase; enum values UPPER_CASE
- Redundant naming: `query`/`get` prefixes on Query fields, `mutation`/`subscription` affixes on root fields
- Type name suffixes that repeat the kind (`UserType`, `StatusEnum`) should be simplified
- Consistent naming: same concept should use the same name across types

##### Nullability and Type Design
- Fields typed as nullable that can never actually be null should be non-null
- Fields typed as non-null that represent genuinely optional data should be nullable
- Repeated nullable wrapper patterns suggest a design issue with the field's domain model
- List fields with mandatory elements (`[Type!]!`) vs optional elements should accurately reflect data shapes

##### Schema Organization
- Related types and fields scattered across the schema should be grouped with `extend` or descriptions
- Missing descriptions on public types that form the API contract should be added
- Deprecated fields without a non-empty `reason` string
- `@deprecated` fields used in operations should be migrated to replacements

##### Schema Evolution
- Adding new required (non-null, no-default) arguments to existing fields should be planned carefully
- New types/fields should follow existing naming and design patterns in the schema
- Schema additions not following the project's established nullability conventions

##### Duplication
- Repeated field sets across types that could share an interface
- Identical input types used in different mutations should be extracted
- Repeated pagination patterns should use a shared page info type

##### Security and Limits
- Unbounded list fields (no pagination/first/last arguments) should add limit controls
- Deeply nested recursive type selections with no documented depth guard
- Fields carrying sensitive data without auth-related directive or documentation

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
