<script lang="ts">
	import { onMount } from 'svelte';
	import { api, apiJson } from '$lib/api';
	import { toastSuccess, toastError } from '$lib/components/toast';
	import { navRefresh } from '$lib/navRefresh';

	let loading = $state(true);
	let saving = $state(false);

	// Registration
	let registrationMode = $state('open');
	let inviteCode = $state('');

	// URLs
	let publicSiteUrl = $state('');
	let publicApiUrl = $state('');
	let publicWsUrl = $state('');

	// Email
	let smtpHost = $state('');
	let smtpPort = $state('465');
	let smtpUser = $state('');
	let smtpPassword = $state('');
	let smtpFromEmail = $state('');
	let emailVerification = $state(false);

	// SSL
	let acmeStaging = $state(false);

	// DNS Export/Import
	let showExportModal = $state(false);
	let showImportModal = $state(false);
	let exportPassword = $state('');
	let importPassword = $state('');
	let importFile = $state(null);
	let importData = $state(null);
	let exporting = $state(false);
	let importing = $state(false);
	let exportError = $state('');
	let importError = $state('');
	let importResult = $state(null);

	onMount(async () => {
		try {
			const data: any = await apiJson('/api/admin/settings');
			const s = data.settings || {};
			registrationMode = s.registration_mode || 'open';
			inviteCode = s.invite_code || '';
			publicSiteUrl = s.PUBLIC_SITE_URL || '';
			publicApiUrl = s.PUBLIC_API_URL || '';
			publicWsUrl = s.PUBLIC_WS_URL || '';
			smtpHost = s.SMTP_HOST || '';
			smtpPort = s.SMTP_PORT || '465';
			smtpUser = s.SMTP_USER || '';
			smtpPassword = s.SMTP_PASSWORD || '';
			smtpFromEmail = s.SMTP_FROM_EMAIL || '';
			emailVerification = s.ENABLE_EMAIL_VERIFICATION === 'true';
			acmeStaging = s.ACME_STAGING === 'true';
		} catch (e: any) {
			toastError(e.message || 'Failed to load settings');
		} finally {
			loading = false;
		}
	});

	function openExportModal() {
		showExportModal = true;
		exportPassword = '';
		exportError = '';
	}

	function openImportModal() {
		showImportModal = true;
		importPassword = '';
		importFile = null;
		importData = null;
		importError = '';
		importResult = null;
	}

	async function handleExport() {
		exportError = '';
		exporting = true;
		try {
			const response = await api('/api/dns/export', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ password: exportPassword })
			});
			const data = await response.json();
			if (!response.ok) throw new Error(data.error || 'Export failed');

			const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `proxera-backup-${new Date().toISOString().slice(0, 10)}.json`;
			a.click();
			URL.revokeObjectURL(url);
			showExportModal = false;
		} catch (err: any) {
			exportError = err.message;
		} finally {
			exporting = false;
		}
	}

	function handleFileSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		importFile = file;
		importError = '';
		importResult = null;
		const reader = new FileReader();
		reader.onload = (ev) => {
			try {
				importData = JSON.parse(ev.target?.result as string);
				if (!importData.ciphertext || !importData.salt) {
					throw new Error('Not a valid Proxera backup file');
				}
			} catch (err: any) {
				importError = err.message;
				importData = null;
			}
		};
		reader.readAsText(file);
	}

	async function handleImport() {
		importError = '';
		importResult = null;
		importing = true;
		try {
			const response = await api('/api/dns/import', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ password: importPassword, backup: importData })
			});
			const data = await response.json();
			if (!response.ok) throw new Error(data.error || 'Import failed');
			importResult = data;
			navRefresh.update(n => n + 1);
		} catch (err: any) {
			importError = err.message;
		} finally {
			importing = false;
		}
	}

	async function save() {
		saving = true;
		try {
			const s: Record<string, string> = {
				registration_mode: registrationMode,
				PUBLIC_SITE_URL: publicSiteUrl,
				PUBLIC_API_URL: publicApiUrl,
				PUBLIC_WS_URL: publicWsUrl,
				SMTP_HOST: smtpHost,
				SMTP_PORT: smtpPort,
				SMTP_USER: smtpUser,
				SMTP_FROM_EMAIL: smtpFromEmail,
				ENABLE_EMAIL_VERIFICATION: emailVerification ? 'true' : 'false',
				ACME_STAGING: acmeStaging ? 'true' : 'false',
			};
			if (registrationMode === 'invite') {
				s.invite_code = inviteCode;
			}
			// Only send password if changed (non-empty)
			if (smtpPassword) {
				s.SMTP_PASSWORD = smtpPassword;
			}
			await apiJson('/api/admin/settings', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(s),
			});
			toastSuccess('Settings saved');
		} catch (e: any) {
			toastError(e.message || 'Failed to save settings');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Settings - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Settings</h1>
	</div>

	{#if loading}
		<div class="empty-state">Loading...</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); save(); }}>
			<div class="settings-grid">
				<!-- Left column -->
				<div class="settings-col">
					<!-- Registration -->
					<div class="card">
						<h2 class="card-title">Registration</h2>
						<p class="card-desc">Control how new users can sign up.</p>

						<label class="field">
							<span class="field-label">Registration Mode</span>
							<select class="input" bind:value={registrationMode}>
								<option value="open">Open — anyone can register</option>
								<option value="invite">Invite — requires an invite code</option>
								<option value="disabled">Disabled — no new registrations</option>
							</select>
						</label>

						{#if registrationMode === 'invite'}
							<label class="field">
								<span class="field-label">Invite Code</span>
								<input class="input" type="text" bind:value={inviteCode} placeholder="Enter the invite code users must provide" />
							</label>
						{/if}
					</div>

					<!-- Email / SMTP -->
					<div class="card">
						<h2 class="card-title">Email (SMTP)</h2>
						<p class="card-desc">Outgoing email for notifications and verification.</p>

						<div class="field-row">
							<label class="field flex-grow">
								<span class="field-label">SMTP Host</span>
								<input class="input" type="text" bind:value={smtpHost} placeholder="smtp.example.com" />
							</label>

							<label class="field field-sm">
								<span class="field-label">Port</span>
								<input class="input" type="text" bind:value={smtpPort} placeholder="465" />
							</label>
						</div>

						<label class="field">
							<span class="field-label">Username</span>
							<input class="input" type="text" bind:value={smtpUser} placeholder="user@example.com" />
						</label>

						<label class="field">
							<span class="field-label">Password</span>
							<input class="input" type="password" bind:value={smtpPassword} placeholder="••••••••" />
						</label>

						<label class="field">
							<span class="field-label">From Email</span>
							<input class="input" type="email" bind:value={smtpFromEmail} placeholder="noreply@example.com" />
						</label>

						<label class="toggle-field">
							<input type="checkbox" bind:checked={emailVerification} />
							<span>Require email verification for new accounts</span>
						</label>
					</div>
				</div>

				<!-- Right column -->
				<div class="settings-col">
					<!-- URLs -->
					<div class="card">
						<h2 class="card-title">URLs</h2>
						<p class="card-desc">Public-facing URLs for email links and agent connections. Leave empty for same-origin.</p>

						<label class="field">
							<span class="field-label">Site URL</span>
							<input class="input" type="url" bind:value={publicSiteUrl} placeholder="https://proxera.example.com" />
						</label>

						<label class="field">
							<span class="field-label">API URL</span>
							<input class="input" type="url" bind:value={publicApiUrl} placeholder="https://proxera.example.com" />
						</label>

						<label class="field">
							<span class="field-label">WebSocket URL</span>
							<input class="input" type="url" bind:value={publicWsUrl} placeholder="wss://proxera.example.com/ws" />
						</label>
					</div>

					<!-- SSL -->
					<div class="card">
						<h2 class="card-title">SSL</h2>
						<p class="card-desc">Certificate settings for Let's Encrypt.</p>

						<label class="toggle-field">
							<input type="checkbox" bind:checked={acmeStaging} />
							<span>Use Let's Encrypt staging (for testing — certs won't be trusted)</span>
						</label>
					</div>

					<!-- Backup -->
					<div class="card">
						<h2 class="card-title">Backup</h2>
						<p class="card-desc">Export or import DNS providers, hosts, and SSL certificates with encrypted credentials.</p>

						<div class="backup-actions">
							<button type="button" class="btn-ghost" onclick={openExportModal}>
								<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
								Export
							</button>
							<button type="button" class="btn-ghost" onclick={openImportModal}>
								<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
								Import
							</button>
						</div>
					</div>
				</div>
			</div>

			<div class="form-actions">
				<button type="submit" class="btn-fill" disabled={saving}>
					{saving ? 'Saving...' : 'Save Settings'}
				</button>
			</div>
		</form>
	{/if}
</div>

{#if showExportModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={() => showExportModal = false} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal modal-sm" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<h2>Export Backup</h2>
			<p class="modal-sub">DNS providers, hosts, and SSL certificates will be exported. Credentials and private keys are encrypted with your password.</p>

			{#if exportError}
				<div class="error-msg">{exportError}</div>
			{/if}

			<div class="form-group">
				<label for="export-pw">Encryption Password</label>
				<input id="export-pw" type="password" class="input" bind:value={exportPassword} placeholder="Min. 8 characters" autocomplete="off" />
			</div>

			<div class="modal-foot">
				<button class="btn-ghost" onclick={() => showExportModal = false}>Cancel</button>
				<button class="btn-fill" onclick={handleExport} disabled={exportPassword.length < 8 || exporting}>
					{exporting ? 'Encrypting...' : 'Download Backup'}
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showImportModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={() => showImportModal = false} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal modal-sm" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<h2>Import Backup</h2>
			<p class="modal-sub">Select a Proxera backup file and enter the password used during export.</p>

			{#if importError}
				<div class="error-msg">{importError}</div>
			{/if}

			{#if importResult}
				<div class="success-msg">
					<div>Providers: {importResult.providers_imported} imported{importResult.providers_skipped > 0 ? `, ${importResult.providers_skipped} skipped` : ''}</div>
					<div>Hosts: {importResult.hosts_imported} imported{importResult.hosts_skipped > 0 ? `, ${importResult.hosts_skipped} skipped` : ''}</div>
					<div>Certificates: {importResult.certs_imported} imported{importResult.certs_skipped > 0 ? `, ${importResult.certs_skipped} skipped` : ''}</div>
				</div>
			{:else}
				<div class="form-group">
					<label for="import-file">Backup File</label>
					<input id="import-file" type="file" accept=".json" class="input" onchange={handleFileSelect} />
				</div>

				{#if importData}
					<div class="form-group">
						<label for="import-pw">Decryption Password</label>
						<input id="import-pw" type="password" class="input" bind:value={importPassword} placeholder="Password used during export" autocomplete="off" />
					</div>
				{/if}

				<div class="modal-foot">
					<button class="btn-ghost" onclick={() => showImportModal = false}>Cancel</button>
					<button class="btn-fill" onclick={handleImport} disabled={!importData || !importPassword || importing}>
						{importing ? 'Importing...' : 'Import'}
					</button>
				</div>
			{/if}

			{#if importResult}
				<div class="modal-foot">
					<button class="btn-fill" onclick={() => showImportModal = false}>Done</button>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.settings-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
		align-items: start;
	}

	@media (max-width: 900px) {
		.settings-grid {
			grid-template-columns: 1fr;
		}
	}

	.settings-col {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.card {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
	}

	.card-title {
		font-size: var(--text-base);
		font-weight: 600;
		color: var(--text-primary);
		margin: 0 0 0.25rem;
	}

	.card-desc {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		margin: 0 0 1.25rem;
	}

	.field {
		display: block;
		margin-bottom: 1rem;
	}

	.field-label {
		display: block;
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		margin-bottom: 0.375rem;
	}

	.field-row {
		display: flex;
		gap: 0.75rem;
	}

	.flex-grow { flex: 1; }
	.field-sm { width: 100px; }

	select.input {
		appearance: none;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E");
		background-repeat: no-repeat;
		background-position: right 0.75rem center;
		padding-right: 2rem;
	}

	.toggle-field {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: var(--text-sm);
		color: var(--text-secondary);
		cursor: pointer;
		margin-bottom: 0.75rem;
	}

	.toggle-field input[type="checkbox"] {
		width: 16px;
		height: 16px;
		accent-color: var(--accent);
		cursor: pointer;
	}

	.form-actions { margin-top: 0.5rem; }

	.backup-actions {
		display: flex;
		gap: 0.5rem;
	}

	/* ── Modals ── */
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.modal {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
		width: 90%;
		max-width: 560px;
	}

	.modal h2 {
		font-size: var(--text-base);
		font-weight: 600;
		color: var(--text-primary);
		margin: 0 0 0.25rem;
	}

	.modal-sm { max-width: 480px; }

	.modal-sub {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		margin: 0 0 1.25rem;
	}

	.modal-foot {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1rem;
	}

	.form-group { margin-bottom: 1.25rem; }
	.form-group label {
		display: block;
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-tertiary);
		margin-bottom: 0.5rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.success-msg {
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		background: var(--green-dim, #10b98112);
		border: 1px solid var(--green, #10b981);
		color: var(--green, #10b981);
		font-size: var(--text-sm);
		margin-bottom: 1rem;
	}
</style>
