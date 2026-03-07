<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

	let agents = [];
	let loading = true;
	let error = null;

	onMount(async () => {
		await fetchAgents();
	});

	async function fetchAgents() {
		try {
			const response = await api('/api/agents');
			if (!response.ok) {
				throw new Error('Failed to fetch agents');
			}

			agents = await response.json();
			loading = false;
		} catch (err) {
			error = err.message;
			loading = false;
		}
	}

</script>

<svelte:head>
	<title>CrowdSec - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<h1>CrowdSec IPS/IDS</h1>
	</header>

	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading agents...</p></div>
	{:else if error}
		<div class="placeholder error"><p>{error}</p></div>
	{:else if agents.length === 0}
		<div class="placeholder">
			<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
				<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
			</svg>
			<h2>No agents registered</h2>
			<p>Register an agent first, then come back to manage CrowdSec.</p>
			<button class="btn-fill" onclick={() => goto('/agents')}>Go to Agents</button>
		</div>
	{:else}
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Agent</th>
						<th>Status</th>
						<th>CrowdSec</th>
						<th>WAN IP</th>
						<th>Last Seen</th>
						<th>Action</th>
					</tr>
				</thead>
				<tbody>
					{#each agents as agent}
						<tr class="clickable-row" onclick={() => goto(`/crowdsec/${agent.agent_id}`)}>
							<td>
								<div class="agent-name-cell">
									<span class="dot" class:dot-on={agent.status === 'online'} class:dot-err={agent.status === 'error'}></span>
									<span class="agent-name">{agent.name}</span>
								</div>
							</td>
							<td>
								<span class="badge" class:badge-on={agent.status === 'online'} class:badge-off={agent.status !== 'online'}>
									{agent.status}
								</span>
							</td>
							<td>
								{#if agent.crowdsec_installed}
									<span class="cs-pill installed">Installed</span>
								{:else}
									<span class="cs-pill">Not installed</span>
								{/if}
							</td>
							<td class="mono">{agent.wan_ip || '—'}</td>
							<td class="dim">{formatRelativeTime(agent.last_seen)}</td>
							<td>
								<button
									class="btn-fill btn-sm"
									onclick={(e) => { e.stopPropagation(); goto(`/crowdsec/${agent.agent_id}`); }}
									disabled={agent.status !== 'online'}
								>
									{agent.crowdsec_installed ? 'Manage' : 'Install'}
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	/* ── CrowdSec-specific buttons ── */
	.btn-sm { padding: 0.35rem 0.875rem; font-size: var(--text-xs); }

	/* ── Table rows ── */
	.clickable-row { cursor: pointer; transition: background var(--transition); }
	.clickable-row:hover { background: var(--surface-raised); }

	.agent-name-cell { display: flex; align-items: center; gap: 0.625rem; }
	.agent-name { font-weight: 600; color: var(--text-primary); }

	.dot {
		width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0;
		background: var(--text-muted);
	}
	.dot-on { background: var(--success); }
	.dot-err { background: var(--danger); }

	.badge {
		font-size: 0.675rem; font-weight: 600; text-transform: capitalize;
		padding: 0.125rem 0.5rem; border-radius: 20px;
	}
	.badge-on { background: var(--success); color: #fff; }
	.badge-off { background: var(--text-muted); color: var(--surface); }

	.cs-pill {
		display: inline-block; padding: 0.125rem 0.5rem;
		border-radius: var(--radius); font-size: var(--text-xs); font-weight: 600;
		background: var(--surface-raised); color: var(--text-secondary);
	}
	.cs-pill.installed { background: var(--info-dim); color: var(--info); }

	.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }
</style>
