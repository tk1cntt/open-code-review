#### Dart/Flutter Refactoring Rules

##### Complexity
- Widget build methods exceeding 50 lines should extract child widgets or helper methods
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a parameter object or named parameters
- Deeply nested conditions (>3 levels) should use guard clauses with early returns
- Complex state machines should use sealed classes with exhaustive switch

##### Naming
- Use lowerCamelCase for variables, methods, parameters; UpperCamelCase for classes, enums, typedefs
- File names should be snake_case and match the primary class
- Boolean variables should use `is`/`has`/`can`/`should` prefixes
- Private members should use underscore prefix (`_privateField`)
- Avoid abbreviations — prefer clarity over brevity

##### Immutability
- Prefer `final` for local variables and `const` for compile-time constants
- Use `const` constructors wherever all fields are `final`
- Return `List.unmodifiable` / `Map.unmodifiable` from public APIs
- Use `copyWith()` pattern for immutable state updates
- Avoid `late final` when a nullable field or constructor init would suffice

##### Null Safety
- Repeated `?.` chains on the same object — use `?.let`-style pattern or early null guard
- `!` used when `?.` or `??` would avoid a potential crash in production
- Nullable fields in classes that should be non-null by domain contract

##### Widget Extraction
- Large widgets with many responsibilities should be split into smaller, focused widgets
- Repeated widget patterns across screens should be extracted into reusable components
- Inline styling repeated across widgets should use a shared theme or constants
- Business logic in widget classes should be moved to controllers, notifiers, or blocs

##### Duplication
- Identical code blocks in the same file should be extracted into helper functions
- Similar widget trees across screens should share a custom widget
- Repeated validation logic should be centralized
- Repeated file/network logic should be extracted to repository or service classes

##### Dead Code and Simplification
- Unused imports, private methods, and fields should be removed
- Commented-out code blocks should be deleted
- Empty `setState(() {})` calls — state hasn't actually changed
- Redundant `const` / `new` keywords in contexts where they're implied

##### Control Flow
- Nested if-else chains should use switch expressions (Dart 3) or guard clauses
- `if`/`else` chains on sealed types should use exhaustive `switch` (Dart 3+)
- Boolean flag parameters suggest splitting the function

##### Data and State
- Primitive obsession (string for email, int for status) should be sealed classes or enums
- Mutable state exposed through public API should use `UnmodifiableListView` or similar
- Groups of related data suggest a model class rather than positional parameters

##### Coupling and Dependency
- Direct constructor instantiation of dependencies prevents testing — inject via constructor
- `ChangeNotifier` with large scope rebuilding too many widgets — narrow the provider scope
- Direct `Navigator.push` in deep widget trees — use declarative routing (GoRouter, Navigator 2.0)

##### Testability
- Direct `DateTime.now()` — inject clock for reproducibility
- Direct `HttpClient` usage — inject client or use mockable repository
- Widget tests that depend on real network/file system — wrap in mockable services
- `WidgetTester.pumpAndSettle` with perpetual animations — use `pump` with specific duration

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
