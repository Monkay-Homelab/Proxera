<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { toastError } from '$lib/components/toast';
	import type { User, Agent } from '$lib/types';

	let user: User | null = null;
	let agents: Agent[] = [];
	let loading = true;

	onMount(async () => {
		try {
			const userResponse = await api('/api/user/me');
			if (!userResponse.ok) throw new Error('Not authenticated');
			user = await userResponse.json();

			const agentsResponse = await api('/api/agents');
			if (agentsResponse.ok) agents = await agentsResponse.json();
		} catch (err) {
			toastError('Failed to load dashboard data');
		} finally {
			loading = false;
		}
	});

	function getOnlineCount(agents: Agent[]) {
		return agents.filter(a => a.status === 'online').length;
	}

	function getTotalHosts(agents: Agent[]) {
		return agents.reduce((sum, a) => sum + (a.host_count || 0), 0);
	}
</script>

<svelte:head>
	<title>Dashboard - Proxera</title>
</svelte:head>

{#if loading}
	<div class="loading-state" aria-live="polite">
		<div class="loader"></div>
		<p>Loading...</p>
	</div>
{:else if user}
	<div class="page">
		<header class="page-head">
			<h1>Dashboard</h1>
		</header>

		<!-- KPI row -->
		<div class="kpi-row">
			<div class="kpi">
				<span class="kpi-num">{agents.length}</span>
				<span class="kpi-label">Agents</span>
			</div>
			<div class="kpi">
				<span class="kpi-num">{getOnlineCount(agents)}</span>
				<span class="kpi-label">Online</span>
			</div>
			<div class="kpi">
				<span class="kpi-num">{getTotalHosts(agents)}</span>
				<span class="kpi-label">Hosts</span>
			</div>
		</div>

		{#if agents.length === 0}
			<div class="empty-card">
				<h2>Get Started</h2>
				<p>You don't have any agents yet. Install your first agent to start managing your infrastructure.</p>
				<button class="btn-fill" onclick={() => goto('/agents')}>Add Agent</button>
			</div>
		{:else}
			<div class="panels">
				<!-- Agents -->
				<div class="panel">
					<div class="panel-head">
						<h2>Agents</h2>
						<a href="/agents" class="panel-link">View all</a>
					</div>
					<div class="panel-list">
						{#each agents as agent}
							<a href="/agents" class="list-row">
								<div class="row-left">
									<span class="status-dot" class:online={agent.status === 'online'} class:offline={agent.status !== 'online'}></span>
									<span class="row-name">{agent.name || agent.id}</span>
								</div>
								<span class="row-status" class:status-on={agent.status === 'online'} class:status-off={agent.status !== 'online'}>
									{agent.status || 'unknown'}
								</span>
							</a>
						{/each}
					</div>
				</div>

				<!-- Hosts -->
				<div class="panel">
					<div class="panel-head">
						<h2>Hosts by Agent</h2>
						<a href="/hosts" class="panel-link">View all</a>
					</div>
					{#if getTotalHosts(agents) === 0}
						<p class="dim">No hosts configured yet.</p>
					{:else}
						<div class="panel-list">
							{#each agents.filter(a => a.host_count > 0) as agent}
								<div class="list-row">
									<div class="row-left">
										<span class="status-dot" class:online={agent.status === 'online'} class:offline={agent.status !== 'online'}></span>
										<span class="row-name">{agent.name || agent.agent_id}</span>
									</div>
									<span class="row-count">{agent.host_count} host{agent.host_count !== 1 ? 's' : ''}</span>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>
{/if}

<style>
	/* ── Loading ── */
	.loading-state {
		min-height: 100vh;
		display: flex; flex-direction: column;
		align-items: center; justify-content: center; gap: 0.75rem;
	}
	.loading-state p { color: var(--text-secondary); font-size: var(--text-sm); }

	/* KPI styles in global.css */

	/* ── Empty state ── */
	.empty-card {
		text-align: center; padding: 3rem;
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg);
	}
	.empty-card h2 {
		margin: 0 0 0.5rem; font-size: var(--text-lg);
		color: var(--text-primary); font-weight: 600;
	}
	.empty-card p {
		margin: 0 0 1.5rem; color: var(--text-secondary);
		font-size: var(--text-sm); max-width: 420px; margin-left: auto; margin-right: auto;
	}
	/* ── Panels ── */
	.panels {
		display: grid; grid-template-columns: 1fr 1fr;
		gap: 1rem;
	}

	.panel {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 1.25rem;
	}
	.panel-head {
		display: flex; justify-content: space-between; align-items: center;
		margin-bottom: 1rem;
	}
	.panel-head h2 {
		margin: 0; font-size: var(--text-base); font-weight: 600;
		color: var(--text-primary);
	}
	.panel-link {
		font-size: var(--text-xs); color: var(--accent); font-weight: 500;
		transition: color var(--transition);
	}
	.panel-link:hover { color: var(--accent-bright); }

	.dim { color: var(--text-tertiary); font-size: var(--text-sm); margin: 0; }

	/* ── List rows ── */
	.panel-list { display: flex; flex-direction: column; }

	.list-row {
		display: flex; justify-content: space-between; align-items: center;
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius);
		transition: background var(--transition);
		text-decoration: none; color: inherit;
	}
	.list-row:hover { background: var(--surface-raised); }

	.row-left { display: flex; align-items: center; gap: 0.625rem; }

	.status-dot {
		width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
	}
	.status-dot.online { background: var(--success); }
	.status-dot.offline { background: var(--text-muted); }

	.row-name { font-size: var(--text-sm); font-weight: 500; color: var(--text-primary); }

	.row-status { font-size: var(--text-xs); font-weight: 500; text-transform: capitalize; }
	.status-on { color: var(--success); }
	.status-off { color: var(--text-tertiary); }

	.row-count {
		font-size: var(--text-xs); color: var(--text-tertiary);
		font-weight: 500; font-variant-numeric: tabular-nums;
	}

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.panels { grid-template-columns: 1fr; }
	}
</style>
