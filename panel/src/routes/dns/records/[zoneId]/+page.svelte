<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { toastError } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import { formatRelativeTime } from '$lib/utils';
	import type { DnsRecord, DnsProvider, Agent } from '$lib/types';

	let domain = '';
	let providerType = '';
	let records: DnsRecord[] = [];
	let agents: Agent[] = [];
	let loading = true;
	let syncing = false;
	let lastSynced: Date | null = null;
	let error = '';
	let collapsed: Record<string, boolean> = {};

	// Modal state
	let showRecordModal = false;
	let editingRecord: DnsRecord | null = null; // null = add mode, object = edit mode
	let recordType = 'A';
	let recordName = '';
	let recordContent = '';
	let recordTTL = 1;
	let recordProxied = false;
	let savingRecord = false;
	let modalError = '';

	const providerId = $page.params.zoneId;

	const typeOrder = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA', 'PTR', 'SOA'];
	const recordTypes = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA'];
	const ttlOptions = [
		{ label: 'Auto', value: 1 },
		{ label: '1 min', value: 60 },
		{ label: '5 min', value: 300 },
		{ label: '1 hour', value: 3600 },
		{ label: '1 day', value: 86400 },
	];

	$: grouped = groupByType(records);
	$: isCloudflare = providerType === 'cloudflare';
	$: showProxied = isCloudflare && ['A', 'AAAA', 'CNAME'].includes(recordType);

	function groupByType(recs: DnsRecord[]) {
		const map: Record<string, DnsRecord[]> = {};
		for (const r of recs) {
			if (!map[r.type]) map[r.type] = [];
			map[r.type].push(r);
		}
		return Object.keys(map)
			.sort((a, b) => {
				const ai = typeOrder.indexOf(a);
				const bi = typeOrder.indexOf(b);
				return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
			})
			.map(type => ({ type, records: map[type].sort((a: DnsRecord, b: DnsRecord) => {
			if (a.name === domain) return -1;
			if (b.name === domain) return 1;
			return a.name.localeCompare(b.name);
		}) }));
	}

	function toggleSection(type: string) {
		collapsed[type] = !collapsed[type];
		collapsed = collapsed;
	}

	onMount(async () => {
		try {
			const [provRes, agentRes] = await Promise.all([
				api('/api/dns/providers'),
				api('/api/agents')
			]);
			if (provRes.ok) {
				const providers = await provRes.json();
				const match = providers.find((p: DnsProvider) => p.id === parseInt(providerId ?? ''));
				if (match) {
					domain = match.domain;
					providerType = match.provider;
				}
			}
			if (agentRes.ok) {
				agents = await agentRes.json();
			}
		} catch (err) {
			toastError('Failed to load DNS data');
		}

		await fetchRecords();
	});

	async function fetchRecords() {
		error = '';

		try {
			const res = await api(`/api/dns/providers/${providerId}/records`);
			if (res.ok) {
				records = await res.json();
				if (records.length > 0 && (records[0] as any).last_synced) {
					lastSynced = new Date((records[0] as any).last_synced);
				}
			} else {
				const data = await res.json();
				error = data.message || 'Failed to fetch records';
			}
		} catch (err) {
			error = 'Failed to connect to API';
		} finally {
			loading = false;
		}
	}

	async function syncRecords() {
		syncing = true;
		error = '';

		try {
			const res = await api(`/api/dns/providers/${providerId}/sync`, { method: 'POST' });
			if (res.ok) {
				records = await res.json();
				lastSynced = new Date();
			} else {
				const data = await res.json();
				error = data.message || 'Sync failed';
			}
		} catch (err) {
			error = 'Failed to sync records';
		} finally {
			syncing = false;
		}
	}

	function openAddModal() {
		editingRecord = null;
		recordType = 'A';
		recordName = '';
		recordContent = '';
		recordTTL = 1;
		recordProxied = false;
		modalError = '';
		showRecordModal = true;
	}

	function openEditModal(record: DnsRecord) {
		editingRecord = record;
		recordType = record.type;
		recordName = record.name;
		recordContent = record.content;
		recordTTL = record.ttl;
		recordProxied = record.proxied;
		modalError = '';
		showRecordModal = true;
	}

	function closeModal() {
		showRecordModal = false;
		editingRecord = null;
	}

	async function saveRecord() {
		if (!recordName.trim() || !recordContent.trim()) return;
		savingRecord = true;
		modalError = '';

		const body = {
			type: recordType,
			name: recordName.trim(),
			content: recordContent.trim(),
			ttl: recordTTL,
			proxied: showProxied ? recordProxied : false,
		};

		try {
			let url, method;
			if (editingRecord) {
				url = `/api/dns/providers/${providerId}/records/${editingRecord.id}`;
				method = 'PATCH';
			} else {
				url = `/api/dns/providers/${providerId}/records`;
				method = 'POST';
			}

			const res = await api(url, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body),
			});

			if (res.ok) {
				closeModal();
				await fetchRecords();
			} else {
				const data = await res.json();
				modalError = data.message || 'Failed to save record';
			}
		} catch (err) {
			modalError = 'Failed to connect to API';
		} finally {
			savingRecord = false;
		}
	}

	async function deleteRecord(record: DnsRecord) {
		if (!await confirmDialog(`Delete ${record.type} record "${record.name}"?`, { title: 'Delete Record', confirmLabel: 'Delete', danger: true })) return;

		try {
			const res = await api(`/api/dns/providers/${providerId}/records/${record.id}`, { method: 'DELETE' });
			if (res.ok) {
				records = records.filter(r => r.id !== record.id);
			} else {
				const data = await res.json();
				error = data.message || 'Failed to delete record';
			}
		} catch (err) {
			error = 'Failed to delete record';
		}
	}

	async function assignAgent(record: DnsRecord, event: Event) {
		const val = (event.target as HTMLSelectElement).value;
		const agentId = val === '' ? null : parseInt(val);

		try {
			const res = await api(`/api/dns/providers/${providerId}/records/${record.id}/agent`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ agent_id: agentId }),
			});
			if (res.ok) {
				const updated = await res.json();
				records = records.map(r => r.id === updated.id ? updated : r);
			} else {
				const data = await res.json();
				error = data.message || 'Failed to assign agent';
			}
		} catch (err) {
			error = 'Failed to assign agent';
		}
	}

	let syncingRecord: string | null = null;

	async function ddnsSync(record: DnsRecord) {
		syncingRecord = record.id;
		try {
			const res = await api(`/api/dns/providers/${providerId}/records/${record.id}/ddns-sync`, {
				method: 'POST',
			});
			if (res.ok) {
				const updated = await res.json();
				records = records.map(r => r.id === updated.id ? updated : r);
			} else {
				const data = await res.json();
				error = data.error || 'DDNS sync failed';
			}
		} catch (err) {
			error = 'Failed to sync';
		} finally {
			syncingRecord = null;
		}
	}

	// Build a map of agentId → agent for quick lookup
	$: agentMap = Object.fromEntries(agents.map((a: Agent) => [a.id, a]));

	function ddnsStatus(record: DnsRecord) {
		if (!record.agent_id) return null;
		const agent = agentMap[record.agent_id];
		if (!agent || !agent.wan_ip) return 'unknown';
		return record.content === agent.wan_ip ? 'synced' : 'pending';
	}

	function formatTTL(ttl: number) {
		if (ttl === 1) return 'Auto';
		if (ttl < 60) return `${ttl}s`;
		if (ttl < 3600) return `${Math.floor(ttl / 60)}m`;
		if (ttl < 86400) return `${Math.floor(ttl / 3600)}h`;
		return `${Math.floor(ttl / 86400)}d`;
	}


	function typeLabel(type: string) {
		const labels: Record<string, string> = {
			A: 'A Records', AAAA: 'AAAA Records', CNAME: 'CNAME Records',
			MX: 'MX Records', TXT: 'TXT Records', NS: 'NS Records',
			SRV: 'SRV Records', CAA: 'CAA Records', PTR: 'PTR Records', SOA: 'SOA Records',
		};
		return labels[type] || `${type} Records`;
	}

	function typeBadgeClass(type: string) {
		const map: Record<string, string> = {
			A: 'type-a', AAAA: 'type-aaaa', CNAME: 'type-cname',
			MX: 'type-mx', TXT: 'type-txt', NS: 'type-ns',
			SRV: 'type-srv', CAA: 'type-caa', PTR: 'type-ptr', SOA: 'type-soa',
		};
		return map[type] || '';
	}
</script>

<svelte:head>
	<title>{domain ? `DNS Records - ${domain}` : 'DNS Records'} - Proxera</title>
</svelte:head>

<div class="page">
	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading records...</p></div>
	{:else}
		<header class="page-head">
			<div class="head-left">
				<button class="breadcrumb" onclick={() => goto('/dns')}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
					DNS
				</button>
				<h1>{domain || 'DNS Records'}</h1>
				<p class="subtitle">
					{records.length} record{records.length !== 1 ? 's' : ''}
					{#if lastSynced}
						<span class="sync-info">synced {formatRelativeTime(lastSynced.toISOString())}</span>
					{/if}
				</p>
			</div>
			<div class="head-actions">
				<button class="btn-fill" onclick={openAddModal}>+ Add Record</button>
				<button class="btn-ghost" onclick={syncRecords} disabled={syncing}>
					<svg class="sync-icon" class:spinning={syncing} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<polyline points="23 4 23 10 17 10"></polyline>
						<polyline points="1 20 1 14 7 14"></polyline>
						<path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path>
					</svg>
					{syncing ? 'Syncing...' : 'Sync'}
				</button>
			</div>
		</header>

		{#if error}
			<div class="error-msg">{error}</div>
		{/if}

		{#if grouped.length > 0}
			<div class="sections">
				{#each grouped as group}
					<div class="section">
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<button class="section-header" onclick={() => toggleSection(group.type)}>
							<div class="section-title">
								<span class="type-badge {typeBadgeClass(group.type)}">{group.type}</span>
								<span class="section-label">{typeLabel(group.type)}</span>
								<span class="section-count">{group.records.length}</span>
							</div>
							<svg
								class="chevron"
								class:rotated={collapsed[group.type]}
								width="18" height="18" viewBox="0 0 24 24"
								fill="none" stroke="currentColor" stroke-width="2"
								stroke-linecap="round" stroke-linejoin="round"
							>
								<polyline points="6 9 12 15 18 9"></polyline>
							</svg>
						</button>

						{#if !collapsed[group.type]}
							<div class="section-body">
								<table>
									<thead>
										<tr>
											<th>Name</th>
											<th>Content</th>
											<th>TTL</th>
											{#if isCloudflare && (group.type === 'A' || group.type === 'AAAA' || group.type === 'CNAME')}
												<th>Proxy</th>
											{/if}
											{#if group.type === 'A' || group.type === 'AAAA'}
												<th>Agent</th>
											{/if}
											<th class="actions-th"></th>
										</tr>
									</thead>
									<tbody>
										{#each group.records as record}
											<tr>
												<td class="name-cell">{record.name}</td>
												<td class="content-cell">{record.content}</td>
												<td class="ttl-cell">{formatTTL(record.ttl)}</td>
												{#if isCloudflare && (group.type === 'A' || group.type === 'AAAA' || group.type === 'CNAME')}
													<td>
														<span class="proxy-badge" class:proxied={record.proxied}>
															{record.proxied ? 'On' : 'Off'}
														</span>
													</td>
												{/if}
												{#if group.type === 'A' || group.type === 'AAAA'}
													<td class="ddns-cell">
														<div class="ddns-row">
															<select
																class="agent-select"
																value={record.agent_id ?? ''}
																onchange={(e) => assignAgent(record, e)}
															>
																<option value="">None</option>
																{#each agents as agent}
																	<option value={agent.id}>{agent.name}</option>
																{/each}
															</select>
															{#if record.agent_id}
																{@const status = ddnsStatus(record)}
																{@const agentWan = agentMap[record.agent_id]?.wan_ip}
																<span
																	class="ddns-badge ddns-{status}"
																	title={status === 'synced'
																		? `In sync — record IP matches agent WAN IP (${agentWan})`
																		: status === 'pending'
																			? `Pending — agent WAN IP is ${agentWan}, record has ${record.content}`
																			: 'Agent WAN IP not yet known'}
																>DDNS</span>
																<button
																	class="ddns-sync-btn"
																	onclick={() => ddnsSync(record)}
																	disabled={syncingRecord === record.id}
																	title="Force DDNS sync now"
																	aria-label="Force DDNS sync"
																>
																	<svg class:spinning={syncingRecord === record.id} width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
																		<polyline points="23 4 23 10 17 10"></polyline>
																		<polyline points="1 20 1 14 7 14"></polyline>
																		<path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path>
																	</svg>
																</button>
															{/if}
														</div>
													</td>
												{/if}
												<td class="actions-cell">
													<button class="act act-accent" title="Edit" aria-label="Edit record" onclick={() => openEditModal(record)}>
														<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
															<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
															<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
														</svg>
													</button>
													<button class="act act-danger" title="Delete" aria-label="Delete record" onclick={() => deleteRecord(record)}>
														<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
															<polyline points="3 6 5 6 21 6"></polyline>
															<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
														</svg>
													</button>
												</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{:else if !error}
			<div class="placeholder"><p>No DNS records found for this domain.</p></div>
		{/if}
	{/if}
</div>

{#if showRecordModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="overlay" onclick={closeModal} onkeydown={() => {}}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<div class="modal-header">
				<h2>{editingRecord ? 'Edit Record' : 'Add Record'}</h2>
				<button class="close-btn" onclick={closeModal} aria-label="Close modal">
					<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<line x1="18" y1="6" x2="6" y2="18"></line>
						<line x1="6" y1="6" x2="18" y2="18"></line>
					</svg>
				</button>
			</div>

			<div class="modal-body">
				{#if modalError}
					<div class="error-msg">{modalError}</div>
				{/if}

				<div class="form-row">
					<div class="form-group">
						<label for="rec-type">Type</label>
						<select id="rec-type" bind:value={recordType}>
							{#each recordTypes as t}
								<option value={t}>{t}</option>
							{/each}
						</select>
					</div>
					<div class="form-group flex-1">
						<label for="rec-name">Name</label>
						<input id="rec-name" type="text" bind:value={recordName} placeholder="e.g. @ or subdomain" autocomplete="off" />
					</div>
				</div>

				<div class="form-group">
					<label for="rec-content">Content</label>
					<input id="rec-content" type="text" bind:value={recordContent} placeholder={recordType === 'A' ? 'e.g. 192.0.2.1' : recordType === 'AAAA' ? 'e.g. 2001:db8::1' : recordType === 'CNAME' ? 'e.g. target.example.com' : 'Value'} autocomplete="off" />
				</div>

				<div class="form-row">
					<div class="form-group">
						<label for="rec-ttl">TTL</label>
						<select id="rec-ttl" bind:value={recordTTL}>
							{#each ttlOptions as opt}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
					</div>
					{#if showProxied}
						<div class="form-group">
							<label for="rec-proxy">Proxy</label>
							<button
								id="rec-proxy"
								class="toggle-btn"
								class:active={recordProxied}
								onclick={() => recordProxied = !recordProxied}
								type="button"
							>
								<span class="toggle-track">
									<span class="toggle-thumb"></span>
								</span>
								<span class="toggle-label">{recordProxied ? 'Proxied' : 'DNS only'}</span>
							</button>
						</div>
					{/if}
				</div>

				<div class="modal-actions">
					<button class="btn-ghost" onclick={closeModal}>Cancel</button>
					<button
						class="btn-fill"
						onclick={saveRecord}
						disabled={!recordName.trim() || !recordContent.trim() || savingRecord}
					>
						{savingRecord ? 'Saving...' : (editingRecord ? 'Update Record' : 'Add Record')}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	/* ── Header extras ── */
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }
	.head-actions { display: flex; gap: 0.5rem; align-items: center; }
	.subtitle { color: var(--text-secondary); margin: 0; font-size: var(--text-sm); }
	.sync-info { color: var(--text-tertiary); }
	.sync-icon.spinning { animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }

	/* ── Sections ── */
	.sections { display: flex; flex-direction: column; gap: 0.5rem; }
	.section {
		background: var(--surface); border-radius: var(--radius-lg);
		border: 1px solid var(--border); overflow: hidden;
	}
	.section-header {
		display: flex; align-items: center; justify-content: space-between;
		width: 100%; padding: 0.75rem 1.25rem; background: var(--surface);
		border: none; cursor: pointer; font-family: inherit;
		transition: background var(--transition);
	}
	.section-header:hover { background: var(--surface-raised); }
	.section-title { display: flex; align-items: center; gap: 0.625rem; }
	.section-label { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
	.section-count {
		font-size: var(--text-xs); font-weight: 600; color: var(--text-tertiary);
		background: var(--bg); padding: 0.1rem 0.4rem;
		border-radius: var(--radius); border: 1px solid var(--border);
		font-family: var(--font-mono);
	}
	.chevron { color: var(--text-tertiary); transition: transform 0.2s; flex-shrink: 0; }
	.chevron.rotated { transform: rotate(-90deg); }
	.section-body { border-top: 1px solid var(--border); }

	/* ── Table ── */
	table { width: 100%; border-collapse: collapse; }
	thead th {
		text-align: left; padding: 0.5rem 1.25rem;
		font-weight: 600; color: var(--text-tertiary);
		font-size: var(--text-xs); text-transform: uppercase;
		letter-spacing: 0.04em; border-bottom: 1px solid var(--border);
	}
	.actions-th { width: 70px; }
	td {
		padding: 0.5rem 1.25rem; font-size: var(--text-sm);
		color: var(--text-secondary); border-bottom: 1px solid var(--border);
	}
	tbody tr:last-child td { border-bottom: none; }
	tbody tr:hover td { background: var(--surface-raised); }

	/* ── Type badges ── */
	.type-badge {
		display: inline-block; padding: 0.125rem 0.5rem;
		border-radius: var(--radius); font-size: var(--text-xs);
		font-weight: 700; font-family: var(--font-mono);
		background: var(--bg); color: var(--text-tertiary);
	}
	.type-a { background: rgba(77, 148, 255, 0.15); color: #4d94ff; }
	.type-aaaa { background: rgba(100, 120, 255, 0.15); color: #6478ff; }
	.type-cname { background: rgba(0, 229, 160, 0.15); color: #00e5a0; }
	.type-mx { background: rgba(255, 176, 32, 0.15); color: #ffb020; }
	.type-txt { background: rgba(168, 130, 255, 0.15); color: #a882ff; }
	.type-ns { background: rgba(255, 100, 120, 0.15); color: #ff6478; }
	.type-srv { background: rgba(77, 200, 255, 0.15); color: #4dc8ff; }
	.type-caa { background: rgba(255, 200, 60, 0.15); color: #ffc83c; }
	.type-ptr { background: rgba(80, 220, 180, 0.15); color: #50dcb4; }
	.type-soa { background: rgba(180, 140, 255, 0.15); color: #b48cff; }

	.name-cell {
		font-family: var(--font-mono); font-size: var(--text-xs);
		color: var(--text-primary); max-width: 280px;
		overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
	}
	.content-cell {
		font-family: var(--font-mono); font-size: var(--text-xs);
		max-width: 400px; overflow: hidden; text-overflow: ellipsis;
		white-space: nowrap; color: var(--text-secondary);
	}
	.ttl-cell {
		color: var(--text-tertiary); font-size: var(--text-xs);
		white-space: nowrap; font-family: var(--font-mono);
	}

	/* ── Proxy badge ── */
	.proxy-badge {
		display: inline-block; padding: 0.1rem 0.45rem;
		border-radius: var(--radius); font-size: var(--text-xs); font-weight: 600;
		background: var(--surface-raised); color: var(--text-tertiary);
	}
	.proxy-badge.proxied { background: var(--accent-dim); color: var(--accent); }

	/* ── Agent select & DDNS ── */
	.ddns-cell { white-space: nowrap; }
	.ddns-row { display: flex; align-items: center; gap: 0.375rem; flex-wrap: nowrap; }
	.agent-select {
		padding: 0.25rem 0.5rem; border: 1px solid var(--border);
		border-radius: var(--radius); font-size: var(--text-xs);
		font-family: var(--font-mono); color: var(--text-secondary);
		background: var(--bg); cursor: pointer;
		transition: border-color var(--transition); max-width: 140px;
	}
	.agent-select:hover { border-color: var(--border-bright); }
	.agent-select:focus { outline: none; border-color: var(--accent); }
	.agent-select option { background: var(--surface-raised); color: var(--text-primary); }
	.ddns-badge {
		display: inline-block; padding: 0.1rem 0.35rem;
		border-radius: var(--radius); font-size: 10px; font-weight: 700;
		font-family: var(--font-mono); letter-spacing: 0.04em;
		cursor: default; flex-shrink: 0;
	}
	.ddns-synced { background: rgba(0, 229, 160, 0.15); color: #00e5a0; }
	.ddns-pending { background: rgba(255, 176, 32, 0.15); color: #ffb020; }
	.ddns-unknown { background: var(--surface-raised); color: var(--text-tertiary); }
	.ddns-sync-btn {
		display: flex; align-items: center; justify-content: center;
		padding: 0.2rem; border: 1px solid var(--border);
		border-radius: var(--radius); background: var(--bg);
		color: var(--text-tertiary); cursor: pointer; flex-shrink: 0;
		transition: all var(--transition);
	}
	.ddns-sync-btn:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
	.ddns-sync-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.ddns-sync-btn svg { display: block; }
	.ddns-sync-btn svg.spinning { animation: spin 1s linear infinite; }

	/* ── Actions ── */
	.actions-cell { white-space: nowrap; text-align: right; }

	/* ── Modal (page-specific structure) ── */
	.modal-header {
		display: flex; align-items: center; padding: 1rem 1.25rem;
		border-bottom: 1px solid var(--border);
	}
	.modal-header h2 {
		font-size: var(--text-sm); font-weight: 600;
		color: var(--text-primary); margin: 0; flex: 1;
	}
	.close-btn {
		background: none; border: 1px solid var(--border);
		cursor: pointer; color: var(--text-secondary); padding: 0.3rem;
		border-radius: var(--radius); display: flex;
		align-items: center; justify-content: center;
		transition: all var(--transition);
	}
	.close-btn:hover { border-color: var(--border-bright); color: var(--text-primary); }
	.modal-body { padding: 1.25rem; }

	.form-row { display: flex; gap: 0.75rem; }
	.flex-1 { flex: 1; }
	.form-group { margin-bottom: 1rem; }
	.form-group label {
		display: block; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); margin-bottom: 0.375rem;
		text-transform: uppercase; letter-spacing: 0.04em;
	}
	.form-group input, .form-group select {
		width: 100%; padding: 0.5rem 0.625rem;
		border: 1px solid var(--border); border-radius: var(--radius);
		font-size: var(--text-sm); font-family: var(--font-mono);
		transition: border-color var(--transition); box-sizing: border-box;
		background: var(--bg); color: var(--text-primary);
	}
	.form-group input::placeholder { color: var(--text-tertiary); }
	.form-group input:focus, .form-group select:focus { outline: none; border-color: var(--accent); }
	.form-group select option { background: var(--surface-raised); color: var(--text-primary); }

	/* ── Toggle ── */
	.toggle-btn {
		display: flex; align-items: center; gap: 0.5rem;
		background: none; border: none; cursor: pointer;
		padding: 0.5rem 0; font-family: inherit;
		color: var(--text-secondary);
	}
	.toggle-track {
		width: 36px; height: 20px; background: var(--border);
		border-radius: 10px; position: relative; transition: background 0.2s;
	}
	.toggle-btn.active .toggle-track { background: var(--accent); }
	.toggle-thumb {
		position: absolute; top: 2px; left: 2px; width: 16px; height: 16px;
		background: white; border-radius: 50%; transition: transform 0.2s;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
	}
	.toggle-btn.active .toggle-thumb { transform: translateX(16px); }
	.toggle-label { font-size: var(--text-xs); color: var(--text-secondary); font-weight: 500; }

	.modal-actions {
		display: flex; justify-content: flex-end; gap: 0.5rem;
		margin-top: 0.5rem; padding-top: 1rem; border-top: 1px solid var(--border);
	}

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.head-actions { width: 100%; }
		.form-row { flex-direction: column; gap: 0; }
	}
</style>
