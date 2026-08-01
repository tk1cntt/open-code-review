#### MyBatis Mapper XML Refactoring Rules

##### SQL Organization
- Long XML mapper files with >100 lines should be organized by operation type (select, insert, update, delete)
- Repeated SQL fragments (column lists, join clauses, WHERE conditions) should use `<sql>` + `<include>`
- Complex dynamic SQL with deeply nested `<if>`/`<choose>` should be decomposed into `<sql>` fragments

##### Naming and Clarity
- Statement `id` attributes should match the mapper interface method name exactly
- `<resultMap>` and `<sql>` IDs should clearly describe their purpose
- Parameter names should match between XML and Java/Kotlin interface
- Column aliases in queries should use meaningful names matching result mapping

##### Duplication
- Repeated column lists across SELECT statements should be `<sql>` fragments
- Repeated JOIN patterns should be extracted into shared `<sql>` fragments
- Identical WHERE clause patterns across queries suggest `<sql>` fragments
- Repeated result mapping definitions should be referenced via `extends`

##### Performance
- Queries without pagination that may return large datasets should add LIMIT/pagination
- Missing WHERE conditions that cause full table scans
- Repeated subqueries should be extracted or optimized
- `<foreach>` on large collections without batching consideration

##### SQL Simplification
- Overly complex nested subqueries should be flattened or extracted to CTEs where supported
- Long CASE expressions should be considered for lookup tables or application logic
- Dynamic SQL with many conditional branches may be clearer as application logic

##### Dead Code
- Unused `<resultMap>`, `<sql>`, and statement definitions should be removed
- Commented-out SQL blocks should be deleted
- Unused parameter mappings in result maps

##### Parameter Safety
- `${}` string substitution on user-controlled values should use `#{}` parameter binding
- Direct LIKE concatenation should use CONCAT with parameter binding
- Dynamic ORDER BY / GROUP BY fields should be validated against an allowlist

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
