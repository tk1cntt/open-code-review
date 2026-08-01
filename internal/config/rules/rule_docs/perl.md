#### Perl Review Rules

> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat security and correctness findings as blocking, and style or idiom suggestions as non-blocking.

#### Obvious Typos or Spelling Errors
- Spelling errors in subroutine names, package names, or variable names at declaration sites
- Typos in error messages, log messages, or user-facing strings

#### Security
- SQL injection via string interpolation in DBI queries — use placeholders (`?`) always
- Hardcoded API keys, tokens, or credentials in source code
- Command injection via `system()`/`qx//`/backticks/`open()` with unsanitized user input
- Unsafe use of `eval` on user-controlled strings — prefer safe evaluation or validation
- File path traversal via user input passed unvalidated to `open()`/`unlink()`/`opendir()`
- `CGI` module HTML generation with unescaped user input — XSS vulnerability

#### Strict and Warnings
- Missing `use strict;` and `use warnings;` — always enable in every file
- Global variables created by omitting `my`/`our`/`state` on first assignment
- Bareword filehandles (`open FH, ...`) — use lexical filehandles (`open my $fh, ...`)

#### Error Handling
- `open()` without `or die` / `or croak` on critical file operations
- `eval {}` without checking `$@` for exception content
- `system()` return value not checked for failure
- `DBI` calls without `RaiseError => 1` or manual error checking

#### Resource Management
- File handles not closed — use lexical filehandles with scope-based closure or explicit `close()`
- DBI database handles not disconnected — use connection pooling or explicit `disconnect()`
- File locks acquired without corresponding unlock/timeout

#### Input Validation
- Untainted data used in file/shell operations — enable taint mode (`-T`) and use regex to untaint
- CGI/form parameters passed directly to templates without HTML escaping
- Email addresses/URLs used without format validation when security-relevant

#### Memory and Performance
- Slurping entire files into memory (`@lines = <$fh>`) when `while (<$fh>)` line-by-line would suffice
- `map` in void context — use `for`/`foreach` for side effects
- Repeated function calls in loop conditions — hoist to variable

For each finding, indicate severity (critical/high/medium/low) and include before/after code snippets where applicable.
