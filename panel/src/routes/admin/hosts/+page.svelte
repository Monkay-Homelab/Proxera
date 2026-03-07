<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

	interface AdminHost {
		id: number;
		domain: string;
		upstream_url: string;
		ssl: boolean;
		websocket: boolean;
		updated_at: string;
		user_id: number;
		user_name: string;
		agent_name: string;
		agent_id: string;
	}

	let hosts: AdminHost[] = $state([]);
	let error = $state('');
	let loading = $state(true);

	const sslEnabled = $derived(hosts.filter((h) => h.ssl).length);
	const withoutAgent = $derived(hosts.filter((h) => !h.agent_name).length);

	onMount(async () => {
		try {
			const data: any = await apiJson('/api/admin/hosts');
			hosts = data.hosts;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load hosts';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Hosts - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Hosts</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state">Loading...</div>
	{:else if hosts.length === 0}
		<div class="empty-state">No hosts found.</div>
	{:else}
		<div class="kpi-row">
			<div class="kpi">
				<div class="kpi-num">{hosts.length}</div>
				<div class="kpi-label">Total Hosts</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{sslEnabled}</div>
				<div class="kpi-label">SSL Enabled</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{withoutAgent}</div>
				<div class="kpi-label">Without Agent</div>
			</div>
		</div>

		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Domain</th>
						<th>Upstream</th>
						<th>Owner</th>
						<th>Agent</th>
						<th>SSL</th>
						<th>WS</th>
						<th>Updated</th>
					</tr>
				</thead>
				<tbody>
					{#each hosts as host}
						<tr>
							<td><span class="cell-domain">{host.domain}</span></td>
							<td><code>{host.upstream_url}</code></td>
							<td>{host.user_name}</td>
							<td>
								{#if host.agent_name}
									<span class="agent-tag">{host.agent_name}</span>
								{:else}
									<span class="text-muted">—</span>
								{/if}
							</td>
							<td>
								{#if host.ssl}
									<span class="flag-on">Yes</span>
								{:else}
									<span class="flag-off">No</span>
								{/if}
							</td>
							<td>
								{#if host.websocket}
									<span class="flag-on">Yes</span>
								{:else}
									<span class="flag-off">No</span>
								{/if}
							</td>
							<td class="cell-sub">{formatRelativeTime(host.updated_at)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.cell-domain {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-primary);
	}

	code {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		background: var(--bg);
		border: 1px solid var(--border);
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		color: var(--text-secondary);
		word-break: break-all;
	}

	.cell-sub {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.agent-tag {
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		background: var(--info-dim);
		color: var(--info);
	}

	.text-muted { color: var(--text-muted); }

	.flag-on {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--success);
	}

	.flag-off {
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
</style>
