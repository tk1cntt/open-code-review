#### Dart/Flutter Refactoring Rules (delta)

Overrides common where noted.

##### Complexity
- Widget `build` methods exceeding ~50 lines should extract child widgets/helpers
- Prefer sealed classes + exhaustive switch for complex state machines

##### Naming
- lowerCamelCase vars/methods; UpperCamelCase types
- File names snake_case matching primary type
- Private members: `_` prefix

##### Immutability
- Prefer `final` locals and `const` constructors when possible
- Use `copyWith` for immutable state updates
- Prefer unmodifiable views from public APIs

##### Null Safety
- Avoid bang `!` when `?.` / `??` / early guards suffice
- Reduce long `?.` chains on the same object

##### Widget Extraction
- Move business logic out of widgets into controllers/notifiers/blocs
- Share repeated widget trees and theme tokens

##### Control Flow
- Prefer exhaustive `switch` on sealed types (Dart 3+)
