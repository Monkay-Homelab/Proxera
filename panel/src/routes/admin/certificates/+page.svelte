<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJson } from '$lib/api';
	import { formatRelativeTime } from '$lib/utils';

	interface AdminCertificate {
		id: number;
		domain: string;
		san: string;
		status: string;
		issued_at: string;
		expires_at: string;
		created_at: string;
		user_id: number;
		user_name: string;
		user_email: string;
	}

	let certificates: AdminCertificate[] = $state([]);
	let error = $state('');
	let loading = $state(true);

	function daysRemaining(expiresAt: string): number | null {
		if (!expiresAt) return null;
		return Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 86400000);
	}

	const validCount = $derived(
		certificates.filter((c) => c.expires_at && daysRemaining(c.expires_at)! > 30).length
	);
	const expiringCount = $derived(
		certificates.filter((c) => {
			const d = daysRemaining(c.expires_at);
			return d !== null && d > 0 && d <= 30;
		}).length
	);
	const expiredCount = $derived(
		certificates.filter((c) => {
			const d = daysRemaining(c.expires_at);
			return d !== null && d <= 0;
		}).length
	);

	onMount(async () => {
		try {
			const data: any = await apiJson('/api/admin/certificates');
			certificates = data.certificates;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load certificates';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Certificates - Admin - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-header">
		<h1>Certificates</h1>
	</div>

	{#if error}
		<div class="empty-state">{error}</div>
	{:else if loading}
		<div class="empty-state">Loading...</div>
	{:else if certificates.length === 0}
		<div class="empty-state">No certificates found.</div>
	{:else}
		<div class="kpi-row">
			<div class="kpi">
				<div class="kpi-num">{certificates.length}</div>
				<div class="kpi-label">Total</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" style="color: var(--accent)">{validCount}</div>
				<div class="kpi-label">Valid</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" style="color: var(--warning)">{expiringCount}</div>
				<div class="kpi-label">Expiring Soon</div>
			</div>
			<div class="kpi">
				<div class="kpi-num" style="color: var(--danger)">{expiredCount}</div>
				<div class="kpi-label">Expired</div>
			</div>
		</div>

		<div class="tbl-wrap">
			<table>
				<thead>
					<tr>
						<th>Domain</th>
						<th>Owner</th>
						<th>Status</th>
						<th>Issued</th>
						<th>Expires</th>
						<th class="mono">Days Left</th>
					</tr>
				</thead>
				<tbody>
					{#each certificates as cert}
						{@const days = daysRemaining(cert.expires_at)}
						<tr>
							<td>
								<div class="cell-main">{cert.domain}</div>
								{#if cert.san}
									<div class="cell-sub">{cert.san}</div>
								{/if}
							</td>
							<td>
								<div class="cell-main">{cert.user_name}</div>
								<div class="cell-sub">{cert.user_email}</div>
							</td>
							<td>
								<span
									class="cert-status"
									class:is-active={cert.status === 'active' || cert.status === 'valid'}
									class:is-pending={cert.status === 'pending'}
									class:is-expired={cert.status === 'expired'}
								>{cert.status}</span>
							</td>
							<td class="cell-sub">{cert.issued_at ? formatRelativeTime(cert.issued_at) : '—'}</td>
							<td class="cell-sub">{cert.expires_at ? formatRelativeTime(cert.expires_at) : '—'}</td>
							<td class="mono">
								{#if days === null}
									<span class="text-muted">—</span>
								{:else if days > 30}
									<span class="days-ok">{days}d</span>
								{:else if days >= 7}
									<span class="days-warn">{days}d</span>
								{:else}
									<span class="days-crit">{days}d</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.cell-main { font-weight: 600; }

	.cell-sub {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.cert-status {
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		background: var(--surface-raised);
		color: var(--text-tertiary);
	}

	.cert-status.is-active {
		background: var(--success-dim);
		color: var(--success);
	}

	.cert-status.is-pending {
		background: var(--warning-dim);
		color: var(--warning);
	}

	.cert-status.is-expired {
		background: var(--danger-dim);
		color: var(--danger);
	}

	.text-muted { color: var(--text-muted); }

	.days-ok {
		color: var(--accent);
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.days-warn {
		color: var(--warning);
		font-weight: 600;
		font-variant-numeric: tabular-nums;
	}

	.days-crit {
		color: var(--danger);
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}
</style>
