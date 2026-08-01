#### JSON Refactoring Rules

##### Structure and Organization
- Large JSON files (>500 lines) should be split by logical section or domain
- Repeated object patterns should follow consistent schema
- Deeply nested structures (>4 levels) should be flattened where possible
- Key ordering should be consistent across similar object types

##### Naming
- Keys should use consistent casing (camelCase or snake_case, not mixed)
- Key names should clearly describe their values without abbreviations
- Avoid numeric suffixes on keys (`key1`, `key2`); use meaningful names or arrays
- Enum-like string values should follow consistent formatting conventions

##### Duplication
- Repeated configuration blocks suggest extraction to shared references
- Repeated schema objects should be `$ref` targets (in JSON Schema-compatible files)
- Identical arrays in related sections should be consolidated

##### Dead Data
- Unused and unreferenced keys should be removed
- Empty objects and arrays that serve no purpose
- Commented-out sections (if supported by the file's JSON variant)
- Default values that match the consumer's default behavior

##### Schema Integrity
- Inconsistent field presence across objects of the same type
- Values using wrong types (string for number, array for single value)
- Date/time values with inconsistent format across the file

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
