<script>
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { toastError, toastSuccess } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import { formatDate, copyToClipboard } from '$lib/utils';

	let certificates = [];
	let providers = [];
	let loading = true;
	let showIssueModal = false;
	let showViewModal = false;
	let viewCert = null;
	let error = '';
	let privateKeyRevealed = false;
	let pollTimer = null;

	// Issue form
	let selectedProviderId = '';
	let domainInput = '';
	let includeRoot = true;
	let isWildcard = false;

	onMount(() => {
		fetchCertificates();
		fetchProviders();
	});

	async function fetchCertificates() {
		try {
			const response = await api('/api/certificates');
			if (response.ok) {
				certificates = (await response.json()).sort((a, b) => (a.domain || '').localeCompare(b.domain || ''));
				if (certificates.some(c => c.status === 'pending')) {
					startPolling();
				}
			}
		} catch (err) {
			toastError('Failed to fetch certificates');
		} finally {
			loading = false;
		}
	}

	async function fetchProviders() {
		try {
			const response = await api('/api/dns/providers');
			if (response.ok) {
				providers = await response.json();
			}
		} catch (err) {
			toastError('Failed to fetch providers');
		}
	}

	function getSelectedProvider() {
		return providers.find(p => p.id === Number(selectedProviderId));
	}

	function openIssueModal() {
		showIssueModal = true;
		selectedProviderId = '';
		domainInput = '';
		includeRoot = true;
		isWildcard = false;
		error = '';
	}

	function closeIssueModal() {
		showIssueModal = false;
	}

	function startPolling() {
		stopPolling();
		pollTimer = setInterval(async () => {
			const hasPending = certificates.some(c => c.status === 'pending');
			if (!hasPending) {
				stopPolling();
				return;
			}
			try {
				const response = await api('/api/certificates');
				if (response.ok) {
					const updated = (await response.json()).sort((a, b) => (a.domain || '').localeCompare(b.domain || ''));
					// Check for status changes to notify user
					for (const cert of updated) {
						const old = certificates.find(c => c.id === cert.id);
						if (old && old.status === 'pending' && cert.status === 'active') {
							toastSuccess(`Certificate for ${cert.domain} issued successfully`);
						} else if (old && old.status === 'pending' && cert.status === 'error') {
							toastError(`Certificate for ${cert.domain} failed`);
						}
					}
					certificates = updated;
				}
			} catch {}
		}, 5000);
	}

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	}

	onDestroy(() => stopPolling());

	async function handleIssue() {
		const provider = getSelectedProvider();
		if (!provider || !selectedProviderId) return;

		let domains = [];
		const providerDomain = provider.domain;

		if (domainInput.trim()) {
			const full = `${domainInput.trim()}.${providerDomain}`;
			if (isWildcard) {
				domains = [full, `*.${full}`];
			} else {
				domains = [full];
			}
			if (includeRoot) {
				domains.push(providerDomain);
			}
		} else {
			if (isWildcard) {
				domains = [providerDomain, `*.${providerDomain}`];
			} else {
				domains = [providerDomain];
			}
		}

		error = '';

		try {
			const response = await api('/api/certificates', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					provider_id: Number(selectedProviderId),
					domains
				})
			});

			if (!response.ok) {
				const data = await response.json();
				throw new Error(data.message || data.error || 'Failed to issue certificate');
			}

			const pendingCert = await response.json();
			certificates = [...certificates, pendingCert].sort((a, b) => (a.domain || '').localeCompare(b.domain || ''));
			closeIssueModal();
			startPolling();
		} catch (err) {
			error = err.message;
		}
	}

	async function retryCertificate(certId) {
		try {
			const response = await api(`/api/certificates/${certId}/retry`, { method: 'POST' });
			if (!response.ok) {
				const data = await response.json();
				throw new Error(data.error || 'Retry failed');
			}
			// Update local state to pending
			certificates = certificates.map(c =>
				c.id === certId ? { ...c, status: 'pending' } : c
			);
			startPolling();
		} catch (err) {
			toastError(err.message);
		}
	}

	async function viewCertificate(cert) {
		try {
			const response = await api(`/api/certificates/${cert.id}`);
			if (response.ok) {
				viewCert = await response.json();
				showViewModal = true;
			}
		} catch (err) {
			toastError('Failed to fetch certificate details');
		}
	}

	function closeViewModal() {
		showViewModal = false;
		viewCert = null;
		privateKeyRevealed = false;
	}

	async function revealPrivateKey() {
		try {
			const response = await api(`/api/certificates/${viewCert.id}?include_key=true`);
			if (response.ok) {
				const data = await response.json();
				viewCert.private_key_pem = data.private_key_pem;
				privateKeyRevealed = true;
			}
		} catch (err) {
			toastError('Failed to fetch private key');
		}
	}

	async function deleteCertificate(id) {
		if (!await confirmDialog('Are you sure you want to delete this certificate?', { title: 'Delete Certificate', confirmLabel: 'Delete', danger: true })) return;

		try {
			const response = await api(`/api/certificates/${id}`, { method: 'DELETE' });
			if (response.ok) {
				certificates = certificates.filter(c => c.id !== id);
			}
		} catch (err) {
			toastError('Failed to delete certificate');
		}
	}

	function statusClass(status) {
		switch (status) {
			case 'active': return 'badge-ok';
			case 'pending': return 'badge-warn';
			case 'expiring': return 'badge-warn';
			case 'expired': return 'badge-bad';
			case 'error': return 'badge-bad';
			default: return '';
		}
	}
</script>

<svelte:head>
	<title>Certificates - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<h1>Certificates</h1>
		<button class="btn-fill" onclick={openIssueModal}>
			<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
			New Certificate
		</button>
	</header>

	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading certificates...</p></div>
	{:else if certificates.length > 0}
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Domain</th>
						<th>Status</th>
						<th>Issued</th>
						<th>Expires</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each certificates as cert}
						<tr class:pending-row={cert.status === 'pending'}>
							<td class="domain-cell">
								{cert.domain}
								{#if cert.san}
									<span class="san-tag">{cert.san.split(',').length + 1} domains</span>
								{/if}
							</td>
							<td>
								{#if cert.status === 'pending'}
									<span class="status-badge badge-warn pending-badge">
										<span class="spinner"></span>
										issuing
									</span>
								{:else}
									<span class="status-badge {statusClass(cert.status)}">{cert.status}</span>
								{/if}
							</td>
							<td class="dim">{cert.status === 'pending' ? '—' : formatDate(cert.issued_at)}</td>
							<td class="dim">{cert.status === 'pending' ? '—' : formatDate(cert.expires_at)}</td>
							<td class="td-actions">
								{#if cert.status === 'pending'}
									<!-- no actions while issuing -->
								{:else if cert.status === 'error'}
									<button class="act act-warn" onclick={() => retryCertificate(cert.id)} title="Retry" aria-label="Retry certificate">
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
									</button>
									<button class="act act-danger" onclick={() => deleteCertificate(cert.id)} title="Delete" aria-label="Delete certificate">
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
									</button>
								{:else}
									<button class="act act-accent" onclick={() => viewCertificate(cert)} title="View" aria-label="View certificate">
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
									</button>
									<button class="act act-danger" onclick={() => deleteCertificate(cert.id)} title="Delete" aria-label="Delete certificate">
										<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<div class="placeholder">
			<p>No certificates yet. Click "New Certificate" to issue your first SSL certificate.</p>
		</div>
	{/if}
</div>

<!-- Issue Certificate Modal -->
{#if showIssueModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={closeIssueModal} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<h2>Issue New Certificate</h2>
			<p class="modal-sub">Issue a Let's Encrypt SSL certificate using DNS-01 challenge.</p>

			{#if error}
				<div class="error-msg">{error}</div>
			{/if}

			<div class="form-group">
				<label for="provider">DNS Provider</label>
				<select id="provider" class="input" bind:value={selectedProviderId}>
					<option value="">Select a provider...</option>
					{#each providers as provider}
						<option value={provider.id}>{provider.domain} ({provider.provider})</option>
					{/each}
				</select>
			</div>

			<div class="form-group">
				<label for="domain">Subdomain (optional)</label>
				<div class="domain-input-wrap">
					<input
						id="domain"
						type="text"
						bind:value={domainInput}
						placeholder="e.g. app"
						autocomplete="off"
					/>
					{#if getSelectedProvider()}
						<span class="domain-suffix">.{getSelectedProvider().domain}</span>
					{/if}
				</div>
				<span class="form-hint">Leave empty to issue for the root domain only.</span>
			</div>

			<div class="form-group checkbox-group">
				<label class="checkbox-label">
					<input type="checkbox" bind:checked={includeRoot} />
					<span>Include root domain (@)</span>
				</label>
				<span class="form-hint">Also cover the bare domain alongside the subdomain.</span>
			</div>

			<div class="form-group checkbox-group">
				<label class="checkbox-label">
					<input type="checkbox" bind:checked={isWildcard} />
					<span>Include wildcard (*.domain)</span>
				</label>
				<span class="form-hint">Issues a certificate that covers all subdomains.</span>
			</div>

			{#if selectedProviderId}
				<div class="preview-box">
					<div class="preview-label">Certificate will cover:</div>
					{#if domainInput.trim()}
						<div class="preview-domain">{domainInput.trim()}.{getSelectedProvider()?.domain}</div>
						{#if isWildcard}
							<div class="preview-domain">*.{domainInput.trim()}.{getSelectedProvider()?.domain}</div>
						{/if}
						{#if includeRoot}
							<div class="preview-domain">{getSelectedProvider()?.domain}</div>
						{/if}
					{:else}
						<div class="preview-domain">{getSelectedProvider()?.domain}</div>
						{#if isWildcard}
							<div class="preview-domain">*.{getSelectedProvider()?.domain}</div>
						{/if}
					{/if}
				</div>
			{/if}

			<div class="modal-foot">
				<button class="btn-ghost" onclick={closeIssueModal}>Cancel</button>
				<button
					class="btn-fill"
					onclick={handleIssue}
					disabled={!selectedProviderId}
				>
					Issue Certificate
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- View Certificate Modal -->
{#if showViewModal && viewCert}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={closeViewModal} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal modal-wide" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<h2>Certificate: {viewCert.domain}</h2>
			<p class="modal-sub">Certificate details and PEM data</p>

			<div class="cert-details">
				<div class="detail-row">
					<span class="lbl">Status</span>
					<span class="status-badge {statusClass(viewCert.status)}">{viewCert.status}</span>
				</div>
				<div class="detail-row">
					<span class="lbl">Domain</span>
					<span class="val">{viewCert.domain}</span>
				</div>
				{#if viewCert.san}
					<div class="detail-row">
						<span class="lbl">SANs</span>
						<span class="val mono">{viewCert.san}</span>
					</div>
				{/if}
				<div class="detail-row">
					<span class="lbl">Issued</span>
					<span class="val">{formatDate(viewCert.issued_at)}</span>
				</div>
				<div class="detail-row">
					<span class="lbl">Expires</span>
					<span class="val">{formatDate(viewCert.expires_at)}</span>
				</div>
			</div>

			{#if viewCert.certificate_pem}
				<div class="pem-section">
					<div class="pem-head">
						<h3>Certificate (PEM)</h3>
						<button class="btn-ghost" onclick={() => copyToClipboard(viewCert.certificate_pem)}>Copy</button>
					</div>
					<pre class="pem-content">{viewCert.certificate_pem}</pre>
				</div>
			{/if}

			<div class="pem-section">
				<div class="pem-head">
					<h3>Private Key (PEM)</h3>
					{#if privateKeyRevealed && viewCert.private_key_pem}
						<button class="btn-ghost" onclick={() => copyToClipboard(viewCert.private_key_pem)}>Copy</button>
					{/if}
				</div>
				{#if privateKeyRevealed && viewCert.private_key_pem}
					<pre class="pem-content">{viewCert.private_key_pem}</pre>
				{:else}
					<div class="key-reveal">
						<p>Private key is hidden for security. Click to reveal.</p>
						<button class="btn-ghost btn-reveal" onclick={revealPrivateKey}>Reveal Private Key</button>
					</div>
				{/if}
			</div>

			{#if viewCert.issuer_pem}
				<div class="pem-section">
					<div class="pem-head">
						<h3>Issuer Certificate (PEM)</h3>
						<button class="btn-ghost" onclick={() => copyToClipboard(viewCert.issuer_pem)}>Copy</button>
					</div>
					<pre class="pem-content">{viewCert.issuer_pem}</pre>
				</div>
			{/if}

			<div class="modal-foot">
				<button class="btn-ghost" onclick={closeViewModal}>Close</button>
			</div>
		</div>
	</div>
{/if}

<style>
	/* ── Table cells ── */
	.domain-cell {
		display: flex; align-items: center; gap: 0.5rem;
	}
	.san-tag {
		font-size: var(--text-xs); font-weight: 500;
		padding: 0.125rem 0.4rem;
		background: var(--accent-dim); color: var(--accent);
		border-radius: var(--radius); font-family: var(--font-sans);
	}
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); }

	/* ── Status badges ── */
	.status-badge {
		display: inline-block; padding: 0.125rem 0.5rem;
		border-radius: var(--radius); font-size: var(--text-xs);
		font-weight: 600; text-transform: capitalize;
	}
	.badge-ok { background: var(--success-dim); color: var(--success); }
	.badge-warn { background: var(--warning-dim); color: var(--warning); }
	.badge-bad { background: var(--danger-dim); color: var(--danger); }

	/* ── Actions ── */
	.td-actions { text-align: right; white-space: nowrap; }
	.act-warn { color: var(--warning); }
	.act-warn:hover { background: var(--warning-dim); }

	/* ── Modal ── */
	.modal-wide { max-width: 900px; }

	/* ── Form ── */
	.form-group { margin-bottom: 1.25rem; }
	.form-group label {
		display: block; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); margin-bottom: 0.5rem;
		text-transform: uppercase; letter-spacing: 0.04em;
	}
	.form-hint { display: block; font-size: var(--text-xs); color: var(--text-tertiary); margin-top: 0.375rem; }

	.domain-input-wrap {
		display: flex; align-items: center;
		border: 1px solid var(--border); border-radius: var(--radius);
		overflow: hidden; background: var(--bg);
		transition: border-color var(--transition);
	}
	.domain-input-wrap:focus-within { border-color: var(--accent); }
	.domain-input-wrap input {
		border: none; border-radius: 0; flex: 1; background: transparent;
		padding: 0.625rem 0.875rem; font-size: var(--text-sm);
		color: var(--text-primary); outline: none;
	}
	.domain-suffix {
		padding: 0.625rem 0.875rem; background: var(--surface);
		color: var(--text-tertiary); font-size: var(--text-sm);
		border-left: 1px solid var(--border); white-space: nowrap;
		font-family: var(--font-mono);
	}

	.checkbox-group { margin-bottom: 1.25rem; }
	.checkbox-label {
		display: flex !important; align-items: center; gap: 0.5rem;
		cursor: pointer; font-weight: 500 !important;
		color: var(--text-secondary) !important;
		text-transform: none !important; letter-spacing: normal !important;
		font-size: var(--text-sm) !important;
	}
	.checkbox-label input[type="checkbox"] { width: auto; accent-color: var(--accent); }

	.preview-box {
		background: var(--bg); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 0.75rem 1rem; margin-bottom: 1.25rem;
	}
	.preview-label {
		font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); text-transform: uppercase;
		letter-spacing: 0.04em; margin-bottom: 0.375rem;
	}
	.preview-domain {
		font-family: var(--font-mono); font-size: var(--text-sm);
		color: var(--accent); padding: 0.125rem 0;
	}

	/* ── Pending row ── */
	.pending-row {
		opacity: 0.7;
	}

	.pending-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
	}

	.spinner {
		width: 10px;
		height: 10px;
		border: 2px solid var(--warning);
		border-top-color: transparent;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	/* ── View modal ── */
	.cert-details {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 0.75rem 1rem; margin-bottom: 1.5rem;
	}
	.detail-row {
		display: flex; align-items: center; gap: 1rem;
		padding: 0.4375rem 0; border-bottom: 1px solid var(--border);
	}
	.detail-row:last-child { border-bottom: none; }
	.lbl {
		font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); text-transform: uppercase;
		letter-spacing: 0.04em; width: 80px; flex-shrink: 0;
	}
	.val { color: var(--text-primary); font-size: var(--text-sm); font-weight: 500; }
	.val.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }

	.pem-section { margin-bottom: 1.25rem; }
	.pem-head {
		display: flex; align-items: center; justify-content: space-between;
		margin-bottom: 0.375rem;
	}
	.pem-head h3 { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); margin: 0; }
	.pem-content {
		background: var(--bg); color: var(--text-secondary);
		border: 1px solid var(--border); padding: 0.75rem 1rem;
		border-radius: var(--radius); font-size: var(--text-xs);
		line-height: 1.5; overflow-x: auto; max-height: 200px;
		overflow-y: auto; white-space: pre-wrap; word-break: break-all;
		margin: 0; font-family: var(--font-mono);
	}

	/* ── Key reveal ── */
	.key-reveal {
		background: var(--bg); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 1.25rem;
		text-align: center;
	}
	.key-reveal p {
		color: var(--text-tertiary); font-size: var(--text-sm);
		margin: 0 0 0.75rem;
	}
	.btn-reveal {
		border-color: var(--warning); color: var(--warning);
	}
	.btn-reveal:hover { background: var(--warning-dim); color: var(--warning); }
</style>
