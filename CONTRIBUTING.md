# Contributing to Virtual Private Node

Thanks for your interest in contributing. This project exists to make running a private, self-hosted Bitcoin and Lightning node as frictionless as possible, and contributions from the community are welcome — whether that's code, documentation, translations, bug reports, or ideas.

## Code of conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). Be respectful, assume good faith, and focus discussions on what's best for the project and its users.

## Reporting security issues

**Do not file security issues as public GitHub issues.** If you discover a vulnerability that could compromise user funds, private keys, or node integrity, please disclose it privately first. See [SECURITY.md](SECURITY.md) for the full security disclosure policy.

For non-security bugs, open a GitHub issue with steps to reproduce, expected vs actual behavior, and your environment.

## Ways to contribute

### Good first issues

Issues tagged [`good first issue`](https://github.com/ripsline/virtual-private-node/labels/good%20first%20issue) are scoped to be approachable without deep knowledge of the codebase. Typical examples: small UI tweaks, documentation improvements, accessibility polish.

Issues tagged [`help wanted`](https://github.com/ripsline/virtual-private-node/labels/help%20wanted) are larger but well-defined — good for contributors who want to tackle something meatier.

### Translations

The TUI is currently English-only. If you'd like to help translate it into another language, open an issue to discuss. Translation work is especially welcome — Bitcoin sovereignty tools shouldn't be gated by English literacy.

### Documentation

Documentation improvements are always welcome. This includes fixing typos, clarifying confusing sections, adding examples, or translating docs. The `README.md`, `docs/syncthing.md`, `docs/reproducing.md`, and `docs/verifying.md` are all fair game.

### Bug reports and feature requests

Open an issue with as much context as you can. Screenshots help for TUI bugs. For feature requests, describe the user problem first, then your proposed solution — this makes it easier to find the best fix.

## Development setup

### Prerequisites

- Debian 13+ box
- Go 1.26.1+ (check `go.mod` for the exact version)
- `git`, `wget`, `curl`

Install Go from [go.dev/dl](https://go.dev/dl/). Do not use distribution packages — they can produce non-reproducible builds.

### Clone and build

```bash
git clone https://github.com/ripsline/virtual-private-node.git
cd virtual-private-node
go mod tidy
go build -o rlvpn ./cmd/
```

### Building a reproducible release

See [docs/reproducing.md](docs/reproducing.md) for the exact build flags and verification steps.

## Pull request guidelines

- Open a PR against `main`
- Keep PRs focused — one logical change per PR
- Write a descriptive commit message explaining the **why**, not just the what
- Run `go fmt ./...` and `go vet ./...` before submitting
- Update documentation if your change affects user-visible behavior

### Commit style

The project uses plain prose commit messages, not Conventional Commits. A good commit message:

- First line: short summary under 72 characters
- Blank line
- Body: explains the motivation, the approach, and any trade-offs

Look at recent commits with `git log` for examples.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep line length reasonable (the project targets ~60 columns in most screen files, but this isn't strictly enforced)
- Prefer explicit over clever
- Add comments for non-obvious decisions, especially around invariants

## Questions?

Open a [discussion](https://github.com/ripsline/virtual-private-node/discussions) or drop a comment on an existing issue. This is a small project and maintainers read everything.

Thanks for helping build a more sovereign Bitcoin ecosystem.