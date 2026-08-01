#### Correctness
Is the logic correct? Are there missing boundary conditions?
Are exceptions handled properly?
Is it thread-safe in concurrent scenarios?

#### Immutability and Mutation
Is mutable state minimized? Are shared mutable references a risk in concurrent code?
Are defensive copies made when exposing internal collections or state?
Are `const`/`final`/`val`/`let` used by default with mutability as the explicit choice?

#### Input Validation
Are values validated at system boundaries (API entry, user input, file reads, message queues)?
Does the code distinguish between recoverable validation errors and unrecoverable preconditions?
Are error messages clear and actionable when validation fails?

#### Security
Are there security vulnerabilities such as SQL injection or XSS?
Is sensitive information handled correctly?
Are secrets (API keys, tokens, credentials) managed outside source code (env vars, vault, secrets manager)?
Is permission validation complete?

#### Performance
Are there obvious performance issues (e.g., N+1 queries, unnecessary loops)?
Are resources properly released?

#### Maintainability
Is the code clear and easy to understand?
Do names accurately express intent?
Does it follow the project’s existing code style and architecture patterns?

#### Test Coverage
Do critical logic paths have corresponding test cases?
Do test cases cover boundary conditions?
