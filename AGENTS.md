# AGENTS

This repository is pre-alpha. Agents do not need to preserve existing APIs, interfaces, package boundaries, or internal designs. If a goal is better served by rewriting, removing, collapsing, or replacing an existing abstraction, do that.

Prioritize:

- simpler architecture
- fewer layers and indirections when they are not earning their keep
- clearer ownership of side effects and persistence
- safer defaults
- easier testing and maintenance

Do not keep compatibility shims unless there is a concrete current need.

Prefer the smallest change that materially improves the system, but feel free to do larger structural rewrites when they are the clearest path to a better result.

## Workflow

Prefer using configured `mise` tasks to run repository workflows whenever one is available.

When creating commits, use the configured `mise` task: `mise run commit ...` so commits go through `cog commit` rather than calling `git commit` directly.

When a task is finished, and the agent is confident nothing else broke and nothing was left undone, commit and push the work.

For Go formatting, use `gofumpt` via the configured `mise` task instead of running `gofmt` directly.
