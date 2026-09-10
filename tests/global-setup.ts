import { chromium, type FullConfig } from '@playwright/test'

/**
 * Warms the dev server before the workers start.
 *
 * Vite transforms each lazy route's chunk on first request, and that cost is
 * paid per route, not per worker -- but a cold parallel run has every worker
 * queueing behind the same single-threaded server, which is what turns an 8s
 * spec file into a wall of timeouts. Visiting every route once, sequentially,
 * moves that work out of the tests.
 *
 * Only GETs happen here: no route is a write, and the API is left alone.
 */
const ROUTES = [
	'/workplace',
	'/incidents',
	'/knowledge',
	'/plans',
	'/rules',
	'/tools',
	'/skills',
	'/variables',
	'/system-tools',
]

export default async function globalSetup(config: FullConfig) {
	const baseURL =
		config.projects[0]?.use?.baseURL ??
		process.env.NIB_E2E_BASE_URL ??
		'http://localhost:5173'

	const browser = await chromium.launch()
	const page = await browser.newPage()

	// Block every write, exactly as the per-test guard does: warming must not
	// touch the backend.
	await page.route(
		(url) => url.pathname.startsWith('/api/'),
		(route) =>
			route.request().method() === 'GET'
				? route.continue()
				: route.abort('blockedbyclient'),
	)

	const started = Date.now()
	for (const route of ROUTES) {
		try {
			await page.goto(`${baseURL}${route}`, {
				waitUntil: 'networkidle',
				timeout: 60_000,
			})
		} catch {
			// A route that will not warm is the tests' problem to report, not this
			// script's: it must never block the run.
		}
	}
	// One plan detail load, for the heaviest chunk in the app.
	try {
		await page.goto(`${baseURL}/workplace/warmup`, {
			waitUntil: 'networkidle',
			timeout: 60_000,
		})
	} catch {
		// ignored, as above
	}

	console.log(`warmed ${ROUTES.length + 1} routes in ${Date.now() - started}ms`)
	await browser.close()
}
