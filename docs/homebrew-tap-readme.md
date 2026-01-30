# Hyperengineering Homebrew Tap

Homebrew formulae for Hyperengineering tools.

## Installation

```bash
brew tap hyperengineering/tap
brew install recall
```

## Formulas

| Formula | Description |
|---------|-------------|
| `recall` | CLI for managing experiential lore from AI agent workflows |

## Updating

```bash
brew update
brew upgrade recall
```

## Troubleshooting

### "No available formula" error

If you get this error, ensure the tap is added:

```bash
brew tap hyperengineering/tap
```

### Version mismatch

To get the latest version:

```bash
brew update
brew upgrade recall
```

### Uninstalling

```bash
brew uninstall recall
brew untap hyperengineering/tap
```

## About

This tap is automatically updated by [GoReleaser](https://goreleaser.com/) when new versions of Recall are released.

For issues with the Recall CLI itself, please file issues at:
https://github.com/hyperengineering/recall/issues
