# Contributing

Assume only `mise` is installed globally.

## Setup

```sh
mise install
```

## Workflow

```sh
mise run fix
mise run check
```

Common tasks:

- `mise run fix`
- `mise run lint`
- `mise run test`
- `mise run build`

## Commits

Use Conventional Commits:

```sh
mise exec cocogitto -- cog commit <type> "<message>" [scope]
```

Use `-B` for breaking changes.

## Pull requests

- run `mise run check`
- keep changes focused
- update tests and docs when behavior changes
- use a Conventional Commit title
