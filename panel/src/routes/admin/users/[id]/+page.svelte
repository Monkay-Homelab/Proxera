<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';
	import { toastSuccess, toastError } from '$lib/components/toast';

	interface AdminUser {
		id: number;
		email: string;
		name: string;
		role: string;
		email_verified: boolean;
		created_at: string;
		updated_at: string;
		agent_count: number;
		host_count: number;
		suspended: boolean;
		suspended_at: string | null;
		suspended_reason: string;
	}

	interface Assignment {
		agents: { id: number; agent_id: string; name: string }[];
		providers: { id: number; domain: string; provider: string }[];
	}

	const roles = ['admin', 'member', 'viewer'];
	const userId = $page.params.id;

	let activeTab = $state<'details' | 'assignments' | 'actions'>('details');
	let user: AdminUser | null = $state(null);
	let assignments: Assignment | null = $state(null);
	let error = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let success = $state('');
	let assignmentsLoading = $state(false);

	let name = $state('');
	let email = $state('');
	let role = $state('member');
	let emailVerified = $state(false);

	let suspendReason = $state('');
	let confirmSuspend = $state(false);
	let confirmUnsuspend = $state(false);
	let sendingReset = $state(false);

	// For adding assignments
	let allAgents: { id: number; agent_id: string; name: string }[] = $state([]);
	let allProviders: { id: number; domain: string; provider: string }[] = $state([]);
	let selectedAgentId = $state('');
	let selectedProviderId = $state('');

	onMount(async () => {
		try {
			const data: AdminUser = await apiJson(`/api/admin/users/${userId}`);
			user = data;
			name = data.name;
			email = data.email;
			role = data.role;
			emailVerified = data.email_verified;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load user';
		} finally {
			loading = false;
		}
	});

	async function loadAssignments() {
		if (assignments) return;
		assignmentsLoading = true;
		try {
			assignments = await apiJson(`/api/admin/users/${userId}/assignments`);
			// Load all agents and providers for the add dropdowns
			const [agentsData, providersData]: [any, any] = await Promise.all([
				apiJson('/api/admin/agents'),
				apiJson('/api/dns/providers')
			]);
			allAgents = agentsData.agents || agentsData || [];
			allProviders = providersData.providers || providersData || [];
		} catch (e: any) {
			toastError(e.message || 'Failed to load assignments');
		} finally {
			assignmentsLoading = false;
		}
	}

	async function save() {
		if (!user) return;
		saving = true;
		success = '';
		error = '';
		try {
			const data: AdminUser = await apiJson(`/api/admin/users/${user.id}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name, email, role, email_verified: emailVerified }),
			});
			user = data;
			success = 'User updated successfully.';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update user';
		} finally {
			saving = false;
		}
	}

	async function assignAgent() {
		if (!selectedAgentId) return;
		try {
			await apiJson(`/api/admin/users/${userId}/assignments/agents`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ agent_id: parseInt(selectedAgentId) }),
			});
			toastSuccess('Agent assigned');
			selectedAgentId = '';
			assignments = null;
			await loadAssignments();
		} catch (e: any) {
			toastError(e.message || 'Failed to assign agent');
		}
	}

	async function removeAgent(agentId: number) {
		try {
			await apiJson(`/api/admin/users/${userId}/assignments/agents/${agentId}`, { method: 'DELETE' });
			toastSuccess('Agent removed');
			assignments = null;
			await loadAssignments();
		} catch (e: any) {
			toastError(e.message || 'Failed to remove agent');
		}
	}

	async function assignProvider() {
		if (!selectedProviderId) return;
		try {
			await apiJson(`/api/admin/users/${userId}/assignments/providers`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ provider_id: parseInt(selectedProviderId) }),
			});
			toastSuccess('DNS provider assigned');
			selectedProviderId = '';
			assignments = null;
			await loadAssignments();
		} catch (e: any) {
			toastError(e.message || 'Failed to assign provider');
		}
	}

	async function removeProvider(providerId: number) {
		try {
			await apiJson(`/api/admin/users/${userId}/assignments/providers/${providerId}`, { method: 'DELETE' });
			toastSuccess('DNS provider removed');
			assignments = null;
			await loadAssignments();
		} catch (e: any) {
			toastError(e.message || 'Failed to remove provider');
		}
	}

	async function suspendUser() {
		confirmSuspend = false;
		try {
			user = await apiJson(`/api/admin/users/${userId}/suspend`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ reason: suspendReason }),
			});
			suspendReason = '';
			toastSuccess('User suspended');
		} catch (e: any) {
			toastError(e.message || 'Failed to suspend user');
		}
	}

	async function unsuspendUser() {
		confirmUnsuspend = false;
		try {
			user = await apiJson(`/api/admin/users/${userId}/unsuspend`, { method: 'POST' });
			toastSuccess('Suspension lifted');
		} catch (e: any) {
			toastError(e.message || 'Failed to unsuspend user');
		}
	}

	async function forcePasswordReset() {
		sendingReset = true;
		try {
			await apiJson(`/api/admin/users/${userId}/password-reset`, { method: 'POST' });
			toastSuccess('Password reset email sent');
		} catch (e: any) {
			toastError(e.message || 'Failed to send reset email');
		} finally {
			sendingReset = false;
		}
	}

	function switchTab(tab: 'details' | 'assignments' | 'actions') {
		activeTab = tab;
		if (tab === 'assignments') loadAssignments();
	}

	function fmtDate(s: string | null) {
		if (!s) return '—';
		return new Date(s).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}
</script>

<svelte:head>
	<title>{user ? user.name : 'User'} - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<a href="/admin/users" class="back-link">
			<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>
			</svg>
			Users
		</a>
		<h1>{user ? user.name || 'Unnamed User' : 'Edit User'}</h1>
	</div>

	{#if loading}
		<div class="empty-state">Loading...</div>
	{:else if error && !user}
		<div class="empty-state">{error}</div>
	{:else if user}
		<!-- Tab bar -->
		<div class="tabs">
			<button class="tab" class:active={activeTab === 'details'} onclick={() => switchTab('details')}>Details</button>
			<button class="tab" class:active={activeTab === 'assignments'} onclick={() => switchTab('assignments')}>Assignments</button>
			<button class="tab" class:active={activeTab === 'actions'} onclick={() => switchTab('actions')}>
				Actions
				{#if user.suspended}<span class="tab-badge suspended">Suspended</span>{/if}
			</button>
		</div>

		{#if activeTab === 'details'}
			<div class="edit-layout">
				<div class="card">
					<h2 class="card-title">Details</h2>

					{#if error}
						<div class="msg msg-error">{error}</div>
					{/if}
					{#if success}
						<div class="msg msg-success">{success}</div>
					{/if}

					<form onsubmit={(e) => { e.preventDefault(); save(); }}>
						<label class="field">
							<span class="field-label">Name</span>
							<input class="input" type="text" bind:value={name} />
						</label>

						<label class="field">
							<span class="field-label">Email</span>
							<input class="input" type="email" bind:value={email} />
						</label>

						<label class="field">
							<span class="field-label">Role</span>
							<select class="input" bind:value={role}>
								{#each roles as r}
									<option value={r}>{r}</option>
								{/each}
							</select>
						</label>

						<label class="field checkbox-field">
							<input type="checkbox" bind:checked={emailVerified} />
							<span class="field-label">Email Verified</span>
						</label>

						<div class="form-actions">
							<button type="submit" class="btn-fill" disabled={saving}>
								{saving ? 'Saving...' : 'Save Changes'}
							</button>
						</div>
					</form>
				</div>

				<div class="card info-card">
					<h2 class="card-title">Info</h2>
					<dl class="info-list">
						<div class="info-row"><dt>User ID</dt><dd>{user.id}</dd></div>
						<div class="info-row"><dt>Role</dt><dd><span class="role-badge role-{user.role}">{user.role}</span></dd></div>
						<div class="info-row"><dt>Agents</dt><dd>{user.agent_count}</dd></div>
						<div class="info-row"><dt>Hosts</dt><dd>{user.host_count}</dd></div>
						<div class="info-row"><dt>Joined</dt><dd>{formatRelativeTime(user.created_at)}</dd></div>
					</dl>
				</div>
			</div>

		{:else if activeTab === 'assignments'}
			{#if assignmentsLoading}
				<div class="empty-state">Loading assignments...</div>
			{:else if assignments}
				<div class="assignments-grid">
					<!-- Assigned Agents -->
					<div class="card">
						<h2 class="card-title">Assigned Agents</h2>
						{#if assignments.agents.length === 0}
							<p class="dim">No agents assigned.</p>
						{:else}
							<div class="assign-list">
								{#each assignments.agents as agent}
									<div class="assign-row">
										<span class="assign-name">{agent.name || agent.agent_id}</span>
										<button class="btn-remove" onclick={() => removeAgent(agent.id)}>Remove</button>
									</div>
								{/each}
							</div>
						{/if}
						<div class="assign-add">
							<select class="input" bind:value={selectedAgentId}>
								<option value="">Select agent...</option>
								{#each allAgents.filter(a => !assignments?.agents.some(aa => aa.id === a.id)) as agent}
									<option value={agent.id}>{agent.name || agent.agent_id}</option>
								{/each}
							</select>
							<button class="btn-fill btn-sm" onclick={assignAgent} disabled={!selectedAgentId}>Add</button>
						</div>
					</div>

					<!-- Assigned DNS Providers -->
					<div class="card">
						<h2 class="card-title">Assigned DNS Providers</h2>
						{#if assignments.providers.length === 0}
							<p class="dim">No DNS providers assigned.</p>
						{:else}
							<div class="assign-list">
								{#each assignments.providers as provider}
									<div class="assign-row">
										<span class="assign-name">{provider.domain} <span class="assign-sub">({provider.provider})</span></span>
										<button class="btn-remove" onclick={() => removeProvider(provider.id)}>Remove</button>
									</div>
								{/each}
							</div>
						{/if}
						<div class="assign-add">
							<select class="input" bind:value={selectedProviderId}>
								<option value="">Select provider...</option>
								{#each allProviders.filter(p => !assignments?.providers.some(ap => ap.id === p.id)) as provider}
									<option value={provider.id}>{provider.domain} ({provider.provider})</option>
								{/each}
							</select>
							<button class="btn-fill btn-sm" onclick={assignProvider} disabled={!selectedProviderId}>Add</button>
						</div>
					</div>
				</div>
			{/if}

		{:else if activeTab === 'actions'}
			<div class="actions-grid">
				<!-- Suspend / Unsuspend -->
				{#if user.suspended}
					<div class="card danger-card">
						<h2 class="card-title">Account Suspended</h2>
						<dl class="info-list" style="margin-bottom: 1rem;">
							<div class="info-row"><dt>Suspended</dt><dd>{fmtDate(user.suspended_at)}</dd></div>
							<div class="info-row"><dt>Reason</dt><dd>{user.suspended_reason || '—'}</dd></div>
						</dl>
						{#if confirmUnsuspend}
							<div class="confirm-row">
								<span class="danger-confirm">Lift suspension?</span>
								<button class="btn-fill" onclick={unsuspendUser}>Yes, Unsuspend</button>
								<button class="btn-ghost-small" onclick={() => confirmUnsuspend = false}>Cancel</button>
							</div>
						{:else}
							<button class="btn-fill" onclick={() => confirmUnsuspend = true}>Lift Suspension</button>
						{/if}
					</div>
				{:else}
					<div class="card">
						<h2 class="card-title">Suspend Account</h2>
						<p class="danger-note">Suspended users are blocked from all API access with a 403 error.</p>
						<div class="field">
							<label class="field-label" for="suspend-reason">Reason (optional)</label>
							<input id="suspend-reason" class="input" type="text" bind:value={suspendReason} placeholder="e.g. Policy violation" />
						</div>
						{#if confirmSuspend}
							<div class="confirm-row">
								<span class="danger-confirm">Suspend this user?</span>
								<button class="btn-danger" onclick={suspendUser}>Yes, Suspend</button>
								<button class="btn-ghost-small" onclick={() => confirmSuspend = false}>Cancel</button>
							</div>
						{:else}
							<button class="btn-danger" onclick={() => confirmSuspend = true}>Suspend User</button>
						{/if}
					</div>
				{/if}

				<!-- Force Password Reset -->
				<div class="card">
					<h2 class="card-title">Force Password Reset</h2>
					<p class="danger-note">Sends a password reset email to <strong>{user.email}</strong>. The link expires in 1 hour.</p>
					<button class="btn-fill" onclick={forcePasswordReset} disabled={sendingReset}>
						{sendingReset ? 'Sending...' : 'Send Reset Email'}
					</button>
				</div>
			</div>
		{/if}
	{/if}
</div>

<style>
	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		margin-bottom: 0.5rem;
		transition: color var(--transition);
	}
	.back-link:hover { color: var(--text-primary); }

	.tabs {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border);
		margin-bottom: 1.5rem;
	}

	.tab {
		padding: 0.625rem 1.25rem;
		font-size: var(--text-sm);
		font-weight: 500;
		color: var(--text-secondary);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		cursor: pointer;
		transition: all var(--transition);
	}

	.tab:hover { color: var(--text-primary); }

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.edit-layout {
		display: grid;
		grid-template-columns: 1fr 280px;
		gap: 1.25rem;
		align-items: start;
	}

	.assignments-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1.25rem;
		align-items: start;
	}

	.actions-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 1rem;
		align-items: start;
	}

	.tab-badge {
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		margin-left: 0.4rem;
		vertical-align: middle;
	}
	.tab-badge.suspended { background: var(--danger-dim); color: var(--danger); }

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

	.checkbox-field {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
	}
	.checkbox-field .field-label { margin-bottom: 0; }
	.checkbox-field input[type="checkbox"] { width: 16px; height: 16px; accent-color: var(--accent); }

	select.input {
		appearance: none;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E");
		background-repeat: no-repeat;
		background-position: right 0.75rem center;
		padding-right: 2rem;
	}

	.form-actions { margin-top: 1.25rem; }

	.msg { font-size: var(--text-sm); padding: 0.5rem 0.75rem; border-radius: var(--radius); margin-bottom: 1rem; }
	.msg-error { color: var(--danger); background: var(--danger-dim); }
	.msg-success { color: var(--accent); background: var(--accent-dim); }

	.info-list { margin: 0; }
	.info-row { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid var(--border); }
	.info-row:last-child { border-bottom: none; }
	.info-row dt { font-size: var(--text-sm); color: var(--text-secondary); }
	.info-row dd { font-size: var(--text-sm); color: var(--text-primary); font-weight: 500; margin: 0; max-width: 60%; overflow: hidden; text-overflow: ellipsis; }

	.role-badge {
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.2rem 0.5rem;
		border-radius: 4px;
	}
	.role-admin { background: var(--accent-dim); color: var(--accent); }
	.role-member { background: var(--surface-raised); color: var(--text-secondary); }
	.role-viewer { background: var(--surface-raised); color: var(--text-tertiary); }

	.danger-card { border-color: var(--danger); }
	.danger-note { font-size: var(--text-xs); color: var(--text-tertiary); margin-bottom: 0.75rem; }

	.confirm-row { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
	.danger-confirm { font-size: var(--text-sm); color: var(--danger); font-weight: 600; }

	.btn-danger {
		padding: 0.5rem 1rem;
		font-size: var(--text-sm);
		font-weight: 600;
		background: var(--danger-dim);
		color: var(--danger);
		border: 1px solid var(--danger);
		border-radius: var(--radius);
		cursor: pointer;
		transition: all var(--transition);
	}
	.btn-danger:hover { background: var(--danger); color: #fff; }

	.btn-ghost-small {
		padding: 0.5rem 0.75rem;
		font-size: var(--text-sm);
		background: none;
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text-secondary);
		cursor: pointer;
	}

	.dim { font-size: var(--text-xs); color: var(--text-tertiary); }

	/* Assignments */
	.assign-list { display: flex; flex-direction: column; gap: 0.5rem; margin-bottom: 1rem; }
	.assign-row {
		display: flex; justify-content: space-between; align-items: center;
		padding: 0.5rem 0.75rem; background: var(--surface-raised);
		border: 1px solid var(--border); border-radius: var(--radius);
	}
	.assign-name { font-size: var(--text-sm); font-weight: 500; color: var(--text-primary); }
	.assign-sub { font-size: var(--text-xs); color: var(--text-tertiary); }
	.btn-remove {
		font-size: var(--text-xs); font-weight: 500; color: var(--danger);
		background: none; border: 1px solid var(--danger); border-radius: var(--radius);
		padding: 0.2rem 0.5rem; cursor: pointer; transition: all var(--transition);
	}
	.btn-remove:hover { background: var(--danger-dim); }
	.assign-add { display: flex; gap: 0.5rem; align-items: center; }
	.assign-add .input { flex: 1; }
	.btn-sm { padding: 0.5rem 0.875rem; font-size: var(--text-sm); }

	@media (max-width: 768px) {
		.edit-layout { grid-template-columns: 1fr; }
		.assignments-grid { grid-template-columns: 1fr; }
		.actions-grid { grid-template-columns: 1fr; }
	}
</style>
