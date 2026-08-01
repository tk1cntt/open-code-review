#### TypeScript / JavaScript Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use an options object parameter
- Deeply nested conditions should use guard clauses or early returns
- Complex async pipelines may benefit from splitting into named steps

##### Naming
- Variable and function names should clearly communicate intent
- Boolean variables should use `is`/`has`/`can`/`should` prefixes
- Avoid abbreviations unless universally understood in the codebase
- React component names should match the file name (PascalCase)

##### TypeScript-Specific
- `any` type should be avoided — use `unknown` or proper types
- Unnecessary type assertions (`as Type`) should be removed when inference works
- Union types that are too wide may hide invalid states — consider discriminated unions
- Duplicate interface/type alias definitions should be consolidated
- `as const` should be used for literal types where applicable

##### Async Handling
- Promise chains should prefer `async/await` for readability
- Mixed `async/await` and `.then()/.catch()` in the same function is confusing
- `Promise.all` should be used over sequential awaits when calls are independent
- Async functions should always handle errors (try/catch or `.catch()`)

##### React Best Practices
- Components with >300 lines should be split into smaller components
- `useEffect` with many dependencies suggests the effect does too much
- Inline render functions should be extracted as named components for readability
- Props drilling >3 levels deep suggests Context or state management
- Memoization (`useMemo`, `useCallback`) should be applied where expensive computations exist

##### Duplication
- Repeated JSX patterns should be extracted into reusable components
- Repeated validation or transformation logic should be in shared utility functions
- Similar API call patterns suggest a shared fetch wrapper or hook

##### Error Handling
- Try/catch blocks should handle specific error types, not just log and rethrow
- API calls without error handling leave the UI in indeterminate states
- Optional chaining (`?.`) abused to hide null issues — fix the root cause

##### Dead Code
- Unused imports, variables, and functions should be removed
- Commented-out code should be deleted
- Unreachable branches in conditionals should be eliminated

##### Data and State
- Mutable state spread across many `useState` calls may be better as `useReducer`
- Object/array props that change reference every render cause unnecessary re-renders
- Derived state that can be computed from existing state should not be stored separately

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
