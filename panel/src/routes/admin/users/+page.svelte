<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

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
	}

	let users: AdminUser[] = $state([]);
	let error = $state('');
	let loading = $state(true);

	onMount(async () => {
		try {
			const data: any = await apiJson('/api/admin/users');
			users = data.users;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load users';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Users - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Users</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state">Loading...</div>
	{:else if users.length === 0}
		<div class="empty-state">No users found.</div>
	{:else}
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Name</th>
						<th>Email</th>
						<th>Role</th>
						<th class="mono">Agents</th>
						<th class="mono">Hosts</th>
						<th>Verified</th>
						<th>Joined</th>
					</tr>
				</thead>
				<tbody>
					{#each users as user}
						<tr class="clickable" onclick={() => goto(`/admin/users/${user.id}`)}>
							<td class="cell-name">{user.name}</td>
							<td class="cell-email">{user.email}</td>
							<td><span class="role-badge role-{user.role}">{user.role}</span></td>
							<td class="mono">{user.agent_count}</td>
							<td class="mono">{user.host_count}</td>
							<td>
								{#if user.email_verified}
									<span class="verified-yes">Yes</span>
								{:else}
									<span class="verified-no">No</span>
								{/if}
							</td>
							<td class="cell-sub">{formatRelativeTime(user.created_at)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.clickable { cursor: pointer; }

	.cell-name { font-weight: 600; }

	.cell-email {
		color: var(--text-secondary);
	}

	.cell-sub {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.verified-yes {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--success);
	}

	.verified-no {
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
</style>
