#### PO Translation File Refactoring Rules

##### Translation Organization
- Large PO files with >500 entries should be split by domain or module
- Related translation entries should be grouped together with comments
- Repeated translation patterns suggest inconsistent terminology that should be standardized
- Context (`msgctxt`) should be used consistently for ambiguous strings

##### Entry Cleanliness
- Fuzzy translations should be reviewed and either confirmed or re-translated
- Obsolete entries (`#~`) should be removed once migration is complete
- Entries with empty `msgstr` that are not templates should be translated
- Duplicate `msgid` definitions with conflicting translations should be resolved

##### Naming and Clarity
- Translator comments (`# `) should explain context for ambiguous strings
- Extracted comments (`#.`) should provide useful hints for translators
- Reference comments (`#:`) that point to deleted source files should be cleaned

##### Placeholder Hygiene
- Inconsistent placeholder formatting across entries with similar structure
- Format specifiers that differ between `msgid` and `msgstr` should be fixed
- `msgid` values with hardcoded text that should use placeholders for dynamic content

##### Dead Entries
- Entries with references only to deleted or moved source files
- Unused translations (not referenced by any current source code)
- Commented-out entries left from merge conflicts or debugging

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
