<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let domains = [];
	let loading = true;

	onMount(() => {
		fetchDomains();
	});

	async function fetchDomains() {
		try {
			const response = await api('/api/dns/providers');
			if (response.ok) {
				domains = (await response.json()).sort((a, b) => (a.domain || '').localeCompare(b.domain || ''));
			}
		} catch (err) {
			console.error('Failed to fetch domains:', err);
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Hosts - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<h1>Hosts</h1>
		<button class="btn-fill" onclick={() => goto('/hosts/all')}>
			<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
			All Hosts
		</button>
	</header>

	{#if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading domains...</p></div>
	{:else if domains.length > 0}
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Domain</th>
						<th>Provider</th>
						<th>Added</th>
					</tr>
				</thead>
				<tbody>
					{#each domains as domain}
						<tr class="clickable-row" onclick={() => goto(`/hosts/${domain.id}`)}>
							<td class="domain-cell">{domain.domain || '—'}</td>
							<td>
								{#if domain.provider === 'cloudflare'}
									<span class="provider-badge">
										<svg viewBox="0 0 64 64" width="14" height="14">
											<path d="M44.52 37.3l2.67-9.32a1.6 1.6 0 00-.06-1.1 1.57 1.57 0 00-.8-.78l-26.3-0.07a.47.47 0 01-.4-.24.49.49 0 01-.02-.47l.34-.93a.93.93 0 01.87-.63h27.33a8.1 8.1 0 007.74-5.74 10.14 10.14 0 00-5.21-11.73 15.8 15.8 0 00-29.86 5.04A10.44 10.44 0 0010 22.69a10.56 10.56 0 003.85 8.21.63.63 0 01.23.56l-1.43 5a.31.31 0 00.11.32.3.3 0 00.33.04l5.44-2.59" fill="#f6821f"/>
										</svg>
										Cloudflare
									</span>
								{:else if domain.provider === 'ionos'}
									<span class="provider-badge provider-badge-ionos">
										<svg viewBox="0 0 24 24" width="14" height="14" fill="none">
											<circle cx="12" cy="12" r="10" fill="#003D8F"/>
											<text x="12" y="16.5" text-anchor="middle" font-size="10" font-weight="700" font-family="sans-serif" fill="white">i</text>
										</svg>
										IONOS
									</span>
								{:else if domain.provider === 'porkbun'}
									<span class="provider-badge provider-badge-porkbun">
										<svg viewBox="0 0 14 14" width="14" height="14" fill="none">
											<rect width="14" height="14" rx="3" fill="#EF4A5B"/>
											<text x="7" y="10.5" text-anchor="middle" font-size="7" font-weight="800" font-family="sans-serif" fill="white">PB</text>
										</svg>
										Porkbun
									</span>
								{:else}
									<span class="provider-badge">{domain.provider}</span>
								{/if}
							</td>
							<td class="date-cell">{new Date(domain.created_at).toLocaleDateString()}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<div class="placeholder">
			<p>No domains available. <a href="/dns">Add a domain via DNS Management</a> to get started.</p>
		</div>
	{/if}
</div>

<style>
	.clickable-row { cursor: pointer; }
	.date-cell { color: var(--text-tertiary); }

	.provider-badge {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.125rem 0.5rem;
		background: rgba(246, 130, 31, 0.1);
		border: 1px solid rgba(246, 130, 31, 0.25);
		border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 500; color: #f6821f;
	}
	.provider-badge-ionos { color: #003D8F; border-color: #003D8F33; background: #003D8F0d; }
	.provider-badge-porkbun { color: #EF4A5B; border-color: #EF4A5B33; background: #EF4A5B0d; }

	.placeholder a { color: var(--accent); font-weight: 500; }
	.placeholder a:hover { color: var(--accent-bright); }
</style>
