<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { PUBLIC_API_URL } from '$env/static/public';
	import { api } from '$lib/api';
	import { navRefresh } from '$lib/navRefresh';
	import { toastError, toastSuccess } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import { formatDateTime, formatRelativeTime, copyToClipboard } from '$lib/utils';
	import type { Agent } from '$lib/types';

	interface RegistrationResponse {
		agent_id: string;
		api_key: string;
	}

	let agents: Agent[] = [];
	let loading = true;
	let error: string | null = null;
	let showRegisterModal = false;
	let agentName = '';
	let registrationData: RegistrationResponse | null = null;
	let registerError = '';
	let updatingAgents = new Set<string>();
	let deployingAgents = new Set<string>();
	let upgradingNginxAgents = new Set<string>();
	let latestVersion = '';
	let expandedAgents = new Set<string>();
	let pendingTimeout: ReturnType<typeof setTimeout> | null = null;
	let renamingAgentId: string | null = null;
	let renameValue = '';

	function toggleExpand(agentId: string) {
		if (expandedAgents.has(agentId)) { expandedAgents.delete(agentId); }
		else { expandedAgents.add(agentId); }
		expandedAgents = expandedAgents;
	}

	const metricsIntervals = [
		{ value: 30, label: '30s' },
		{ value: 60, label: '1 min' },
		{ value: 120, label: '2 min' },
		{ value: 300, label: '5 min' },
		{ value: 600, label: '10 min' },
		{ value: 900, label: '15 min' },
	];

	onMount(async () => {
		await fetchAgents();
		try {
			const res = await api('/api/agent/version');
			if (res.ok) { const data = await res.json(); latestVersion = data.latest_version || ''; }
		} catch { toastError('Failed to fetch version info'); }
	});

	onDestroy(() => { if (pendingTimeout) clearTimeout(pendingTimeout); });

	async function fetchAgents() {
		try {
			const response = await api('/api/agents');
			if (!response.ok) throw new Error('Failed to fetch agents');
			agents = await response.json();
			loading = false;
		} catch (err) { error = err instanceof Error ? err.message : String(err); loading = false; }
	}

	async function registerAgent() {
		if (!agentName) return;
		registerError = '';
		try {
			const response = await api('/api/agents/register', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name: agentName, version: '0.0.0', os: 'pending', arch: 'pending' })
			});
			if (!response.ok) {
				const data = await response.json();
				registerError = data.error || 'Failed to register agent';
				return;
			}
			registrationData = await response.json();
			agentName = '';
			await fetchAgents();
			navRefresh.update(n => n + 1);
		} catch (err) { registerError = err instanceof Error ? err.message : String(err); }
	}

	async function deleteAgent(agentId: string) {
		if (!await confirmDialog('Are you sure you want to delete this agent?', { title: 'Delete Agent', confirmLabel: 'Delete', danger: true })) return;
		try {
			const response = await api(`/api/agents/${agentId}`, { method: 'DELETE' });
			if (!response.ok) throw new Error('Failed to delete agent');
			await fetchAgents();
			navRefresh.update(n => n + 1);
		} catch (err) { error = err instanceof Error ? err.message : String(err); }
	}

	async function updateAgent(agentId: string) {
		if (!await confirmDialog('Update this agent to the latest version? The agent will restart automatically.', { title: 'Update Agent', confirmLabel: 'Update' })) return;
		updatingAgents.add(agentId); updatingAgents = updatingAgents;
		try {
			const response = await api(`/api/agents/${agentId}/update`, { method: 'POST' });
			if (!response.ok) {
				let errorMsg = 'Failed to send update command';
				try { const text = await response.text(); try { const data = JSON.parse(text); errorMsg = data.error || errorMsg; } catch { errorMsg = text || `HTTP ${response.status}: ${response.statusText}`; } } catch (e) { errorMsg = `HTTP ${response.status}: ${response.statusText}`; }
				throw new Error(errorMsg);
			}
			const result = await response.json();
			toastSuccess(result.message || 'Update command sent successfully');
			pendingTimeout = setTimeout(() => { fetchAgents(); }, 5000);
		} catch (err) { toastError('Failed to update agent: ' + (err instanceof Error ? err.message : String(err))); }
		finally { updatingAgents.delete(agentId); updatingAgents = updatingAgents; }
	}

	async function deployAndReload(agentId: string) {
		deployingAgents.add(agentId); deployingAgents = deployingAgents;
		try {
			const deployRes = await api(`/api/agents/${agentId}/deploy`, { method: 'POST' });
			const deployData = await deployRes.json();
			if (!deployRes.ok) throw new Error(deployData.error || 'Deploy failed');

			const reloadRes = await api(`/api/agents/${agentId}/reload`, { method: 'POST' });
			const reloadData = await reloadRes.json();
			if (!reloadRes.ok) throw new Error(reloadData.error || 'Reload failed');

			toastSuccess('Deploy & reload completed successfully');
		} catch (err) { toastError('Deploy & reload failed: ' + (err instanceof Error ? err.message : String(err))); }
		finally { deployingAgents.delete(agentId); deployingAgents = deployingAgents; }
	}

	function truncateAgentId(agentId: string) {
		if (!agentId || agentId.length <= 22) return agentId;
		return agentId.substring(0, 22) + '...';
	}

	function isNginxOutdated(version: string) {
		if (!version) return false;
		const parts = version.split('.').map(Number);
		if (parts[0] < 1) return true;
		if (parts[0] === 1 && parts[1] < 28) return true;
		return false;
	}

	async function upgradeNginx(agentId: string) {
		if (!await confirmDialog('Upgrade nginx to the latest version from nginx.org? This may cause a brief restart.', { title: 'Upgrade Nginx', confirmLabel: 'Upgrade' })) return;
		upgradingNginxAgents.add(agentId); upgradingNginxAgents = upgradingNginxAgents;
		try {
			const response = await api(`/api/agents/${agentId}/upgrade-nginx`, { method: 'POST' });
			if (!response.ok) {
				let errorMsg = 'Failed to send upgrade command';
				try { const data = await response.json(); errorMsg = data.error || errorMsg; } catch {}
				throw new Error(errorMsg);
			}
			const result = await response.json();
			toastSuccess(result.message || 'Nginx upgrade command sent');
			pendingTimeout = setTimeout(() => { fetchAgents(); }, 15000);
		} catch (err) { toastError('Nginx upgrade failed: ' + (err instanceof Error ? err.message : String(err))); }
		finally { upgradingNginxAgents.delete(agentId); upgradingNginxAgents = upgradingNginxAgents; }
	}

	function startRename(agent: Agent) {
		renamingAgentId = agent.agent_id;
		renameValue = agent.name;
	}

	function cancelRename() {
		renamingAgentId = null;
		renameValue = '';
	}

	async function doRename(agentId: string) {
		const name = renameValue.trim();
		if (!name) return;
		try {
			const response = await api(`/api/agents/${agentId}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name })
			});
			if (!response.ok) { const d = await response.json(); throw new Error(d.error || 'Failed to rename'); }
			agents = agents.map(a => a.agent_id === agentId ? { ...a, name } : a);
			toastSuccess('Agent renamed');
		} catch (err) { toastError('Rename failed: ' + (err instanceof Error ? err.message : String(err))); }
		finally { cancelRename(); }
	}

	async function setMetricsInterval(agentId: string, e: Event) {
		const seconds = parseInt((e.target as HTMLSelectElement).value);
		try {
			const response = await api(`/api/agents/${agentId}/metrics-interval`, {
				method: 'POST', headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ interval: seconds })
			});
			if (!response.ok) { const data = await response.json(); throw new Error(data.error || 'Failed to set interval'); }
		} catch (err) { toastError('Failed to set metrics interval: ' + (err instanceof Error ? err.message : String(err))); }
	}
</script>

<svelte:head><title>Agents - Proxera</title></svelte:head>

<div class="page">
	<header class="page-head">
		<h1>Agents</h1>
		<button class="btn-fill" on:click={() => { showRegisterModal = true; registerError = '';}}>
			<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
			Register Agent
		</button>
	</header>

	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading agents...</p></div>
	{:else if error}
		<div class="placeholder error" aria-live="assertive"><p>{error}</p></div>
	{:else if agents.length === 0}
		<div class="placeholder">
			<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
			<h2>No agents registered</h2>
			<p>Get started by registering your first agent</p>
			<button class="btn-fill" on:click={() => { showRegisterModal = true; registerError = '';}}>Register Agent</button>
		</div>
	{:else}
		<div class="tbl">
			<div class="tbl-head">
				<span class="th">Agent</span>
				<span class="th">OS</span>
				<span class="th">Hosts</span>
				<span class="th">WAN IP</span>
				<span class="th">Last Seen</span>
				<span class="th">Metrics</span>
				<span class="th">Actions</span>
			</div>
			{#each agents as agent}
				<div class="tbl-row-wrap" class:expanded={expandedAgents.has(agent.agent_id)}>
					<div class="tbl-row" on:click={() => renamingAgentId !== agent.agent_id && toggleExpand(agent.agent_id)} on:keydown={(e) => e.key === 'Enter' && renamingAgentId !== agent.agent_id && toggleExpand(agent.agent_id)} role="button" tabindex="0" aria-expanded={expandedAgents.has(agent.agent_id)}>
						<span class="td td-name">
							<svg class="chevron" class:chevron-open={expandedAgents.has(agent.agent_id)} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
							<span class="dot" class:dot-on={agent.status === 'online'} class:dot-err={agent.status === 'error'}></span>
							{#if renamingAgentId === agent.agent_id}
								<!-- svelte-ignore a11y-click-events-have-key-events -->
								<!-- svelte-ignore a11y_autofocus -->
								<input
									class="rename-input"
									type="text"
									bind:value={renameValue}
									on:click|stopPropagation
									on:keydown={(e) => { if (e.key === 'Enter') doRename(agent.agent_id); else if (e.key === 'Escape') cancelRename(); }}
									autofocus
								/>
								<button class="act act-ok" on:click|stopPropagation={() => doRename(agent.agent_id)} title="Save">
									<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
								</button>
								<button class="act act-cancel" on:click|stopPropagation={cancelRename} title="Cancel">
									<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
								</button>
							{:else}
								<span class="agent-name">{agent.name}</span>
								<span class="badge" class:badge-on={agent.status === 'online'} class:badge-off={agent.status !== 'online'}>{agent.status}</span>
								<span class="pill" class:pill-ok={agent.version && latestVersion && agent.version === latestVersion} class:pill-outdated={agent.version && latestVersion && agent.version !== latestVersion}>{agent.version || '—'}</span>
							{/if}
						</span>
						<span class="td td-os mono">{agent.os || '—'}/{agent.arch || '—'}</span>
						<span class="td td-hosts">{agent.host_count}</span>
						<span class="td td-ip mono">{agent.wan_ip || '—'}</span>
						<span class="td td-seen" title={formatDateTime(agent.last_seen)}>{formatRelativeTime(agent.last_seen)}</span>
						<!-- svelte-ignore a11y-click-events-have-key-events -->
						<span class="td td-metrics" on:click|stopPropagation role="cell" tabindex="0">
							<select class="sel-sm" on:change={(e) => setMetricsInterval(agent.agent_id, e)} disabled={agent.status !== 'online'}>
								{#each metricsIntervals as mi}
									<option value={mi.value} selected={mi.value === (agent.metrics_interval || 300)}>{mi.label}</option>
								{/each}
							</select>
						</span>
						<!-- svelte-ignore a11y-click-events-have-key-events -->
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
						<span class="td td-actions" on:click|stopPropagation role="group">
							<button class="act act-ghost" on:click={() => startRename(agent)} title="Rename" aria-label="Rename agent">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
							</button>
							<button class="act act-info" on:click={() => goto(`/agents/${agent.agent_id}/logs`)} disabled={agent.status !== 'online'} title="Logs" aria-label="View logs">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
							</button>
							<button class="act act-accent" on:click={() => deployAndReload(agent.agent_id)} disabled={deployingAgents.has(agent.agent_id) || agent.status !== 'online'} title="Deploy & Reload" aria-label="Deploy and reload">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 16 12 12 8 16"/><line x1="12" y1="12" x2="12" y2="21"/><path d="M20.39 18.39A5 5 0 0 0 18 9h-1.26A8 8 0 1 0 3 16.3"/></svg>
							</button>
							<button class="act act-accent" on:click={() => updateAgent(agent.agent_id)} disabled={updatingAgents.has(agent.agent_id) || agent.status !== 'online'} title="Update" aria-label="Update agent">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
							</button>
							<button class="act act-danger" on:click={() => deleteAgent(agent.agent_id)} title="Delete" aria-label="Delete agent">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
							</button>
						</span>
					</div>

					{#if expandedAgents.has(agent.agent_id)}
						<div class="tbl-detail">
							<div class="detail-grid">
								<div class="detail-cell">
									<span class="lbl">Agent ID</span>
									<span class="val mono" title={agent.agent_id}>{truncateAgentId(agent.agent_id)}</span>
								</div>
								<div class="detail-cell">
									<span class="lbl">LAN IP</span>
									<span class="val mono">{agent.lan_ip || '—'}</span>
								</div>
								<div class="detail-cell">
									<span class="lbl">DNS Records</span>
									<span class="val">{agent.dns_record_count}</span>
								</div>
								<div class="detail-cell">
									<span class="lbl">CrowdSec</span>
									<span class="val" class:val-ok={agent.crowdsec_installed} class:val-warn={!agent.crowdsec_installed}>{agent.crowdsec_installed ? 'Installed' : 'Not installed'}</span>
								</div>
								<div class="detail-cell">
									<span class="lbl">Nginx</span>
									<span class="val nginx-ver">
										<span class:val-ok={agent.nginx_version && !isNginxOutdated(agent.nginx_version)} class:val-warn={!agent.nginx_version || isNginxOutdated(agent.nginx_version)}>{agent.nginx_version || '—'}</span>
										{#if agent.status === 'online' && isNginxOutdated(agent.nginx_version)}
											<button class="btn-upgrade" on:click|stopPropagation={() => upgradeNginx(agent.agent_id)} disabled={upgradingNginxAgents.has(agent.agent_id)}>
												{upgradingNginxAgents.has(agent.agent_id) ? 'Upgrading...' : 'Upgrade'}
											</button>
										{/if}
									</span>
								</div>
								<div class="detail-cell">
									<span class="lbl">Registered</span>
									<span class="val" title={formatDateTime(agent.created_at)}>{formatRelativeTime(agent.created_at)}</span>
								</div>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Register Modal -->
{#if showRegisterModal}
	<div class="overlay" on:click={() => { showRegisterModal = false; registrationData = null; }} on:keydown={(e) => e.key === 'Escape' && (showRegisterModal = false, registrationData = null)} role="button" tabindex="0">
		<div class="modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog" aria-modal="true" tabindex="-1">
			{#if registrationData}
				<h2>Agent Registered</h2>
				<p class="modal-sub">Copy and run this command on your server:</p>

				<div class="modal-section">
					<div class="section-top">
						<span class="section-label">Installation Command</span>
						<button class="btn-copy" on:click={() => copyToClipboard(`curl -sSL ${PUBLIC_API_URL}/install.sh | sudo bash -s -- ${registrationData!.agent_id} ${registrationData!.api_key} ${PUBLIC_API_URL}`)}>
							<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
							Copy
						</button>
					</div>
					<div class="code-block">
						<code>curl -sSL {PUBLIC_API_URL}/install.sh | sudo bash -s -- {registrationData.agent_id} {registrationData.api_key} {PUBLIC_API_URL}</code>
					</div>
				</div>

				<div class="modal-section">
					<span class="section-label">What This Does</span>
					<ul class="check-list">
						<li>Downloads the Proxera agent binary</li>
						<li>Installs agent to /usr/local/bin/proxera</li>
						<li>Sets up nginx automatically (if not installed)</li>
						<li>Creates configuration with your credentials</li>
						<li>Installs systemd service for auto-start</li>
						<li>Connects agent to your panel</li>
					</ul>
					<p class="note">The agent will appear online in your dashboard within a few seconds.</p>
				</div>

				<div class="modal-section">
					<span class="section-label">Save These Credentials</span>
					<p class="section-sub">Keep these safe for manual configuration if needed:</p>
					<div class="cred-rows">
						<div class="cred-row">
							<span class="cred-label">Agent ID</span>
							<div class="cred-val">
								<code>{registrationData.agent_id}</code>
								<button class="btn-copy-sm" on:click={() => copyToClipboard(registrationData!.agent_id)}>Copy</button>
							</div>
						</div>
						<div class="cred-row">
							<span class="cred-label">API Key</span>
							<div class="cred-val">
								<code>{registrationData.api_key}</code>
								<button class="btn-copy-sm" on:click={() => copyToClipboard(registrationData!.api_key)}>Copy</button>
							</div>
						</div>
					</div>
				</div>

				<div class="modal-section">
					<span class="section-label">Useful Commands</span>
					<div class="cmd-list">
						<div class="cmd-row"><code>sudo systemctl status proxera</code><span>Check status</span></div>
						<div class="cmd-row"><code>sudo journalctl -u proxera -f</code><span>View logs</span></div>
						<div class="cmd-row"><code>sudo systemctl restart proxera</code><span>Restart</span></div>
					</div>
				</div>

				<button class="btn-fill btn-full" on:click={() => { showRegisterModal = false; registrationData = null; }}>Done</button>
			{:else}
				<h2>Register New Agent</h2>
				<p class="modal-sub">Enter a friendly name for your agent:</p>
				{#if registerError}
					<div class="register-error">
						<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
						{registerError}
					</div>
				{/if}
				<input type="text" bind:value={agentName} placeholder="e.g., Production Server 1" class="input" />
				<div class="modal-foot">
					<button class="btn-ghost" on:click={() => showRegisterModal = false}>Cancel</button>
					<button class="btn-fill" on:click={registerAgent} disabled={!agentName}>Register</button>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	/* ── Buttons (page-specific) ── */
	.btn-full { width: 100%; justify-content: center; padding: 0.625rem; }

	/* ── Table ── */
	.tbl {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); overflow: hidden;
	}
	.tbl-head {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr);
		gap: 0.75rem;
		padding: 0.625rem 1.25rem;
		background: var(--surface-raised);
		border-bottom: 1px solid var(--border);
	}
	.th {
		font-size: var(--text-xs); font-weight: 600; color: var(--text-tertiary);
		text-transform: uppercase; letter-spacing: 0.04em;
		text-align: left;
	}

	/* ── Row wrapper ── */
	.tbl-row-wrap { border-bottom: 1px solid var(--border); }
	.tbl-row-wrap:last-child { border-bottom: none; }
	.tbl-row-wrap.expanded { background: var(--surface-raised); }

	/* ── Row ── */
	.tbl-row {
		display: grid;
		grid-template-columns: minmax(0, 3fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr);
		gap: 0.75rem;
		padding: 0.75rem 1.25rem;
		align-items: center;
		cursor: pointer;
		transition: background var(--transition);
	}
	.tbl-row:hover { background: var(--surface-raised); }

	.td { font-size: var(--text-sm); color: var(--text-primary); text-align: left; }
	.td.mono { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); }

	/* ── Name cell ── */
	.td-name { display: flex; align-items: center; gap: 0.5rem; min-width: 0; }
	.agent-name { font-weight: 600; color: var(--text-primary); font-size: var(--text-sm); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	.dot {
		width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0;
		background: var(--text-muted);
	}
	.dot-on { background: var(--success); }
	.dot-err { background: var(--danger); }

	.chevron {
		flex-shrink: 0; color: var(--text-muted);
		transition: transform 0.2s ease;
	}
	.chevron-open { transform: rotate(180deg); }

	.badge {
		font-size: 0.675rem; font-weight: 600; text-transform: capitalize;
		padding: 0.125rem 0.5rem; border-radius: 20px;
		background: var(--text-muted); color: var(--bg); line-height: 1.4;
	}
	.badge-on { background: var(--success); color: #fff; }
	.badge-off { background: var(--text-muted); color: var(--surface); }

	.pill {
		font-size: 0.675rem; font-weight: 500;
		font-family: var(--font-mono);
		padding: 0.125rem 0.4rem; border-radius: var(--radius);
		background: var(--surface-raised); color: var(--text-secondary);
		border: 1px solid var(--border); line-height: 1.4;
	}
	.pill-ok { color: var(--success); background: var(--success-dim); border-color: var(--success); }
	.pill-outdated { color: var(--danger); background: var(--danger-dim); border-color: var(--danger); }

	/* ── Actions cell ── */
	.td-actions { display: flex; gap: 0.25rem; }

	.act-ghost { background: transparent; color: var(--text-tertiary); border: 1px solid var(--border); }
	.act-ghost:hover:not(:disabled) { color: var(--text-primary); border-color: var(--border-bright); background: var(--surface-raised); }

	.act-info { background: transparent; color: var(--info); border: 1px solid var(--info); }
	.act-info:hover:not(:disabled) { background: var(--info-dim); }

	.act-ok { background: transparent; color: var(--success); border: 1px solid var(--success); }
	.act-ok:hover:not(:disabled) { background: var(--success-dim); }

	.act-cancel { background: transparent; color: var(--text-tertiary); border: 1px solid var(--border); }
	.act-cancel:hover:not(:disabled) { color: var(--danger); border-color: var(--danger); background: var(--danger-dim); }

	/* ── Inline rename input ── */
	.rename-input {
		flex: 1; min-width: 0;
		padding: 0.1rem 0.375rem;
		border: 1px solid var(--accent);
		border-radius: var(--radius);
		background: var(--bg);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 600;
		font-family: inherit;
		outline: none;
	}

	/* ── Expanded detail ── */
	.tbl-detail {
		padding: 0.75rem 1.25rem 1rem 2.75rem;
		border-top: 1px solid var(--border);
	}
	.detail-grid {
		display: grid; grid-template-columns: repeat(6, 1fr);
		gap: 0.75rem 1.5rem;
	}
	.detail-cell { display: flex; flex-direction: column; gap: 0.125rem; }
	.lbl { color: var(--text-tertiary); font-size: var(--text-xs); font-weight: 500; }
	.val { color: var(--text-primary); font-weight: 500; font-size: var(--text-sm); }
	.val.mono { font-family: var(--font-mono); font-size: var(--text-xs); }
	.val-ok { color: var(--success); }
	.val-warn { color: var(--text-muted); }

	.nginx-ver { display: flex; align-items: center; gap: 0.5rem; }
	.btn-upgrade {
		padding: 0.125rem 0.5rem; border-radius: var(--radius);
		font-size: 0.675rem; font-weight: 600; cursor: pointer;
		background: var(--warning-dim, rgba(245,158,11,0.1)); color: var(--warning, #f59e0b);
		border: 1px solid var(--warning, #f59e0b);
		transition: background var(--transition);
	}
	.btn-upgrade:hover:not(:disabled) { background: var(--warning, #f59e0b); color: #fff; }
	.btn-upgrade:disabled { opacity: 0.5; cursor: not-allowed; }

	.sel-sm {
		padding: 0.2rem 0.375rem; border: 1px solid var(--border);
		border-radius: var(--radius); font-size: var(--text-xs);
		color: var(--text-primary); background: var(--bg); cursor: pointer;
		font-weight: 500; width: fit-content;
	}
	.sel-sm:focus { outline: none; border-color: var(--accent); }
	.sel-sm:disabled { opacity: 0.3; cursor: not-allowed; }

	/* ── Local input override ── */
	.input { margin-bottom: 1.25rem; }

	/* ── Generic register error ── */
	.register-error {
		display: flex; align-items: center; gap: 0.5rem;
		background: var(--danger-dim); border: 1px solid var(--danger);
		border-radius: var(--radius); padding: 0.625rem 0.875rem;
		font-size: var(--text-sm); color: var(--danger);
		margin-bottom: 1rem;
	}

	/* ── Modal sections ── */
	.modal-section {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 1.25rem;
		margin-bottom: 1rem;
	}

	.section-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; }
	.section-top .section-label { margin-bottom: 0; }

	.section-label {
		display: block; font-size: var(--text-xs); font-weight: 600;
		color: var(--text-tertiary); text-transform: uppercase;
		letter-spacing: 0.04em; margin-bottom: 0.75rem;
	}

	.section-sub { margin: 0 0 0.75rem; color: var(--text-tertiary); font-size: var(--text-sm); }

	.btn-copy {
		background: transparent; color: var(--accent); border: 1px solid var(--accent);
		padding: 0.375rem 0.75rem; border-radius: var(--radius);
		cursor: pointer; font-size: var(--text-xs); font-weight: 600;
		display: inline-flex; align-items: center; gap: 0.375rem;
		transition: background var(--transition);
	}
	.btn-copy:hover { background: var(--accent-dim); }

	.code-block {
		background: var(--bg); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 0.875rem 1rem;
	}
	.code-block code {
		display: block; color: var(--accent); font-size: var(--text-sm);
		line-height: 1.6; word-break: break-all; white-space: pre-wrap;
		font-family: var(--font-mono); background: none; border: none; padding: 0;
	}

	.check-list { margin: 0; padding: 0; list-style: none; }
	.check-list li {
		padding: 0.375rem 0; color: var(--text-secondary); font-size: var(--text-sm);
		display: flex; align-items: center; gap: 0.625rem;
	}
	.check-list li::before {
		content: ''; width: 16px; height: 16px; flex-shrink: 0;
		border: 2px solid var(--accent); border-radius: 3px;
		background: var(--accent-dim);
		display: inline-flex; align-items: center; justify-content: center;
	}

	.note {
		margin: 1rem 0 0; padding: 0.625rem 0.875rem;
		background: var(--info-dim); border-left: 3px solid var(--info);
		border-radius: var(--radius); font-size: var(--text-sm); color: var(--info);
	}

	.cred-rows { display: flex; flex-direction: column; gap: 0.75rem; }
	.cred-row { display: flex; flex-direction: column; gap: 0.375rem; }
	.cred-label { font-size: var(--text-xs); color: var(--text-tertiary); text-transform: uppercase; letter-spacing: 0.04em; font-weight: 600; }
	.cred-val { display: flex; gap: 0.5rem; align-items: center; }
	.cred-val code {
		flex: 1; background: var(--bg); border: 1px solid var(--border);
		padding: 0.5rem 0.75rem; border-radius: var(--radius);
		font-size: var(--text-sm); font-family: var(--font-mono);
		color: var(--text-primary); overflow-x: auto;
	}
	.btn-copy-sm {
		background: transparent; color: var(--text-secondary);
		border: 1px solid var(--border); padding: 0.5rem 0.75rem;
		border-radius: var(--radius); cursor: pointer;
		font-size: var(--text-xs); font-weight: 500;
		transition: all var(--transition);
	}
	.btn-copy-sm:hover { border-color: var(--border-bright); color: var(--text-primary); }

	.cmd-list { display: flex; flex-direction: column; gap: 0.5rem; }
	.cmd-row {
		display: flex; align-items: center; gap: 1rem;
		padding: 0.625rem 0.875rem; background: var(--bg);
		border: 1px solid var(--border); border-radius: var(--radius);
	}
	.cmd-row code {
		flex: 1; font-size: var(--text-sm); color: var(--text-primary);
		font-family: var(--font-mono); background: none; border: none; padding: 0;
	}
	.cmd-row span { color: var(--text-tertiary); font-size: var(--text-xs); white-space: nowrap; }

	/* ── Responsive ── */
	@media (max-width: 900px) {
		.page { padding: 1.25rem; }
		h1 { font-size: var(--text-xl); }
		.page-head { flex-direction: column; gap: 0.75rem; align-items: flex-start; }
		.tbl-head { display: none; }
		.tbl-row {
			grid-template-columns: 1fr;
			gap: 0.25rem;
		}
		.td-name { order: 1; }
		.td-os, .td-hosts, .td-ip, .td-seen, .td-metrics { display: none; }
		.td-actions { order: 2; justify-content: flex-start; }
		.detail-grid { grid-template-columns: 1fr 1fr; }
	}
</style>
