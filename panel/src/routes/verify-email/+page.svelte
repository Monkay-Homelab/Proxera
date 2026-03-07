<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { PUBLIC_API_URL } from '$env/static/public';

	let status: 'loading' | 'success' | 'error' = 'loading';
	let message = '';

	onMount(async () => {
		const token = $page.url.searchParams.get('token');
		if (!token) {
			status = 'error';
			message = 'Missing verification token.';
			return;
		}

		try {
			const resp = await fetch(`${PUBLIC_API_URL}/api/auth/verify-email?token=${encodeURIComponent(token)}`);
			const data = await resp.json();

			if (resp.ok) {
				status = 'success';
				message = data.message || 'Email verified successfully.';
			} else {
				status = 'error';
				message = data.error || 'Verification failed.';
			}
		} catch {
			status = 'error';
			message = 'Network error. Please try again.';
		}
	});
</script>

<svelte:head>
	<title>Verify Email - Proxera</title>
</svelte:head>

<div class="auth-container">
	<div class="dot-grid"></div>
	<div class="auth-card">
		<div class="logo">
			<span class="logo-text">Proxera</span>
		</div>

		{#if status === 'loading'}
			<h2>Verifying your email...</h2>
			<p class="subtitle">Please wait a moment.</p>
		{:else if status === 'success'}
			<h2>Email Verified</h2>
			<p class="subtitle">{message}</p>
			<a href="/login" class="btn-primary">Sign In</a>
		{:else}
			<h2>Verification Failed</h2>
			<div class="error-message">{message}</div>
			<a href="/login" class="btn-secondary">Back to Login</a>
		{/if}
	</div>
</div>

<style>
	/* Page-specific styles only — shared auth styles in global.css */
	.auth-card { text-align: center; }
	.auth-card h2 { margin-bottom: 0.5rem; }

	/* Verify-email uses inline-block btn-primary (link styled as button) */
	.auth-card .btn-primary {
		display: inline-block; width: auto;
		padding: 0.625rem 1.5rem; text-decoration: none;
	}

	.btn-secondary {
		display: inline-block;
		padding: 0.625rem 1.5rem;
		background: var(--surface); color: var(--text-primary);
		border: 1px solid var(--border); border-radius: var(--radius);
		font-size: 0.875rem; font-weight: 500;
		text-decoration: none; cursor: pointer;
		transition: border-color var(--transition);
	}
	.btn-secondary:hover { border-color: var(--text-secondary); }
</style>
