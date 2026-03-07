<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { toastSuccess, toastError } from '$lib/components/toast';

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
			<!-- Registration -->
			<div class="card">
				<h2 class="card-title">Registration</h2>
				<p class="card-desc">Control how new users can sign up for this instance.</p>

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

			<!-- URLs -->
			<div class="card">
				<h2 class="card-title">URLs</h2>
				<p class="card-desc">Public-facing URLs for email links and agent connections. Leave empty for same-origin (Docker AIO).</p>

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

			<!-- Email / SMTP -->
			<div class="card">
				<h2 class="card-title">Email (SMTP)</h2>
				<p class="card-desc">Configure outgoing email for notifications and verification.</p>

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

			<!-- SSL -->
			<div class="card">
				<h2 class="card-title">SSL</h2>
				<p class="card-desc">Certificate settings for Let's Encrypt.</p>

				<label class="toggle-field">
					<input type="checkbox" bind:checked={acmeStaging} />
					<span>Use Let's Encrypt staging (for testing — certs won't be trusted)</span>
				</label>
			</div>

			<div class="form-actions">
				<button type="submit" class="btn-fill" disabled={saving}>
					{saving ? 'Saving...' : 'Save Settings'}
				</button>
			</div>
		</form>
	{/if}
</div>

<style>
	.card {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
		max-width: 600px;
		margin-bottom: 1rem;
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
</style>
