# Consumers & pinned versions

This table tracks every known consumer of the Criteria adapter wire contract and
the `criteria-adapter-proto` version it pins. Consumers update their row when
they upgrade.

| Consumer | Language | Package / module | Pinned version | Consumes published package? |
|---|---|---|---|---|
| [criteria](https://github.com/brokenbots/criteria) (host) | Go | `github.com/brokenbots/criteria-adapter-proto` | `v0.5.1` | ✅ yes |
| [criteria-go-adapter-sdk](https://github.com/brokenbots/criteria-go-adapter-sdk) | Go | `github.com/brokenbots/criteria-adapter-proto` | `v0.5.1` | ✅ yes |
| [criteria-adapter-copilot](https://github.com/brokenbots/criteria-adapter-copilot) | Go | `github.com/brokenbots/criteria-adapter-proto` | `v0.5.1` | ✅ yes |
| [criteria-adapter-shell](https://github.com/brokenbots/criteria-adapter-shell) | Go | `github.com/brokenbots/criteria-adapter-proto` | `v0.5.1` | ✅ yes |
| [criteria-typescript-adapter-sdk](https://github.com/brokenbots/criteria-typescript-adapter-sdk) | TypeScript | `@criteria/adapter-proto` | — | ⏳ bundles its own proto; switches after the first npm publish |
| [criteria-python-adapter-sdk](https://github.com/brokenbots/criteria-python-adapter-sdk) | Python | `criteria-adapter-proto` | — | ⏳ bundles its own proto; switches after the first PyPI publish |

## Multi-language publishing status

- **Go** — published automatically: tagging `vX.Y.Z` makes
  `github.com/brokenbots/criteria-adapter-proto@vX.Y.Z` resolvable via the Go
  module proxy. The four Go consumers above already pin it.
- **npm** (`@criteria/adapter-proto`) and **PyPI** (`criteria-adapter-proto`) —
  the generation + build + publish pipeline is wired in
  [`.github/workflows/publish-langs.yml`](.github/workflows/publish-langs.yml)
  but **gated on credentials**: it publishes only when the `NPM_TOKEN` (and the
  `@criteria` npm scope) / `PYPI_API_TOKEN` repository secrets are configured.
  Until then those steps skip gracefully on each tag. Once the first npm/PyPI
  release lands, the TypeScript and Python SDKs switch from their bundled proto
  to these packages (WS41 Step 4) and update their rows above.
