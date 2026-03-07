<script>
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { createFetchGroup } from '$lib/fetchGroup';

	const agentId = $page.params.agentId;
	const fetchGroup = createFetchGroup();
	let agent = null;
	let logs = '';
	let loading = true;
	let logsLoading = false;
	let error = null;
	let autoRefresh = false;
	let refreshInterval = null;
	let lineCount = 200;

	// Filters
	let searchQuery = '';
	let statusFilter = 'all';
	let methodFilter = 'all';
	let ipFilter = '';

	const lineCounts = [100, 200, 500, 1000];
	const statusFilters = ['all', '2xx', '3xx', '4xx', '5xx'];
	const methodFilters = ['all', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE'];

	// Parse a combined-format log line
	function parseLine(line) {
		const m = line.match(/^(\S+)\s+\S+\s+\S+\s+\[([^\]]+)\]\s+"(\S+)\s+(\S+)\s+\S+"\s+(\d{3})\s+(\d+)\s+"([^"]*)"\s+"([^"]*)"/);
		if (!m) return null;
		return { raw: line, ip: m[1], time: m[2], method: m[3], path: m[4], status: parseInt(m[5]), size: parseInt(m[6]), referer: m[7], ua: m[8] };
	}

	$: parsedLines = logs ? logs.split('\n').filter(l => l.trim()).map(parseLine).filter(Boolean) : [];

	$: filteredLines = parsedLines.filter(l => {
		if (statusFilter !== 'all') {
			const prefix = parseInt(statusFilter[0]);
			if (Math.floor(l.status / 100) !== prefix) return false;
		}
		if (methodFilter !== 'all' && l.method !== methodFilter) return false;
		if (ipFilter && !l.ip.includes(ipFilter)) return false;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			if (!l.raw.toLowerCase().includes(q)) return false;
		}
		return true;
	});

	$: uniqueIPs = [...new Set(parsedLines.map(l => l.ip))].sort();
	$: filterStats = {
		total: parsedLines.length,
		shown: filteredLines.length,
		s2xx: parsedLines.filter(l => l.status >= 200 && l.status < 300).length,
		s3xx: parsedLines.filter(l => l.status >= 300 && l.status < 400).length,
		s4xx: parsedLines.filter(l => l.status >= 400 && l.status < 500).length,
		s5xx: parsedLines.filter(l => l.status >= 500).length,
	};

	function startInterval() {
		if (refreshInterval) clearInterval(refreshInterval);
		refreshInterval = setInterval(fetchLogs, 5000);
	}

	function stopInterval() {
		if (refreshInterval) clearInterval(refreshInterval);
		refreshInterval = null;
	}

	function handleVisibility() {
		if (document.hidden) { stopInterval(); }
		else if (autoRefresh) { fetchLogs(); startInterval(); }
	}

	onMount(async () => {
		await fetchAgent();
		await fetchLogs();
		document.addEventListener('visibilitychange', handleVisibility);
	});

	onDestroy(() => {
		stopInterval();
		document.removeEventListener('visibilitychange', handleVisibility);
		fetchGroup.abort();
	});

	function toggleAutoRefresh() {
		autoRefresh = !autoRefresh;
		if (autoRefresh) { fetchLogs(); startInterval(); }
		else { stopInterval(); }
	}

	async function fetchAgent() {
		try {
			const resp = await api(`/api/agents/${agentId}`);
			if (!resp.ok) throw new Error('Agent not found');
			agent = await resp.json();
		} catch (err) { error = err.message; }
		finally { loading = false; }
	}

	async function fetchLogs() {
		logsLoading = true;
		try {
			const resp = await api(`/api/metrics/logs?agent=${encodeURIComponent(agentId)}&lines=${lineCount}`, { signal: fetchGroup.signal() });
			if (!resp.ok) {
				const data = await resp.json().catch(() => ({}));
				throw new Error(data.error || 'Failed to fetch logs');
			}
			const data = await resp.json();
			logs = data.logs || '';
		} catch (err) {
			if (fetchGroup.isAborted(err)) return;
			error = err.message;
		} finally { logsLoading = false; }
	}

	function changeLineCount(n) {
		lineCount = n;
		fetchLogs();
	}

	function clearFilters() {
		searchQuery = '';
		statusFilter = 'all';
		methodFilter = 'all';
		ipFilter = '';
	}

	function isOnline() {
		if (!agent || !agent.last_seen) return false;
		return (Date.now() - new Date(agent.last_seen).getTime()) < 90000;
	}

	function scrollToBottom() {
		const el = document.querySelector('.log-output');
		if (el) el.scrollTop = el.scrollHeight;
	}

	function scrollToTop() {
		const el = document.querySelector('.log-output');
		if (el) el.scrollTop = 0;
	}

	function statusClass(code) {
		if (code >= 500) return 'st-5xx';
		if (code >= 400) return 'st-4xx';
		if (code >= 300) return 'st-3xx';
		return 'st-2xx';
	}

	$: hasActiveFilters = searchQuery || statusFilter !== 'all' || methodFilter !== 'all' || ipFilter;
</script>

<svelte:head><title>{agent?.name || 'Agent'} Logs - Proxera</title></svelte:head>

<div class="page">
	<header class="page-head">
		<div class="head-left">
			<a href="/logs" class="back-link">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
				Back
			</a>
			{#if agent}
				<div class="agent-info">
					<span class="status-dot" class:online={isOnline()}></span>
					<h1>{agent.name}</h1>
					<span class="tag">{agent.wan_ip || agent.ip_address || '-'}</span>
				</div>
			{:else}
				<h1>Agent Logs</h1>
			{/if}
		</div>
	</header>

	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading...</p></div>
	{:else if error && !logs}
		<div class="placeholder error" aria-live="assertive"><p>{error}</p><a href="/logs" class="btn-fill">Back to Agents</a></div>
	{:else}
		<div class="toolbar">
			<div class="toolbar-left">
				<div class="line-selector">
					{#each lineCounts as n}
						<button class="range-btn" class:active={lineCount === n} on:click={() => changeLineCount(n)}>{n}</button>
					{/each}
					<span class="lines-label">lines</span>
				</div>
			</div>
			<div class="toolbar-right">
				<button class="btn-ghost" class:on={autoRefresh} on:click={toggleAutoRefresh}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
					{autoRefresh ? 'Live' : 'Auto'}
				</button>
				<button class="btn-ghost" on:click={scrollToTop}>Top</button>
				<button class="btn-ghost" on:click={scrollToBottom}>Bottom</button>
				<button class="btn-fill" on:click={fetchLogs} disabled={logsLoading}>{logsLoading ? 'Loading...' : 'Refresh'}</button>
			</div>
		</div>

		<div class="filters">
			<div class="filter-row">
				<div class="search-box">
					<svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
					<input type="text" class="search-input" placeholder="Search logs..." bind:value={searchQuery} />
				</div>

				<select class="sel" bind:value={ipFilter}>
					<option value="">All IPs</option>
					{#each uniqueIPs as ip}<option value={ip}>{ip}</option>{/each}
				</select>

				<div class="status-bar">
					{#each statusFilters as s}
						<button
							class="status-btn"
							class:active={statusFilter === s}
							class:st-2xx={s === '2xx' && statusFilter === '2xx'}
							class:st-3xx={s === '3xx' && statusFilter === '3xx'}
							class:st-4xx={s === '4xx' && statusFilter === '4xx'}
							class:st-5xx={s === '5xx' && statusFilter === '5xx'}
							on:click={() => statusFilter = statusFilter === s ? 'all' : s}
						>
							{s === 'all' ? 'All' : s}
							{#if s !== 'all'}
								<span class="cnt">{s === '2xx' ? filterStats.s2xx : s === '3xx' ? filterStats.s3xx : s === '4xx' ? filterStats.s4xx : filterStats.s5xx}</span>
							{/if}
						</button>
					{/each}
				</div>

				<div class="method-bar">
					{#each methodFilters as m}
						<button class="method-btn" class:active={methodFilter === m} on:click={() => methodFilter = methodFilter === m ? 'all' : m}>
							{m === 'all' ? 'All' : m}
						</button>
					{/each}
				</div>

				{#if hasActiveFilters}
					<button class="btn-ghost clear-btn" on:click={clearFilters}>
						<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
						Clear
					</button>
				{/if}
			</div>

			{#if parsedLines.length > 0}
				<div class="filter-meta">
					<span class="meta-count">{filteredLines.length}</span> of <span class="meta-count">{parsedLines.length}</span> entries
					{#if hasActiveFilters}&mdash; filtered{/if}
				</div>
			{/if}
		</div>

		<div class="logs-container">
			{#if logsLoading && !logs}
				<div class="placeholder"><div class="loader"></div><p>Fetching logs from agent...</p></div>
			{:else if !logs}
				<div class="placeholder"><p>No logs available. The agent may not have any nginx access logs yet.</p></div>
			{:else if filteredLines.length === 0 && hasActiveFilters}
				<div class="placeholder"><p>No log entries match your filters.</p><button class="btn-ghost" on:click={clearFilters}>Clear Filters</button></div>
			{:else}
				<div class="log-output">
					{#each filteredLines as line}
						<div class="log-line">
							<span class="l-ip">{line.ip}</span>
							<span class="l-method">{line.method}</span>
							<span class="l-path">{line.path}</span>
							<span class="l-status {statusClass(line.status)}">{line.status}</span>
							<span class="l-size">{line.size}</span>
							<span class="l-ua" title={line.ua}>{line.ua}</span>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		{#if error && logs}
			<p class="error-inline">{error}</p>
		{/if}
	{/if}
</div>

<style>
	.page { display: flex; flex-direction: column; height: calc(100vh - 0px); }
	.page-head { margin-bottom: 1rem; flex-shrink: 0; }
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }
	h1 { font-size: var(--text-xl); }

	.back-link {
		display: inline-flex; align-items: center; gap: 0.25rem;
		color: var(--text-tertiary); font-size: var(--text-sm); font-weight: 500;
		transition: color var(--transition);
	}
	.back-link:hover { color: var(--accent); }

	.agent-info { display: flex; align-items: center; gap: 0.5rem; }
	.status-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--danger); flex-shrink: 0; }
	.status-dot.online { background: #42c990; }
	.tag { font-size: var(--text-xs); color: var(--text-tertiary); background: var(--surface-raised); padding: 0.125rem 0.5rem; border-radius: 999px; }

	/* ── Toolbar ── */
	.toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.625rem; gap: 1rem; flex-wrap: wrap; flex-shrink: 0; }
	.toolbar-left { display: flex; align-items: center; gap: 0.5rem; }
	.toolbar-right { display: flex; align-items: center; gap: 0.375rem; }

	.line-selector {
		display: flex; align-items: center; background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 3px; gap: 2px;
	}
	.range-btn {
		background: transparent; border: none; padding: 0.3rem 0.5rem; border-radius: var(--radius);
		cursor: pointer; font-size: var(--text-xs); font-weight: 600; color: var(--text-tertiary);
		transition: all var(--transition);
	}
	.range-btn.active { background: var(--accent); color: #fff; }
	.range-btn:hover:not(.active) { color: var(--text-primary); background: var(--surface-raised); }
	.lines-label { font-size: var(--text-xs); color: var(--text-muted); padding: 0 0.375rem; }

	.btn-ghost.on { color: var(--accent); border-color: var(--accent); background: var(--accent-dim); }

	/* ── Filters ── */
	.filters { flex-shrink: 0; margin-bottom: 0.625rem; }

	.filter-row {
		display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;
	}

	.search-box {
		position: relative; flex: 1; min-width: 180px; max-width: 280px;
	}
	.search-icon {
		position: absolute; left: 0.625rem; top: 50%; transform: translateY(-50%);
		color: var(--text-muted); pointer-events: none;
	}
	.search-input {
		width: 100%; padding: 0.4rem 0.625rem 0.4rem 2rem;
		background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
		color: var(--text-primary); font-size: var(--text-xs);
	}
	.search-input::placeholder { color: var(--text-muted); }
	.search-input:focus { outline: none; border-color: var(--accent); }

	.sel {
		padding: 0.4rem 0.5rem; border: 1px solid var(--border); border-radius: var(--radius);
		font-size: var(--text-xs); color: var(--text-primary); background: var(--surface);
		cursor: pointer; min-width: 120px;
	}
	.sel:focus { outline: none; border-color: var(--accent); }

	.status-bar, .method-bar {
		display: flex; background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 2px; gap: 1px;
	}
	.status-btn, .method-btn {
		background: transparent; border: none; padding: 0.25rem 0.5rem; border-radius: 4px;
		cursor: pointer; font-size: 11px; font-weight: 600; color: var(--text-tertiary);
		transition: all var(--transition); display: flex; align-items: center; gap: 0.25rem;
	}
	.status-btn:hover:not(.active), .method-btn:hover:not(.active) { color: var(--text-primary); background: var(--surface-raised); }
	.status-btn.active { color: #fff; }
	.method-btn.active { background: var(--accent); color: #fff; }
	.status-btn.active:not(.st-2xx):not(.st-3xx):not(.st-4xx):not(.st-5xx) { background: var(--accent); }
	.status-btn.st-2xx { background: #42c990; }
	.status-btn.st-3xx { background: #6C8EEF; }
	.status-btn.st-4xx { background: #e8a840; }
	.status-btn.st-5xx { background: #ef6068; }

	.cnt { font-size: 10px; opacity: 0.8; font-weight: 500; }

	.clear-btn { color: var(--text-muted); }
	.clear-btn:hover { color: var(--danger); border-color: var(--danger); }

	.filter-meta {
		font-size: var(--text-xs); color: var(--text-muted); margin-top: 0.375rem; padding-left: 0.125rem;
	}
	.meta-count { font-weight: 600; color: var(--text-secondary); }

	/* ── Logs ── */
	.logs-container { flex: 1; min-height: 0; display: flex; flex-direction: column; }

	.log-output {
		background: #0d0f17; border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 0.5rem 0; margin: 0;
		font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', monospace;
		font-size: 11.5px; line-height: 1;
		flex: 1; overflow: auto;
	}

	.log-line {
		display: flex; align-items: baseline; gap: 0; padding: 0.25rem 1rem;
		border-bottom: 1px solid rgba(255,255,255,0.03);
		transition: background 0.1s;
	}
	.log-line:hover { background: rgba(255,255,255,0.04); }
	.log-line:last-child { border-bottom: none; }

	.l-ip { color: #8b9cc7; min-width: 130px; flex-shrink: 0; }
	.l-method { color: #a78bfa; min-width: 55px; flex-shrink: 0; font-weight: 600; }
	.l-path { color: #c8cdd8; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin-right: 0.75rem; }
	.l-status { font-weight: 700; min-width: 30px; text-align: right; margin-right: 0.75rem; flex-shrink: 0; }
	.l-size { color: #6b6f88; min-width: 55px; text-align: right; margin-right: 0.75rem; flex-shrink: 0; }
	.l-ua { color: #4a4e63; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 260px; flex-shrink: 1; }

	.st-2xx { color: #42c990; }
	.st-3xx { color: #6C8EEF; }
	.st-4xx { color: #e8a840; }
	.st-5xx { color: #ef6068; }

	.error-inline { color: var(--danger); font-size: var(--text-sm); margin-top: 0.5rem; }

	@media (max-width: 768px) {
		.toolbar { flex-direction: column; align-items: stretch; }
		.toolbar-right { justify-content: flex-end; }
		.filter-row { flex-direction: column; }
		.search-box { max-width: 100%; }
		.l-ua { display: none; }
		.l-ip { min-width: 100px; }
	}
</style>
