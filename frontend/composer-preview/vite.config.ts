import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

export default defineConfig({
	root: path.resolve(__dirname),
	publicDir: path.resolve(__dirname, '../public'),
	base: './',
	plugins: [react(), tailwindcss()],
	resolve: { alias: { '@': path.resolve(__dirname, '../src') } },
	build: { outDir: '/out', emptyOutDir: true },
})
