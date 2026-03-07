<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { toastError, toastSuccess } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';

	const agentId = $page.params.agentId;
	let activeTab = 'overview';
	let loading = true;
	let error = null;
	let installing = false;
	let uninstalling = false;
	let enrollmentKey = '';

	// Consent modal
	let showConsentModal = false;
	let consentChecked = false;

	// Agent selector
	let agents = [];
	let agentName = agentId;

	// Data states
	let status = null;
	let decisions = [];
	let alerts = [];
	let collections = [];
	let whitelist = [];
	let bouncers = [];
	let metricsData = null;

	// Form states
	let banIP = '';
	let banDuration = '24h';
	let banReason = '';
	let whitelistIP = '';
	let whitelistDesc = '';
	let collectionName = '';
	let addingDecision = false;
	let deletingDecisions = new Set();
	let deletingAlerts = new Set();
	let addingCollection = false;
	let removingCollections = new Set();
	let addingWhitelist = false;
	let removingWhitelist = new Set();
	let removingBouncers = new Set();
	let installingCollections = new Set();

	const popularCollections = [
		{ name: 'crowdsecurity/wordpress', desc: 'WordPress-specific attack detection' },
		{ name: 'crowdsecurity/iptables', desc: 'Iptables log parser for firewall-level detection' },
		{ name: 'crowdsecurity/postfix', desc: 'Postfix SMTP abuse detection' },
		{ name: 'crowdsecurity/dovecot', desc: 'Dovecot IMAP/POP3 brute-force detection' },
		{ name: 'crowdsecurity/mariadb', desc: 'MariaDB/MySQL brute-force detection' },
		{ name: 'crowdsecurity/pgsql', desc: 'PostgreSQL brute-force detection' },
		{ name: 'crowdsecurity/endlessh', desc: 'SSH tarpit integration for wasting attacker time' },
		{ name: 'crowdsecurity/apache2', desc: 'Apache2 log parser and attack scenarios' },
		{ name: 'crowdsecurity/proftpd', desc: 'ProFTPD brute-force detection' },
		{ name: 'crowdsecurity/grafana', desc: 'Grafana brute-force and CVE detection' },
	];

	let installingBouncers = new Set();

	const popularBouncers = [
		{ pkg: 'crowdsec-firewall-bouncer-iptables', name: 'Firewall Bouncer (iptables)', desc: 'Blocks IPs at the kernel level via iptables - drops packets before they reach nginx' },
		{ pkg: 'crowdsec-firewall-bouncer-nftables', name: 'Firewall Bouncer (nftables)', desc: 'Blocks IPs at the kernel level via nftables - modern replacement for iptables' },
		{ pkg: 'crowdsec-cloudflare-bouncer', name: 'Cloudflare Bouncer', desc: 'Pushes ban decisions to Cloudflare firewall rules' },
		{ pkg: 'crowdsec-blocklist-mirror', name: 'Blocklist Mirror', desc: 'Serves CrowdSec decisions as a blocklist over HTTP for external systems' },
		{ pkg: 'crowdsec-custom-bouncer', name: 'Custom Bouncer', desc: 'Generic bouncer that executes scripts/webhooks on ban decisions' },
	];

	// Loading states per tab
	let tabLoading = false;

	const durations = [
		{ value: '1h', label: '1 hour' },
		{ value: '4h', label: '4 hours' },
		{ value: '12h', label: '12 hours' },
		{ value: '24h', label: '24 hours' },
		{ value: '48h', label: '48 hours' },
		{ value: '168h', label: '7 days' },
		{ value: '720h', label: '30 days' },
	];

	async function apiGet(path) {
		const res = await api(`/api/agents/${agentId}/crowdsec${path}`);
		if (!res.ok) {
			const data = await res.json().catch(() => ({}));
			throw new Error(data.error || `Request failed: ${res.status}`);
		}
		const text = await res.text();
		try { return JSON.parse(text); } catch { return text; }
	}

	async function apiPost(path, body = {}) {
		const res = await api(`/api/agents/${agentId}/crowdsec${path}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body)
		});
		if (!res.ok) {
			const data = await res.json().catch(() => ({}));
			throw new Error(data.error || `Request failed: ${res.status}`);
		}
		return res.json();
	}

	async function apiDelete(path) {
		const res = await api(`/api/agents/${agentId}/crowdsec${path}`, { method: 'DELETE' });
		if (!res.ok) {
			const data = await res.json().catch(() => ({}));
			throw new Error(data.error || `Request failed: ${res.status}`);
		}
		return res.json();
	}

	onMount(async () => {
		await fetchAgents();
		await fetchStatus();
	});

	async function fetchAgents() {
		try {
			const res = await api('/api/agents');
			if (res.ok) {
				agents = await res.json();
				const current = agents.find(a => a.agent_id === agentId);
				if (current) agentName = current.name;
			}
		} catch { toastError('Failed to load agents'); }
	}

	async function fetchStatus() {
		loading = true;
		error = null;
		try {
			status = await apiGet('/status');
			loading = false;
			if (status && status.installed) {
				loadTabData('overview');
			}
		} catch (err) {
			error = err.message;
			loading = false;
		}
	}

	function openInstallConsent() {
		consentChecked = false;
		showConsentModal = true;
	}

	async function installCrowdSec() {
		showConsentModal = false;
		installing = true;
		error = null;
		try {
			await apiPost('/install', { enrollment_key: enrollmentKey });
			toastSuccess('CrowdSec installed successfully!');
			enrollmentKey = '';
			await fetchStatus();
		} catch (err) {
			error = err.message;
			toastError('Installation failed: ' + err.message);
		} finally {
			installing = false;
		}
	}

	async function uninstallCrowdSec() {
		if (!await confirmDialog('Are you sure you want to uninstall CrowdSec? All protection will be removed.', { title: 'Uninstall CrowdSec', confirmLabel: 'Uninstall', danger: true })) return;
		uninstalling = true;
		error = null;
		try {
			await apiPost('/uninstall');
			toastSuccess('CrowdSec uninstalled successfully');
			await fetchStatus();
		} catch (err) {
			error = err.message;
			toastError('Uninstall failed: ' + err.message);
		} finally {
			uninstalling = false;
		}
	}

	async function switchTab(tab) {
		activeTab = tab;
		await loadTabData(tab);
	}

	async function loadTabData(tab) {
		tabLoading = true;
		error = null;
		try {
			switch (tab) {
				case 'overview':
					status = await apiGet('/status');
					bouncers = await apiGet('/bouncers') || [];
					break;
				case 'decisions':
					decisions = await apiGet('/decisions') || [];
					break;
				case 'alerts':
					alerts = await apiGet('/alerts') || [];
					break;
				case 'collections':
					collections = await apiGet('/collections') || [];
					break;
				case 'whitelist':
					whitelist = await apiGet('/whitelist') || [];
					break;
				case 'bouncers':
					bouncers = await apiGet('/bouncers') || [];
					break;
				case 'metrics':
					metricsData = await apiGet('/metrics');
					break;
			}
		} catch (err) {
			error = err.message;
		} finally {
			tabLoading = false;
		}
	}

	async function addDecision() {
		if (!banIP) return;
		addingDecision = true;
		try {
			await apiPost('/decisions', { ip: banIP, duration: banDuration, reason: banReason });
			banIP = ''; banReason = '';
			await loadTabData('decisions');
		} catch (err) {
			toastError('Failed to ban IP: ' + err.message);
		} finally {
			addingDecision = false;
		}
	}

	async function deleteDecision(id) {
		if (!await confirmDialog('Remove this ban?', { title: 'Remove Ban', confirmLabel: 'Remove', danger: true })) return;
		deletingDecisions.add(id); deletingDecisions = deletingDecisions;
		try {
			await apiDelete(`/decisions/${id}`);
			await loadTabData('decisions');
		} catch (err) {
			toastError('Failed to remove ban: ' + err.message);
		} finally {
			deletingDecisions.delete(id); deletingDecisions = deletingDecisions;
		}
	}

	async function deleteAlert(id) {
		if (!await confirmDialog('Delete this alert?', { title: 'Delete Alert', confirmLabel: 'Delete', danger: true })) return;
		deletingAlerts.add(id); deletingAlerts = deletingAlerts;
		try {
			await apiDelete(`/alerts/${id}`);
			await loadTabData('alerts');
		} catch (err) {
			toastError('Failed to delete alert: ' + err.message);
		} finally {
			deletingAlerts.delete(id); deletingAlerts = deletingAlerts;
		}
	}

	async function installCollection() {
		if (!collectionName) return;
		addingCollection = true;
		try {
			await apiPost('/collections', { name: collectionName });
			collectionName = '';
			await loadTabData('collections');
		} catch (err) {
			toastError('Failed to install collection: ' + err.message);
		} finally {
			addingCollection = false;
		}
	}

	async function installPopularCollection(name) {
		installingCollections.add(name);
		installingCollections = installingCollections;
		try {
			await apiPost('/collections', { name });
			await loadTabData('collections');
		} catch (err) {
			toastError('Failed to install collection: ' + err.message);
		} finally {
			installingCollections.delete(name);
			installingCollections = installingCollections;
		}
	}

	function isCollectionInstalled(name) {
		return collections.some(c => c.name === name);
	}

	async function installBouncer(pkg) {
		installingBouncers.add(pkg);
		installingBouncers = installingBouncers;
		try {
			await apiPost('/bouncers', { package: pkg });
			await loadTabData('bouncers');
		} catch (err) {
			toastError('Failed to install bouncer: ' + err.message);
		} finally {
			installingBouncers.delete(pkg);
			installingBouncers = installingBouncers;
		}
	}

	async function removeBouncer(pkg) {
		if (!await confirmDialog(`Remove bouncer package ${pkg}?`, { title: 'Remove Bouncer', confirmLabel: 'Remove', danger: true })) return;
		removingBouncers.add(pkg); removingBouncers = removingBouncers;
		try {
			await apiDelete(`/bouncers/${pkg}`);
			await loadTabData('bouncers');
		} catch (err) {
			toastError('Failed to remove bouncer: ' + err.message);
		} finally {
			removingBouncers.delete(pkg); removingBouncers = removingBouncers;
		}
	}

	function isBouncerInstalled(pkg) {
		return bouncers.some(b => b.type && b.type.includes(pkg.replace('crowdsec-', '').replace('-iptables', '').replace('-nftables', '')));
	}

	async function removeCollection(name) {
		if (!await confirmDialog(`Remove collection ${name}?`, { title: 'Remove Collection', confirmLabel: 'Remove', danger: true })) return;
		removingCollections.add(name); removingCollections = removingCollections;
		try {
			await apiDelete(`/collections/${name}`);
			await loadTabData('collections');
		} catch (err) {
			toastError('Failed to remove collection: ' + err.message);
		} finally {
			removingCollections.delete(name); removingCollections = removingCollections;
		}
	}

	async function addWhitelistEntry() {
		if (!whitelistIP) return;
		addingWhitelist = true;
		try {
			await apiPost('/whitelist', { ip: whitelistIP, description: whitelistDesc });
			whitelistIP = ''; whitelistDesc = '';
			await loadTabData('whitelist');
		} catch (err) {
			toastError('Failed to whitelist IP: ' + err.message);
		} finally {
			addingWhitelist = false;
		}
	}

	async function removeWhitelistEntry(ip) {
		if (!await confirmDialog(`Remove ${ip} from whitelist?`, { title: 'Remove from Whitelist', confirmLabel: 'Remove', danger: true })) return;
		removingWhitelist.add(ip); removingWhitelist = removingWhitelist;
		try {
			await apiDelete(`/whitelist/${ip}`);
			await loadTabData('whitelist');
		} catch (err) {
			toastError('Failed to remove from whitelist: ' + err.message);
		} finally {
			removingWhitelist.delete(ip); removingWhitelist = removingWhitelist;
		}
	}

	function parseMetrics(data) {
		if (!data) return null;
		if (typeof data === 'string') {
			try { return JSON.parse(data); } catch { return null; }
		}
		return data;
	}

	function getMetricsSummary(m) {
		if (!m) return { totalReads: 0, totalParsed: 0, totalUnparsed: 0, bouncerRequests: 0, apiCalls: 0 };
		let totalReads = 0, totalParsed = 0, totalUnparsed = 0;
		if (m.acquisition) {
			for (const src of Object.values(m.acquisition)) {
				totalReads += src.reads || 0;
				totalParsed += src.parsed || 0;
				totalUnparsed += src.unparsed || 0;
			}
		}
		let bouncerRequests = 0;
		if (m.bouncers) {
			for (const b of Object.values(m.bouncers)) {
				for (const endpoint of Object.values(b)) {
					if (endpoint.processed) bouncerRequests += endpoint.processed.request || 0;
				}
			}
		}
		let apiCalls = 0;
		if (m.lapi) {
			for (const endpoint of Object.values(m.lapi)) {
				for (const count of Object.values(endpoint)) {
					apiCalls += count || 0;
				}
			}
		}
		return { totalReads, totalParsed, totalUnparsed, bouncerRequests, apiCalls };
	}

	function hasEntries(obj) {
		return obj && Object.keys(obj).length > 0;
	}

	function shortSource(name) {
		return name.replace('file:', '').replace(/^\/var\/log\//, '');
	}
</script>

<svelte:head>
	<title>CrowdSec - Proxera</title>
</svelte:head>

<div class="page">
	<header class="page-head">
		<div class="head-left">
			<button class="breadcrumb" on:click={() => goto('/crowdsec')}>
				<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
				CrowdSec
			</button>
			<h1>{agentName}</h1>
		</div>
		<div class="head-right">
			<select class="agent-select" on:change={(e) => goto(`/crowdsec/${e.target.value}`)}>
				{#each agents as a}
					<option value={a.agent_id} selected={a.agent_id === agentId}>
						{a.name} ({a.status})
					</option>
				{/each}
			</select>
		</div>
	</header>

	{#if loading}
		<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading CrowdSec status...</p></div>
	{:else if !status || !status.installed}
		<!-- Install Flow -->
		<div class="install-card">
			<div class="install-icon">
				<svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
					<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
				</svg>
			</div>
			<h2>Install CrowdSec</h2>
			<p>CrowdSec is an open-source IPS/IDS that protects your server from malicious traffic. It integrates with nginx to block bad actors at the HTTP level.</p>

			<div class="install-features">
				<h3>What this installs:</h3>
				<ul>
					<li>CrowdSec engine - Detects and blocks malicious behavior</li>
					<li>Nginx bouncer - Blocks bad IPs at the nginx level</li>
					<li>Community blocklists - Shared threat intelligence from CrowdSec network</li>
					<li>Default collections - nginx, base HTTP scenarios, HTTP CVE protection</li>
				</ul>
			</div>

			<div class="enrollment-section">
				<label for="enrollment-key">CrowdSec Console Enrollment Key (optional)</label>
				<input
					id="enrollment-key"
					type="text"
					bind:value={enrollmentKey}
					placeholder="Enter enrollment key for CrowdSec Console"
					class="input"
				/>
				<p class="input-hint">Get your key from <strong>app.crowdsec.net</strong> to access the CrowdSec Console dashboard</p>
			</div>

			{#if error}
				<div class="error-msg" aria-live="assertive">{error}</div>
			{/if}

			<button class="btn-fill btn-lg" on:click={openInstallConsent} disabled={installing}>
				{installing ? 'Installing... (this may take a few minutes)' : 'Install CrowdSec'}
			</button>
		</div>
	{:else}
		<!-- Management Interface -->
		<div class="tabs">
			{#each ['overview', 'decisions', 'alerts', 'collections', 'whitelist', 'bouncers', 'metrics'] as tab}
				<button
					class="tab"
					class:active={activeTab === tab}
					on:click={() => switchTab(tab)}
				>
					{tab.charAt(0).toUpperCase() + tab.slice(1)}
				</button>
			{/each}
		</div>

		{#if error}
			<div class="error-banner" aria-live="assertive">{error}</div>
		{/if}

		<div class="tab-content">
			{#if tabLoading}
				<div class="placeholder" aria-live="polite"><div class="loader"></div><p>Loading...</p></div>
			{:else if activeTab === 'overview'}
				<div class="overview-grid">
					<div class="stat-card">
						<div class="stat-label">Status</div>
						<div class="stat-value" class:val-ok={status.running} class:val-err={!status.running}>
							{status.running ? 'Running' : 'Stopped'}
						</div>
					</div>
					<div class="stat-card">
						<div class="stat-label">Version</div>
						<div class="stat-value">{status.version || 'Unknown'}</div>
					</div>
					<div class="stat-card">
						<div class="stat-label">Bouncers</div>
						<div class="stat-value">{bouncers.length}</div>
					</div>
				</div>

				{#if bouncers.length > 0}
					<div class="section">
						<h3>Active Bouncers</h3>
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>Name</th>
										<th>Type</th>
										<th>Version</th>
										<th>Last Pull</th>
									</tr>
								</thead>
								<tbody>
									{#each bouncers as bouncer}
										<tr>
											<td>{bouncer.name}</td>
											<td>{bouncer.type || 'N/A'}</td>
											<td>{bouncer.version || 'N/A'}</td>
											<td>{bouncer.last_pull || 'N/A'}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				{/if}

				<div class="section danger-section">
					<h3>Danger Zone</h3>
					<p>Uninstalling CrowdSec will remove all protection from this agent.</p>
					<button class="btn-danger" on:click={uninstallCrowdSec} disabled={uninstalling}>
						{uninstalling ? 'Uninstalling...' : 'Uninstall CrowdSec'}
					</button>
				</div>

			{:else if activeTab === 'decisions'}
				<div class="section">
					<h3>Manual Ban</h3>
					<div class="form-row">
						<input type="text" bind:value={banIP} placeholder="IP address (e.g. 1.2.3.4)" class="input" />
						<select bind:value={banDuration} class="select">
							{#each durations as d}
								<option value={d.value}>{d.label}</option>
							{/each}
						</select>
						<input type="text" bind:value={banReason} placeholder="Reason (optional)" class="input" />
						<button class="btn-fill" on:click={addDecision} disabled={!banIP || addingDecision}>{addingDecision ? 'Banning...' : 'Ban'}</button>
					</div>
				</div>

				<div class="section">
					<h3>Active Decisions ({decisions.length})</h3>
					{#if decisions.length === 0}
						<p class="empty">No active decisions</p>
					{:else}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>ID</th>
										<th>Origin</th>
										<th>Scope</th>
										<th>Value</th>
										<th>Reason</th>
										<th>Action</th>
										<th>Duration</th>
										<th></th>
									</tr>
								</thead>
								<tbody>
									{#each decisions as d}
										<tr>
											<td>{d.id}</td>
											<td><span class="badge">{d.origin}</span></td>
											<td>{d.scope}</td>
											<td><code>{d.value}</code></td>
											<td>{d.reason || '-'}</td>
											<td>{d.action}</td>
											<td>{d.duration}</td>
											<td><button class="btn-sm-danger" on:click={() => deleteDecision(d.id)} disabled={deletingDecisions.has(d.id)}>{deletingDecisions.has(d.id) ? 'Removing...' : 'Remove'}</button></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

			{:else if activeTab === 'alerts'}
				<div class="section">
					<h3>Alerts ({alerts.length})</h3>
					{#if alerts.length === 0}
						<p class="empty">No alerts</p>
					{:else}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>ID</th>
										<th>Scenario</th>
										<th>Source</th>
										<th>Events</th>
										<th>Created</th>
										<th></th>
									</tr>
								</thead>
								<tbody>
									{#each alerts as a}
										<tr>
											<td>{a.id}</td>
											<td><code>{a.scenario}</code></td>
											<td><code>{a.value || '-'}</code></td>
											<td>{a.events_count}</td>
											<td>{a.created_at || '-'}</td>
											<td><button class="btn-sm-danger" on:click={() => deleteAlert(a.id)} disabled={deletingAlerts.has(a.id)}>{deletingAlerts.has(a.id) ? 'Deleting...' : 'Delete'}</button></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

			{:else if activeTab === 'collections'}
				<div class="section">
					<h3>Install Collection</h3>
					<div class="form-row">
						<input type="text" bind:value={collectionName} placeholder="e.g. crowdsecurity/nginx" class="input" />
						<button class="btn-fill" on:click={installCollection} disabled={!collectionName || addingCollection}>{addingCollection ? 'Installing...' : 'Install'}</button>
					</div>
				</div>

				<div class="section">
					<h3>Installed Collections ({collections.length})</h3>
					{#if collections.length === 0}
						<p class="empty">No collections installed</p>
					{:else}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>Name</th>
										<th>Status</th>
										<th>Version</th>
										<th>Description</th>
										<th></th>
									</tr>
								</thead>
								<tbody>
									{#each collections as col}
										<tr>
											<td><code>{col.name}</code></td>
											<td><span class="badge" class:badge-green={col.status === 'enabled'}>{col.status}</span></td>
											<td>{col.local_version || col.version || '-'}</td>
											<td>{col.description || '-'}</td>
											<td><button class="btn-sm-danger" on:click={() => removeCollection(col.name)} disabled={removingCollections.has(col.name)}>{removingCollections.has(col.name) ? 'Removing...' : 'Remove'}</button></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

				<div class="section">
					<h3>Popular Collections</h3>
					<p class="section-desc">One-click install for commonly used CrowdSec collections</p>
					<div class="popular-grid">
						{#each popularCollections as pc}
							{@const installed = isCollectionInstalled(pc.name)}
							<div class="popular-card" class:installed>
								<div class="popular-info">
									<code class="popular-name">{pc.name}</code>
									<span class="popular-desc">{pc.desc}</span>
								</div>
								<button
									class="btn-popular"
									class:btn-popular-installed={installed}
									on:click={() => installPopularCollection(pc.name)}
									disabled={installed || installingCollections.has(pc.name)}
								>
									{#if installed}
										Installed
									{:else if installingCollections.has(pc.name)}
										Installing...
									{:else}
										Install
									{/if}
								</button>
							</div>
						{/each}
					</div>
				</div>

			{:else if activeTab === 'whitelist'}
				<div class="section">
					<h3>Add to Whitelist</h3>
					<div class="form-row">
						<input type="text" bind:value={whitelistIP} placeholder="IP address" class="input" />
						<input type="text" bind:value={whitelistDesc} placeholder="Description (optional)" class="input" />
						<button class="btn-fill" on:click={addWhitelistEntry} disabled={!whitelistIP || addingWhitelist}>{addingWhitelist ? 'Adding...' : 'Add'}</button>
					</div>
				</div>

				<div class="section">
					<h3>Whitelisted IPs ({whitelist.length})</h3>
					{#if whitelist.length === 0}
						<p class="empty">No IPs whitelisted</p>
					{:else}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>IP Address</th>
										<th>Description</th>
										<th></th>
									</tr>
								</thead>
								<tbody>
									{#each whitelist as entry}
										<tr>
											<td><code>{entry.ip}</code></td>
											<td>{entry.description || '-'}</td>
											<td><button class="btn-sm-danger" on:click={() => removeWhitelistEntry(entry.ip)} disabled={removingWhitelist.has(entry.ip)}>{removingWhitelist.has(entry.ip) ? 'Removing...' : 'Remove'}</button></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

			{:else if activeTab === 'bouncers'}
				<div class="section">
					<h3>Active Bouncers ({bouncers.length})</h3>
					{#if bouncers.length === 0}
						<p class="empty">No bouncers registered</p>
					{:else}
						<div class="tbl-wrap">
							<table>
								<thead>
									<tr>
										<th>Name</th>
										<th>IP Address</th>
										<th>Type</th>
										<th>Version</th>
										<th>Last Pull</th>
										<th>Valid</th>
									</tr>
								</thead>
								<tbody>
									{#each bouncers as b}
										<tr>
											<td>{b.name}</td>
											<td><code>{b.ip_address || '-'}</code></td>
											<td>{b.type || '-'}</td>
											<td>{b.version || '-'}</td>
											<td>{b.last_pull || '-'}</td>
											<td><span class="badge" class:badge-green={b.valid}>{b.valid ? 'Valid' : 'Invalid'}</span></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

				<div class="section">
					<h3>Available Bouncers</h3>
					<p class="section-desc">Install additional bouncers to extend CrowdSec protection</p>
					<div class="popular-grid">
						{#each popularBouncers as pb}
							<div class="popular-card">
								<div class="popular-info">
									<code class="popular-name">{pb.name}</code>
									<span class="popular-desc">{pb.desc}</span>
									<span class="popular-pkg">Package: {pb.pkg}</span>
								</div>
								<div class="bouncer-actions">
									<button
										class="btn-popular"
										on:click={() => installBouncer(pb.pkg)}
										disabled={installingBouncers.has(pb.pkg)}
									>
										{installingBouncers.has(pb.pkg) ? 'Installing...' : 'Install'}
									</button>
									<button
										class="btn-sm-danger"
										on:click={() => removeBouncer(pb.pkg)}
										disabled={removingBouncers.has(pb.pkg)}
									>
										{removingBouncers.has(pb.pkg) ? 'Removing...' : 'Remove'}
									</button>
								</div>
							</div>
						{/each}
					</div>
				</div>

			{:else if activeTab === 'metrics'}
				{@const m = parseMetrics(metricsData)}
				{@const summary = getMetricsSummary(m)}
				<div class="section">
					<div class="metrics-header">
						<h3>CrowdSec Metrics</h3>
						<button class="btn-fill" on:click={() => loadTabData('metrics')}>Refresh</button>
					</div>

					{#if !m}
						<p class="empty">No metrics available</p>
					{:else}
						<div class="metrics-summary">
							<div class="metric-card">
								<div class="metric-number">{summary.totalReads.toLocaleString()}</div>
								<div class="metric-label">Log Lines Read</div>
							</div>
							<div class="metric-card">
								<div class="metric-number">{summary.totalParsed.toLocaleString()}</div>
								<div class="metric-label">Lines Parsed</div>
							</div>
							<div class="metric-card">
								<div class="metric-number">{summary.totalUnparsed.toLocaleString()}</div>
								<div class="metric-label">Lines Unparsed</div>
							</div>
							<div class="metric-card accent">
								<div class="metric-number">{summary.bouncerRequests.toLocaleString()}</div>
								<div class="metric-label">Bouncer Requests</div>
							</div>
							<div class="metric-card">
								<div class="metric-number">{summary.apiCalls.toLocaleString()}</div>
								<div class="metric-label">LAPI Calls</div>
							</div>
						</div>

						{#if hasEntries(m.acquisition)}
							<div class="metrics-section">
								<h4>Log Sources (Acquisition)</h4>
								<div class="tbl-wrap">
									<table>
										<thead>
											<tr><th>Source</th><th class="num">Reads</th><th class="num">Parsed</th><th class="num">Unparsed</th></tr>
										</thead>
										<tbody>
											{#each Object.entries(m.acquisition) as [source, stats]}
												<tr>
													<td><code>{shortSource(source)}</code></td>
													<td class="num">{(stats.reads || 0).toLocaleString()}</td>
													<td class="num">{(stats.parsed || 0).toLocaleString()}</td>
													<td class="num">{(stats.unparsed || 0).toLocaleString()}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}

						{#if hasEntries(m.parsers)}
							<div class="metrics-section">
								<h4>Parsers</h4>
								<div class="tbl-wrap">
									<table>
										<thead>
											<tr><th>Parser</th><th class="num">Hits</th><th class="num">Parsed</th><th class="num">Unparsed</th></tr>
										</thead>
										<tbody>
											{#each Object.entries(m.parsers) as [parser, stats]}
												<tr>
													<td><code>{parser}</code></td>
													<td class="num">{(stats.hits || 0).toLocaleString()}</td>
													<td class="num parsed">{(stats.parsed || 0).toLocaleString()}</td>
													<td class="num unparsed">{(stats.unparsed || 0).toLocaleString()}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}

						{#if hasEntries(m.bouncers)}
							<div class="metrics-section">
								<h4>Bouncers</h4>
								<div class="tbl-wrap">
									<table>
										<thead>
											<tr><th>Bouncer</th><th class="num">Requests Processed</th></tr>
										</thead>
										<tbody>
											{#each Object.entries(m.bouncers) as [name, endpoints]}
												<tr>
													<td><code>{name}</code></td>
													<td class="num">
														{#each Object.entries(endpoints) as [, stats]}
															{(stats.processed?.request || 0).toLocaleString()}
														{/each}
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}

						{#if hasEntries(m.lapi)}
							<div class="metrics-section">
								<h4>Local API</h4>
								<div class="tbl-wrap">
									<table>
										<thead>
											<tr><th>Endpoint</th><th>Method</th><th class="num">Calls</th></tr>
										</thead>
										<tbody>
											{#each Object.entries(m.lapi) as [endpoint, methods]}
												{#each Object.entries(methods) as [method, count]}
													<tr>
														<td><code>{endpoint}</code></td>
														<td><span class="badge method-{method.toLowerCase()}">{method}</span></td>
														<td class="num">{(count || 0).toLocaleString()}</td>
													</tr>
												{/each}
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}

						{#if hasEntries(m['lapi-decisions'])}
							<div class="metrics-section">
								<h4>Decision Fetches (per Bouncer)</h4>
								<div class="tbl-wrap">
									<table>
										<thead>
											<tr><th>Bouncer</th><th class="num">With Decisions</th><th class="num">Empty</th></tr>
										</thead>
										<tbody>
											{#each Object.entries(m['lapi-decisions']) as [name, stats]}
												<tr>
													<td><code>{name}</code></td>
													<td class="num">{(stats.NonEmpty || 0).toLocaleString()}</td>
													<td class="num">{(stats.Empty || 0).toLocaleString()}</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>

{#if showConsentModal}
	<div class="modal-overlay" on:click|self={() => showConsentModal = false} role="presentation">
		<div class="modal" role="dialog" aria-modal="true" aria-labelledby="consent-title">
			<div class="modal-header">
				<div class="modal-icon">
					<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
					</svg>
				</div>
				<h2 id="consent-title">CrowdSec — Terms &amp; Data Sharing</h2>
			</div>
			<div class="modal-body">
				<p>Before installing CrowdSec, please review how it works and what data is shared with their network.</p>

				<div class="consent-section">
					<h3>How the community model works</h3>
					<p>CrowdSec uses a <strong>crowdsourced threat intelligence</strong> model. Your server shares data about attacks it detects with CrowdSec's global network. In return, you receive a curated blocklist of known malicious IPs contributed by the entire community.</p>
				</div>

				<div class="consent-section">
					<h3>Data shared with CrowdSec</h3>
					<ul>
						<li>IP addresses that attack or probe your server</li>
						<li>Timestamp of the detected event</li>
						<li>Which detection scenario was triggered (e.g. "nginx brute-force")</li>
					</ul>
					<p class="consent-note">No private content, user data, or request payloads are shared — only attacker IPs and metadata.</p>
				</div>

				<div class="consent-section">
					<h3>CrowdSec EULA — key restrictions</h3>
					<ul>
						<li>You may <strong>not</strong> redistribute or commercialize the IP reputation data received from CrowdSec</li>
						<li>The CrowdSec software itself is MIT licensed (free &amp; open source)</li>
						<li>Community data sharing applies when connected to CrowdSec's Central API</li>
					</ul>
					<a href="https://www.crowdsec.net/eula" target="_blank" rel="noopener noreferrer" class="eula-link">Read CrowdSec's full EULA →</a>
				</div>

				<label class="consent-checkbox">
					<input type="checkbox" bind:checked={consentChecked} />
					<span>I understand that attack data from this server will be shared with CrowdSec's community network, and I agree to CrowdSec's <a href="https://www.crowdsec.net/eula" target="_blank" rel="noopener noreferrer">EULA</a>.</span>
				</label>
			</div>
			<div class="modal-footer">
				<button class="btn-ghost" on:click={() => showConsentModal = false}>Cancel</button>
				<button class="btn-fill" on:click={installCrowdSec} disabled={!consentChecked}>
					Proceed with Installation
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	/* ── Header layout ── */
	.page-head { align-items: flex-start; gap: 1rem; }
	.head-left { display: flex; flex-direction: column; gap: 0.5rem; }
	.head-right { display: flex; align-items: center; gap: 0.5rem; }

	.agent-select {
		padding: 0.5rem 0.75rem; border: 1px solid var(--border);
		border-radius: var(--radius); font-size: var(--text-sm);
		background: var(--bg); color: var(--text-primary);
		cursor: pointer; min-width: 200px;
	}
	.agent-select:focus { outline: none; border-color: var(--accent); }

	/* ── Install Flow ── */
	.install-card {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 3rem;
		max-width: 700px; margin: 0 auto; text-align: center;
	}
	.install-icon { margin-bottom: 1rem; color: var(--text-tertiary); }
	.install-card h2 { margin: 0 0 1rem; font-size: var(--text-xl); color: var(--text-primary); }
	.install-card > p { color: var(--text-secondary); line-height: 1.6; max-width: 500px; margin: 0 auto 2rem; font-size: var(--text-sm); }

	.install-features {
		background: var(--bg); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 1.5rem;
		margin-bottom: 2rem; text-align: left;
	}
	.install-features h3 { margin: 0 0 1rem; font-size: var(--text-base); color: var(--text-primary); }
	.install-features ul { margin: 0; padding: 0; list-style: none; }
	.install-features li {
		padding: 0.4rem 0 0.4rem 1.5rem; color: var(--text-secondary);
		font-size: var(--text-sm); position: relative;
	}
	.install-features li::before { content: '\2713'; position: absolute; left: 0; color: var(--accent); font-weight: bold; }

	.enrollment-section { text-align: left; margin-bottom: 1.5rem; }
	.enrollment-section label { display: block; font-weight: 500; margin-bottom: 0.5rem; color: var(--text-primary); font-size: var(--text-sm); }
	.input-hint { margin: 0.5rem 0 0; font-size: var(--text-xs); color: var(--text-tertiary); }
	.input-hint strong { color: var(--text-secondary); }

	/* ── Buttons ── */
	.btn-lg { padding: 0.75rem 2rem; font-size: var(--text-base); }

	.btn-danger {
		background: var(--danger); color: #fff; border: none;
		padding: 0.5rem 1.125rem; border-radius: var(--radius);
		font-size: var(--text-sm); font-weight: 600; cursor: pointer;
		transition: background var(--transition);
	}
	.btn-danger:hover:not(:disabled) { opacity: 0.85; }
	.btn-danger:disabled { opacity: 0.45; cursor: wait; }

	.btn-sm-danger {
		background: var(--danger-dim); color: var(--danger);
		border: 1px solid var(--danger); padding: 0.25rem 0.5rem;
		border-radius: var(--radius); cursor: pointer; font-size: var(--text-xs);
		transition: background var(--transition);
	}
	.btn-sm-danger:hover { background: var(--danger); color: #fff; }

	.btn-popular {
		background: var(--accent); color: #fff; border: none;
		padding: 0.4rem 1rem; border-radius: var(--radius);
		font-size: var(--text-xs); font-weight: 600; cursor: pointer;
		white-space: nowrap; transition: background var(--transition); flex-shrink: 0;
	}
	.btn-popular:hover:not(:disabled) { background: var(--accent-bright); }
	.btn-popular:disabled { opacity: 0.45; cursor: not-allowed; }
	.btn-popular-installed { background: var(--accent-dim); color: var(--accent); }

	/* ── Tabs ── */
	.tabs {
		display: flex; gap: 0.25rem; background: var(--surface);
		border: 1px solid var(--border); padding: 0.5rem;
		border-radius: var(--radius-lg); margin-bottom: 1.5rem; overflow-x: auto;
	}
	.tab {
		padding: 0.5rem 0.875rem; border: none; background: transparent;
		color: var(--text-secondary); cursor: pointer; font-size: var(--text-sm);
		font-weight: 500; border-radius: var(--radius);
		transition: all var(--transition); white-space: nowrap;
	}
	.tab:hover { background: var(--surface-raised); color: var(--text-primary); }
	.tab.active { background: var(--accent); color: #fff; font-weight: 600; }

	.tab-content {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 1.5rem;
	}
	.error-banner {
		background: var(--danger-dim); color: var(--danger);
		border: 1px solid var(--danger); padding: 0.75rem 1rem;
		border-radius: var(--radius); margin-bottom: 1rem; font-size: var(--text-sm);
	}

	/* ── Overview ── */
	.overview-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 0.75rem; margin-bottom: 1.5rem; }
	.stat-card {
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 1rem 1.25rem; text-align: center;
	}
	.stat-label {
		font-size: var(--text-xs); color: var(--text-tertiary);
		text-transform: uppercase; letter-spacing: 0.04em; margin-bottom: 0.5rem;
	}
	.stat-value { font-size: var(--text-xl); font-weight: 700; color: var(--text-primary); }
	.val-ok { color: var(--success); }
	.val-err { color: var(--danger); }

	/* ── Sections ── */
	.section { margin-bottom: 2rem; }
	.section h3 { margin: 0 0 1rem; font-size: var(--text-base); color: var(--text-primary); }
	.section-desc { color: var(--text-secondary); font-size: var(--text-sm); margin: -0.5rem 0 1rem; }

	.danger-section {
		border: 1px solid var(--danger); border-radius: var(--radius);
		padding: 1.5rem; background: var(--danger-dim);
	}
	.danger-section p { color: var(--danger); font-size: var(--text-sm); margin: 0 0 1rem; }

	/* ── Forms ── */
	.form-row { display: flex; gap: 0.75rem; flex-wrap: wrap; align-items: flex-end; }
	.input { flex: 1; min-width: 150px; }
	.select {
		padding: 0.5rem 0.75rem; border: 1px solid var(--border);
		border-radius: var(--radius); font-size: var(--text-sm);
		background: var(--bg); color: var(--text-primary);
	}

	/* ── Tables ── */
	.tbl-wrap { overflow-x: auto; }

	code {
		background: var(--bg); border: 1px solid var(--border);
		padding: 0.15rem 0.4rem; border-radius: var(--radius);
		font-size: var(--text-xs); font-family: var(--font-mono); color: var(--text-primary);
	}

	.badge {
		display: inline-block; padding: 0.125rem 0.5rem;
		border-radius: var(--radius); font-size: var(--text-xs);
		font-weight: 600; background: var(--surface-raised); color: var(--text-secondary);
	}
	.badge-green { background: var(--accent-dim); color: var(--accent); }

	/* ── Metrics ── */
	.metrics-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
	.metrics-header h3 { margin: 0; }
	.metrics-summary {
		display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: 0.75rem; margin-bottom: 2rem;
	}
	.metric-card {
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius); padding: 1rem 1.25rem; text-align: center;
	}
	.metric-card.accent { background: var(--accent-dim); border-color: var(--accent); }
	.metric-number { font-size: var(--text-xl); font-weight: 700; color: var(--text-primary); line-height: 1.2; }
	.metric-label {
		font-size: var(--text-xs); color: var(--text-tertiary);
		text-transform: uppercase; letter-spacing: 0.04em; margin-top: 0.375rem;
	}
	.metrics-section { margin-bottom: 1.75rem; }
	.metrics-section h4 { margin: 0 0 0.75rem; font-size: var(--text-sm); color: var(--text-primary); font-weight: 600; }

	td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
	td.parsed { color: var(--accent); }
	td.unparsed { color: var(--warning); }
	.method-get { background: var(--info-dim); color: var(--info); }
	.method-post { background: var(--accent-dim); color: var(--accent); }

	/* ── Popular grid ── */
	.popular-grid { display: flex; flex-direction: column; gap: 0.5rem; }
	.popular-card {
		display: flex; align-items: center; justify-content: space-between;
		gap: 1rem; padding: 0.75rem 1rem;
		border: 1px solid var(--border); border-radius: var(--radius);
		transition: border-color var(--transition);
	}
	.popular-card:hover { border-color: var(--border-bright); }
	.popular-card.installed { background: var(--accent-dim); border-color: var(--accent); }
	.popular-info { display: flex; flex-direction: column; gap: 0.25rem; min-width: 0; }
	.popular-name { font-size: var(--text-sm); font-weight: 600; }
	.popular-desc { font-size: var(--text-xs); color: var(--text-secondary); }
	.popular-pkg { font-size: var(--text-xs); color: var(--text-tertiary); font-family: var(--font-mono); }
	.bouncer-actions { display: flex; flex-direction: column; gap: 0.375rem; flex-shrink: 0; }

	/* ── States ── */
	.empty { color: var(--text-tertiary); font-size: var(--text-sm); font-style: italic; }

	/* ── Consent Modal ── */
	.modal-overlay {
		position: fixed; inset: 0; background: rgba(0, 0, 0, 0.6);
		display: flex; align-items: center; justify-content: center;
		z-index: 1000; padding: 1rem;
	}
	.modal {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); max-width: 560px; width: 100%;
		max-height: 90vh; overflow-y: auto;
	}
	.modal-header {
		display: flex; align-items: center; gap: 0.75rem;
		padding: 1.5rem 1.5rem 1.25rem;
		border-bottom: 1px solid var(--border);
	}
	.modal-icon { color: var(--accent); flex-shrink: 0; }
	.modal-header h2 { margin: 0; font-size: var(--text-lg); color: var(--text-primary); }
	.modal-body { padding: 1.25rem 1.5rem; }
	.modal-body > p { color: var(--text-secondary); font-size: var(--text-sm); margin: 0 0 1.25rem; line-height: 1.6; }
	.consent-section { margin-bottom: 1.25rem; }
	.consent-section h3 { margin: 0 0 0.5rem; font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
	.consent-section > p { margin: 0; color: var(--text-secondary); font-size: var(--text-sm); line-height: 1.6; }
	.consent-section ul { margin: 0.5rem 0 0; padding: 0; list-style: none; }
	.consent-section li {
		padding: 0.3rem 0 0.3rem 1.25rem; color: var(--text-secondary);
		font-size: var(--text-sm); position: relative;
	}
	.consent-section li::before { content: '•'; position: absolute; left: 0.375rem; color: var(--accent); }
	.consent-note { color: var(--text-tertiary) !important; font-size: var(--text-xs) !important; margin-top: 0.5rem !important; font-style: italic; }
	.eula-link { display: inline-block; margin-top: 0.75rem; font-size: var(--text-sm); color: var(--accent); text-decoration: none; }
	.eula-link:hover { text-decoration: underline; }
	.consent-checkbox {
		display: flex; gap: 0.75rem; align-items: flex-start;
		padding: 1rem; background: var(--bg); border: 1px solid var(--border);
		border-radius: var(--radius); cursor: pointer; margin-top: 1.25rem;
	}
	.consent-checkbox input[type="checkbox"] {
		margin-top: 0.125rem; flex-shrink: 0;
		accent-color: var(--accent); width: 1rem; height: 1rem; cursor: pointer;
	}
	.consent-checkbox span { font-size: var(--text-sm); color: var(--text-primary); line-height: 1.5; }
	.consent-checkbox a { color: var(--accent); }
	.modal-footer {
		display: flex; gap: 0.75rem; justify-content: flex-end;
		padding: 1.25rem 1.5rem; border-top: 1px solid var(--border);
	}

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.page-head { flex-direction: column; gap: 0.75rem; }
		.tabs { flex-wrap: wrap; }
		.modal { max-height: 85vh; }
	}
</style>
