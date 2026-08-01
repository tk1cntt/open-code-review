#### Protocol Buffers Refactoring Rules

##### Message Design and Simplification
- Messages with >20 fields should consider grouping related fields into nested messages
- Repeated field patterns across messages suggest extraction into a shared message
- Nested messages re-encoding the same domain concept already modeled elsewhere should be replaced with references
- Oneof fields leaving an invalid zero-state representable should use explicit sentinels

##### Naming Conventions
- Messages and services in PascalCase; fields and rpc methods in snake_case; enums in UPPER_CASE
- Enum values must be prefixed with the enum type name (e.g., `COLOR_RED` not `RED`)
- Service and rpc names should clearly communicate their domain purpose
- Avoid redundant naming: `UserMessage` → `User`, `GetUserRequest` is idiomatic

##### Field Organization
- Fields should be organized by domain concern, not by addition order
- Related fields should be grouped together with comments explaining the grouping
- Reserved fields should have comments explaining what was removed and why
- Consistent field numbering ranges for extensions vs core fields

##### Duplication
- Repeated field validation/computation patterns suggest extraction to a shared utility
- Similar request/response pairs across RPCs suggest common patterns
- Duplicate enum definitions across files should use shared proto imports

##### API Design
- Multiple RPCs sharing identical request/response types may need distinct messages
- Unbounded streaming RPCs should document flow control expectations
- Non-idempotent methods should be clearly distinguished from idempotent ones
- Missing request/response wrappers for scalar RPC parameters

##### Dead Code
- Unused messages and enums should be removed or marked reserved
- Deprecated fields and RPCs without migration path documentation
- Commented-out fields and messages should be deleted

##### Error and Default Design
- First enum value should be a zero sentinel (`*_UNSPECIFIED`)
- Relying on implicit zero defaults when zero is meaningful data should use `optional`
- Additive enum values should be appended at the end, not inserted mid-range

##### Security and Documentation
- `google.protobuf.Any` fields should document expected types
- Unbounded `repeated`/`map` fields should document size expectations
- Sensitive fields should be flagged with comments or separated into auth-gated messages

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
