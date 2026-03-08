<script lang="ts">
	import { onMount } from 'svelte';
	import { api, apiJson } from '$lib/api';
	import { toastSuccess, toastError } from '$lib/components/toast';
	import { confirmDialog } from '$lib/components/confirm';
	import { formatRelativeTime } from '$lib/utils';
	import type { AlertRule, NotificationChannel, AlertHistoryEntry } from '$lib/types';

	let tab = $state<'rules' | 'channels' | 'history'>('rules');
	let loading = $state(true);
	let error = $state('');

	// Data
	let rules = $state<AlertRule[]>([]);
	let channels = $state<NotificationChannel[]>([]);
	let history = $state<AlertHistoryEntry[]>([]);
	let historyTotal = $state(0);
	let historyOffset = $state(0);

	// Modals
	let showRuleModal = $state(false);
	let showChannelModal = $state(false);
	let editingRule = $state<AlertRule | null>(null);
	let editingChannel = $state<NotificationChannel | null>(null);

	// Rule form
	let ruleName = $state('');
	let ruleType = $state('agent_offline');
	let ruleCooldown = $state(5);
	let ruleChannelIds = $state<number[]>([]);
	let ruleConfigAgentIds = $state('all');
	let ruleConfigWarnDays = $state([30, 7, 1]);
	let ruleConfigThreshold = $state(5);
	let ruleConfigDomains = $state('all');
	let ruleConfigWindow = $state(5);
	let ruleConfigLatencyThreshold = $state(500);
	let ruleConfigMultiplier = $state(3);
	let ruleConfigBaselineMinutes = $state(60);
	let ruleConfigBandwidthGB = $state(10);
	let ruleConfigPeriodHours = $state(1);
	let ruleConfigMinEvents = $state(1);
	let ruleSaving = $state(false);

	// Channel form
	let channelName = $state('');
	let channelType = $state<'email' | 'webhook' | 'discord'>('email');
	let channelEmail = $state('');
	let channelWebhookUrl = $state('');
	let channelWebhookMethod = $state('POST');
	let channelDiscordUrl = $state('');
	let channelSaving = $state(false);

	// History filters
	let filterType = $state('');
	let filterSeverity = $state('');
	let filterResolved = $state('');

	onMount(async () => {
		await loadAll();
	});

	async function loadAll() {
		loading = true;
		error = '';
		try {
			const [rulesRes, channelsRes, historyRes] = await Promise.all([
				apiJson<{ rules: AlertRule[] }>('/api/alerts/rules'),
				apiJson<{ channels: NotificationChannel[] }>('/api/alerts/channels'),
				apiJson<{ alerts: AlertHistoryEntry[]; total: number }>('/api/alerts/history?limit=50'),
			]);
			rules = rulesRes.rules;
			channels = channelsRes.channels;
			history = historyRes.alerts;
			historyTotal = historyRes.total;
			historyOffset = 0;
		} catch (err: any) {
			error = err.message || 'Failed to load alert data';
		} finally {
			loading = false;
		}
	}

	async function loadHistory() {
		try {
			let url = `/api/alerts/history?limit=50&offset=${historyOffset}`;
			if (filterType) url += `&type=${filterType}`;
			if (filterSeverity) url += `&severity=${filterSeverity}`;
			if (filterResolved) url += `&resolved=${filterResolved}`;
			const res = await apiJson<{ alerts: AlertHistoryEntry[]; total: number }>(url);
			history = res.alerts;
			historyTotal = res.total;
		} catch (err: any) {
			toastError(err.message || 'Failed to load history');
		}
	}

	// Quick setup
	async function quickSetup() {
		try {
			await apiJson('/api/alerts/quick-setup', { method: 'POST' });
			toastSuccess('Quick setup complete! Default rules and email channel created.');
			await loadAll();
		} catch (err: any) {
			toastError(err.message || 'Quick setup failed');
		}
	}

	// Rule CRUD
	function openCreateRule() {
		editingRule = null;
		ruleName = '';
		ruleType = 'agent_offline';
		ruleCooldown = 5;
		ruleChannelIds = [];
		ruleConfigAgentIds = 'all';
		ruleConfigWarnDays = [30, 7, 1];
		ruleConfigThreshold = 5;
		ruleConfigDomains = 'all';
		ruleConfigWindow = 5;
		ruleConfigLatencyThreshold = 500;
		ruleConfigMultiplier = 3;
		ruleConfigBaselineMinutes = 60;
		ruleConfigBandwidthGB = 10;
		ruleConfigPeriodHours = 1;
		ruleConfigMinEvents = 1;
		showRuleModal = true;
	}

	function openEditRule(rule: AlertRule) {
		editingRule = rule;
		ruleName = rule.name;
		ruleType = rule.alert_type;
		ruleCooldown = rule.cooldown_minutes;
		ruleChannelIds = [...rule.channel_ids];

		const cfg = rule.config || {};
		if (rule.alert_type === 'agent_offline') {
			ruleConfigAgentIds = cfg.agent_ids?.[0] === 'all' ? 'all' : (cfg.agent_ids || []).join(', ');
		} else if (rule.alert_type === 'cert_expiry') {
			ruleConfigWarnDays = cfg.warn_days || [30, 7, 1];
		} else if (rule.alert_type === 'error_rate') {
			ruleConfigThreshold = cfg.threshold_pct || 5;
			ruleConfigDomains = cfg.domains?.[0] === 'all' ? 'all' : (cfg.domains || []).join(', ');
			ruleConfigWindow = cfg.window_minutes || 5;
		} else if (rule.alert_type === 'high_latency') {
			ruleConfigLatencyThreshold = cfg.threshold_ms || 500;
			ruleConfigDomains = cfg.domains?.[0] === 'all' ? 'all' : (cfg.domains || []).join(', ');
			ruleConfigWindow = cfg.window_minutes || 1;
		} else if (rule.alert_type === 'traffic_spike') {
			ruleConfigMultiplier = cfg.multiplier || 3;
			ruleConfigBaselineMinutes = cfg.baseline_minutes || 60;
			ruleConfigDomains = cfg.domains?.[0] === 'all' ? 'all' : (cfg.domains || []).join(', ');
		} else if (rule.alert_type === 'host_down') {
			ruleConfigDomains = cfg.domains?.[0] === 'all' ? 'all' : (cfg.domains || []).join(', ');
			ruleConfigWindow = cfg.window_minutes || 1;
		} else if (rule.alert_type === 'bandwidth_threshold') {
			ruleConfigBandwidthGB = cfg.threshold_gb || 10;
			ruleConfigPeriodHours = cfg.period_hours || 1;
			ruleConfigDomains = cfg.domains?.[0] === 'all' ? 'all' : (cfg.domains || []).join(', ');
		} else if (rule.alert_type === 'crowdsec_ban') {
			ruleConfigAgentIds = cfg.agent_ids?.[0] === 'all' ? 'all' : (cfg.agent_ids || []).join(', ');
			ruleConfigMinEvents = cfg.min_events || 1;
		}
		showRuleModal = true;
	}

	function buildRuleConfig(): Record<string, any> {
		switch (ruleType) {
			case 'agent_offline':
				return { agent_ids: ruleConfigAgentIds === 'all' ? ['all'] : ruleConfigAgentIds.split(',').map(s => s.trim()).filter(Boolean) };
			case 'cert_expiry':
				return { warn_days: ruleConfigWarnDays };
			case 'cert_renewal_failed':
				return {};
			case 'error_rate':
				return {
					threshold_pct: ruleConfigThreshold,
					domains: ruleConfigDomains === 'all' ? ['all'] : ruleConfigDomains.split(',').map(s => s.trim()).filter(Boolean),
					window_minutes: ruleConfigWindow,
				};
			case 'high_latency':
				return {
					threshold_ms: ruleConfigLatencyThreshold,
					domains: ruleConfigDomains === 'all' ? ['all'] : ruleConfigDomains.split(',').map(s => s.trim()).filter(Boolean),
					window_minutes: ruleConfigWindow,
				};
			case 'traffic_spike':
				return {
					multiplier: ruleConfigMultiplier,
					baseline_minutes: ruleConfigBaselineMinutes,
					domains: ruleConfigDomains === 'all' ? ['all'] : ruleConfigDomains.split(',').map(s => s.trim()).filter(Boolean),
				};
			case 'host_down':
				return {
					domains: ruleConfigDomains === 'all' ? ['all'] : ruleConfigDomains.split(',').map(s => s.trim()).filter(Boolean),
					window_minutes: ruleConfigWindow,
				};
			case 'bandwidth_threshold':
				return {
					threshold_gb: ruleConfigBandwidthGB,
					period_hours: ruleConfigPeriodHours,
					domains: ruleConfigDomains === 'all' ? ['all'] : ruleConfigDomains.split(',').map(s => s.trim()).filter(Boolean),
				};
			case 'crowdsec_ban':
				return {
					agent_ids: ruleConfigAgentIds === 'all' ? ['all'] : ruleConfigAgentIds.split(',').map(s => s.trim()).filter(Boolean),
					min_events: ruleConfigMinEvents,
				};
			default:
				return {};
		}
	}

	async function saveRule() {
		if (!ruleName.trim()) { toastError('Name is required'); return; }
		ruleSaving = true;
		try {
			const config = buildRuleConfig();
			if (editingRule) {
				await apiJson(`/api/alerts/rules/${editingRule.id}`, {
					method: 'PATCH',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: ruleName, config, cooldown_minutes: ruleCooldown, channel_ids: ruleChannelIds }),
				});
				toastSuccess('Rule updated');
			} else {
				await apiJson('/api/alerts/rules', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ alert_type: ruleType, name: ruleName, config, cooldown_minutes: ruleCooldown, channel_ids: ruleChannelIds }),
				});
				toastSuccess('Rule created');
			}
			showRuleModal = false;
			await loadAll();
		} catch (err: any) {
			toastError(err.message || 'Failed to save rule');
		} finally {
			ruleSaving = false;
		}
	}

	async function toggleRule(rule: AlertRule) {
		try {
			await apiJson(`/api/alerts/rules/${rule.id}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: !rule.enabled }),
			});
			rule.enabled = !rule.enabled;
		} catch (err: any) {
			toastError(err.message || 'Failed to toggle rule');
		}
	}

	async function deleteRule(rule: AlertRule) {
		if (!await confirmDialog(`Delete rule "${rule.name}"?`, { title: 'Delete Rule', confirmLabel: 'Delete', danger: true })) return;
		try {
			await api(`/api/alerts/rules/${rule.id}`, { method: 'DELETE' });
			toastSuccess('Rule deleted');
			await loadAll();
		} catch (err: any) {
			toastError(err.message || 'Failed to delete rule');
		}
	}

	// Channel CRUD
	function openCreateChannel() {
		editingChannel = null;
		channelName = '';
		channelType = 'email';
		channelEmail = '';
		channelWebhookUrl = '';
		channelWebhookMethod = 'POST';
		channelDiscordUrl = '';
		showChannelModal = true;
	}

	function openEditChannel(ch: NotificationChannel) {
		editingChannel = ch;
		channelName = ch.name;
		channelType = ch.channel_type as 'email' | 'webhook' | 'discord';
		if (ch.channel_type === 'email') {
			channelEmail = ch.config?.address || '';
		} else if (ch.channel_type === 'discord') {
			channelDiscordUrl = ch.config?.url || '';
		} else {
			channelWebhookUrl = ch.config?.url || '';
			channelWebhookMethod = ch.config?.method || 'POST';
		}
		showChannelModal = true;
	}

	async function saveChannel() {
		if (!channelName.trim()) { toastError('Name is required'); return; }
		channelSaving = true;
		try {
			let config: Record<string, any>;
			if (channelType === 'email') {
				if (!channelEmail.trim()) { toastError('Email address is required'); channelSaving = false; return; }
				config = { address: channelEmail };
			} else if (channelType === 'discord') {
				if (!channelDiscordUrl.trim()) { toastError('Discord webhook URL is required'); channelSaving = false; return; }
				config = { url: channelDiscordUrl };
			} else {
				if (!channelWebhookUrl.trim()) { toastError('Webhook URL is required'); channelSaving = false; return; }
				config = { url: channelWebhookUrl, method: channelWebhookMethod };
			}

			if (editingChannel) {
				await apiJson(`/api/alerts/channels/${editingChannel.id}`, {
					method: 'PATCH',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: channelName, config }),
				});
				toastSuccess('Channel updated');
			} else {
				await apiJson('/api/alerts/channels', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: channelName, channel_type: channelType, config }),
				});
				toastSuccess('Channel created');
			}
			showChannelModal = false;
			await loadAll();
		} catch (err: any) {
			toastError(err.message || 'Failed to save channel');
		} finally {
			channelSaving = false;
		}
	}

	async function toggleChannel(ch: NotificationChannel) {
		try {
			await apiJson(`/api/alerts/channels/${ch.id}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: !ch.enabled }),
			});
			ch.enabled = !ch.enabled;
		} catch (err: any) {
			toastError(err.message || 'Failed to toggle channel');
		}
	}

	async function testChannel(ch: NotificationChannel) {
		try {
			await apiJson(`/api/alerts/channels/${ch.id}/test`, { method: 'POST' });
			toastSuccess('Test notification sent');
		} catch (err: any) {
			toastError(err.message || 'Test failed');
		}
	}

	async function deleteChannel(ch: NotificationChannel) {
		if (!await confirmDialog(`Delete channel "${ch.name}"?`, { title: 'Delete Channel', confirmLabel: 'Delete', danger: true })) return;
		try {
			await api(`/api/alerts/channels/${ch.id}`, { method: 'DELETE' });
			toastSuccess('Channel deleted');
			await loadAll();
		} catch (err: any) {
			toastError(err.message || 'Failed to delete channel');
		}
	}

	// History
	async function resolveAlert(entry: AlertHistoryEntry) {
		try {
			await apiJson(`/api/alerts/history/${entry.id}/resolve`, { method: 'POST' });
			entry.resolved = true;
			toastSuccess('Alert resolved');
		} catch (err: any) {
			toastError(err.message || 'Failed to resolve');
		}
	}

	async function loadMoreHistory() {
		historyOffset += 50;
		await loadHistory();
	}

	function alertTypeLabel(type: string): string {
		switch (type) {
			case 'agent_offline': return 'Agent Offline';
			case 'cert_expiry': return 'Cert Expiry';
			case 'cert_renewal_failed': return 'Renewal Failed';
			case 'error_rate': return 'Error Rate';
			case 'high_latency': return 'High Latency';
			case 'traffic_spike': return 'Traffic Spike';
			case 'host_down': return 'Host Down';
			case 'bandwidth_threshold': return 'Bandwidth';
			case 'crowdsec_ban': return 'CrowdSec Ban';
			default: return type;
		}
	}

	function severityColor(severity: string): string {
		switch (severity) {
			case 'critical': return 'var(--danger)';
			case 'warning': return 'var(--warning)';
			default: return 'var(--info)';
		}
	}

	function toggleWarnDay(day: number) {
		if (ruleConfigWarnDays.includes(day)) {
			ruleConfigWarnDays = ruleConfigWarnDays.filter(d => d !== day);
		} else {
			ruleConfigWarnDays = [...ruleConfigWarnDays, day].sort((a, b) => b - a);
		}
	}

	function toggleChannelId(id: number) {
		if (ruleChannelIds.includes(id)) {
			ruleChannelIds = ruleChannelIds.filter(i => i !== id);
		} else {
			ruleChannelIds = [...ruleChannelIds, id];
		}
	}

	function channelSummary(ch: NotificationChannel): string {
		if (ch.channel_type === 'email') return ch.config?.address || '';
		return ch.config?.url || '';
	}
</script>

<svelte:head>
	<title>Alerts - Proxera</title>
</svelte:head>

<div class="page">
	<div class="page-head">
		<h1>Alerts</h1>
		{#if tab === 'rules'}
			<div class="head-actions">
				{#if rules.length > 0 && rules.length < 9}
					<button class="btn-ghost" onclick={quickSetup}>Enable All Rules</button>
				{/if}
				<button class="btn-fill" onclick={openCreateRule}>
					<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
					New Rule
				</button>
			</div>
		{:else if tab === 'channels'}
			<button class="btn-fill" onclick={openCreateChannel}>
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
				New Channel
			</button>
		{/if}
	</div>

	<!-- Tabs -->
	<div class="tabs">
		<button class="tab" class:active={tab === 'rules'} onclick={() => tab = 'rules'}>Rules</button>
		<button class="tab" class:active={tab === 'channels'} onclick={() => tab = 'channels'}>Channels</button>
		<button class="tab" class:active={tab === 'history'} onclick={() => { tab = 'history'; historyOffset = 0; loadHistory(); }}>History</button>
	</div>

	{#if error}
		<div class="placeholder error"><p>{error}</p></div>
	{:else if loading}
		<div class="placeholder"><div class="loader"></div><p>Loading...</p></div>
	{:else}

		<!-- RULES TAB -->
		{#if tab === 'rules'}
			{#if rules.length === 0 && channels.length === 0}
				<div class="placeholder">
					<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
					<h2>No alert rules yet</h2>
					<p>Get notified when agents go offline, certificates expire, latency spikes, hosts go down, and more.</p>
					<button class="btn-fill" onclick={quickSetup}>Enable All Rules</button>
				</div>
			{:else if rules.length === 0}
				<div class="placeholder">
					<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
					<h2>No alert rules</h2>
					<p>Create rules one by one, or enable all with sensible defaults.</p>
					<div class="placeholder-actions">
						<button class="btn-fill" onclick={quickSetup}>Enable All Rules</button>
						<button class="btn-ghost" onclick={openCreateRule}>Create Rule</button>
					</div>
				</div>
			{:else}
				<div class="tbl-wrap">
					<table>
						<thead>
							<tr>
								<th>Name</th>
								<th>Type</th>
								<th>Channels</th>
								<th>Cooldown</th>
								<th>Last Triggered</th>
								<th>Enabled</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							{#each rules as rule}
								<tr>
									<td class="cell-name">{rule.name}</td>
									<td><span class="type-pill">{alertTypeLabel(rule.alert_type)}</span></td>
									<td class="cell-sub">
										{#if rule.channel_ids.length === 0}
											<span class="text-muted">None</span>
										{:else}
											{rule.channel_ids.length} channel{rule.channel_ids.length !== 1 ? 's' : ''}
										{/if}
									</td>
									<td class="cell-sub">{rule.cooldown_minutes}m</td>
									<td class="cell-sub">{rule.last_triggered_at ? formatRelativeTime(rule.last_triggered_at) : 'Never'}</td>
									<td>
										<button class="toggle" class:on={rule.enabled} onclick={() => toggleRule(rule)} aria-label="Toggle rule">
											<span class="toggle-dot"></span>
										</button>
									</td>
									<td class="cell-actions">
										<button class="act act-accent" title="Edit" onclick={() => openEditRule(rule)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
										</button>
										<button class="act act-danger" title="Delete" onclick={() => deleteRule(rule)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

		<!-- CHANNELS TAB -->
		{:else if tab === 'channels'}
			{#if channels.length === 0}
				<div class="placeholder">
					<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6A19.79 19.79 0 0 1 2.12 4.18 2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.127.96.361 1.903.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.907.339 1.85.573 2.81.7A2 2 0 0 1 22 16.92z"/></svg>
					<h2>No notification channels</h2>
					<p>Add an email or webhook channel to receive alerts.</p>
					<button class="btn-fill" onclick={openCreateChannel}>Add Channel</button>
				</div>
			{:else}
				<div class="tbl-wrap">
					<table>
						<thead>
							<tr>
								<th>Name</th>
								<th>Type</th>
								<th>Config</th>
								<th>Enabled</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							{#each channels as ch}
								<tr>
									<td class="cell-name">{ch.name}</td>
									<td><span class="type-pill type-{ch.channel_type}">{ch.channel_type}</span></td>
									<td class="cell-sub cell-mono">{channelSummary(ch)}</td>
									<td>
										<button class="toggle" class:on={ch.enabled} onclick={() => toggleChannel(ch)} aria-label="Toggle channel">
											<span class="toggle-dot"></span>
										</button>
									</td>
									<td class="cell-actions">
										<button class="act act-accent" title="Test" onclick={() => testChannel(ch)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
										</button>
										<button class="act act-accent" title="Edit" onclick={() => openEditChannel(ch)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
										</button>
										<button class="act act-danger" title="Delete" onclick={() => deleteChannel(ch)}>
											<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}

		<!-- HISTORY TAB -->
		{:else if tab === 'history'}
			<div class="history-filters">
				<select class="input filter-select" bind:value={filterType} onchange={() => { historyOffset = 0; loadHistory(); }}>
					<option value="">All Types</option>
					<option value="agent_offline">Agent Offline</option>
					<option value="cert_expiry">Cert Expiry</option>
					<option value="cert_renewal_failed">Renewal Failed</option>
					<option value="error_rate">Error Rate</option>
					<option value="high_latency">High Latency</option>
					<option value="traffic_spike">Traffic Spike</option>
					<option value="host_down">Host Down</option>
					<option value="bandwidth_threshold">Bandwidth</option>
					<option value="crowdsec_ban">CrowdSec Ban</option>
				</select>
				<select class="input filter-select" bind:value={filterSeverity} onchange={() => { historyOffset = 0; loadHistory(); }}>
					<option value="">All Severities</option>
					<option value="info">Info</option>
					<option value="warning">Warning</option>
					<option value="critical">Critical</option>
				</select>
				<select class="input filter-select" bind:value={filterResolved} onchange={() => { historyOffset = 0; loadHistory(); }}>
					<option value="">All Status</option>
					<option value="false">Active</option>
					<option value="true">Resolved</option>
				</select>
			</div>

			{#if history.length === 0}
				<div class="placeholder">
					<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
					<h2>No alert history</h2>
					<p>Triggered alerts will appear here.</p>
				</div>
			{:else}
				<div class="history-list">
					{#each history as entry}
						<div class="history-item" class:resolved={entry.resolved}>
							<div class="history-severity" style="color: {severityColor(entry.severity)}">
								{#if entry.severity === 'critical'}
									<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
								{:else if entry.severity === 'warning'}
									<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
								{:else}
									<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
								{/if}
							</div>
							<div class="history-content">
								<div class="history-head">
									<span class="history-title">{entry.title}</span>
									<span class="type-pill type-sm">{alertTypeLabel(entry.alert_type)}</span>
									{#if entry.resolved}
										<span class="resolved-badge">Resolved</span>
									{:else}
										<span class="active-badge">Active</span>
									{/if}
								</div>
								<p class="history-message">{entry.message}</p>
								<span class="history-time">{formatRelativeTime(entry.created_at)}</span>
							</div>
							{#if !entry.resolved}
								<button class="btn-ghost btn-sm" onclick={() => resolveAlert(entry)}>Resolve</button>
							{/if}
						</div>
					{/each}
				</div>
				{#if history.length < historyTotal}
					<div class="load-more">
						<button class="btn-ghost" onclick={loadMoreHistory}>Load More</button>
					</div>
				{/if}
			{/if}
		{/if}
	{/if}
</div>

<!-- Rule Modal -->
{#if showRuleModal}
	<div class="overlay" onclick={() => showRuleModal = false} onkeydown={(e) => e.key === 'Escape' && (showRuleModal = false)} role="button" tabindex="0">
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
			<h2>{editingRule ? 'Edit Rule' : 'New Alert Rule'}</h2>
			<p class="modal-sub">Configure when and how you get notified.</p>

			<div class="form-group">
				<label for="rule-name">Name</label>
				<input id="rule-name" class="input" type="text" bind:value={ruleName} placeholder="e.g. All agents offline alert" />
			</div>

			{#if !editingRule}
				<div class="form-group">
					<label for="rule-type">Alert Type</label>
					<select id="rule-type" class="input" bind:value={ruleType}>
						<option value="agent_offline">Agent Offline</option>
						<option value="cert_expiry">Certificate Expiry</option>
						<option value="cert_renewal_failed">Certificate Renewal Failed</option>
						<option value="error_rate">Error Rate</option>
						<option value="high_latency">High Latency</option>
						<option value="traffic_spike">Traffic Spike</option>
						<option value="host_down">Host Down</option>
						<option value="bandwidth_threshold">Bandwidth Threshold</option>
						<option value="crowdsec_ban">CrowdSec Ban</option>
					</select>
				</div>
			{/if}

			<!-- Dynamic config -->
			{#if ruleType === 'agent_offline'}
				<div class="form-group">
					<label for="rule-agent-ids">Agent IDs</label>
					<input id="rule-agent-ids" class="input" type="text" bind:value={ruleConfigAgentIds} placeholder='&quot;all&quot; or comma-separated agent IDs' />
					<span class="form-hint">Use "all" to monitor every agent, or list specific agent IDs.</span>
				</div>
			{:else if ruleType === 'cert_expiry'}
				<div class="form-group">
					<!-- svelte-ignore a11y_label_has_associated_control -->
					<label>Warning Thresholds (days)</label>
					<div class="checkbox-group">
						<label class="checkbox-label">
							<input type="checkbox" checked={ruleConfigWarnDays.includes(30)} onchange={() => toggleWarnDay(30)} /> 30 days
						</label>
						<label class="checkbox-label">
							<input type="checkbox" checked={ruleConfigWarnDays.includes(7)} onchange={() => toggleWarnDay(7)} /> 7 days
						</label>
						<label class="checkbox-label">
							<input type="checkbox" checked={ruleConfigWarnDays.includes(1)} onchange={() => toggleWarnDay(1)} /> 1 day
						</label>
					</div>
				</div>
			{:else if ruleType === 'error_rate'}
				<div class="form-group">
					<label for="rule-error-threshold">Error Rate Threshold (%)</label>
					<input id="rule-error-threshold" class="input" type="number" min="0.1" step="0.1" bind:value={ruleConfigThreshold} />
				</div>
				<div class="form-group">
					<label for="rule-domains-err">Domains</label>
					<input id="rule-domains-err" class="input" type="text" bind:value={ruleConfigDomains} placeholder='"all" or comma-separated domains' />
				</div>
				<div class="form-group">
					<label for="rule-window-err">Window (minutes)</label>
					<input id="rule-window-err" class="input" type="number" min="1" bind:value={ruleConfigWindow} />
				</div>
			{:else if ruleType === 'high_latency'}
				<div class="form-group">
					<label for="rule-latency-ms">P95 Latency Threshold (ms)</label>
					<input id="rule-latency-ms" class="input" type="number" min="1" bind:value={ruleConfigLatencyThreshold} />
					<span class="form-hint">Alert when P95 latency exceeds this value. Critical if over 2x threshold.</span>
				</div>
				<div class="form-group">
					<label for="rule-domains-lat">Domains</label>
					<input id="rule-domains-lat" class="input" type="text" bind:value={ruleConfigDomains} placeholder='"all" or comma-separated domains' />
				</div>
				<div class="form-group">
					<label for="rule-window-lat">Window (minutes)</label>
					<input id="rule-window-lat" class="input" type="number" min="1" bind:value={ruleConfigWindow} />
				</div>
			{:else if ruleType === 'traffic_spike'}
				<div class="form-group">
					<label for="rule-multiplier">Spike Multiplier</label>
					<input id="rule-multiplier" class="input" type="number" min="1.5" step="0.5" bind:value={ruleConfigMultiplier} />
					<span class="form-hint">Alert when traffic exceeds baseline by this multiplier (e.g. 3 = 3x normal).</span>
				</div>
				<div class="form-group">
					<label for="rule-baseline">Baseline Period (minutes)</label>
					<input id="rule-baseline" class="input" type="number" min="5" bind:value={ruleConfigBaselineMinutes} />
					<span class="form-hint">How far back to look for normal traffic levels.</span>
				</div>
				<div class="form-group">
					<label for="rule-domains-spike">Domains</label>
					<input id="rule-domains-spike" class="input" type="text" bind:value={ruleConfigDomains} placeholder='"all" or comma-separated domains' />
				</div>
			{:else if ruleType === 'host_down'}
				<div class="form-group">
					<label for="rule-domains-down">Domains</label>
					<input id="rule-domains-down" class="input" type="text" bind:value={ruleConfigDomains} placeholder='"all" or comma-separated domains' />
					<span class="form-hint">Alert when a domain returns 100% 5xx errors (min 5 requests).</span>
				</div>
				<div class="form-group">
					<label for="rule-window-down">Window (minutes)</label>
					<input id="rule-window-down" class="input" type="number" min="1" bind:value={ruleConfigWindow} />
				</div>
			{:else if ruleType === 'bandwidth_threshold'}
				<div class="form-group">
					<label for="rule-bandwidth-gb">Threshold (GB)</label>
					<input id="rule-bandwidth-gb" class="input" type="number" min="0.1" step="0.1" bind:value={ruleConfigBandwidthGB} />
					<span class="form-hint">Alert when bandwidth exceeds this limit. Critical if over 2x threshold.</span>
				</div>
				<div class="form-group">
					<label for="rule-period-hours">Period (hours)</label>
					<input id="rule-period-hours" class="input" type="number" min="1" bind:value={ruleConfigPeriodHours} />
				</div>
				<div class="form-group">
					<label for="rule-domains-bw">Domains</label>
					<input id="rule-domains-bw" class="input" type="text" bind:value={ruleConfigDomains} placeholder='"all" or comma-separated domains' />
				</div>
			{:else if ruleType === 'crowdsec_ban'}
				<div class="form-group">
					<label for="rule-cs-agents">Agent IDs</label>
					<input id="rule-cs-agents" class="input" type="text" bind:value={ruleConfigAgentIds} placeholder='"all" or comma-separated agent IDs' />
					<span class="form-hint">Use "all" to monitor every agent with CrowdSec, or list specific agent IDs.</span>
				</div>
				<div class="form-group">
					<label for="rule-cs-min-events">Minimum Events</label>
					<input id="rule-cs-min-events" class="input" type="number" min="1" bind:value={ruleConfigMinEvents} />
					<span class="form-hint">Only alert when a ban has at least this many events. Critical if 10+.</span>
				</div>
			{/if}

			<div class="form-group">
				<label for="rule-cooldown">Cooldown (minutes)</label>
				<input id="rule-cooldown" class="input" type="number" min="1" bind:value={ruleCooldown} />
				<span class="form-hint">Minimum time between repeated alerts for the same resource.</span>
			</div>

			{#if channels.length > 0}
				<div class="form-group">
					<!-- svelte-ignore a11y_label_has_associated_control -->
					<label>Notification Channels</label>
					<div class="checkbox-group">
						{#each channels as ch}
							<label class="checkbox-label">
								<input type="checkbox" checked={ruleChannelIds.includes(ch.id)} onchange={() => toggleChannelId(ch.id)} />
								{ch.name} <span class="text-muted">({ch.channel_type})</span>
							</label>
						{/each}
					</div>
				</div>
			{/if}

			<div class="modal-foot">
				<button class="btn-ghost" onclick={() => showRuleModal = false}>Cancel</button>
				<button class="btn-fill" onclick={saveRule} disabled={ruleSaving}>
					{ruleSaving ? 'Saving...' : editingRule ? 'Update' : 'Create'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Channel Modal -->
{#if showChannelModal}
	<div class="overlay" onclick={() => showChannelModal = false} onkeydown={(e) => e.key === 'Escape' && (showChannelModal = false)} role="button" tabindex="0">
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
			<h2>{editingChannel ? 'Edit Channel' : 'New Notification Channel'}</h2>
			<p class="modal-sub">Configure how notifications are delivered.</p>

			<div class="form-group">
				<label for="channel-name">Name</label>
				<input id="channel-name" class="input" type="text" bind:value={channelName} placeholder="e.g. My Email" />
			</div>

			{#if !editingChannel}
				<div class="form-group">
					<label for="channel-type">Type</label>
					<select id="channel-type" class="input" bind:value={channelType}>
						<option value="email">Email</option>
						<option value="discord">Discord</option>
						<option value="webhook">Webhook</option>
					</select>
				</div>
			{/if}

			{#if channelType === 'email'}
				<div class="form-group">
					<label for="channel-email">Email Address</label>
					<input id="channel-email" class="input" type="email" bind:value={channelEmail} placeholder="you@example.com" />
				</div>
			{:else if channelType === 'discord'}
				<div class="form-group">
					<label for="channel-discord-url">Discord Webhook URL</label>
					<input id="channel-discord-url" class="input" type="url" bind:value={channelDiscordUrl} placeholder="https://discord.com/api/webhooks/..." />
					<span class="form-hint">Found in Discord: channel settings → Integrations → Webhooks.</span>
				</div>
			{:else}
				<div class="form-group">
					<label for="channel-webhook-url">Webhook URL</label>
					<input id="channel-webhook-url" class="input" type="url" bind:value={channelWebhookUrl} placeholder="https://hooks.slack.com/..." />
					<span class="form-hint">Must use HTTPS.</span>
				</div>
				<div class="form-group">
					<label for="channel-method">HTTP Method</label>
					<select id="channel-method" class="input" bind:value={channelWebhookMethod}>
						<option value="POST">POST</option>
						<option value="PUT">PUT</option>
					</select>
				</div>
			{/if}

			<div class="modal-foot">
				<button class="btn-ghost" onclick={() => showChannelModal = false}>Cancel</button>
				<button class="btn-fill" onclick={saveChannel} disabled={channelSaving}>
					{channelSaving ? 'Saving...' : editingChannel ? 'Update' : 'Create'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	/* Tabs */
	.tabs {
		display: flex;
		gap: 2px;
		margin-bottom: 1.5rem;
		border-bottom: 1px solid var(--border);
	}
	.tab {
		padding: 0.625rem 1.25rem;
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 500;
		cursor: pointer;
		transition: all var(--transition);
	}
	.tab:hover { color: var(--text-primary); }
	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	/* Table cells */
	.cell-name { font-weight: 600; color: var(--text-primary); }
	.cell-sub { color: var(--text-secondary); font-size: var(--text-xs); }
	.cell-mono { font-family: var(--font-mono); font-size: var(--text-xs); max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cell-actions { display: flex; gap: 0.375rem; justify-content: flex-end; }
	.text-muted { color: var(--text-muted); }

	/* Type pill */
	.type-pill {
		display: inline-block;
		padding: 0.125rem 0.5rem;
		border-radius: var(--radius);
		font-size: var(--text-xs);
		font-weight: 600;
		background: var(--surface-raised);
		color: var(--text-secondary);
		white-space: nowrap;
	}
	.type-pill.type-email { color: var(--info); background: var(--info-dim); }
	.type-pill.type-discord { color: #5865F2; background: rgba(88, 101, 242, 0.15); }
	.type-pill.type-webhook { color: var(--warning); background: var(--warning-dim); }
	.type-pill.type-sm { font-size: 0.75rem; padding: 0.0625rem 0.375rem; }

	/* Toggle switch */
	.toggle {
		width: 36px;
		height: 20px;
		border-radius: 10px;
		border: 1px solid var(--border);
		background: var(--surface-raised);
		cursor: pointer;
		position: relative;
		transition: all var(--transition);
		padding: 0;
	}
	.toggle.on { background: var(--accent); border-color: var(--accent); }
	.toggle-dot {
		display: block;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--text-secondary);
		position: absolute;
		top: 2px;
		left: 2px;
		transition: all var(--transition);
	}
	.toggle.on .toggle-dot { background: #fff; left: 18px; }

	/* Form */
	.form-group { margin-bottom: 1.25rem; }
	.form-group label {
		display: block;
		margin-bottom: 0.375rem;
		font-weight: 500;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.form-hint {
		display: block;
		margin-top: 0.25rem;
		font-size: 0.8125rem;
		color: var(--text-tertiary);
	}
	.checkbox-group { display: flex; flex-direction: column; gap: 0.5rem; }
	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: var(--text-sm);
		color: var(--text-primary);
		cursor: pointer;
	}
	.checkbox-label input[type="checkbox"] {
		accent-color: var(--accent);
		width: 16px;
		height: 16px;
	}

	/* History */
	.history-filters {
		display: flex;
		gap: 0.625rem;
		margin-bottom: 1rem;
	}
	.head-actions { display: flex; gap: 0.5rem; align-items: center; }
	.placeholder-actions { display: flex; gap: 0.75rem; align-items: center; }
	.filter-select {
		max-width: 180px;
	}
	.history-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.history-item {
		display: flex;
		align-items: flex-start;
		gap: 0.875rem;
		padding: 1rem 1.25rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-lg);
		transition: border-color var(--transition);
	}
	.history-item:hover { border-color: var(--border-bright); }
	.history-item.resolved { opacity: 0.6; }
	.history-severity { flex-shrink: 0; margin-top: 0.125rem; }
	.history-content { flex: 1; min-width: 0; }
	.history-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.375rem;
		flex-wrap: wrap;
	}
	.history-title {
		font-weight: 600;
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.history-message {
		color: var(--text-secondary);
		font-size: var(--text-xs);
		white-space: pre-line;
		line-height: 1.5;
		margin: 0;
	}
	.history-time {
		display: block;
		margin-top: 0.375rem;
		font-size: 0.75rem;
		color: var(--text-tertiary);
	}
	.resolved-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		background: var(--success-dim);
		color: var(--success);
	}
	.active-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.0625rem 0.375rem;
		border-radius: 4px;
		background: var(--danger-dim);
		color: var(--danger);
	}
	.btn-sm { padding: 0.25rem 0.625rem; font-size: var(--text-xs); }
	.load-more { text-align: center; margin-top: 1rem; }

	@media (max-width: 768px) {
		.history-filters { flex-direction: column; }
		.filter-select { max-width: 100%; }
	}
</style>
