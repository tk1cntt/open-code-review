#### FreeMarker Template Refactoring Rules

##### Template Organization
- Large templates with >200 lines should be split using `<#import>` and `<#macro>` libraries
- Repeated UI patterns across templates should be extracted into shared macros
- Business logic in templates should be moved to the controller/model layer
- `<#assign>` building state that should be prepared before rendering suggests missing model preparation

##### Naming
- Macro and function names should use camelCase and clearly describe their purpose
- `<#assign>` variable names should convey intent; single-letter names discouraged
- Template file names should reflect the content they render
- Macro library files should be grouped by concern and named clearly

##### Duplication
- Repeated conditional blocks across templates should be macros
- Shared layout structure (header, footer, nav) should be in a base layout macro
- Repeated `<#include>` patterns suggest namespace isolation with `<#import>`
- Duplicated macro definitions across templates should be consolidated into shared libraries

##### Macro and Import Hygiene
- `<#include>` where `<#import>` (namespaced) prevents variable shadowing
- Macros defined but never called should be removed
- Relative template paths in `<#include>` that would be ambiguous in nested templates
- Unused imports adding noise

##### Dead Code
- Unused macros, functions, and `<#assign>` variables should be removed
- Commented-out template blocks should be deleted
- Empty `<#macro>` definitions
- Unreachable branches in `<#if>`/`<#else>` chains

##### Control Flow
- Deeply nested `<#if>`/`<#list>` blocks should be flattened with early exits or extracted macros
- Complex conditional expressions should be simplified with `<#assign>` intermediate values
- Boolean flags in the data model controlling template flow suggest the logic belongs in the controller

##### Output Safety
- User-controlled values interpolated without `?html`/`?url`/`?js_string` when auto-escaping is inactive
- Explicit `?no_esc` on user data should be justified or replaced with proper escaping
- Numbers/dates without `?c`/`?string` formatting where machine format is required (URLs, JSON, IDs)

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
