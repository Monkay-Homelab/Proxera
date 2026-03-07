<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, clearToken } from '$lib/api';
	import { toastError, toastSuccess } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import type { User } from '$lib/types';

	let user = $state<User | null>(null);
	let loading = $state(true);

	let showDeleteConfirm = $state(false);
	let deletePassword = $state('');
	let deleting = $state(false);

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let changingPassword = $state(false);

	// API Keys
	interface APIKey {
		id: number;
		name: string;
		prefix: string;
		last_used_at: string | null;
		expires_at: string | null;
		created_at: string;
	}
	let apiKeys = $state<APIKey[]>([]);
	let newKeyName = $state('');
	let newKeyExpiry = $state('never');
	let creatingKey = $state(false);
	let revealedKey = $state<string | null>(null);
	let copiedKey = $state(false);

	async function fetchAPIKeys() {
		try {
			const res = await api('/api/user/api-keys');
			if (res.ok) apiKeys = await res.json();
		} catch {}
	}

	async function createAPIKey() {
		if (!newKeyName.trim()) { toastError('Name is required'); return; }
		creatingKey = true;
		try {
			const res = await api('/api/user/api-keys', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name: newKeyName.trim(), expires_in: newKeyExpiry })
			});
			const data = await res.json();
			if (!res.ok) throw new Error(data.error || 'Failed to create key');
			revealedKey = data.key;
			copiedKey = false;
			newKeyName = '';
			newKeyExpiry = 'never';
			await fetchAPIKeys();
			toastSuccess('API key created');
		} catch (err: any) {
			toastError(err.message);
		} finally {
			creatingKey = false;
		}
	}

	async function revokeKey(id: number, name: string) {
		if (!await confirmDialog(`Revoke API key "${name}"? Any integrations using this key will stop working.`, {
			title: 'Revoke API Key', confirmLabel: 'Revoke', danger: true
		})) return;
		try {
			const res = await api(`/api/user/api-keys/${id}`, { method: 'DELETE' });
			if (!res.ok) { const d = await res.json(); throw new Error(d.error); }
			toastSuccess('API key revoked');
			await fetchAPIKeys();
		} catch (err: any) {
			toastError(err.message || 'Failed to revoke key');
		}
	}

	function copyKey() {
		if (revealedKey) {
			navigator.clipboard.writeText(revealedKey);
			copiedKey = true;
			setTimeout(() => copiedKey = false, 2000);
		}
	}

	function formatRelative(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const d = new Date(dateStr);
		const now = new Date();
		const diff = now.getTime() - d.getTime();
		if (diff < 60000) return 'Just now';
		if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
		if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
		return d.toLocaleDateString();
	}

	function formatExpiry(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const d = new Date(dateStr);
		if (d < new Date()) return 'Expired';
		return d.toLocaleDateString();
	}

	async function handleChangePassword() {
		if (!currentPassword || !newPassword || !confirmPassword) {
			toastError('All password fields are required');
			return;
		}
		if (newPassword !== confirmPassword) {
			toastError('New passwords do not match');
			return;
		}
		if (newPassword.length < 8) {
			toastError('New password must be at least 8 characters');
			return;
		}
		changingPassword = true;
		try {
			const response = await api('/api/user/change-password', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
			});
			const data = await response.json();
			if (!response.ok) throw new Error(data.error || 'Failed to update password');
			toastSuccess('Password updated successfully');
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
		} catch (err: any) {
			toastError(err.message || 'Failed to update password');
		} finally {
			changingPassword = false;
		}
	}

	onMount(async () => {
		try {
			const response = await api('/api/user/me');
			if (!response.ok) {
				throw new Error('Not authenticated');
			}
			user = await response.json();
			fetchAPIKeys();
		} catch (err) {
			toastError('Failed to load profile');
		} finally {
			loading = false;
		}
	});

	async function handleDeleteAccount() {
		if (!deletePassword) {
			toastError('Please enter your password');
			return;
		}

		const confirmed = await confirmDialog(
			'This will permanently delete your account and all associated data (agents, hosts, DNS records, certificates, and metrics). This action cannot be undone.',
			{ title: 'Delete Account', confirmLabel: 'Permanently Delete', danger: true }
		);
		if (!confirmed) return;

		deleting = true;
		try {
			const response = await api('/api/user/me', {
				method: 'DELETE',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ password: deletePassword })
			});
			const data = await response.json();
			if (!response.ok) {
				throw new Error(data.error || 'Failed to delete account');
			}
			toastSuccess('Account deleted successfully');
			clearToken();
			goto('/login');
		} catch (err: any) {
			toastError(err.message || 'Failed to delete account');
		} finally {
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>Profile - Proxera</title>
</svelte:head>

{#if loading}
	<div class="placeholder center-page"><div class="loader"></div><p>Loading...</p></div>
{:else if user}
	<div class="page">
		<header class="page-head">
			<h1>Profile</h1>
		</header>

		<div class="panel">
			<div class="profile-header">
				<div class="avatar">
					<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
						<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
						<circle cx="12" cy="7" r="4"/>
					</svg>
				</div>
				<div class="profile-info">
					<h2>{user.name || 'User'}</h2>
					<p class="dim">{user.email}</p>
				</div>
			</div>

			<div class="details">
				<span class="section-label">Account Information</span>
				<div class="detail-grid">
					<div class="detail-item">
						<span class="lbl">Name</span>
						<span class="val">{user.name || 'Not set'}</span>
					</div>
					<div class="detail-item">
						<span class="lbl">Email</span>
						<span class="val">{user.email}</span>
					</div>
					<div class="detail-item">
						<span class="lbl">Account Created</span>
						<span class="val">{new Date(user.created_at).toLocaleDateString()}</span>
					</div>
				</div>
			</div>
		</div>

		<div class="panel password-panel">
			<div class="details">
				<span class="section-label">Change Password</span>
				<div class="pw-form">
					<div class="pw-field">
						<label for="current-pw">Current Password</label>
						<input
							id="current-pw"
							type="password"
							class="input"
							placeholder="Enter current password"
							bind:value={currentPassword}
							disabled={changingPassword}
						/>
					</div>
					<div class="pw-field">
						<label for="new-pw">New Password</label>
						<input
							id="new-pw"
							type="password"
							class="input"
							placeholder="At least 8 characters"
							bind:value={newPassword}
							disabled={changingPassword}
						/>
					</div>
					<div class="pw-field">
						<label for="confirm-pw">Confirm New Password</label>
						<input
							id="confirm-pw"
							type="password"
							class="input"
							placeholder="Repeat new password"
							bind:value={confirmPassword}
							disabled={changingPassword}
							onkeydown={(e) => e.key === 'Enter' && handleChangePassword()}
						/>
					</div>
					<div class="pw-foot">
						<button
							class="btn-fill"
							onclick={handleChangePassword}
							disabled={changingPassword || !currentPassword || !newPassword || !confirmPassword}
						>
							{#if changingPassword}
								<span class="spinner-accent"></span> Updating...
							{:else}
								Update Password
							{/if}
						</button>
					</div>
				</div>
			</div>
		</div>

		<div class="panel api-keys-panel">
			<div class="details">
				<span class="section-label">API Keys</span>
				<p class="section-desc">Use API keys to authenticate with the Proxera API. Keys inherit your account's role and permissions.</p>

				{#if revealedKey}
					<div class="key-reveal">
						<div class="key-reveal-header">
							<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
								<path d="M12 9v4m0 4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"/>
							</svg>
							<span>Copy your API key now — it won't be shown again.</span>
						</div>
						<div class="key-reveal-row">
							<code class="key-value">{revealedKey}</code>
							<button class="btn-copy" onclick={copyKey}>
								{copiedKey ? 'Copied!' : 'Copy'}
							</button>
						</div>
						<button class="btn-ghost btn-sm" onclick={() => revealedKey = null}>Dismiss</button>
					</div>
				{/if}

				<div class="key-create">
					<input
						class="input"
						type="text"
						placeholder="Key name (e.g. CI/CD, monitoring)"
						bind:value={newKeyName}
						disabled={creatingKey}
						onkeydown={(e) => e.key === 'Enter' && createAPIKey()}
					/>
					<select class="input select-sm" bind:value={newKeyExpiry} disabled={creatingKey}>
						<option value="never">No expiration</option>
						<option value="30d">30 days</option>
						<option value="90d">90 days</option>
						<option value="365d">1 year</option>
					</select>
					<button class="btn-fill" onclick={createAPIKey} disabled={creatingKey || !newKeyName.trim()}>
						{#if creatingKey}
							<span class="spinner-accent"></span> Creating...
						{:else}
							Generate Key
						{/if}
					</button>
				</div>

				{#if apiKeys.length > 0}
					<div class="key-list">
						{#each apiKeys as key}
							<div class="key-item">
								<div class="key-info">
									<span class="key-name">{key.name}</span>
									<span class="key-meta">
										<code class="key-prefix">{key.prefix}...</code>
										<span class="sep">·</span>
										Created {formatRelative(key.created_at)}
										<span class="sep">·</span>
										Last used: {formatRelative(key.last_used_at)}
										<span class="sep">·</span>
										Expires: {formatExpiry(key.expires_at)}
									</span>
								</div>
								<button class="btn-delete btn-sm" onclick={() => revokeKey(key.id, key.name)}>Revoke</button>
							</div>
						{/each}
					</div>
				{:else}
					<p class="no-keys">No API keys yet.</p>
				{/if}
			</div>
		</div>

		<div class="danger-zone">
			<div class="danger-row">
				<div class="danger-icon">
					<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
						<line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
					</svg>
				</div>
				<div class="danger-text">
					<span class="danger-title">Delete Account</span>
					<span class="danger-desc">Permanently remove your account, agents, hosts, DNS records, certificates, and all metrics.</span>
				</div>
				{#if !showDeleteConfirm}
					<button class="btn-delete" onclick={() => showDeleteConfirm = true}>
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
							<polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
							<path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/>
						</svg>
						Delete Account
					</button>
				{/if}
			</div>
			{#if showDeleteConfirm}
				<div class="delete-confirm">
					<p class="confirm-prompt">Enter your password to confirm account deletion:</p>
					<div class="confirm-row">
						<input
							class="input confirm-input"
							type="password"
							placeholder="Your password"
							bind:value={deletePassword}
							disabled={deleting}
							onkeydown={(e) => e.key === 'Enter' && handleDeleteAccount()}
						/>
						<button class="btn-ghost" onclick={() => { showDeleteConfirm = false; deletePassword = ''; }} disabled={deleting}>Cancel</button>
						<button class="btn-destroy" onclick={handleDeleteAccount} disabled={deleting || !deletePassword}>
							{#if deleting}
								<span class="spinner"></span> Deleting...
							{:else}
								Permanently Delete
							{/if}
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	/* ── Loading ── */
	.center-page {
		margin: 2rem 2.25rem;
	}

	/* ── Panel ── */
	.panel {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); overflow: hidden;
	}

	.profile-header {
		display: flex; align-items: center; gap: 1.5rem;
		padding: 1.5rem; border-bottom: 1px solid var(--border);
	}

	.avatar {
		width: 64px; height: 64px;
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		display: flex; align-items: center; justify-content: center;
		flex-shrink: 0; color: var(--text-tertiary);
	}

	.profile-info h2 {
		margin: 0 0 0.25rem; font-size: var(--text-lg);
		font-weight: 700; color: var(--text-primary);
	}
	.dim { color: var(--text-tertiary); font-size: var(--text-sm); margin: 0; }

	/* ── Details ── */
	.details { padding: 1.5rem; }
	.section-label {
		display: block; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); text-transform: uppercase;
		letter-spacing: 0.04em; margin-bottom: 1rem;
	}
	.detail-grid { display: flex; flex-direction: column; gap: 0.75rem; }
	.detail-item {
		display: flex; justify-content: space-between; align-items: center;
		padding: 0.75rem 1rem;
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.lbl { font-weight: 500; color: var(--text-tertiary); font-size: var(--text-sm); }
	.val { color: var(--text-primary); font-weight: 500; font-size: var(--text-sm); }

	/* ── Password Panel ── */
	.password-panel { margin-top: 1.5rem; }

	.pw-form { display: flex; flex-direction: column; gap: 1rem; }
	.pw-field { display: flex; flex-direction: column; gap: 0.375rem; }
	.pw-field label {
		font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); text-transform: uppercase; letter-spacing: 0.04em;
	}
	.pw-foot { display: flex; justify-content: flex-end; padding-top: 0.25rem; }

	.spinner-accent {
		display: inline-block; width: 12px; height: 12px;
		border: 2px solid rgba(255,255,255,0.3);
		border-top-color: #fff; border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	/* ── API Keys ── */
	.api-keys-panel { margin-top: 1.5rem; }
	.section-desc {
		font-size: var(--text-xs); color: var(--text-tertiary);
		margin: -0.5rem 0 1rem; line-height: 1.5;
	}
	.key-create {
		display: flex; gap: 0.5rem; margin-bottom: 1rem; align-items: center;
	}
	.key-create .input:first-child { flex: 1; }
	.select-sm {
		width: auto; min-width: 140px;
		appearance: auto;
	}
	.key-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.key-item {
		display: flex; align-items: center; justify-content: space-between;
		padding: 0.75rem 1rem; gap: 1rem;
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.key-info { display: flex; flex-direction: column; gap: 0.25rem; min-width: 0; }
	.key-name { font-weight: 600; font-size: var(--text-sm); color: var(--text-primary); }
	.key-meta {
		font-size: var(--text-xs); color: var(--text-tertiary);
		display: flex; flex-wrap: wrap; gap: 0.25rem; align-items: center;
	}
	.key-prefix {
		font-family: var(--font-mono); font-size: var(--text-xs);
		background: var(--surface); padding: 0.1rem 0.35rem;
		border-radius: 3px; border: 1px solid var(--border);
	}
	.sep { opacity: 0.4; }
	.no-keys {
		font-size: var(--text-sm); color: var(--text-tertiary);
		text-align: center; padding: 1.5rem 0; margin: 0;
	}
	.btn-sm { padding: 0.35rem 0.75rem; font-size: var(--text-xs); }

	.key-reveal {
		background: var(--info-dim); border: 1px solid var(--info);
		border-radius: var(--radius); padding: 1rem;
		margin-bottom: 1rem; display: flex; flex-direction: column; gap: 0.75rem;
	}
	.key-reveal-header {
		display: flex; align-items: center; gap: 0.5rem;
		font-size: var(--text-xs); font-weight: 600; color: var(--info);
	}
	.key-reveal-row {
		display: flex; gap: 0.5rem; align-items: center;
	}
	.key-value {
		flex: 1; font-family: var(--font-mono); font-size: var(--text-xs);
		background: var(--surface); padding: 0.5rem 0.75rem;
		border-radius: var(--radius); border: 1px solid var(--border);
		word-break: break-all; color: var(--text-primary);
	}
	.btn-copy {
		padding: 0.5rem 1rem; border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 600; white-space: nowrap;
		color: #fff; background: var(--info); border: none;
		cursor: pointer; transition: opacity var(--transition);
	}
	.btn-copy:hover { opacity: 0.85; }

	/* ── Danger Zone ── */
	.danger-zone {
		margin-top: 2rem;
		background: var(--surface); border: 1px solid var(--danger);
		border-radius: var(--radius-lg); overflow: hidden;
	}
	.danger-row {
		display: flex; align-items: center; gap: 1rem;
		padding: 1.25rem 1.5rem;
	}
	.danger-icon {
		width: 36px; height: 36px; flex-shrink: 0;
		display: flex; align-items: center; justify-content: center;
		background: var(--danger-dim); border-radius: var(--radius);
		color: var(--danger);
	}
	.danger-text {
		flex: 1; display: flex; flex-direction: column; gap: 0.125rem;
	}
	.danger-title {
		font-size: var(--text-sm); font-weight: 600; color: var(--text-primary);
	}
	.danger-desc {
		font-size: var(--text-xs); color: var(--text-tertiary); line-height: 1.4;
	}
	.btn-delete {
		display: inline-flex; align-items: center; gap: 0.4rem;
		padding: 0.5rem 1rem; border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 600;
		color: var(--danger); background: transparent;
		border: 1px solid color-mix(in srgb, var(--danger) 40%, transparent);
		cursor: pointer; transition: all var(--transition); white-space: nowrap;
	}
	.btn-delete:hover {
		background: var(--danger-dim);
		border-color: var(--danger);
	}

	/* ── Confirm ── */
	.delete-confirm {
		padding: 0 1.5rem 1.25rem;
		border-top: 1px solid color-mix(in srgb, var(--danger) 20%, var(--border));
		padding-top: 1.25rem;
	}
	.confirm-prompt {
		font-size: var(--text-xs); color: var(--text-tertiary);
		margin: 0 0 0.75rem;
	}
	.confirm-row {
		display: flex; gap: 0.5rem; align-items: center;
	}
	.confirm-input {
		flex: 1; min-width: 0;
	}
	.confirm-input:focus {
		border-color: var(--danger) !important;
		box-shadow: 0 0 0 2px var(--danger-dim);
	}
	.btn-ghost {
		background: var(--surface-raised); color: var(--text-secondary);
		border: 1px solid var(--border); padding: 0.5rem 0.875rem;
		border-radius: var(--radius); font-size: var(--text-xs); font-weight: 500;
		cursor: pointer; transition: all var(--transition); white-space: nowrap;
	}
	.btn-ghost:hover:not(:disabled) { border-color: var(--border-bright); color: var(--text-primary); }
	.btn-ghost:disabled { opacity: 0.4; cursor: not-allowed; }
	.btn-destroy {
		display: inline-flex; align-items: center; gap: 0.375rem;
		padding: 0.5rem 1rem; border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 600;
		color: #fff; background: var(--danger);
		border: none; cursor: pointer;
		transition: opacity var(--transition); white-space: nowrap;
	}
	.btn-destroy:hover:not(:disabled) { opacity: 0.85; }
	.btn-destroy:disabled { opacity: 0.45; cursor: not-allowed; }
	.spinner {
		display: inline-block; width: 12px; height: 12px;
		border: 2px solid rgba(255,255,255,0.3);
		border-top-color: #fff; border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.profile-header { flex-direction: column; text-align: center; }
		.danger-row { flex-wrap: wrap; }
		.confirm-row { flex-direction: column; }
		.confirm-row .confirm-input { width: 100%; }
		.confirm-row .btn-ghost, .confirm-row .btn-destroy { width: 100%; justify-content: center; }
		.key-create { flex-direction: column; }
		.key-create .input:first-child, .select-sm { width: 100%; }
		.key-item { flex-direction: column; align-items: flex-start; }
		.key-reveal-row { flex-direction: column; }
	}
</style>
