<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { apiJson, getToken } from '$lib/api';

	let { children } = $props();
	let authorized = $state(false);

	onMount(async () => {
		if (!getToken()) {
			goto('/login');
			return;
		}
		try {
			const me: any = await apiJson('/api/user/me');
			if (me.role !== 'admin') {
				goto('/home');
				return;
			}
			authorized = true;
		} catch {
			goto('/home');
		}
	});

	function isActive(path: string) {
		return $page.url.pathname === path;
	}

	function isActivePrefix(path: string) {
		return $page.url.pathname === path || $page.url.pathname.startsWith(path + '/');
	}
</script>

{#if authorized}
	<div class="admin-shell">
		<nav class="sidebar">
			<a href="/admin/dashboard" class="brand">
				<svg class="brand-icon" width="24" height="24" viewBox="0 0 32 32" fill="none">
					<path d="M16 2L28.66 9.5V24.5L16 32L3.34 24.5V9.5L16 2Z" stroke="currentColor" stroke-width="2.5" fill="none"/>
					<path d="M16 8L22.93 12V20L16 24L9.07 20V12L16 8Z" fill="currentColor" opacity="0.2"/>
					<circle cx="16" cy="16" r="3" fill="currentColor"/>
				</svg>
				<span class="brand-name">Proxera</span>
				<span class="admin-badge">Admin</span>
			</a>

			<div class="nav-links">
				<a href="/admin/dashboard" class="nav-link" class:active={isActive('/admin/dashboard')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>
					</svg>
					Dashboard
				</a>
				<a href="/admin/users" class="nav-link" class:active={isActivePrefix('/admin/users')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>
					</svg>
					Users
				</a>
				<a href="/admin/agents" class="nav-link" class:active={isActivePrefix('/admin/agents')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>
					</svg>
					Agents
				</a>
				<a href="/admin/hosts" class="nav-link" class:active={isActive('/admin/hosts')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
					</svg>
					Hosts
				</a>
				<a href="/admin/certificates" class="nav-link" class:active={isActive('/admin/certificates')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
					</svg>
					Certificates
				</a>
				<a href="/admin/metrics" class="nav-link" class:active={isActive('/admin/metrics')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>
					</svg>
					Metrics
				</a>
				<a href="/admin/alerts" class="nav-link" class:active={isActive('/admin/alerts')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>
					</svg>
					Alerts
				</a>
				<a href="/admin/settings" class="nav-link" class:active={isActive('/admin/settings')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<circle cx="12" cy="12" r="3"/><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/>
					</svg>
					Settings
				</a>
			</div>

			<div class="nav-bottom">
				<a href="/home" class="nav-link">
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>
					</svg>
					Back to Panel
				</a>
			</div>
		</nav>

		<main class="content">
			{@render children()}
		</main>
	</div>
{/if}

<style>
	.admin-shell {
		display: flex;
		height: 100vh;
	}

	.sidebar {
		width: 224px;
		background: var(--surface);
		border-right: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		position: fixed;
		height: 100vh;
		left: 0;
		top: 0;
		z-index: 100;
		padding: 0 0.5rem;
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem 0.625rem;
		margin-bottom: 0.25rem;
		border-bottom: 1px solid var(--border);
	}

	.brand-icon { color: var(--accent); }

	.brand-name {
		font-size: var(--text-lg);
		font-weight: 600;
		color: var(--text-primary);
		letter-spacing: -0.01em;
	}

	.admin-badge {
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--accent);
		background: var(--accent-dim);
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
	}

	.nav-links {
		padding: 0.5rem 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.nav-bottom {
		margin-top: auto;
		padding: 0.5rem 0;
		border-top: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.nav-link {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 450;
		border: none;
		background: none;
		width: 100%;
		cursor: pointer;
		transition: all var(--transition);
	}

	.nav-link:hover {
		color: var(--text-primary);
		background: var(--surface-raised);
	}

	.nav-link.active {
		color: var(--accent);
		background: var(--accent-dim);
	}

	.nav-link svg {
		flex-shrink: 0;
		opacity: 0.5;
	}

	.nav-link:hover svg,
	.nav-link.active svg {
		opacity: 1;
	}

	.nav-link.active svg {
		color: var(--accent);
	}

	.content {
		flex: 1;
		margin-left: 224px;
		overflow-y: auto;
		background: var(--bg);
	}

	@media (max-width: 768px) {
		.sidebar { width: 56px; }
		.brand-name, .admin-badge { display: none; }
		.nav-link { justify-content: center; padding: 0.625rem; }
		.content { margin-left: 56px; }
	}
</style>
