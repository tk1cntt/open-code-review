#### POT Template File Refactoring Rules

##### Template Organization
- Large POT files should be organized by source module or domain
- Related entries should be grouped with section comments
- Inconsistent header metadata should be standardized across related templates

##### Entry Cleanliness
- Non-empty `msgstr` values (translations accidentally committed into template)
- Entries with references pointing to deleted or moved source files
- Duplicate `msgid` definitions should be consolidated
- Missing `msgid_plural` where the source string embeds a count placeholder

##### Placeholder Consistency
- Format placeholders that differ between `msgid` and `msgid_plural` should be aligned
- Named placeholders that differ between singular and plural forms
- Inconsistent use of positional markers (`%1$s`) across template

##### Dead Entries
- Entries with no source references (orphaned from extractor cleanup)
- Commented-out entries left from merge conflicts
- Obsolete entries that should have been removed by the extraction tool

##### Structure and Standards
- Plural forms header inconsistent with the target languages
- Encoding or charset that doesn't match actual file encoding
- Missing Content-Type or other required headers

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
