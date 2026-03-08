<script lang="ts">
	import './global.css';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { PUBLIC_API_URL } from '$env/static/public';
	import { getToken, clearToken, getUserRole, setUserRole, apiJson } from '$lib/api';
	import Toast from '$lib/components/Toast.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';

	let { children } = $props();

	const publicPaths = ['/login', '/register', '/verify-email', '/setup'];
	let authChecked = $state(false);

	let isPublicPage = $derived(publicPaths.some(p => $page.url.pathname.startsWith(p)));
	let isAdminPage = $derived($page.url.pathname.startsWith('/admin'));
	let showSidebar = $derived(!isPublicPage && !isAdminPage);
	let userRole = $state('');
	let isAdmin = $derived(userRole === 'admin');

	async function refreshRole() {
		try {
			const data = await apiJson<{ role: string }>('/api/user/me');
			if (data.role && data.role !== getUserRole()) {
				setUserRole(data.role);
				userRole = data.role;
			}
		} catch { /* ignore */ }
	}

	onMount(() => {
		if (!isPublicPage && !getToken()) {
			goto('/login');
			return;
		}
		userRole = getUserRole();
		authChecked = true;
		refreshRole();
	});

	$effect(() => {
		$page.url.pathname;
		if (!isPublicPage && getToken()) {
			userRole = getUserRole();
			if (!authChecked) authChecked = true;
		}
	});

	function logout() {
		clearToken();
		fetch(`${PUBLIC_API_URL}/api/auth/logout`, { method: 'POST' }).catch(() => {});
		goto('/login');
	}

	function isActive(path: string) {
		return $page.url.pathname === path;
	}

	function isActivePrefix(path: string) {
		return $page.url.pathname.startsWith(path);
	}
</script>

<svelte:head>
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>Proxera - Nginx Proxy Management</title>
</svelte:head>

<div class="shell">
	{#if showSidebar}
		<nav class="sidebar">
			<a href="/home" class="brand">
				<svg class="brand-icon" width="24" height="24" viewBox="0 0 32 32" fill="none">
					<path d="M16 2L28.66 9.5V24.5L16 32L3.34 24.5V9.5L16 2Z" stroke="currentColor" stroke-width="2.5" fill="none"/>
					<path d="M16 8L22.93 12V20L16 24L9.07 20V12L16 8Z" fill="currentColor" opacity="0.2"/>
					<circle cx="16" cy="16" r="3" fill="currentColor"/>
				</svg>
				<span class="brand-name">Proxera</span>
			</a>

			<div class="nav-links">
				<a href="/home" class="nav-link" class:active={isActive('/home')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>
					</svg>
					Dashboard
				</a>
				<a href="/agents" class="nav-link" class:active={isActive('/agents')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><circle cx="6" cy="6" r="1" fill="currentColor"/><circle cx="6" cy="18" r="1" fill="currentColor"/>
					</svg>
					Agents
				</a>
				<a href="/dns" class="nav-link" class:active={isActivePrefix('/dns')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
					</svg>
					DNS
				</a>
				<a href="/logs" class="nav-link" class:active={isActivePrefix('/logs')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/>
					</svg>
					Logs
				</a>
				<a href="/hosts" class="nav-link" class:active={isActivePrefix('/hosts')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>
					</svg>
					Hosts
				</a>
				<a href="/certificates" class="nav-link" class:active={isActive('/certificates')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M9 12l2 2 4-4" stroke-width="2"/>
					</svg>
					Certificates
				</a>
				<a href="/metrics" class="nav-link" class:active={isActive('/metrics')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<rect x="3" y="12" width="4" height="9"/><rect x="10" y="7" width="4" height="14"/><rect x="17" y="3" width="4" height="18"/>
					</svg>
					Metrics
				</a>
				<a href="/crowdsec" class="nav-link" class:active={isActivePrefix('/crowdsec')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M12 8v4M12 16h.01"/>
					</svg>
					CrowdSec
				</a>
				<a href="/alerts" class="nav-link" class:active={isActivePrefix('/alerts')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>
					</svg>
					Alerts
				</a>
			</div>

			<div class="nav-bottom">
				{#if authChecked && isAdmin}
					<a href="/admin" class="nav-link" class:active={isActivePrefix('/admin')}>
						<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
							<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>
						</svg>
						Admin
					</a>
				{/if}
				<a href="/profile" class="nav-link" class:active={isActive('/profile')}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>
					</svg>
					Profile
				</a>
				<button class="nav-link nav-logout" onclick={logout}>
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>
					</svg>
					Logout
				</button>
			</div>
		</nav>
	{/if}

	<main class="content" class:full-width={!showSidebar}>
		{#if isPublicPage || authChecked}
			{@render children()}
		{:else if !isPublicPage}
			<div class="auth-loading">
				<div class="auth-spinner"></div>
			</div>
		{/if}
	</main>
</div>

<Toast />
<ConfirmDialog />

<style>
	.shell {
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

	.nav-logout:hover {
		color: var(--danger);
		background: var(--danger-dim);
	}

	.nav-logout:hover svg {
		color: var(--danger);
	}

	.content {
		flex: 1;
		margin-left: 224px;
		overflow-y: auto;
		background: var(--bg);
	}

	.content.full-width { margin-left: 0; }

	.auth-loading {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100vh;
	}

	.auth-spinner {
		width: 32px;
		height: 32px;
		border: 2px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	@media (max-width: 768px) {
		.sidebar { width: 56px; overflow: hidden; }
		.brand-name { display: none; }
		.nav-link { justify-content: center; padding: 0.625rem; }
		.content { margin-left: 56px; }
	}
</style>
