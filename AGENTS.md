# AGENTS

This repository is pre-alpha.

Agents working in this codebase do not need to preserve existing APIs, interfaces, package boundaries, or internal designs.

If a goal is better served by rewriting, removing, collapsing, or replacing an existing abstraction, do that.

Prioritize:

- simpler architecture
- fewer layers and indirections when they are not earning their keep
- clearer ownership of side effects and persistence
- safer defaults
- easier testing and maintenance

Do not keep compatibility shims unless there is a concrete current need.

Prefer the smallest change that materially improves the system, but feel free to do larger structural rewrites when they are the clearest path to a better result.
