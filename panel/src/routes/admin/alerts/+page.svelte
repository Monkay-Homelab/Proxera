<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime, formatNumber } from '$lib/utils';

	let loading = $state(true);
	let error = $state('');

	let stats = $state({ active_alerts: 0, triggers_24h: 0, triggers_7d: 0, users_with_active_alerts: 0 });
	let alerts = $state<any[]>([]);
	let total = $state(0);
	let expanded = $state<number | null>(null);

	onMount(async () => {
		try {
			const [statsRes, alertsRes] = await Promise.all([
				apiJson<any>('/api/admin/alerts/stats'),
				apiJson<any>('/api/admin/alerts?limit=100'),
			]);
			stats = statsRes;
			alerts = alertsRes.alerts;
			total = alertsRes.total;
		} catch (err: any) {
			error = err.message || 'Failed to load alert data';
		} finally {
			loading = false;
		}
	});

	function severityColor(severity: string): string {
		switch (severity) {
			case 'critical': return 'var(--danger)';
			case 'warning': return 'var(--warning)';
			default: return 'var(--info)';
		}
	}

	function alertTypeLabel(type: string): string {
		switch (type) {
			case 'agent_offline': return 'Agent Offline';
			case 'cert_expiry': return 'Cert Expiry';
			case 'cert_renewal_failed': return 'Renewal Failed';
			case 'error_rate': return 'Error Rate';
			case 'test': return 'Test';
			default: return type;
		}
	}

	function toggleExpand(id: number) {
		expanded = expanded === id ? null : id;
	}
</script>

<svelte:head>
	<title>Alerts - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Alert Activity</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state"><div class="loader"></div></div>
	{:else}
		<div class="kpi-row">
			<div class="kpi">
				<span class="kpi-num" style="color: var(--danger)">{formatNumber(stats.active_alerts)}</span>
				<span class="kpi-label">Active Alerts</span>
			</div>
			<div class="kpi">
				<span class="kpi-num">{formatNumber(stats.triggers_24h)}</span>
				<span class="kpi-label">Triggers (24h)</span>
			</div>
			<div class="kpi">
				<span class="kpi-num">{formatNumber(stats.triggers_7d)}</span>
				<span class="kpi-label">Triggers (7d)</span>
			</div>
			<div class="kpi">
				<span class="kpi-num">{formatNumber(stats.users_with_active_alerts)}</span>
				<span class="kpi-label">Users with Active</span>
			</div>
		</div>

		{#if alerts.length === 0}
			<div class="empty-state">No alerts have been triggered yet.</div>
		{:else}
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th>Time</th>
							<th>User</th>
							<th>Type</th>
							<th>Severity</th>
							<th>Title</th>
							<th>Status</th>
						</tr>
					</thead>
					<tbody>
						{#each alerts as alert}
							<tr class="clickable-row" onclick={() => toggleExpand(alert.id)}>
								<td class="cell-time">{formatRelativeTime(alert.created_at)}</td>
								<td>
									<span class="cell-user-name">{alert.user_name}</span>
									<span class="cell-user-email">{alert.user_email}</span>
								</td>
								<td><span class="type-pill">{alertTypeLabel(alert.alert_type)}</span></td>
								<td>
									<span class="severity-dot" style="background: {severityColor(alert.severity)}"></span>
									{alert.severity}
								</td>
								<td class="cell-title">{alert.title}</td>
								<td>
									{#if alert.resolved}
										<span class="resolved-badge">Resolved</span>
									{:else}
										<span class="active-badge">Active</span>
									{/if}
								</td>
							</tr>
							{#if expanded === alert.id}
								<tr class="expand-row">
									<td colspan="6">
										<div class="expand-content">
											<p class="expand-message">{alert.message}</p>
											{#if alert.metadata && Object.keys(alert.metadata).length > 0}
												<pre class="expand-meta">{JSON.stringify(alert.metadata, null, 2)}</pre>
											{/if}
										</div>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
			<div class="total-count">Showing {alerts.length} of {total} alerts</div>
		{/if}
	{/if}
</div>

<style>
	.empty-state {
		padding: 3rem 2rem;
		text-align: center;
		color: var(--text-tertiary);
		font-size: var(--text-sm);
	}

	.type-pill {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: var(--radius);
		font-size: var(--text-xs);
		font-weight: 600;
		background: var(--surface-raised);
		color: var(--text-secondary);
		white-space: nowrap;
	}

	.cell-time {
		color: var(--text-tertiary);
		font-size: var(--text-xs);
		white-space: nowrap;
	}

	.cell-user-name { font-weight: 600; color: var(--text-primary); display: block; }
	.cell-user-email { color: var(--text-tertiary); font-size: var(--text-xs); display: block; }

	.cell-title {
		max-width: 280px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.severity-dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		margin-right: 0.375rem;
		vertical-align: middle;
	}

	.resolved-badge {
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		background: var(--success-dim);
		color: var(--success);
	}
	.active-badge {
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		background: var(--danger-dim);
		color: var(--danger);
	}

	.clickable-row { cursor: pointer; }
	.clickable-row:hover { background: var(--surface-raised); }

	.expand-row td { padding: 0 !important; border-bottom: 1px solid var(--border); }
	.expand-content {
		padding: 1rem 1.25rem;
		background: var(--bg);
		border-top: 1px solid var(--border);
	}
	.expand-message {
		color: var(--text-secondary);
		font-size: var(--text-xs);
		white-space: pre-line;
		line-height: 1.5;
		margin: 0 0 0.75rem;
	}
	.expand-meta {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		color: var(--text-tertiary);
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 0.75rem;
		margin: 0;
		overflow-x: auto;
	}

	.total-count {
		text-align: center;
		padding: 0.75rem;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}
</style>
