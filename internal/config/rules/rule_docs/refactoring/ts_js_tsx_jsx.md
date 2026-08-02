#### TypeScript / JavaScript Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- React component names should match the file name (PascalCase)
- Avoid abbreviations unless universally understood in the codebase

##### TypeScript-Specific
- Avoid `any` — use `unknown` or proper types
- Remove unnecessary `as Type` when inference works
- Prefer discriminated unions over overly wide unions
- Consolidate duplicate interface/type aliases
- Use `as const` for literal types where applicable

##### Async Handling
- Prefer `async/await` over long Promise chains
- Do not mix `async/await` and `.then()/.catch()` in the same function
- Use `Promise.all` for independent concurrent work
- Always handle async errors (try/catch or `.catch()`)

##### React Best Practices
- Components with >300 lines should be split
- `useEffect` with many dependencies often does too much
- Extract inline render functions as named components
- Props drilling >3 levels → Context or state management
- Apply `useMemo` / `useCallback` only for expensive work (not by default)

##### Duplication
- Repeated JSX → reusable components
- Repeated fetch patterns → shared wrapper or hook

##### Error Handling
- API calls without error handling leave UI indeterminate
- Do not abuse optional chaining (`?.`) to hide null root causes

##### Data and State
- Many related `useState` calls may be clearer as `useReducer`
- Unstable object/array prop identities cause extra re-renders
- Do not store derived state that can be computed
