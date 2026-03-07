<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { PUBLIC_API_URL } from '$env/static/public';
	import { setToken, setUserRole } from '$lib/api';

	let email = '';
	let password = '';
	let error = '';
	let loading = false;
	let verificationRequired = false;
	let resendStatus = '';
	let registrationOpen = true;

	onMount(async () => {
		try {
			const res = await fetch(`${PUBLIC_API_URL}/api/auth/registration-status`);
			const data = await res.json();
			registrationOpen = data.mode !== 'disabled';
		} catch {}
	});

	async function handleLogin() {
		error = '';
		verificationRequired = false;
		resendStatus = '';
		loading = true;

		try {
			const response = await fetch(`${PUBLIC_API_URL}/api/auth/login`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ email, password }),
				credentials: 'include'
			});

			const data = await response.json();

			if (!response.ok) {
				if (data.verification_required) {
					verificationRequired = true;
				}
				throw new Error(data.error || data.message || 'Login failed');
			}

			setToken(data.token);
			setUserRole(data.user?.role || '');
			goto('/home');
		} catch (err) {
			error = err instanceof Error ? err.message : 'An error occurred';
		} finally {
			loading = false;
		}
	}

	async function resendVerification() {
		resendStatus = 'sending';
		try {
			const resp = await fetch(`${PUBLIC_API_URL}/api/auth/resend-verification`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email })
			});
			const data = await resp.json();
			resendStatus = 'sent';
		} catch {
			resendStatus = 'error';
		}
	}
</script>

<svelte:head>
	<title>Login - Proxera</title>
</svelte:head>

<div class="auth-container">
	<div class="dot-grid"></div>
	<div class="auth-card">
		<div class="logo">
			<span class="logo-text">Proxera</span>
		</div>

		<form on:submit|preventDefault={handleLogin}>
			<h2>Sign In</h2>
			<p class="subtitle">Enter your credentials to continue</p>

			{#if error}
				<div class="error-message">
					{error}
				</div>
			{/if}

			{#if verificationRequired}
				<div class="verify-notice">
					{#if resendStatus === 'sent'}
						<span>Verification email sent. Check your inbox.</span>
					{:else if resendStatus === 'sending'}
						<span>Sending...</span>
					{:else if resendStatus === 'error'}
						<span>Failed to send. Try again.</span>
					{:else}
						<button type="button" class="link-btn" on:click={resendVerification}>Resend verification email</button>
					{/if}
				</div>
			{/if}

			<div class="form-group">
				<label for="email">Email</label>
				<input
					type="email"
					id="email"
					bind:value={email}
					required
					placeholder="you@example.com"
					disabled={loading}
				/>
			</div>

			<div class="form-group">
				<label for="password">Password</label>
				<input
					type="password"
					id="password"
					bind:value={password}
					required
					placeholder="Enter your password"
					disabled={loading}
				/>
			</div>

			<button type="submit" class="btn-primary" disabled={loading}>
				{loading ? 'Signing in...' : 'Sign In'}
			</button>

			{#if registrationOpen}
			<p class="auth-link">
				Don't have an account? <a href="/register">Sign up</a>
			</p>
			{/if}
		</form>
	</div>
</div>

<style>
	/* Page-specific styles only — shared auth styles in global.css */
	.verify-notice {
		background: var(--accent-dim);
		border: 1px solid var(--accent);
		color: var(--text-secondary);
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius);
		margin-bottom: 1.25rem;
		font-size: 0.8125rem;
		text-align: center;
	}

	.link-btn {
		background: none; border: none;
		color: var(--accent); font-size: 0.8125rem;
		font-weight: 500; cursor: pointer;
		text-decoration: underline; padding: 0;
		font-family: var(--font-sans);
	}
	.link-btn:hover { opacity: 0.8; }
</style>
