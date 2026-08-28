import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { readFileSync } from 'fs'
import path from 'path'

/** Default route in /proc/net/route → Docker bridge gateway (host-network backend). */
function readDockerBridgeGateway(): string | undefined {
	try {
		const line = readFileSync('/proc/net/route', 'utf8')
			.split('\n')
			.find((l) => l.includes('00000000'))
		const hex = line?.trim().split(/\s+/)[2]
		if (!hex || hex.length !== 8) return undefined
		return Array.from({ length: 4 }, (_, i) =>
			parseInt(hex.slice((3 - i) * 2, (3 - i) * 2 + 2), 16),
		).join('.')
	} catch {
		return undefined
	}
}

const dockerGateway = readDockerBridgeGateway()
const proxyTarget =
	process.env.API_PROXY_TARGET ??
	(dockerGateway
		? `http://${dockerGateway}:8080`
		: 'http://localhost:8080')

export default defineConfig({
	plugins: [
		react(),
		tailwindcss(),
	],
	resolve: {
		alias: {
			'@': path.resolve(__dirname, './src'),
		},
	},
	server: {
		port: 5173,
		proxy: {
			'/api': {
				target: proxyTarget,
				changeOrigin: true,
				configure: (proxy) => {
					console.log(`[vite proxy] /api -> ${proxyTarget}`)
					proxy.on('error', (err, _req, res) => {
						console.error('[vite proxy] error:', err.message)
						if (!res.headersSent) {
							res.writeHead(502, { 'Content-Type': 'application/json' })
						}
						res.end(JSON.stringify({ error: `proxy error: ${err.message}` }))
					})
				},
			},
		},
	},
})
