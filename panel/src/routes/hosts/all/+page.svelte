<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { toastError } from '$lib/components/toast';
	import type { HostConfig, Agent, DnsProvider } from '$lib/types';

	let hosts: HostConfig[] = [];
	let agents: Agent[] = [];
	let providers: DnsProvider[] = [];
	let loading = true;
	let error = '';

	// Sorting
	let sortKey = 'domain';
	let sortDir = 1;

	// Search
	let search = '';

	onMount(async () => {
		await Promise.all([fetchHosts(), fetchAgents(), fetchProviders()]);
		loading = false;
	});

	async function fetchHosts() {
		try {
			const res = await api('/api/hosts/all');
			if (res.ok) {
				hosts = await res.json();
			} else {
				error = 'Failed to fetch hosts';
			}
		} catch (err) {
			error = 'Failed to connect to API';
		}
	}

	async function fetchAgents() {
		try {
			const res = await api('/api/agents');
			if (res.ok) agents = await res.json();
		} catch { toastError('Failed to load agents'); }
	}

	async function fetchProviders() {
		try {
			const res = await api('/api/dns/providers');
			if (res.ok) providers = await res.json();
		} catch { toastError('Failed to load providers'); }
	}

	function getAgentName(host: HostConfig) {
		if (!host.agent_id) return '';
		const agent = agents.find(a => a.id === Number(host.agent_id));
		return agent ? agent.name : 'Unknown';
	}

	function getAgentStatus(host: HostConfig) {
		if (!host.agent_id) return '';
		const agent = agents.find(a => a.id === Number(host.agent_id));
		return agent ? agent.status : '';
	}

	function getProviderDomain(host: HostConfig) {
		const p = providers.find(prov => prov.id === host.provider_id);
		return p ? p.domain : '';
	}

	function hasAdvancedConfig(host: HostConfig) {
		return host.config && Object.keys(host.config).length > 0;
	}

	function toggleSort(key: string) {
		if (sortKey === key) {
			sortDir = -sortDir;
		} else {
			sortKey = key;
			sortDir = 1;
		}
	}

	$: filtered = hosts.filter(h => {
		if (!search) return true;
		const q = search.toLowerCase();
		return h.domain.toLowerCase().includes(q) ||
			h.upstream_url.toLowerCase().includes(q) ||
			getAgentName(h).toLowerCase().includes(q) ||
			getProviderDomain(h).toLowerCase().includes(q);
	});

	$: sorted = [...filtered].sort((a: HostConfig, b: HostConfig) => {
		let va: string | number, vb: string | number;
		switch (sortKey) {
			case 'domain':
				va = a.domain; vb = b.domain;
				return va.localeCompare(vb) * sortDir;
			case 'upstream_url':
				va = a.upstream_url; vb = b.upstream_url;
				return va.localeCompare(vb) * sortDir;
			case 'provider':
				va = getProviderDomain(a); vb = getProviderDomain(b);
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

	function sortIcon(key: string) {
		if (sortKey === key) return sortDir === 1 ? '\u25B2' : '\u25BC';
		return '\u21C5';
	}
</script>

<svelte:head>
	<title>All Hosts - Proxera</title>
</svelte:head>

<div class="page">
	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading all hosts...</p></div>
	{:else}
		<header class="page-head">
			<div class="head-left">
				<button class="breadcrumb" onclick={() => goto('/hosts')}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
					Hosts
				</button>
				<h1>All Hosts <span class="count">{filtered.length}</span></h1>
			</div>
			<div class="head-right">
				<div class="search-box">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
					<input type="text" bind:value={search} placeholder="Filter hosts..." />
				</div>
			</div>
		</header>

		{#if error}
			<div class="placeholder error"><p>{error}</p></div>
		{/if}

		{#if sorted.length > 0}
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th class="th-sort" onclick={() => toggleSort('domain')}>
								Domain <span class="sort-icon">{sortIcon('domain')}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('upstream_url')}>
								Upstream <span class="sort-icon">{sortIcon('upstream_url')}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('provider')}>
								Provider <span class="sort-icon">{sortIcon('provider')}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('ssl')}>
								SSL <span class="sort-icon">{sortIcon('ssl')}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('websocket')}>
								WS <span class="sort-icon">{sortIcon('websocket')}</span>
							</th>
							<th class="th-sort" onclick={() => toggleSort('agent')}>
								Agent <span class="sort-icon">{sortIcon('agent')}</span>
							</th>
							<th class="th-actions"></th>
						</tr>
					</thead>
					<tbody>
						{#each sorted as host}
							<tr>
								<td class="domain-cell">
									<a href="https://{host.domain}" target="_blank" rel="noopener noreferrer" class="domain-link">
										{host.domain}
										<svg class="external-icon" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
									</a>
									{#if hasAdvancedConfig(host)}
										<span class="config-badge" title="Advanced config">&#9881;</span>
									{/if}
								</td>
								<td class="mono">
									<a href="http://{host.upstream_url}" target="_blank" rel="noopener noreferrer" class="upstream-link">
										{host.upstream_url}
										<svg class="external-icon" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
									</a>
								</td>
								<td>
									<button class="provider-link" onclick={() => goto(`/hosts/${host.provider_id}`)}>
										{getProviderDomain(host) || '—'}
									</button>
								</td>
								<td>
									<span class="status-pill" class:active={host.ssl}>
										{host.ssl ? 'On' : 'Off'}
									</span>
								</td>
								<td>
									<span class="status-pill" class:active={host.websocket}>
										{host.websocket ? 'On' : 'Off'}
									</span>
								</td>
								<td>
									{#if host.agent_id}
										<span class="agent-pill" class:agent-online={getAgentStatus(host) === 'online'}>
											{getAgentName(host)}
										</span>
									{:else}
										<span class="dim">—</span>
									{/if}
								</td>
								<td class="td-actions">
									<button class="act act-accent" title="Edit" onclick={() => goto(`/hosts/${host.provider_id}/edit/${host.id}`)}>
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
											<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
											<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
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
				<p>{search ? 'No hosts match your filter.' : 'No hosts configured yet.'}</p>
			</div>
		{/if}
	{/if}
</div>

<style>
	/* Page-specific — shared host table styles in global.css */
	.page-head { align-items: flex-start; gap: 1rem; }
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }
	.head-right { display: flex; align-items: center; gap: 0.75rem; }
	.count {
		font-size: var(--text-sm); font-weight: 500;
		color: var(--text-tertiary); margin-left: 0.25rem;
	}
	.search-box {
		display: flex; align-items: center; gap: 0.5rem;
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 0.4rem 0.75rem;
		transition: border-color var(--transition);
	}
	.search-box:focus-within { border-color: var(--accent); }
	.search-box svg { color: var(--text-tertiary); flex-shrink: 0; }
	.search-box input {
		background: none; border: none; outline: none;
		font-size: var(--text-sm); color: var(--text-primary);
		width: 180px;
	}
	.search-box input::placeholder { color: var(--text-tertiary); }
	.th-actions { width: 50px; }
	.config-badge { margin-left: 0.375rem; font-size: var(--text-xs); color: var(--text-tertiary); }
	.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }
	.provider-link {
		background: none; border: none; padding: 0; cursor: pointer;
		color: var(--accent); font-size: var(--text-xs); font-weight: 500;
		font-family: var(--font-mono);
		transition: color var(--transition);
	}
	.provider-link:hover { color: var(--accent-bright); }
	.td-actions { white-space: nowrap; text-align: right; }
	@media (max-width: 768px) {
		.search-box input { width: 100%; }
	}
</style>
