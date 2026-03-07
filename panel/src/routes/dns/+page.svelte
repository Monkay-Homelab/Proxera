<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { navRefresh } from '$lib/navRefresh';
	import { toastError } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';

	let providers = [];
	let loading = true;
	let showModal = false;
	let step = 'provider';
	let selectedProvider = '';
	let zoneId = '';
	let apiKey = '';
	let apiSecret = '';
	let domain = '';
	let saving = false;
	let error = '';

	onMount(() => {
		fetchProviders();
	});

	async function fetchProviders() {
		try {
			const response = await api('/api/dns/providers');
			if (response.ok) {
				providers = (await response.json()).sort((a, b) => (a.domain || '').localeCompare(b.domain || ''));
			}
		} catch (err) {
			toastError('Failed to fetch domains');
		} finally {
			loading = false;
		}
	}

	async function deleteProvider(id) {
		if (!await confirmDialog('Are you sure you want to remove this domain?', { title: 'Remove Domain', confirmLabel: 'Remove', danger: true })) return;

		try {
			const response = await api(`/api/dns/providers/${id}`, { method: 'DELETE' });
			if (response.ok) {
				providers = providers.filter(p => p.id !== id);
				navRefresh.update(n => n + 1);
			}
		} catch (err) {
			toastError('Failed to delete domain');
		}
	}

	function openModal() {
		showModal = true;
		step = 'provider';
		selectedProvider = '';
		zoneId = '';
		apiKey = '';
		apiSecret = '';
		domain = '';
		error = '';
	}

	function closeModal() {
		showModal = false;
	}

	function selectProvider(p) {
		selectedProvider = p;
		step = 'credentials';
	}

	function goBack() {
		step = 'provider';
		error = '';
	}

	function isSubmitDisabled() {
		if (saving) return true;
		if (!apiKey.trim()) return true;
		if (selectedProvider === 'cloudflare') return !zoneId.trim();
		if (selectedProvider === 'ionos') return !domain.trim();
		if (selectedProvider === 'porkbun') return !apiSecret.trim() || !domain.trim();
		return true;
	}

	async function handleSubmit() {
		saving = true;
		error = '';

		const body = { provider: selectedProvider, api_key: apiKey.trim() };
		if (selectedProvider === 'cloudflare') body.zone_id = zoneId.trim();
		if (selectedProvider === 'ionos') body.domain = domain.trim();
		if (selectedProvider === 'porkbun') { body.api_secret = apiSecret.trim(); body.domain = domain.trim(); }

		try {
			const response = await api('/api/dns/providers', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});

			if (!response.ok) {
				const data = await response.json();
				throw new Error(data.error || data.message || 'Failed to save provider');
			}

			await fetchProviders();
			navRefresh.update(n => n + 1);
			closeModal();
		} catch (err) {
			error = err.message;
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>DNS Management - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<h1>DNS Management</h1>
		<button class="btn-fill" onclick={openModal}>
			<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
			Add Domain
		</button>
	</header>

	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading domains...</p></div>
	{:else if providers.length > 0}
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Domain</th>
						<th>Provider</th>
						<th>Zone ID</th>
						<th>Added</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each providers as provider}
						<tr class="clickable-row" onclick={() => goto(`/dns/records/${provider.id}`)}>
							<td class="domain-cell">{provider.domain || '—'}</td>
							<td>
								{#if provider.provider === 'cloudflare'}
									<span class="provider-badge">
										<svg viewBox="0 0 64 64" width="14" height="14">
											<path d="M44.52 37.3l2.67-9.32a1.6 1.6 0 00-.06-1.1 1.57 1.57 0 00-.8-.78l-26.3-0.07a.47.47 0 01-.4-.24.49.49 0 01-.02-.47l.34-.93a.93.93 0 01.87-.63h27.33a8.1 8.1 0 007.74-5.74 10.14 10.14 0 00-5.21-11.73 15.8 15.8 0 00-29.86 5.04A10.44 10.44 0 0010 22.69a10.56 10.56 0 003.85 8.21.63.63 0 01.23.56l-1.43 5a.31.31 0 00.11.32.3.3 0 00.33.04l5.44-2.59" fill="#f6821f"/>
										</svg>
										Cloudflare
									</span>
								{:else if provider.provider === 'ionos'}
									<span class="provider-badge provider-badge-ionos">
										<svg viewBox="0 0 24 24" width="14" height="14" fill="none">
											<circle cx="12" cy="12" r="10" fill="#003D8F"/>
											<text x="12" y="16.5" text-anchor="middle" font-size="10" font-weight="700" font-family="sans-serif" fill="white">i</text>
										</svg>
										IONOS
									</span>
								{:else if provider.provider === 'porkbun'}
									<span class="provider-badge provider-badge-porkbun">
										<svg viewBox="0 0 14 14" width="14" height="14" fill="none">
											<rect width="14" height="14" rx="3" fill="#EF4A5B"/>
											<text x="7" y="10.5" text-anchor="middle" font-size="7" font-weight="800" font-family="sans-serif" fill="white">PB</text>
										</svg>
										Porkbun
									</span>
								{:else}
									<span class="provider-badge">{provider.provider}</span>
								{/if}
							</td>
							<td class="mono">
								{#if provider.provider === 'ionos' || provider.provider === 'porkbun'}
									<span class="dim">—</span>
								{:else}
									{provider.zone_id}
								{/if}
							</td>
							<td class="dim">{new Date(provider.created_at).toLocaleDateString()}</td>
							<td class="td-actions">
								<button class="act act-danger" onclick={(e) => { e.stopPropagation(); deleteProvider(provider.id); }} title="Delete" aria-label="Delete domain">
									<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<div class="placeholder">
			<p>No domains connected yet. Click "Add Domain" to get started.</p>
		</div>
	{/if}
</div>

{#if showModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={closeModal} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			{#if step === 'provider'}
				<h2>Select DNS Provider</h2>
				<p class="modal-sub">Choose your DNS provider to get started.</p>

				<div class="providers-grid">
					<button class="provider-card" onclick={() => selectProvider('cloudflare')}>
						<div class="provider-logo">
							<svg viewBox="0 0 64 64" width="48" height="48">
								<path d="M44.52 37.3l2.67-9.32a1.6 1.6 0 00-.06-1.1 1.57 1.57 0 00-.8-.78l-26.3-0.07a.47.47 0 01-.4-.24.49.49 0 01-.02-.47l.34-.93a.93.93 0 01.87-.63h27.33a8.1 8.1 0 007.74-5.74 10.14 10.14 0 00-5.21-11.73 15.8 15.8 0 00-29.86 5.04A10.44 10.44 0 0010 22.69a10.56 10.56 0 003.85 8.21.63.63 0 01.23.56l-1.43 5a.31.31 0 00.11.32.3.3 0 00.33.04l5.44-2.59" fill="#f6821f"/>
								<path d="M34.76 41.5a1.3 1.3 0 001.15-.82l.3-.84a.49.49 0 00-.02-.47.47.47 0 00-.4-.24H19.07a.47.47 0 01-.4-.24.49.49 0 01-.02-.47l.34-.93a.93.93 0 01.87-.63h17.72a.47.47 0 00.4-.24.49.49 0 00.02-.47l-.34-.93a.93.93 0 00-.87-.63H19.94a8.52 8.52 0 00-8.14 6.07l-.66 2.3a.31.31 0 00.11.32.3.3 0 00.22.09z" fill="#fbad41"/>
							</svg>
						</div>
						<div class="provider-info">
							<h3>Cloudflare</h3>
							<p>Manage DNS records via Cloudflare API</p>
						</div>
					</button>

					<button class="provider-card" onclick={() => selectProvider('ionos')}>
						<div class="provider-logo provider-logo-ionos">
							<svg viewBox="0 0 48 48" width="48" height="48" fill="none">
								<rect width="48" height="48" rx="8" fill="#003D8F"/>
								<text x="24" y="33" text-anchor="middle" font-size="26" font-weight="700" font-family="sans-serif" fill="white">i</text>
							</svg>
						</div>
						<div class="provider-info">
							<h3>IONOS</h3>
							<p>Manage DNS records via IONOS API</p>
						</div>
					</button>

					<button class="provider-card" onclick={() => selectProvider('porkbun')}>
						<div class="provider-logo provider-logo-porkbun">
							<svg viewBox="0 0 48 48" width="48" height="48" fill="none">
								<rect width="48" height="48" rx="8" fill="#EF4A5B"/>
								<text x="24" y="31" text-anchor="middle" font-size="13" font-weight="800" font-family="sans-serif" fill="white">PORK</text>
								<text x="24" y="42" text-anchor="middle" font-size="13" font-weight="800" font-family="sans-serif" fill="white">BUN</text>
							</svg>
						</div>
						<div class="provider-info">
							<h3>Porkbun</h3>
							<p>Manage DNS records via Porkbun API</p>
						</div>
					</button>

					<div class="provider-card disabled">
						<div class="provider-logo provider-logo-muted">
							<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
								<circle cx="12" cy="12" r="10"></circle>
								<line x1="12" y1="8" x2="12" y2="16"></line>
								<line x1="8" y1="12" x2="16" y2="12"></line>
							</svg>
						</div>
						<div class="provider-info">
							<h3>More Coming Soon</h3>
							<p>Additional providers will be added in future updates</p>
						</div>
					</div>
				</div>

			{:else if step === 'credentials'}
				<div class="modal-top">
					<button class="breadcrumb" onclick={goBack}>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
						Back
					</button>
					<h2>Connect {{ cloudflare: 'Cloudflare', ionos: 'IONOS', porkbun: 'Porkbun' }[selectedProvider]}</h2>
				</div>

				{#if selectedProvider === 'cloudflare'}
					<p class="modal-sub">Enter your Cloudflare Zone ID and API token to connect your domain.</p>
				{:else if selectedProvider === 'ionos'}
					<p class="modal-sub">Enter your IONOS API key and the domain name to connect.</p>
				{:else if selectedProvider === 'porkbun'}
					<p class="modal-sub">Enter your Porkbun API key, secret, and domain to connect.</p>
				{/if}

				{#if error}
					<div class="error-msg">{error}</div>
				{/if}

				{#if selectedProvider === 'cloudflare'}
					<div class="form-group">
						<label for="zone-id">Zone ID</label>
						<input
							id="zone-id"
							type="text"
							class="input"
							bind:value={zoneId}
							placeholder="e.g. 023e105f4ecef8ad9ca31a8372d0c353"
							autocomplete="off"
						/>
						<span class="form-hint">Found on your domain's Overview page in the Cloudflare dashboard.</span>
					</div>

					<div class="form-group">
						<label for="api-key">API Token</label>
						<input
							id="api-key"
							type="password"
							class="input"
							bind:value={apiKey}
							placeholder="Enter your Cloudflare API token"
							autocomplete="off"
						/>
						<span class="form-hint">Use a scoped API token with DNS edit permissions for best security.</span>
					</div>

				{:else if selectedProvider === 'ionos'}
					<div class="form-group">
						<label for="api-key">API Key</label>
						<input
							id="api-key"
							type="password"
							class="input"
							bind:value={apiKey}
							placeholder="e.g. prefix.secret"
							autocomplete="off"
						/>
						<span class="form-hint">Generated at <strong>developer.hosting.ionos.com</strong> — format is <code>prefix.secret</code>.</span>
					</div>

					<div class="form-group">
						<label for="ionos-domain">Domain</label>
						<input
							id="ionos-domain"
							type="text"
							class="input"
							bind:value={domain}
							placeholder="e.g. example.com"
							autocomplete="off"
						/>
						<span class="form-hint">The domain name as it appears in your IONOS account.</span>
					</div>

				{:else if selectedProvider === 'porkbun'}
					<div class="form-group">
						<label for="api-key">API Key</label>
						<input
							id="api-key"
							type="password"
							class="input"
							bind:value={apiKey}
							placeholder="pk1_..."
							autocomplete="off"
						/>
						<span class="form-hint">Found under <strong>API Access</strong> in your Porkbun account settings.</span>
					</div>

					<div class="form-group">
						<label for="api-secret">API Secret</label>
						<input
							id="api-secret"
							type="password"
							class="input"
							bind:value={apiSecret}
							placeholder="sk1_..."
							autocomplete="off"
						/>
						<span class="form-hint">The secret key paired with your API key.</span>
					</div>

					<div class="form-group">
						<label for="pb-domain">Domain</label>
						<input
							id="pb-domain"
							type="text"
							class="input"
							bind:value={domain}
							placeholder="e.g. example.com"
							autocomplete="off"
						/>
						<span class="form-hint">The domain name as registered in your Porkbun account.</span>
					</div>
				{/if}

				<div class="modal-foot">
					<button class="btn-ghost" onclick={goBack}>Cancel</button>
					<button
						class="btn-fill"
						onclick={handleSubmit}
						disabled={isSubmitDisabled()}
					>
						{saving ? 'Connecting...' : 'Connect Domain'}
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	/* ── Table cells ── */
	.clickable-row { cursor: pointer; }
	.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }

	.provider-badge {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.125rem 0.5rem;
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 500; color: var(--text-secondary);
	}
	.provider-badge-ionos { color: #003D8F; border-color: #003D8F33; background: #003D8F0d; }
	.provider-badge-porkbun { color: #EF4A5B; border-color: #EF4A5B33; background: #EF4A5B0d; }
	.provider-logo-porkbun { background: transparent; border-color: transparent; }

	.td-actions { text-align: right; }

	/* ── Modal extras ── */
	.modal-top { display: flex; flex-direction: column; gap: 0.5rem; margin-bottom: 0.375rem; }

	/* ── Provider cards ── */
	.providers-grid {
		display: grid; grid-template-columns: repeat(2, 1fr); gap: 1rem;
	}
	.provider-card {
		display: flex; flex-direction: column; align-items: center;
		gap: 1rem; padding: 2rem 1.5rem;
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); cursor: pointer;
		transition: all var(--transition); text-align: center;
		width: 100%; font-family: inherit;
	}
	.provider-card:hover:not(.disabled) { border-color: var(--accent); background: var(--accent-dim); }
	.provider-card.disabled { opacity: 0.35; cursor: not-allowed; }

	.provider-logo {
		width: 56px; height: 56px; min-width: 56px;
		display: flex; align-items: center; justify-content: center;
		border-radius: var(--radius); background: var(--bg); border: 1px solid var(--border);
		overflow: hidden;
	}
	.provider-logo-ionos { background: transparent; border-color: transparent; }
	.provider-logo-muted { color: var(--text-tertiary); }

	.provider-info h3 { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); margin: 0 0 0.25rem; }
	.provider-info p { font-size: var(--text-xs); color: var(--text-secondary); margin: 0; }

	/* ── Form ── */
	.form-group { margin-bottom: 1.25rem; }
	.form-group label {
		display: block; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); margin-bottom: 0.5rem;
		text-transform: uppercase; letter-spacing: 0.04em;
	}
	.form-hint { display: block; font-size: var(--text-xs); color: var(--text-tertiary); margin-top: 0.375rem; }
	.form-hint code { font-family: var(--font-mono); background: var(--surface-raised); padding: 0.1em 0.3em; border-radius: 3px; }

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.providers-grid { grid-template-columns: 1fr; }
	}
</style>
