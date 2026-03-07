<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatBytes, formatNumber, formatRelativeTime } from '$lib/utils';

	interface UserStorage {
		id: number;
		name: string;
		email: string;
		role: string;
		agent_count: number;
		host_count: number;
		dns_count: number;
		cert_count: number;
		metrics_rows: number;
		total_bytes: number;
		est_storage: number;
	}

	let stats: any = $state(null);
	let health: any = $state(null);
	let systemHealth: any = $state(null);
	let alertStats: any = $state(null);
	let storage: UserStorage[] = $state([]);
	let storageLoading = $state(true);
	let error = $state('');

	function formatUptime(seconds: number): string {
		const d = Math.floor(seconds / 86400);
		const h = Math.floor((seconds % 86400) / 3600);
		const m = Math.floor((seconds % 3600) / 60);
		if (d > 0) return `${d}d ${h}h ${m}m`;
		if (h > 0) return `${h}h ${m}m`;
		return `${m}m`;
	}

	onMount(async () => {
		try {
			stats = await apiJson('/api/admin/stats');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load stats';
		}

		Promise.all([
			apiJson('/api/admin/stats/health').then((d: any) => { health = d; }),
			apiJson('/api/admin/stats/system').then((d: any) => { systemHealth = d; }).catch(() => {}),
			apiJson('/api/admin/stats/storage').then((d: any) => { storage = d.users; }),
			apiJson('/api/admin/alerts/stats').then((d: any) => { alertStats = d; }).catch(() => {}),
		]).finally(() => { storageLoading = false; });
	});
</script>

<svelte:head>
	<title>Admin Dashboard - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Admin Dashboard</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if !stats}
		<div class="empty-state">Loading...</div>
	{:else}
		<div class="kpi-row">
			<div class="kpi">
				<div class="kpi-num">{stats.user_count}</div>
				<div class="kpi-label">Users</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{stats.domain_count}</div>
				<div class="kpi-label">Domains</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{stats.host_count}</div>
				<div class="kpi-label">Hosts</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{formatBytes(stats.db_size_bytes)}</div>
				<div class="kpi-label">DB Size — {formatBytes(stats.disk_used_bytes)} / {formatBytes(stats.disk_total_bytes)} disk</div>
			</div>
		</div>

		{#if health}
			<div class="health-grid">
				<div class="health-card">
					<div class="health-title">Agent Health</div>
					<div class="health-row">
						<div class="health-stat">
							<span class="dot dot-green"></span>
							<span class="health-num">{health.agents.online}</span>
							<span class="health-label">online</span>
						</div>
						<div class="health-stat">
							<span class="dot dot-red"></span>
							<span class="health-num">{health.agents.offline}</span>
							<span class="health-label">offline</span>
						</div>
						<div class="health-stat">
							<span class="health-num">{health.agents.total}</span>
							<span class="health-label">total</span>
						</div>
					</div>
				</div>

				<div class="health-card">
					<div class="health-title">Certificate Health</div>
					<div class="health-row">
						<div class="health-stat">
							<span class="dot dot-green"></span>
							<span class="health-num">{health.certificates.valid}</span>
							<span class="health-label">valid</span>
						</div>
						<div class="health-stat">
							<span class="dot dot-yellow"></span>
							<span class="health-num">{health.certificates.expiring}</span>
							<span class="health-label">expiring</span>
						</div>
						<div class="health-stat">
							<span class="dot dot-red"></span>
							<span class="health-num">{health.certificates.expired}</span>
							<span class="health-label">expired</span>
						</div>
					</div>
				</div>

				<div class="health-card">
					<div class="health-title">24h Traffic</div>
					{#if health.sparkline && health.sparkline.length > 1}
						{@const maxReq = Math.max(...health.sparkline.map((p: any) => p.requests), 1)}
						{@const w = 200}
						{@const h = 48}
						{@const points = health.sparkline.map((p: any, i: number) => {
							const x = (i / (health.sparkline.length - 1)) * w;
							const y = h - (p.requests / maxReq) * (h - 6) - 3;
							return `${x},${y}`;
						}).join(' ')}
						{@const fillPoints = `0,${h} ${points} ${w},${h}`}
						<svg viewBox="0 0 {w} {h}" class="sparkline" preserveAspectRatio="none">
							<defs>
								<linearGradient id="spark-fill" x1="0" y1="0" x2="0" y2="1">
									<stop offset="0%" stop-color="var(--accent)" stop-opacity="0.15" />
									<stop offset="100%" stop-color="var(--accent)" stop-opacity="0" />
								</linearGradient>
							</defs>
							<polygon points={fillPoints} fill="url(#spark-fill)" />
							<polyline points={points} fill="none" stroke="var(--accent)" stroke-width="1.5" vector-effect="non-scaling-stroke" />
						</svg>
						<div class="spark-total">{formatNumber(health.sparkline.reduce((sum: number, p: any) => sum + p.requests, 0))} requests</div>
					{:else}
						<div class="spark-total faded">No traffic data</div>
					{/if}
				</div>
			</div>
		{/if}

		{#if systemHealth}
			<h2 class="section-title">System Health</h2>
			<div class="health-grid sys-grid">
				<div class="health-card">
					<div class="health-title">Process</div>
					<div class="sys-rows">
						<div class="sys-row"><span class="sys-key">Uptime</span><span class="sys-val">{formatUptime(systemHealth.uptime_seconds)}</span></div>
						<div class="sys-row"><span class="sys-key">Goroutines</span><span class="sys-val">{systemHealth.goroutines}</span></div>
						<div class="sys-row"><span class="sys-key">Heap</span><span class="sys-val">{systemHealth.memory.heap_alloc_mb.toFixed(1)} MB</span></div>
						<div class="sys-row"><span class="sys-key">Sys Mem</span><span class="sys-val">{systemHealth.memory.sys_mb.toFixed(1)} MB</span></div>
						<div class="sys-row"><span class="sys-key">GC Runs</span><span class="sys-val">{systemHealth.memory.gc_runs}</span></div>
					</div>
				</div>

				<div class="health-card">
					<div class="health-title">API Requests (last 1 min)</div>
					<div class="sys-rows">
						<div class="sys-row"><span class="sys-key">Count</span><span class="sys-val">{systemHealth.requests.last_1m.count}</span></div>
						<div class="sys-row"><span class="sys-key">Errors</span><span class="sys-val" class:sys-warn={systemHealth.requests.last_1m.error_rate > 5}>{systemHealth.requests.last_1m.errors} ({systemHealth.requests.last_1m.error_rate.toFixed(1)}%)</span></div>
						<div class="sys-row"><span class="sys-key">Avg Latency</span><span class="sys-val">{systemHealth.requests.last_1m.avg_ms.toFixed(1)} ms</span></div>
						<div class="sys-row"><span class="sys-key">p95 Latency</span><span class="sys-val" class:sys-warn={systemHealth.requests.last_1m.p95_ms > 500}>{systemHealth.requests.last_1m.p95_ms.toFixed(0)} ms</span></div>
					</div>
				</div>

				<div class="health-card">
					<div class="health-title">API Requests (last 5 min)</div>
					<div class="sys-rows">
						<div class="sys-row"><span class="sys-key">Count</span><span class="sys-val">{systemHealth.requests.last_5m.count}</span></div>
						<div class="sys-row"><span class="sys-key">Errors</span><span class="sys-val" class:sys-warn={systemHealth.requests.last_5m.error_rate > 5}>{systemHealth.requests.last_5m.errors} ({systemHealth.requests.last_5m.error_rate.toFixed(1)}%)</span></div>
						<div class="sys-row"><span class="sys-key">Avg Latency</span><span class="sys-val">{systemHealth.requests.last_5m.avg_ms.toFixed(1)} ms</span></div>
						<div class="sys-row"><span class="sys-key">p95 Latency</span><span class="sys-val" class:sys-warn={systemHealth.requests.last_5m.p95_ms > 500}>{systemHealth.requests.last_5m.p95_ms.toFixed(0)} ms</span></div>
					</div>
				</div>

				<div class="health-card">
					<div class="health-title">Database Pool</div>
					<div class="sys-rows">
						<div class="sys-row"><span class="sys-key">Total Conns</span><span class="sys-val">{systemHealth.db_pool.total}</span></div>
						<div class="sys-row"><span class="sys-key">Acquired</span><span class="sys-val">{systemHealth.db_pool.acquired}</span></div>
						<div class="sys-row"><span class="sys-key">Idle</span><span class="sys-val">{systemHealth.db_pool.idle}</span></div>
					</div>
				</div>
			</div>
		{/if}

		{#if alertStats}
			<h2 class="section-title">Alerts</h2>
			<div class="kpi-row">
				<div class="kpi">
					<span class="kpi-num" style="color: {alertStats.active_alerts > 0 ? 'var(--danger)' : 'var(--text-primary)'}">{alertStats.active_alerts}</span>
					<span class="kpi-label">Active Alerts</span>
				</div>
				<div class="kpi">
					<span class="kpi-num">{alertStats.triggers_24h}</span>
					<span class="kpi-label">Triggers (24h)</span>
				</div>
				<div class="kpi">
					<span class="kpi-num">{alertStats.triggers_7d}</span>
					<span class="kpi-label">Triggers (7d)</span>
				</div>
				<div class="kpi">
					<span class="kpi-num">{alertStats.users_with_active_alerts}</span>
					<span class="kpi-label">Users with Active</span>
				</div>
			</div>
		{/if}

		{#if health && health.recent_signups && health.recent_signups.length > 0}
			<h2 class="section-title">Recent Signups</h2>
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th>Name</th>
							<th>Email</th>
							<th>Role</th>
							<th>Joined</th>
						</tr>
					</thead>
					<tbody>
						{#each health.recent_signups.slice(0, 10) as signup}
							<tr>
								<td>{signup.name}</td>
								<td>{signup.email}</td>
								<td><span class="role-badge role-{signup.role}">{signup.role}</span></td>
								<td>{formatRelativeTime(signup.created_at)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

		{#if storageLoading}
			<div class="empty-state">Loading storage data...</div>
		{:else if storage.length > 0}
			<h2 class="section-title">Storage Footprint</h2>
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th>User</th>
							<th>Role</th>
							<th class="mono">Agents</th>
							<th class="mono">Hosts</th>
							<th class="mono">DNS</th>
							<th class="mono">Certs</th>
							<th class="mono">Metrics Rows</th>
							<th class="mono">Traffic</th>
							<th class="mono">Est. Storage</th>
						</tr>
					</thead>
					<tbody>
						{#each storage as u}
							<tr>
								<td>
									<div class="user-cell">
										<span class="user-name">{u.name}</span>
										<span class="user-email">{u.email}</span>
									</div>
								</td>
								<td><span class="role-badge role-{u.role}">{u.role}</span></td>
								<td class="mono">{u.agent_count}</td>
								<td class="mono">{u.host_count}</td>
								<td class="mono">{u.dns_count}</td>
								<td class="mono">{u.cert_count}</td>
								<td class="mono">{formatNumber(u.metrics_rows)}</td>
								<td class="mono">{formatBytes(u.total_bytes)}</td>
								<td class="mono">{formatBytes(u.est_storage)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>

<style>
	.health-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.75rem;
		margin-bottom: 1.75rem;
	}

	.health-card {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 1rem 1.25rem;
	}

	.health-title {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		margin-bottom: 0.75rem;
	}

	.health-row {
		display: flex;
		gap: 1.25rem;
		align-items: center;
	}

	.health-stat {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}

	.health-num {
		font-size: var(--text-sm);
		font-weight: 700;
		color: var(--text-primary);
		font-variant-numeric: tabular-nums;
	}

	.health-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.dot-green { background: var(--accent); }
	.dot-yellow { background: var(--warning); }
	.dot-red { background: var(--danger); }

	.sparkline {
		width: 100%;
		height: 48px;
		display: block;
	}

	.spark-total {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		margin-top: 0.375rem;
		font-variant-numeric: tabular-nums;
	}

	.spark-total.faded {
		color: var(--text-tertiary);
	}

	.section-title {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		margin: 1.75rem 0 0.625rem;
	}

	.user-cell {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}

	.user-name { font-weight: 600; }

	.user-email {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.sys-grid { grid-template-columns: repeat(4, 1fr); }

	.sys-rows { display: flex; flex-direction: column; gap: 0.375rem; }
	.sys-row {
		display: flex; justify-content: space-between; align-items: baseline;
		font-size: var(--text-sm);
	}
	.sys-key { color: var(--text-tertiary); }
	.sys-val { font-weight: 600; color: var(--text-primary); font-variant-numeric: tabular-nums; }
	.sys-warn { color: var(--warning) !important; }

	@media (max-width: 1100px) { .sys-grid { grid-template-columns: repeat(2, 1fr); } }
	@media (max-width: 900px) {
		.health-grid { grid-template-columns: 1fr; }
		.sys-grid { grid-template-columns: 1fr; }
	}
</style>
