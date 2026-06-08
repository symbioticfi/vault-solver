# AGENTS.md

The engineering guidelines and project conventions for this repository live in **[CLAUDE.md](./CLAUDE.md)**.

Any agent or contributor working in this repo must read and follow `CLAUDE.md` before making changes.
It covers the project's purpose, the modular framework/integration boundary (3F today; RFQ, Redstone,
and others next), config-file-driven configuration, modern Go 1.26 style, the required
test/lint/format gate, and secure-coding rules.

In short: keep integrations modular and self-contained, drive everything from the config file, write
unit tests for new logic, and ensure `make format && make test && make lint` is green before finishing.
