<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	let agentId = $page.params.agentId;
	let agentName = '';
	let logs = '';
	let loading = true;
	let error = null;

	onMount(async () => {
		await fetchLogs();
	});

	async function fetchLogs() {
		loading = true;
		error = null;

		try {
			const agentResponse = await api(`/api/agents/${agentId}`);
			if (agentResponse.ok) {
				const agentData = await agentResponse.json();
				agentName = agentData.name;
			}

			const logsResponse = await api(`/api/agents/${agentId}/logs`);
			if (!logsResponse.ok) {
				const data = await logsResponse.json();
				throw new Error(data.error || 'Failed to fetch logs');
			}

			const result = await logsResponse.json();
			logs = result.logs || 'No logs available';
		} catch (err) {
			error = err.message;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{agentName || 'Agent'} Logs - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<div class="head-left">
			<button class="breadcrumb" on:click={() => goto('/agents')}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
				Agents
			</button>
			<h1>{agentName || 'Agent'} Logs</h1>
		</div>
		<button class="btn-fill" on:click={fetchLogs} disabled={loading}>
			{#if loading}
				Loading...
			{:else}
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
				Refresh
			{/if}
		</button>
	</header>

	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Fetching logs from agent...</p></div>
	{:else if error}
		<div class="placeholder error">
			<p>{error}</p>
			<button class="btn-fill" on:click={fetchLogs}>Retry</button>
		</div>
	{:else}
		<div class="logs-panel">
			<pre class="logs-content">{logs}</pre>
		</div>
	{/if}
</div>

<style>
	/* Page-specific — shared .page, .breadcrumb, .btn-fill, .placeholder, .loader in global.css */
	.page-head { align-items: flex-start; }
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }

	.logs-panel {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); overflow: hidden;
	}
	.logs-content {
		background: var(--bg); color: var(--text-secondary);
		padding: 1.5rem; margin: 0;
		font-size: var(--text-xs); line-height: 1.7;
		overflow-x: auto; white-space: pre-wrap; word-break: break-all;
		font-family: var(--font-mono);
		min-height: 600px; border: none;
	}

	@media (max-width: 768px) {
		.btn-fill { width: 100%; justify-content: center; }
		.logs-content { padding: 1rem; }
	}
</style>
