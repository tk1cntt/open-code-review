#### Perl Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- snake_case subs/vars; PascalCase packages
- Boolean prefixes: `is_`/`has_`/`can_`

##### Strictness
- Always `use strict` and `use warnings`
- Prefer modern signatures (`use v5.36+`) over manual `@_` unpacking
- Enable `use utf8` for non-ASCII text

##### Modern Perl Patterns
- Prefer Moo/Moose/Object::Pad over bare constructors and hand-rolled `AUTOLOAD`
- Prefer `use parent` / `extends` over direct `@ISA` hacks

##### Reference Types
- Be consistent with array vs arrayref usage
- Extract intermediate vars for deep dereference chains
- Do not mutate shared structures when callers expect copy semantics

##### Control Flow
- Prefer dispatch tables (hash of coderefs) for long `if/elsif` chains
