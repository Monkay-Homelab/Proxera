<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { navRefresh } from '$lib/navRefresh';
	import { toastError } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import type { HostConfig, DnsProvider, Agent } from '$lib/types';

	let parentDomain = '';
	let configs: HostConfig[] = [];
	let loading = true;
	let error = '';

	// Agent state
	let agents: Agent[] = [];
	let agentsLoaded = false;

	const providerId = $page.params.id;

	onMount(async () => {
		try {
			const res = await api('/api/dns/providers');
			if (res.ok) {
				const providers = await res.json();
				const match = providers.find((p: DnsProvider) => p.id === parseInt(providerId ?? ''));
				if (match) {
					parentDomain = match.domain;
				}
			}
		} catch (err) {
			toastError('Failed to load provider info');
		}

		await fetchConfigs();
		await fetchAgents();
	});

	async function fetchAgents() {
		if (agentsLoaded) return;

		try {
			const res = await api('/api/agents');
			if (res.ok) {
				agents = await res.json();
			}
		} catch (err) {
			toastError('Failed to load agents');
		}
		agentsLoaded = true;
	}

	async function fetchConfigs() {
		error = '';

		try {
			const res = await api(`/api/hosts/${providerId}/configs`);
			if (res.ok) {
				configs = (await res.json()).sort((a: HostConfig, b: HostConfig) => {
					if (a.domain === parentDomain) return -1;
					if (b.domain === parentDomain) return 1;
					return a.domain.localeCompare(b.domain);
				});
			} else {
				const data = await res.json();
				error = data.message || 'Failed to fetch host configs';
			}
		} catch (err) {
			error = 'Failed to connect to API';
		} finally {
			loading = false;
		}
	}

	function hasAdvancedConfig(config: HostConfig) {
		return config.config && Object.keys(config.config).length > 0;
	}

	// Sorting
	let sortKey = 'domain';
	let sortDir = 1; // 1 = asc, -1 = desc

	function toggleSort(key: string) {
		if (sortKey === key) {
			sortDir = -sortDir;
		} else {
			sortKey = key;
			sortDir = 1;
		}
	}

	function getAgentName(config: HostConfig) {
		if (!config.agent_id) return '';
		const agent = agents.find(a => String(a.id) === String(config.agent_id));
		return agent ? agent.name : 'Unknown';
	}

	$: sortedConfigs = [...configs].sort((a, b) => {
		let va, vb;
		switch (sortKey) {
			case 'domain':
				va = a.domain; vb = b.domain;
				return va.localeCompare(vb) * sortDir;
			case 'upstream_url':
				va = a.upstream_url; vb = b.upstream_url;
				return va.localeCompare(vb) * sortDir;
			case 'ssl':
				va = a.ssl ? 1 : 0; vb = b.ssl ? 1 : 0;
				return (va - vb) * sortDir;
			case 'websocket':
				va = a.websocket ? 1 : 0; vb = b.websocket ? 1 : 0;
				return (va - vb) * sortDir;
			case 'agent':
				va = getAgentName(a); vb = getAgentName(b);
				return va.localeCompare(vb) * sortDir;
			default:
				return 0;
		}
	});

	async function deleteConfig(config: HostConfig) {
		if (!await confirmDialog(`Delete host config "${config.domain}"?`, { title: 'Delete Host Config', confirmLabel: 'Delete', danger: true })) return;

		try {
			const res = await api(`/api/hosts/${providerId}/configs/${config.id}`, { method: 'DELETE' });
			if (res.ok) {
				configs = configs.filter(c => c.id !== config.id);
				navRefresh.update(n => n + 1);
			} else {
				const data = await res.json();
				error = data.message || 'Failed to delete host config';
			}
		} catch (err) {
			error = 'Failed to delete host config';
		}
	}
</script>

<svelte:head>
	<title>{parentDomain ? `Hosts - ${parentDomain}` : 'Host Configs'} - Proxera</title>
</svelte:head>

<div class="page">
	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading host configs...</p></div>
	{:else}
		<header class="page-head">
			<div class="head-left">
				<button class="breadcrumb" onclick={() => goto('/hosts')}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
					Hosts
				</button>
				<h1>{parentDomain || 'Host Configs'}</h1>
			</div>
			<button class="btn-fill" onclick={() => goto(`/hosts/${providerId}/edit/new`)}>
				<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
				Add Host
			</button>
		</header>

		{#if error}
			<div class="placeholder error" aria-live="assertive"><p>{error}</p></div>
		{/if}

		{#if configs.length > 0}
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th class="th-sort" onclick={() => toggleSort('domain')}>
								Domain <span class="sort-icon">{sortKey === 'domain' ? (sortDir === 1 ? '▲' : '▼') : '⇅'}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('upstream_url')}>
								Upstream URL <span class="sort-icon">{sortKey === 'upstream_url' ? (sortDir === 1 ? '▲' : '▼') : '⇅'}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('ssl')}>
								SSL <span class="sort-icon">{sortKey === 'ssl' ? (sortDir === 1 ? '▲' : '▼') : '⇅'}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('websocket')}>
								WebSocket <span class="sort-icon">{sortKey === 'websocket' ? (sortDir === 1 ? '▲' : '▼') : '⇅'}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('agent')}>
								Agent <span class="sort-icon">{sortKey === 'agent' ? (sortDir === 1 ? '▲' : '▼') : '⇅'}</span>
							</th>
							<th class="th-actions"></th>
						</tr>
					</thead>
					<tbody>
						{#each sortedConfigs as config}
							<tr>
								<td class="domain-cell">
									<a href="https://{config.domain}" target="_blank" rel="noopener noreferrer" class="domain-link">
										{config.domain}
										<svg class="external-icon" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
									</a>
									{#if hasAdvancedConfig(config)}
										<span class="config-badge" title="Advanced config">&#9881;</span>
									{/if}
								</td>
								<td class="mono">
									<a href="http://{config.upstream_url}" target="_blank" rel="noopener noreferrer" class="upstream-link">
										{config.upstream_url}
										<svg class="external-icon" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
									</a>
								</td>
								<td>
									<span class="status-pill" class:active={config.ssl}>
										{config.ssl ? 'On' : 'Off'}
									</span>
								</td>
								<td>
									<span class="status-pill" class:active={config.websocket}>
										{config.websocket ? 'On' : 'Off'}
									</span>
								</td>
								<td>
									{#if config.agent_id}
										{@const agent = agents.find(a => String(a.id) === String(config.agent_id))}
										{#if agent}
											<span class="agent-pill" class:agent-online={agent.status === 'online'}>
												{agent.name}
											</span>
										{:else}
											<span class="agent-pill">Unknown</span>
										{/if}
									{:else}
										<span class="dim">Unassigned</span>
									{/if}
								</td>
								<td class="td-actions">
									<button class="act act-accent" title="Edit" aria-label="Edit host" onclick={() => goto(`/hosts/${providerId}/edit/${config.id}`)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
											<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
										</svg>
									</button>
									<button class="act act-danger" title="Delete" aria-label="Delete host" onclick={() => deleteConfig(config)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<polyline points="3 6 5 6 21 6"></polyline>
											<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
										</svg>
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if !error}
			<div class="placeholder">
				<p>No host configs yet. Click "Add Host" to create your first reverse proxy configuration.</p>
			</div>
		{/if}
	{/if}
</div>

<style>
	/* Page-specific — shared host table styles in global.css */
	.page-head { align-items: flex-start; gap: 1rem; }
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }
	.th-actions { width: 70px; }
	.config-badge { margin-left: 0.375rem; font-size: var(--text-xs); color: var(--text-tertiary); }
	.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }
	.td-actions { white-space: nowrap; text-align: right; }
	.act { margin-left: 0.25rem; }
	@media (max-width: 768px) {
		.btn-fill { width: 100%; justify-content: center; }
	}
</style>
