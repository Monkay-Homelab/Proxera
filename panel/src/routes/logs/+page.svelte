<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

	let agents = [];
	let loading = true;
	let error = null;

	onMount(async () => {
		try {
			const resp = await api('/api/agents');
			if (!resp.ok) throw new Error('Failed to fetch agents');
			agents = await resp.json();
		} catch (err) { error = err.message; }
		finally { loading = false; }
	});


	function isOnline(agent) {
		if (!agent.last_seen) return false;
		return (Date.now() - new Date(agent.last_seen).getTime()) < 90000;
	}
</script>

<svelte:head><title>Logs - Proxera</title></svelte:head>

<div class="page">
	<header class="page-head">
		<h1>Logs</h1>
		<p class="subtitle">Select an agent to view its nginx access logs</p>
	</header>

	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading agents...</p></div>
	{:else if error}
		<div class="placeholder error"><p>{error}</p></div>
	{:else if agents.length === 0}
		<div class="placeholder"><p>No agents registered yet.</p></div>
	{:else}
		<div class="agent-grid">
			{#each agents as agent}
				<a href="/logs/{agent.agent_id}" class="agent-card" class:offline={!isOnline(agent)}>
					<div class="card-top">
						<span class="status-dot" class:online={isOnline(agent)}></span>
						<span class="agent-name">{agent.name}</span>
					</div>
					<div class="card-meta">
						<span>{agent.wan_ip || agent.ip_address || '-'}</span>
						<span>{agent.os || '-'}/{agent.arch || '-'}</span>
						<span>v{agent.version || '?'}</span>
					</div>
					<div class="card-bottom">
						<span class="last-seen">Last seen {formatRelativeTime(agent.last_seen)}</span>
						<svg class="arrow" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.subtitle { color: var(--text-tertiary); font-size: var(--text-sm); margin-top: 0.25rem; }

	.agent-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 0.75rem; }

	.agent-card {
		background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg);
		padding: 1.25rem; cursor: pointer; transition: all var(--transition); display: flex; flex-direction: column; gap: 0.75rem;
	}
	.agent-card:hover { border-color: var(--accent); background: var(--surface-raised); }
	.agent-card.offline { opacity: 0.5; }
	.agent-card.offline:hover { opacity: 0.8; }

	.card-top { display: flex; align-items: center; gap: 0.5rem; }
	.status-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--danger); flex-shrink: 0; }
	.status-dot.online { background: #42c990; }
	.agent-name { font-weight: 600; color: var(--text-primary); font-size: var(--text-base); }

	.card-meta { display: flex; gap: 0.75rem; font-size: var(--text-xs); color: var(--text-tertiary); }
	.card-meta span { background: var(--surface-raised); padding: 0.125rem 0.5rem; border-radius: 999px; }

	.card-bottom { display: flex; justify-content: space-between; align-items: center; }
	.last-seen { font-size: var(--text-xs); color: var(--text-muted); }
	.arrow { color: var(--text-muted); transition: transform var(--transition); }
	.agent-card:hover .arrow { transform: translateX(3px); color: var(--accent); }

	@media (max-width: 768px) { .agent-grid { grid-template-columns: 1fr; } }
</style>
