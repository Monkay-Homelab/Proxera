<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

	interface AdminAgent {
		id: number;
		agent_id: string;
		name: string;
		status: string;
		version: string;
		os: string;
		arch: string;
		last_seen: string;
		wan_ip: string;
		nginx_version: string;
		crowdsec_installed: boolean;
		host_count: number;
		user_id: number;
		user_name: string;
		user_email: string;
	}

	let agents: AdminAgent[] = $state([]);
	let error = $state('');
	let loading = $state(true);

	const onlineCount = $derived(agents.filter((a) => a.status === 'online').length);
	const offlineCount = $derived(agents.length - onlineCount);

	onMount(async () => {
		try {
			const data: any = await apiJson('/api/admin/agents');
			agents = data.agents;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load agents';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Agents - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Agents</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state">Loading...</div>
	{:else if agents.length === 0}
		<div class="empty-state">No agents found.</div>
	{:else}
		<div class="kpi-row">
			<div class="kpi">
				<div class="kpi-num">{agents.length}</div>
				<div class="kpi-label">Total Agents</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" style="color: var(--accent)">{onlineCount}</div>
				<div class="kpi-label">Online</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" style="color: var(--danger)">{offlineCount}</div>
				<div class="kpi-label">Offline</div>
			</div>
		</div>

		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Agent</th>
						<th>Owner</th>
						<th>Status</th>
						<th>Version</th>
						<th>OS / Arch</th>
						<th>Nginx</th>
						<th>CrowdSec</th>
						<th class="mono">Hosts</th>
						<th>Last Seen</th>
					</tr>
				</thead>
				<tbody>
					{#each agents as agent}
						<tr>
							<td>
								<div class="cell-main">{agent.name}</div>
								{#if agent.wan_ip}
									<div class="cell-sub">{agent.wan_ip}</div>
								{/if}
							</td>
							<td>
								<div class="cell-main">{agent.user_name}</div>
								<div class="cell-sub">{agent.user_email}</div>
							</td>
							<td>
								<span
									class="status-pill"
									class:online={agent.status === 'online'}
									class:offline={agent.status === 'offline'}
								>
									<span class="status-dot" class:online={agent.status === 'online'} class:offline={agent.status === 'offline'}></span>
									{agent.status}
								</span>
							</td>
							<td><code>{agent.version || '—'}</code></td>
							<td>{agent.os || '—'}/{agent.arch || '—'}</td>
							<td><code>{agent.nginx_version || '—'}</code></td>
							<td>
								{#if agent.crowdsec_installed}
									<span class="badge-on">Active</span>
								{:else}
									<span class="badge-off">No</span>
								{/if}
							</td>
							<td class="mono">{agent.host_count}</td>
							<td class="cell-sub">{formatRelativeTime(agent.last_seen)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.cell-main { font-weight: 600; }

	.cell-sub {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.status-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		text-transform: capitalize;
		background: var(--surface-raised);
		color: var(--text-tertiary);
	}

	.status-pill.online {
		background: var(--success-dim);
		color: var(--success);
	}

	.status-pill.offline {
		background: var(--danger-dim);
		color: var(--danger);
	}

	.status-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--text-tertiary);
		flex-shrink: 0;
	}

	.status-dot.online { background: var(--success); }
	.status-dot.offline { background: var(--danger); }

	code {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		background: var(--bg);
		border: 1px solid var(--border);
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		color: var(--text-primary);
	}

	.badge-on {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--success);
	}

	.badge-off {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}
</style>
