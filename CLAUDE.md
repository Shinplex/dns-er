# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands
- Build: `go build` 
- Run: `sudo ./dns-er` or `sudo ./dns-er -config=/path/to/config.toml`
- Test DNS queries: `python3 scripts/test.py`

## Code Style Guidelines
- **Imports**: Standard Go grouping (stdlib first, then external)
- **Formatting**: Use `gofmt -s -w .` to format code
- **Error Handling**: Always check errors and provide context in error messages
- **Naming**: CamelCase for exported variables, functions and types; camelCase for unexported
- **Comments**: Document all exported functions, types, and non-obvious code
- **Constants**: Use constants over magic numbers
- **Types**: Prefer strong typing with clear interfaces
- **Concurrency**: Use mutexes for shared state, channels for communication
- **Logging**: Use the standard `log` package with appropriate levels (Fatalf, Printf)
- **Config**: Use TOML for configuration with clear structure matching Go types