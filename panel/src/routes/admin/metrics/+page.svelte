<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatBytes, formatNumber } from '$lib/utils';

	const ranges = ['1h', '6h', '24h', '7d', '30d'];
	let activeRange = $state('24h');
	let metrics: any = $state(null);
	let loading = $state(true);
	let error = $state('');

	async function loadMetrics(range: string) {
		activeRange = range;
		loading = true;
		error = '';
		try {
			metrics = await apiJson(`/api/admin/metrics?range=${range}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load metrics';
		} finally {
			loading = false;
		}
	}

	onMount(() => { loadMetrics('24h'); });
</script>

<svelte:head>
	<title>Metrics - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Platform Metrics</h1>
	</div>

	<div class="range-bar">
		{#each ranges as range}
			<button
				class="range-btn"
				class:active={activeRange === range}
				onclick={() => loadMetrics(range)}
				disabled={loading}
			>
				{range}
			</button>
		{/each}
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state">Loading...</div>
	{:else if metrics}
		<div class="kpi-row">
			<div class="kpi">
				<div class="kpi-num">{formatNumber(metrics.summary.total_requests)}</div>
				<div class="kpi-label">Total Requests</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{formatBytes(metrics.summary.bytes_received)}</div>
				<div class="kpi-label">Bytes In</div>
			</div>
			<div class="kpi">
				<div class="kpi-num">{formatBytes(metrics.summary.bytes_sent)}</div>
				<div class="kpi-label">Bytes Out</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" class:error-rate={metrics.summary.error_rate > 5}>
					{metrics.summary.error_rate.toFixed(1)}%
				</div>
				<div class="kpi-label">Error Rate (4xx + 5xx)</div>
			</div>
		</div>

		{#if metrics.top_domains && metrics.top_domains.length > 0}
			<div class="tbl-wrap">
				<table>
					<thead>
						<tr>
							<th>Domain</th>
							<th class="mono">Requests</th>
							<th class="mono">Traffic</th>
							<th class="mono">Share</th>
						</tr>
					</thead>
					<tbody>
						{#each metrics.top_domains as domain}
							<tr>
								<td><span class="domain-name">{domain.domain}</span></td>
								<td class="mono">{formatNumber(domain.requests)}</td>
								<td class="mono">{formatBytes(domain.bytes)}</td>
								<td class="mono">
									{metrics.summary.total_requests > 0
										? ((domain.requests / metrics.summary.total_requests) * 100).toFixed(1)
										: '0.0'}%
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>

<style>
	.range-bar {
		display: flex;
		gap: 0.25rem;
		margin-bottom: 1.5rem;
	}

	.range-btn {
		padding: 0.375rem 0.875rem;
		border-radius: var(--radius);
		border: 1px solid var(--border);
		background: var(--surface);
		color: var(--text-secondary);
		cursor: pointer;
		font-size: var(--text-xs);
		font-weight: 600;
		transition: all var(--transition);
	}

	.range-btn:hover:not(:disabled) {
		color: var(--text-primary);
		border-color: var(--border-bright);
	}

	.range-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	.range-btn.active {
		background: var(--accent-dim);
		color: var(--accent);
		border-color: var(--accent);
	}

	.error-rate { color: var(--danger); }

	.domain-name {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		font-weight: 500;
	}
</style>
