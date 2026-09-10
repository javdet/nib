import { defineConfig, devices } from '@playwright/test'

// Layout (Playwright's agent convention -- the planner/generator/healer agents in
// .claude/agents assume it, and `planner_save_plan` has no path parameter):
//
//   specs/            test plans, written by the planner agent
//   tests/            generated specs; tests/seed.spec.ts sets up the starting page
//
// It lives at the repo root rather than under frontend/ for two reasons: vitest is
// rooted at frontend/ with no `include`, so a *.spec.ts there would be swept into
// `bun run test`; and MCP servers get no `cwd`, so `npx playwright run-test-mcp-server`
// has to resolve the CLI from the project root.
//
// The dev stack is started by hand:
//
//   docker compose -f docker-compose.dev.yml up
//
// There is deliberately no `webServer` block. A local `vite build` is broken in this
// repo (the frontend only builds inside `oven/bun:1`), so Playwright must not try to
// own the frontend's lifecycle -- it attaches to the already-running dev server on 5173.
//
// Beware: this suite drives the real backend. Any test that reaches Execute starts an
// actual sub-agent run with real LLM calls. Mock /api/ writes with `page.route` (or
// `browser_route` from the MCP side) rather than letting them through.
export default defineConfig({
	testDir: './tests',
	// Transforms every lazy route once before the workers start; see the file.
	globalSetup: './tests/global-setup.ts',
	fullyParallel: true,
	// The dev server transforms a lazy route's chunk on first request, and every
	// worker pays that cost separately, so a cold parallel run is much slower
	// than a warm one. The defaults (5s per assertion, 30s per test) are not
	// enough for the first navigation of each worker.
	expect: { timeout: 15_000 },
	timeout: 60_000,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	// Serial, locally as well as in CI. One single-threaded Vite dev server backs
	// every worker and transforms each lazy route's chunk on first request, so
	// parallel workers queue behind each other: specs that pass in 8s serially
	// time out wholesale at the default worker count, and the failures read as
	// application defects (blank chat panel, missing headings) rather than as
	// contention. Serial is both faster end to end here and deterministic.
	// Against a production build (static assets, no transform) this cap is
	// unnecessary -- raise it there.
	workers: 1,
	reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
	use: {
		baseURL: process.env.NIB_E2E_BASE_URL ?? 'http://localhost:5173',
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
	},
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] },
		},
	],
})
