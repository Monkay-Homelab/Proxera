<script>
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { createFetchGroup } from '$lib/fetchGroup';
	import { feature } from 'topojson-client';
	import { C, ranges, aggregateBuckets, formatBytes, formatNumber, formatMs, formatBlockedTime } from '$lib/metricsUtils';
	import ChartCanvas from '$lib/components/ChartCanvas.svelte';
	import WorldMap from '$lib/components/WorldMap.svelte';

	let loading = true;
	let error = null;
	let metricsData = null;
	let renderKey = 0;
	let selectedRange = '24h';
	let selectedAgent = '';
	let selectedDomain = '';
	let availableDomains = [];
	let availableAgents = [];
	let autoRefresh = true;
	let refreshInterval = null;
	let tooltip = { visible: false, x: 0, y: 0, html: '' };
	let visitors = [];
	let topVisitors = [];
	let visitorsLoading = false;
	let blocked = [];
	let blockedLoading = false;
	let worldGeo = null;
	const metricsGroup = createFetchGroup();
	const visitorsGroup = createFetchGroup();
	const blockedGroup = createFetchGroup();

	function setTooltip(t) { tooltip = t.visible ? t : { ...tooltip, visible: false }; }

	async function loadWorldMap() {
		try {
			const resp = await fetch('https://cdn.jsdelivr.net/npm/world-atlas@2/land-110m.json');
			const topo = await resp.json();
			worldGeo = feature(topo, topo.objects.land);
		} catch (e) { console.warn('Failed to load world map:', e); }
	}

	function refreshAll() { fetchMetrics(); fetchVisitors(); fetchBlocked(); }

	function startInterval() {
		if (refreshInterval) clearInterval(refreshInterval);
		refreshInterval = setInterval(refreshAll, 30000);
	}

	function stopInterval() {
		if (refreshInterval) clearInterval(refreshInterval);
		refreshInterval = null;
	}

	function handleVisibility() {
		if (document.hidden) { stopInterval(); }
		else if (autoRefresh) { refreshAll(); startInterval(); }
	}

	function handleWindowClick() { exportMenuOpen = false; }

	onMount(async () => {
		await Promise.all([fetchMetrics(), fetchVisitors(), fetchBlocked(), loadWorldMap()]);
		startInterval();
		document.addEventListener('visibilitychange', handleVisibility);
		window.addEventListener('click', handleWindowClick);
	});

	onDestroy(() => {
		stopInterval();
		document.removeEventListener('visibilitychange', handleVisibility);
		window.removeEventListener('click', handleWindowClick);
		metricsGroup.abort();
		visitorsGroup.abort();
		blockedGroup.abort();
	});

	function toggleAutoRefresh() {
		autoRefresh = !autoRefresh;
		if (autoRefresh) { refreshAll(); startInterval(); }
		else { stopInterval(); }
	}

	async function fetchMetrics() {
		loading = true; error = null;
		try {
			let url = `/api/metrics?range=${selectedRange}`;
			if (selectedAgent) url += `&agent=${encodeURIComponent(selectedAgent)}`;
			if (selectedDomain) url += `&domain=${encodeURIComponent(selectedDomain)}`;
			const resp = await api(url, { signal: metricsGroup.signal() });
			if (!resp.ok) { const data = await resp.json(); throw new Error(data.error || 'Failed to fetch'); }
			metricsData = await resp.json();
			availableDomains = metricsData.domains || [];
			availableAgents = metricsData.agents || [];
			renderKey++;
			tooltip = { visible: false, x: 0, y: 0, html: '' };
		} catch (err) {
			if (metricsGroup.isAborted(err)) return;
			error = err.message;
		}
		finally { loading = false; }
	}

	async function fetchVisitors() {
		visitorsLoading = true;
		try {
			let url = `/api/metrics/visitors?range=${selectedRange}`;
			if (selectedAgent) url += `&agent=${encodeURIComponent(selectedAgent)}`;
			if (selectedDomain) url += `&domain=${encodeURIComponent(selectedDomain)}`;
			const resp = await api(url, { signal: visitorsGroup.signal() });
			if (resp.ok) { const data = await resp.json(); visitors = data.visitors || []; topVisitors = visitors.slice(0, 10); }
		} catch (err) {
			if (visitorsGroup.isAborted(err)) return;
		}
		finally { visitorsLoading = false; }
	}

	async function fetchBlocked() {
		blockedLoading = true;
		try {
			let url = `/api/metrics/blocked?limit=50`;
			if (selectedAgent) url += `&agent=${encodeURIComponent(selectedAgent)}`;
			const resp = await api(url, { signal: blockedGroup.signal() });
			if (resp.ok) { const data = await resp.json(); blocked = data.blocked || []; }
		} catch (err) {
			if (blockedGroup.isAborted(err)) return;
		}
		finally { blockedLoading = false; }
	}

	function changeRange(range) { selectedRange = range; fetchMetrics(); fetchVisitors(); fetchBlocked(); }
	function changeAgent(e) { selectedAgent = e.target.value; selectedDomain = ''; fetchMetrics(); fetchVisitors(); fetchBlocked(); }
	function changeDomain(e) { selectedDomain = e.target.value; fetchMetrics(); fetchVisitors(); }

	let exporting = false;
	let exportMenuOpen = false;

	async function exportMetrics(format) {
		exportMenuOpen = false;
		exporting = true;
		try {
			let url = `/api/metrics/export?range=${selectedRange}&format=${format}`;
			if (selectedAgent) url += `&agent=${encodeURIComponent(selectedAgent)}`;
			if (selectedDomain) url += `&domain=${encodeURIComponent(selectedDomain)}`;
			const resp = await api(url);
			if (!resp.ok) throw new Error('Export failed');
			const blob = await resp.blob();
			const disposition = resp.headers.get('Content-Disposition') || '';
			const match = disposition.match(/filename="([^"]+)"/);
			const filename = match ? match[1] : `proxera_metrics.${format}`;
			const link = document.createElement('a');
			link.href = URL.createObjectURL(blob);
			link.download = filename;
			link.click();
			URL.revokeObjectURL(link.href);
		} catch (err) {
			console.error('Export error:', err);
		}
		exporting = false;
	}
</script>

<svelte:head><title>Metrics - Proxera</title></svelte:head>

{#if tooltip.visible}
	<div class="chart-tooltip" style="left:{tooltip.x}px;top:{tooltip.y}px">{@html tooltip.html}</div>
{/if}

<div class="page">
	<header class="page-head">
		<h1>Metrics</h1>
		<div class="head-actions">
			<button class="btn-ghost" class:on={autoRefresh} on:click={toggleAutoRefresh}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
				{autoRefresh ? 'Live' : 'Paused'}
			</button>
			<div class="export-wrap">
				<button class="btn-ghost" disabled={exporting} on:click={() => exportMenuOpen = !exportMenuOpen}>
					{#if exporting}
						<svg class="spin" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>
						Exporting…
					{:else}
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
						Export
					{/if}
				</button>
				{#if exportMenuOpen}
					<!-- svelte-ignore a11y-click-events-have-key-events -->
					<!-- svelte-ignore a11y-no-static-element-interactions -->
					<div class="export-menu" on:click|stopPropagation>
						<button on:click={() => exportMetrics('csv')}>
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
							Download CSV
						</button>
						<button on:click={() => exportMetrics('json')}>
							<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
							Download JSON
						</button>
					</div>
				{/if}
			</div>
			<button class="btn-fill" on:click={fetchMetrics} disabled={loading}>{loading ? 'Loading...' : 'Refresh'}</button>
		</div>
	</header>

	<div class="toolbar">
		<div class="toolbar-filters">
			<select class="sel" on:change={changeAgent} value={selectedAgent}>
				<option value="">All Agents</option>
				{#each availableAgents as agent}<option value={agent.agent_id}>{agent.name}</option>{/each}
			</select>
			<select class="sel" on:change={changeDomain} value={selectedDomain}>
				<option value="">All Domains</option>
				{#each availableDomains as domain}<option value={domain}>{domain}</option>{/each}
			</select>
		</div>
		<div class="range-bar">
			{#each ranges as r}
				<button class="range-btn" class:active={selectedRange === r.value} on:click={() => changeRange(r.value)}>{r.label}</button>
			{/each}
		</div>
	</div>

	{#if loading && !metricsData}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading metrics...</p></div>
	{:else if error}
		<div class="placeholder error" aria-live="assertive"><p>{error}</p><button class="btn-fill" on:click={fetchMetrics}>Retry</button></div>
	{:else if metricsData}
	{#key renderKey}
		{@const agg = aggregateBuckets(metricsData.buckets || [])}
		{@const totalUniqueIPs = agg.reduce((s,b) => s+(b.unique_ips||0), 0)}
		{@const totalCacheHits = agg.reduce((s,b) => s+(b.cache_hits||0), 0)}
		{@const totalCacheMisses = agg.reduce((s,b) => s+(b.cache_misses||0), 0)}
		{@const cacheTotal = totalCacheHits + totalCacheMisses}
		{@const cacheHitRate = cacheTotal > 0 ? (totalCacheHits/cacheTotal*100) : 0}
		{@const totalUpW = agg.reduce((s,b) => s+(b.avg_upstream_ms||0)*(b.request_count||0), 0)}
		{@const totalReqs = agg.reduce((s,b) => s+(b.request_count||0), 0)}
		{@const avgUp = totalReqs > 0 ? totalUpW/totalReqs : 0}

		<div class="kpi-row">
			<div class="kpi"><span class="kpi-num">{formatNumber(metricsData.summary.total_requests)}</span><span class="kpi-label">Requests</span></div>
			<div class="kpi"><span class="kpi-num">{formatBytes(metricsData.summary.total_bytes_sent)}</span><span class="kpi-label">Out</span></div>
			<div class="kpi"><span class="kpi-num">{formatBytes(metricsData.summary.total_bytes_received)}</span><span class="kpi-label">In</span></div>
			<div class="kpi"><span class="kpi-num" class:kpi-bad={metricsData.summary.error_rate > 5}>{metricsData.summary.error_rate.toFixed(1)}%</span><span class="kpi-label">Errors</span></div>
			<div class="kpi"><span class="kpi-num">{formatMs(metricsData.summary.avg_latency_ms)}</span><span class="kpi-label">Latency</span></div>
			<div class="kpi"><span class="kpi-num">{formatMs(avgUp)}</span><span class="kpi-label">Upstream</span></div>
			<div class="kpi"><span class="kpi-num">{formatNumber(totalUniqueIPs)}</span><span class="kpi-label">IPs</span></div>
			<div class="kpi"><span class="kpi-num">{cacheHitRate.toFixed(1)}%</span><span class="kpi-label">Cache</span></div>
		</div>

		{#if metricsData.buckets && metricsData.buckets.length > 0}
			<div class="grid-charts">
				<div class="chart-box"><div class="chart-title">Requests Over Time</div><div class="chart-wrap"><ChartCanvas id="requestsChart" data={agg} keys={['request_count']} colors={[C.blue]} {selectedRange} onTooltip={setTooltip} /></div></div>
				<div class="chart-box">
					<div class="chart-head"><div class="chart-title">Status Codes</div><div class="chart-legend"><span><i style="background:{C.green}"></i>2xx</span><span><i style="background:{C.blue}"></i>3xx</span><span><i style="background:{C.orange}"></i>4xx</span><span><i style="background:{C.red}"></i>5xx</span></div></div>
					<div class="chart-wrap"><ChartCanvas id="statusChart" data={agg} keys={['status_2xx','status_3xx','status_4xx','status_5xx']} colors={[C.green,C.blue,C.orange,C.red]} type="stacked" {selectedRange} onTooltip={setTooltip} /></div>
				</div>
				<div class="chart-box">
					<div class="chart-head"><div class="chart-title">Latency Percentiles</div><div class="chart-legend"><span><i style="background:{C.green}"></i>p50</span><span><i style="background:{C.orange}"></i>p95</span><span><i style="background:{C.red}"></i>p99</span></div></div>
					<div class="chart-wrap"><ChartCanvas id="latencyChart" data={agg} keys={['latency_p50_ms','latency_p95_ms','latency_p99_ms']} colors={[C.green,C.orange,C.red]} formatter="ms" {selectedRange} onTooltip={setTooltip} /></div>
				</div>
				<div class="chart-box">
					<div class="chart-head"><div class="chart-title">Bandwidth</div><div class="chart-legend"><span><i style="background:{C.blue}"></i>Out</span><span><i style="background:{C.purple}"></i>In</span></div></div>
					<div class="chart-wrap"><ChartCanvas id="bandwidthChart" data={agg} keys={['bytes_sent','bytes_received']} colors={[C.blue,C.purple]} formatter="bytes" {selectedRange} onTooltip={setTooltip} /></div>
				</div>
				<div class="chart-box"><div class="chart-title">Unique Visitors</div><div class="chart-wrap"><ChartCanvas id="visitorsChart" data={agg} keys={['unique_ips']} colors={[C.cyan]} {selectedRange} onTooltip={setTooltip} /></div></div>
				<div class="chart-box"><div class="chart-title">Connections</div><div class="chart-wrap"><ChartCanvas id="connectionsChart" data={agg} keys={['connection_count']} colors={[C.orange]} {selectedRange} onTooltip={setTooltip} /></div></div>
				<div class="chart-box"><div class="chart-title">Upstream Latency</div><div class="chart-wrap"><ChartCanvas id="upstreamChart" data={agg} keys={['avg_upstream_ms']} colors={[C.pink]} formatter="ms" {selectedRange} onTooltip={setTooltip} /></div></div>
				<div class="chart-box">
					<div class="chart-head"><div class="chart-title">Cache</div><div class="chart-legend"><span><i style="background:{C.green}"></i>Hits</span><span><i style="background:{C.red}"></i>Misses</span></div></div>
					<div class="chart-wrap"><ChartCanvas id="cacheChart" data={agg} keys={['cache_hits','cache_misses']} colors={[C.green,C.red]} type="stacked" {selectedRange} onTooltip={setTooltip} /></div>
				</div>
				<div class="chart-box">
					<div class="chart-head"><div class="chart-title">Avg Request / Response Size</div><div class="chart-legend"><span><i style="background:{C.purple}"></i>Req</span><span><i style="background:{C.orange}"></i>Res</span></div></div>
					<div class="chart-wrap"><ChartCanvas id="reqSizeChart" data={agg} keys={['avg_request_size','avg_response_size']} colors={[C.purple,C.orange]} formatter="bytes" {selectedRange} onTooltip={setTooltip} /></div>
				</div>
			</div>
		{:else}
			<div class="placeholder"><h2>No data yet</h2><p>Metrics appear once agents start collecting nginx logs.</p></div>
		{/if}

		<div class="visitors-row">
			<div class="map-panel">
				<div class="panel-head"><h2>Visitor Map</h2><span class="tag">geo</span></div>
				<div class="map-wrap">
					<WorldMap {visitors} {worldGeo} onTooltip={setTooltip} />
				</div>
			</div>
			<div class="visitors-panel">
				<div class="panel-head"><h2>Top Visitors</h2><span class="tag">by IP</span></div>
				{#if visitorsLoading && topVisitors.length === 0}
					<p class="dim">Loading visitors...</p>
				{:else if topVisitors.length === 0}
					<p class="dim">No visitor data yet.</p>
				{:else}
					<div class="tbl-scroll">
						<table>
							<thead><tr><th class="narrow center">#</th><th>IP Address</th><th class="right">Requests</th><th>Country</th><th>City</th></tr></thead>
							<tbody>
								{#each topVisitors as v, i}
									<tr>
										<td class="narrow center muted">{i+1}</td>
										<td><code>{v.ip_address}</code></td>
										<td class="right strong">{formatNumber(v.request_count)}</td>
										<td class="country-cell">{#if v.country_code}<img src="https://flagcdn.com/16x12/{v.country_code.toLowerCase()}.png" alt={v.country_code} width="16" height="12"/>{/if}{v.country||'-'}</td>
										<td>{v.city||'-'}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>
		</div>

		<div class="blocked-panel">
			<div class="panel-head"><h2>Recent Blocked Connections</h2><span class="tag blocked-tag">CrowdSec</span></div>
			{#if blockedLoading && blocked.length === 0}
				<p class="dim">Loading blocked connections...</p>
			{:else if blocked.length === 0}
				<p class="dim">No blocked connections found. Agents with CrowdSec installed will report blocked IPs here.</p>
			{:else}
				<div class="tbl-scroll">
					<table>
						<thead>
							<tr>
								<th class="narrow center">#</th>
								<th>IP Address</th>
								<th class="right">Events</th>
								<th>Country</th>
								<th>AS Name</th>
								<th>Agent</th>
								<th>Last Seen</th>
							</tr>
						</thead>
						<tbody>
							{#each blocked as b, i}
								<tr>
									<td class="narrow center muted">{i+1}</td>
									<td><code class="blocked-ip">{b.ip}</code></td>
									<td class="right strong-red">{formatNumber(b.events_count)}</td>
									<td class="country-cell">
										{#if b.country_code}
											<img src="https://flagcdn.com/16x12/{b.country_code.toLowerCase()}.png" alt={b.country_code} width="16" height="12"/>
										{/if}
										{b.country || '-'}
									</td>
									<td class="as-cell">{b.as_name || '-'}</td>
									<td class="agent-cell">{b.agent_name}</td>
									<td class="time-cell">{formatBlockedTime(b.last_seen)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

	{/key}
	{/if}
</div>

<style>
	/* ── Header ── */
	.head-actions { display: flex; gap: 0.5rem; align-items: center; }

	/* ── Buttons ── */
	.btn-ghost.on { color: var(--accent); border-color: var(--accent); background: var(--accent-dim); }

	/* ── Toolbar ── */
	.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; gap: 1rem; flex-wrap: wrap; }
	.toolbar-filters { display: flex; gap: 0.5rem; }

	.sel {
		padding: 0.4375rem 0.75rem; border: 1px solid var(--border);
		border-radius: var(--radius); font-size: var(--text-sm);
		color: var(--text-primary); background: var(--surface);
		cursor: pointer; min-width: 145px;
	}
	.sel:focus { outline: none; border-color: var(--accent); }

	.range-bar {
		display: flex; background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 3px; gap: 2px;
	}
	.range-btn {
		background: transparent; border: none;
		padding: 0.375rem 0.625rem; border-radius: var(--radius);
		cursor: pointer; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); letter-spacing: 0.03em;
		transition: all var(--transition);
	}
	.range-btn.active { background: var(--accent); color: #fff; }
	.range-btn:hover:not(.active) { color: var(--text-primary); background: var(--surface-raised); }

	/* KPI base styles in global.css */
	.kpi-bad { color: var(--danger); }

	/* ── Charts ── */
	.grid-charts { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.75rem; margin-bottom: 1.5rem; }

	.chart-box {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 1.125rem 1.25rem;
	}
	.chart-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.625rem; }
	.chart-title { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); margin-bottom: 0.625rem; }
	.chart-head .chart-title { margin-bottom: 0; }

	.chart-legend { display: flex; gap: 0.625rem; font-size: var(--text-xs); color: var(--text-secondary); }
	.chart-legend span { display: flex; align-items: center; gap: 0.25rem; }
	.chart-legend i { width: 7px; height: 7px; border-radius: 2px; display: inline-block; font-style: normal; }

	.chart-wrap { width: 100%; height: 180px; }
	.chart-wrap :global(canvas) { width: 100%; height: 100%; cursor: crosshair; }

	/* ── Tooltip ── */
	:global(.chart-tooltip) {
		position: fixed; z-index: 1000;
		background: var(--surface-raised); color: var(--text-primary);
		border: 1px solid var(--border-bright); border-radius: var(--radius-lg);
		padding: 0.625rem 0.875rem; font-size: var(--text-sm);
		pointer-events: none; box-shadow: var(--shadow-md);
	}
	:global(.tooltip-time) { font-weight: 500; margin-bottom: 0.375rem; padding-bottom: 0.375rem; border-bottom: 1px solid var(--border); font-size: var(--text-xs); color: var(--text-tertiary); }
	:global(.tooltip-row) { display: flex; align-items: center; gap: 0.5rem; padding: 0.125rem 0; }
	:global(.tooltip-dot) { width: 7px; height: 7px; border-radius: 2px; flex-shrink: 0; }
	:global(.tooltip-label) { color: var(--text-secondary); min-width: 50px; }
	:global(.tooltip-val) { font-weight: 600; margin-left: auto; }

	/* ── Placeholder (metrics-specific overrides) ── */
	.placeholder h2 { font-size: var(--text-lg); color: var(--text-primary); margin: 0; }
	.placeholder .btn-fill { margin-top: 1rem; }

	/* ── Visitors Row ── */
	.visitors-row { display: flex; gap: 0.75rem; margin-top: 1.5rem; }
	.map-panel { flex: 1; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 1.25rem; min-width: 0; display: flex; flex-direction: column; }
	.map-wrap { width: 100%; flex: 1; min-height: 250px; position: relative; }
	.map-wrap :global(canvas) { display: block; width: 100%; height: 100%; border-radius: var(--radius); }
	.visitors-panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 1.25rem; width: 40%; min-width: 320px; flex-shrink: 0; }
	.panel-head { display: flex; align-items: center; gap: 0.625rem; margin-bottom: 1rem; }
	.panel-head h2 { font-size: var(--text-base); font-weight: 600; color: var(--text-primary); }
	.tag { font-size: var(--text-xs); color: var(--text-tertiary); background: var(--surface-raised); padding: 0.125rem 0.5rem; border-radius: 999px; }
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }

	/* ── Table ── */
	.tbl-scroll { overflow-x: auto; }
	table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
	thead th { text-align: left; padding: 0.5rem 0.75rem; font-weight: 600; color: var(--text-tertiary); font-size: var(--text-xs); border-bottom: 1px solid var(--border); }
	tbody td { padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--border); color: var(--text-primary); }
	tbody tr:last-child td { border-bottom: none; }
	tbody tr:hover { background: var(--surface-raised); }
	.narrow { width: 3rem; }
	.center { text-align: center; }
	.right { text-align: right; }
	.muted { color: var(--text-muted); }
	.strong { font-weight: 600; color: var(--accent); }
	.country-cell { display: flex; align-items: center; gap: 0.5rem; }

	/* ── Blocked Panel ── */
	.blocked-panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 1.25rem; margin-top: 1rem; }
	.blocked-tag { background: rgba(239, 96, 104, 0.12); color: #ef6068; }
	.blocked-ip { color: #ef6068; font-size: var(--text-sm); }
	.strong-red { font-weight: 600; color: #ef6068; }
	.as-cell { color: var(--text-secondary); font-size: var(--text-xs); max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.agent-cell { color: var(--text-secondary); font-size: var(--text-sm); white-space: nowrap; }
	.time-cell { color: var(--text-tertiary); font-size: var(--text-xs); white-space: nowrap; }

	/* ── Export ── */
	.export-wrap { position: relative; }
	.export-menu {
		position: absolute; top: calc(100% + 6px); right: 0; z-index: 100;
		background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
		box-shadow: 0 8px 24px rgba(0,0,0,0.25); min-width: 160px; overflow: hidden;
	}
	.export-menu button {
		display: flex; align-items: center; gap: 0.5rem; width: 100%; text-align: left;
		padding: 0.625rem 1rem; background: none; border: none; cursor: pointer;
		font-size: var(--text-sm); color: var(--text-primary); font-family: inherit;
		transition: background var(--transition);
	}
	.export-menu button:hover { background: var(--surface-raised); }
	.spin { animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ── Responsive ── */
	.kpi-row { grid-template-columns: repeat(4, 1fr); }
	@media (max-width: 1200px) { .grid-charts { grid-template-columns: repeat(2,1fr); } .kpi-row { grid-template-columns: repeat(4,1fr); } .kpi:nth-child(4) { border-right: none; } .visitors-row { flex-direction: column; } .visitors-panel { width: 100%; min-width: 0; } }
	@media (max-width: 768px) { .grid-charts { grid-template-columns: 1fr; } .kpi-row { grid-template-columns: repeat(2,1fr); } .kpi:nth-child(2n) { border-right: none; } .toolbar { flex-direction: column; } .range-bar { flex-wrap: wrap; } }
</style>
