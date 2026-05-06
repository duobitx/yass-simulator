# Unit tests

Vitest runs plain TypeScript tests against `src/` modules using the `@/` path alias (see `vitest.config.ts`).

## Commands

From the `eas/eodata` package root:

| Command | Description |
|--------|-------------|
| `npm test` | Run all tests once (`vitest run`) |
| `npm run test:watch` | Watch mode |

## Layout

- **`setup.ts`** – jsdom setup (e.g. `matchMedia`, `@testing-library/jest-dom`).
- **`lib/`** – tests grouped to mirror `src/lib/` (e.g. `utils.test.ts` covers `@/lib/utils`).

Add new files as `unit-tests/**/*.test.ts` (or `*.spec.ts` / `.tsx`).

## Configuration

- Root: `vitest.config.ts` (includes `unit-tests/**/*.{test,spec}.{ts,tsx}`, `setupFiles`, `@` → `./src`).
