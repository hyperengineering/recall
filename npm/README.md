# @hyperengineering/recall

CLI for managing experiential lore from AI agent workflows.

This npm package provides a convenient way to install and use the `recall` CLI tool. The package automatically downloads the correct binary for your platform during installation.

## Installation

```bash
npm install @hyperengineering/recall
```

Or with yarn:

```bash
yarn add @hyperengineering/recall
```

Or with pnpm:

```bash
pnpm add @hyperengineering/recall
```

## Usage

After installation, the `recall` command is available:

```bash
npx recall --help
```

Or if installed globally:

```bash
npm install -g @hyperengineering/recall
recall --help
```

## Supported Platforms

| OS | Architecture | Supported |
|----|--------------|-----------|
| macOS | x64 (Intel) | Yes |
| macOS | arm64 (Apple Silicon) | Yes |
| Linux | x64 | Yes |
| Linux | arm64 | Yes |
| Windows | x64 | Yes |
| Windows | arm64 | No |

## Environment Variables

### `RECALL_SKIP_DOWNLOAD`

Set to `1` to skip the binary download during installation. Useful when:
- You want to install the binary manually
- You're caching the binary in CI/CD
- The download is blocked by network restrictions

```bash
RECALL_SKIP_DOWNLOAD=1 npm install @hyperengineering/recall
```

### `RECALL_BINARY_PATH`

Point to a custom binary location instead of the downloaded one:

```bash
export RECALL_BINARY_PATH=/usr/local/bin/recall
npx recall --help
```

## CI/CD Usage

### GitHub Actions

```yaml
- name: Install recall
  run: npm install @hyperengineering/recall

- name: Use recall
  run: npx recall version
```

With caching:

```yaml
- name: Cache recall binary
  uses: actions/cache@v4
  with:
    path: node_modules/@hyperengineering/recall/vendor
    key: recall-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('package-lock.json') }}

- name: Install recall
  run: npm install @hyperengineering/recall
```

### GitLab CI

```yaml
install:
  script:
    - npm install @hyperengineering/recall
    - npx recall version
  cache:
    paths:
      - node_modules/
```

### Skip Download (Pre-installed Binary)

If you have recall installed via Homebrew or another method:

```yaml
- name: Install recall via Homebrew
  run: brew install hyperengineering/tap/recall

- name: Install npm package (skip download)
  run: RECALL_SKIP_DOWNLOAD=1 npm install @hyperengineering/recall
  env:
    RECALL_BINARY_PATH: /usr/local/bin/recall
```

## Troubleshooting

### Binary not found after installation

The binary may have failed to download. Try:

1. Run `npm rebuild @hyperengineering/recall` to re-download
2. Check for network issues (firewalls, proxies)
3. Install manually using one of the alternatives below

### Download fails in restricted environments

Some environments block downloads during npm install. Solutions:

1. Pre-download the binary and use `RECALL_BINARY_PATH`
2. Use the Homebrew tap: `brew install hyperengineering/tap/recall`
3. Download from [GitHub Releases](https://github.com/hyperengineering/recall/releases)

### Permission denied on Unix

The binary should have executable permissions set automatically. If not:

```bash
chmod +x node_modules/@hyperengineering/recall/vendor/recall
```

## Alternative Installation Methods

If npm installation doesn't work for your use case:

### Homebrew (macOS/Linux)

```bash
brew install hyperengineering/tap/recall
```

### Direct Download

Download the binary for your platform from [GitHub Releases](https://github.com/hyperengineering/recall/releases).

### Go Install

```bash
go install github.com/hyperengineering/recall/cmd/recall@latest
```

## License

MIT

## Links

- [GitHub Repository](https://github.com/hyperengineering/recall)
- [Documentation](https://github.com/hyperengineering/recall#readme)
- [Issues](https://github.com/hyperengineering/recall/issues)
