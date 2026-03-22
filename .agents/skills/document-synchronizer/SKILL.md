---
name: document-synchronizer
description: "Ensures project documentation (README, IDEA.md, artifacts) stays in sync with code changes. Triggers when code is implemented or modified."
---

# Document Synchronizer Skill

## Purpose
This skill ensures that whenever code changes are made to the project, all referenced documentation is reviewed and updated if necessary. This prevents documentation rot and ensures that the project's state is always accurately reflected in its descriptive files.

## When to Use
- **After Implementing a Feature**: Check if `README.md` or `IDEA.md` need updates.
- **After Refactoring**: Verify that architectural maps or component descriptions are still accurate.
- **Before Completing a Task**: Ensure all `walkthrough.md` or `implementation_plan.md` items reflect the final state.
- **When modifying core modules**: Ensure `internal/` documentation matches the new reality.

## Key Instructions
1. **Identify Referenced Docs**: Always look for `README.md`, `IDEA.md`, and files in `docs/` or `.agents/`.
2. **Scan for Stale Info**: Check for updated function signatures, new environment variables, or removed features.
3. **Verify Links**: Ensure all internal file links in documents are still valid after file moves or renames.
4. **Update artifacts**: Ensure `task.md` and `walkthrough.md` in the brain directory are up to date with the latest progress.

## Check-list
- [ ] Are new environment variables added to `.env.example`?
- [ ] Does `README.md` reflect the new setup steps?
- [ ] Is `IDEA.md` updated with the new design decisions?
- [ ] Do manual test instructions in `walkthrough.md` match the actual implementation?
