<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let loading = true;
	let consentChecked = false;
	let accepting = false;
	let error = '';

	onMount(async () => {
		try {
			const res = await api('/api/setup/status');
			const data = await res.json();
			if (!data.crowdsec_eula_required) {
				goto('/home');
				return;
			}
		} catch {
			goto('/home');
			return;
		}
		loading = false;
	});

	async function acceptEula() {
		accepting = true;
		error = '';
		try {
			const res = await api('/api/setup/crowdsec-eula', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});
			if (!res.ok) {
				const data = await res.json();
				throw new Error(data.error || 'Failed to accept');
			}
			goto('/home');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save acceptance';
			accepting = false;
		}
	}

	function skipSetup() {
		goto('/home');
	}
</script>

<svelte:head>
	<title>Setup - Proxera</title>
</svelte:head>

{#if loading}
	<div class="loading-state" aria-live="polite">
		<div class="loader"></div>
		<p>Loading...</p>
	</div>
{:else}
	<div class="setup-container">
		<div class="setup-card">
			<div class="setup-header">
				<div class="setup-icon">
					<svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
					</svg>
				</div>
				<h1>CrowdSec — Terms & Data Sharing</h1>
				<p class="setup-subtitle">CrowdSec is pre-installed on your control node. Before it begins protecting your server, please review and accept the terms below.</p>
			</div>

			{#if error}
				<div class="error-banner">{error}</div>
			{/if}

			<div class="consent-body">
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
						<li>The CrowdSec software itself is MIT licensed (free & open source)</li>
						<li>Community data sharing applies when connected to CrowdSec's Central API</li>
					</ul>
					<a href="https://www.crowdsec.net/eula" target="_blank" rel="noopener noreferrer" class="eula-link">Read CrowdSec's full EULA &rarr;</a>
				</div>

				<label class="consent-checkbox">
					<input type="checkbox" bind:checked={consentChecked} />
					<span>I understand that attack data from this server will be shared with CrowdSec's community network, and I agree to CrowdSec's <a href="https://www.crowdsec.net/eula" target="_blank" rel="noopener noreferrer">EULA</a>.</span>
				</label>
			</div>

			<div class="setup-actions">
				<button class="btn-ghost" on:click={skipSetup}>Skip for now</button>
				<button class="btn-fill" on:click={acceptEula} disabled={!consentChecked || accepting}>
					{accepting ? 'Saving...' : 'Accept & Continue'}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.setup-container {
		display: flex; align-items: center; justify-content: center;
		min-height: 100vh; padding: 2rem;
		background: var(--bg);
	}
	.setup-card {
		background: var(--surface); border: 1px solid var(--border);
		border-radius: var(--radius-lg); max-width: 600px; width: 100%;
		overflow: hidden;
	}
	.setup-header {
		padding: 2rem 2rem 1.5rem; text-align: center;
		border-bottom: 1px solid var(--border);
	}
	.setup-icon { color: var(--accent); margin-bottom: 1rem; }
	.setup-header h1 {
		margin: 0 0 0.75rem; font-size: var(--text-xl);
		color: var(--text-primary);
	}
	.setup-subtitle {
		margin: 0; color: var(--text-secondary);
		font-size: var(--text-sm); line-height: 1.6;
		max-width: 460px; margin: 0 auto;
	}

	.error-banner {
		background: var(--danger-dim); color: var(--danger);
		border: 1px solid var(--danger); padding: 0.75rem 1rem;
		margin: 1rem 2rem 0; border-radius: var(--radius); font-size: var(--text-sm);
	}

	.consent-body { padding: 1.5rem 2rem; }

	.consent-section { margin-bottom: 1.25rem; }
	.consent-section h3 {
		margin: 0 0 0.5rem; font-size: var(--text-sm);
		font-weight: 600; color: var(--text-primary);
	}
	.consent-section > p {
		margin: 0; color: var(--text-secondary);
		font-size: var(--text-sm); line-height: 1.6;
	}
	.consent-section ul { margin: 0.5rem 0 0; padding: 0; list-style: none; }
	.consent-section li {
		padding: 0.3rem 0 0.3rem 1.25rem; color: var(--text-secondary);
		font-size: var(--text-sm); position: relative;
	}
	.consent-section li::before {
		content: '\2022'; position: absolute; left: 0.375rem; color: var(--accent);
	}
	.consent-note {
		color: var(--text-tertiary) !important; font-size: var(--text-xs) !important;
		margin-top: 0.5rem !important; font-style: italic;
	}
	.eula-link {
		display: inline-block; margin-top: 0.75rem;
		font-size: var(--text-sm); color: var(--accent); text-decoration: none;
	}
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
	.consent-checkbox span {
		font-size: var(--text-sm); color: var(--text-primary); line-height: 1.5;
	}
	.consent-checkbox a { color: var(--accent); }

	.setup-actions {
		display: flex; gap: 0.75rem; justify-content: flex-end;
		padding: 1.25rem 2rem; border-top: 1px solid var(--border);
	}
</style>
