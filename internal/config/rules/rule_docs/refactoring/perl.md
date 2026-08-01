#### Perl Refactoring Rules

##### Complexity
- Subroutines exceeding 80 lines should be decomposed by responsibility
- Subroutines with >4 parameters should use named parameters (hash/hashref)
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Complex regex patterns spanning many lines — use `/x` modifier with comments or named subpatterns

##### Naming
- Use snake_case for subroutines and variables; PascalCase for package names
- Boolean variables should use `is_`/`has_`/`can_` prefixes
- Avoid single-letter variables except for well-known idioms (`$i`, `$self`)
- Package names should follow namespace conventions (`App::Module::Submodule`)

##### Strictness
- Always `use strict;` and `use warnings;` — no exceptions for modern Perl
- Enable `use utf8;` when handling non-ASCII text
- Use `use v5.36` or later for subroutine signatures (`sub foo($bar)`)
- Prefer subroutine signatures over `my ($self, $arg) = @_` manual unpacking

##### Modern Perl Patterns
- Bare object constructors — use `Moo`/`Moose`/`Class::Tiny` or `Object::Pad` (5.38+)
- Manual `AUTOLOAD` — use `Moo`/`Moose` attribute generation instead
- Direct `@ISA` manipulation — use `use parent` or `extends` in Moo/Moose
- `Exporter` used when `Sub::Exporter` or `Importer` would give finer control

##### Reference Types
- Confusion between arrays and arrayrefs — prefer consistent use of arrayrefs for complex data
- Deeply nested reference dereferencing (`$x->{a}[0]{b}`) — use intermediate variables or `Data::Diver`
- Mutating a referenced structure in place when the caller doesn't expect it

##### Duplication
- Identical code blocks in the same package should be extracted into private subroutines
- Similar logic across packages suggests a role (Moo/Moose) or base class
- Repeated DBI query patterns should be extracted to a data access layer
- Template boilerplate across scripts — use Template Toolkit or similar

##### Dead Code and Simplification
- Unused subroutines, variables, and `use` statements should be removed
- Commented-out code blocks should be deleted
- `return` at the end of a subroutine body when not needed
- `if ($x == 1)` when `if ($x)` suffices for truthy/falsey checks

##### Control Flow
- Nested `if/elsif/else` chains — consider dispatch table (hash of coderefs) or given/when
- `for` loop with index when `foreach` over elements would be clearer
- Boolean flag parameters suggest splitting the subroutine

##### Data and State
- Primitive obsession (string for email, int for status) — wrap in a simple class
- Package-level mutable state that persists across invocations — prefer explicit state objects
- `local` used excessively to modify globals — pass state explicitly instead

##### Coupling and Dependency
- Direct `DBI->connect` in domain logic — inject database handle
- Direct `LWP::UserAgent` or `HTTP::Tiny` in business logic — inject HTTP client
- Direct `DateTime->now` — inject clock for testability
- Direct filesystem access (`open`, `unlink`, `rename`) — inject filesystem abstraction

##### Testability
- Direct `time()`/`localtime()`/`gmtime()` — inject time provider
- Tests that depend on real network/database — inject test doubles
- `sleep` in test code — use `Test::MockTime` or time simulation

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
