import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const extraHosts = process.env.VITE_ALLOWED_HOSTS
	? process.env.VITE_ALLOWED_HOSTS.split(',').map(h => h.trim())
	: [];

const panelHost = process.env.VITE_PANEL_HOST || '';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		host: '0.0.0.0',
		port: 5173,
		allowedHosts: [
			'localhost',
			'127.0.0.1',
			...(panelHost ? [panelHost] : []),
			...extraHosts
		],
		headers: {
			'Cache-Control': 'no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0',
			'CDN-Cache-Control': 'no-store',
			'Surrogate-Control': 'no-store'
		},
		...(panelHost ? {
			hmr: {
				protocol: 'wss',
				host: panelHost,
				clientPort: 443
			}
		} : {})
	}
});
