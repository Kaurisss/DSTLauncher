# Repository Guidelines

## Project Structure & Module Organization
- Go backend (`module dst-windows`, Go 1.25+): root entrypoint (`main.go`) and domain packages in `internal/` (`web`, `room`, `server`, `backup`, `backupremote`, `migration`, `modmgr`, `steamcmd`, `worldsettings`).
- Verification harnesses live in `cmd/fakeserver` and `cmd/verify`; build and asset scripts live in `tools/`.
- Vue 3 frontend lives in `web/` (`src/views/`, `src/components/`, `src/components/ui/`, `src/stores/`, `src/composables/`, `src/lib/`, static assets in `public/`, check scripts in `scripts/`).
- `web/dist/` is embedded via `go:embed`; always rebuild the frontend before compiling Go. Feature specs live in `docs/`.

## Build, Test, and Development Commands
- `powershell -NoProfile -ExecutionPolicy Bypass -File .\build.ps1`: builds `web/dist` with `pnpm` and outputs `dst-windows.exe`.
- `pnpm --dir web run dev`: starts Vite on `:5173`, proxying `/api` to a running backend on `127.0.0.1:16800`.
- `pnpm --dir web run build`: compiles frontend assets into `web/dist`.
- `pnpm --dir web run lint`: runs `scripts/check-src.mjs` (validates `@/` imports and single-root views) followed by `eslint .`.
- `go test ./...`: runs all Go package tests.
- `pnpm --dir web test`: runs all `web/scripts/check-*.mjs` scripts; run a single check with `node web/scripts/check-shell.mjs`.

## Coding Style & Naming Conventions
- Go: format with `gofmt` (tabs), isolate OS-specific code in `*_windows.go` / `*_linux.go`, and colocate tests as `*_test.go`.
- Frontend: ES modules, 2-space indentation, single quotes, semicolons, and `@/` imports (e.g., `@/lib/api.js`) instead of deep relative paths.
- Naming: PascalCase views/components (`OverviewView.vue`, `RoomCard.vue`), camelCase composables/stores (`useLogs.js`, `rooms.js`). Prefix intentionally unused JS bindings with `_`.
- Keep exactly one root element in every `web/src/views/*.vue` file so `<Transition>` switches render cleanly.

## Testing Guidelines
- Use Go's `testing` package (`go test ./internal/<pkg> -v`); on Windows, `go run ./cmd/verify` validates server process lifecycles.
- Run `pnpm --dir web test` for pure-Node frontend checks in `web/scripts/check-*.mjs`, and use Chrome DevTools MCP for live UI verification.
- Run `tools/test_*.py` via `python -m unittest` when editing Python extractors.

## Commit & Pull Request Guidelines
- Follow Conventional Commits used in Git history: `<type>(<scope>): <summary>` (e.g., `feat: ...`, `fix(room): ...`, `refactor(web): ...`, `style(web): ...`).
- PRs should explain user-facing changes, reference related issues or `docs/`, list executed checks, and include screenshots for UI changes.

## Security & Configuration Tips
- Never commit runtime data or secrets (`panel.json`, `.env`, `*.token`, `dst_save/`, `logs/`, `steamcmd/`, `ugc_mods/`). Use `.env.example` for Docker configuration templates.
