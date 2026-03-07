<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { PUBLIC_API_URL } from '$env/static/public';
	import { setToken, setUserRole } from '$lib/api';

	let email = '';
	let password = '';
	let confirmPassword = '';
	let name = '';
	let inviteCode = '';
	let error = '';
	let loading = false;
	let verificationSent = false;
	let registrationMode = 'open';

	onMount(async () => {
		try {
			const res = await fetch(`${PUBLIC_API_URL}/api/auth/registration-status`);
			const data = await res.json();
			registrationMode = data.mode;
			if (data.mode === 'disabled') {
				goto('/login');
				return;
			}
		} catch {}
	});

	async function handleRegister() {
		error = '';

		// Validation
		if (password !== confirmPassword) {
			error = 'Passwords do not match';
			return;
		}

		if (password.length < 8) {
			error = 'Password must be at least 8 characters';
			return;
		}

		loading = true;

		try {
			const response = await fetch(`${PUBLIC_API_URL}/api/auth/register`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ email, password, name, invite_code: inviteCode || undefined }),
				credentials: 'include'
			});

			const data = await response.json();

			if (!response.ok) {
				throw new Error(data.error || data.message || 'Registration failed');
			}

			if (data.verification_required) {
				verificationSent = true;
				return;
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
</script>

<svelte:head>
	<title>Sign Up - Proxera</title>
</svelte:head>

<div class="auth-container">
	<div class="dot-grid"></div>
	<div class="auth-card">
		<div class="logo">
			<span class="logo-text">Proxera</span>
		</div>

		{#if verificationSent}
			<h2>Check Your Email</h2>
			<p class="subtitle">We sent a verification link to <strong>{email}</strong>. Click the link to activate your account.</p>
			<a href="/login" class="btn-primary" style="display:block;text-align:center;text-decoration:none;">Go to Login</a>
		{:else}
		<form on:submit|preventDefault={handleRegister}>
			<h2>Create Account</h2>
			<p class="subtitle">Set up your account to get started</p>

			{#if error}
				<div class="error-message">
					{error}
				</div>
			{/if}

			<div class="form-group">
				<label for="name">Name</label>
				<input
					type="text"
					id="name"
					bind:value={name}
					required
					placeholder="Your full name"
					disabled={loading}
				/>
			</div>

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
					minlength="8"
				/>
				<small>At least 8 characters</small>
			</div>

			<div class="form-group">
				<label for="confirmPassword">Confirm Password</label>
				<input
					type="password"
					id="confirmPassword"
					bind:value={confirmPassword}
					required
					placeholder="Re-enter your password"
					disabled={loading}
				/>
			</div>

			<div class="form-group">
				<label for="inviteCode">Invite Code</label>
				<input
					type="text"
					id="inviteCode"
					bind:value={inviteCode}
					placeholder="Leave blank if not required"
					disabled={loading}
				/>
				<small>Only needed if registration requires an invite</small>
			</div>

			<button type="submit" class="btn-primary" disabled={loading}>
				{loading ? 'Creating account...' : 'Sign Up'}
			</button>

			<p class="auth-link">
				Already have an account? <a href="/login">Sign in</a>
			</p>
		</form>
		{/if}
	</div>
</div>

<style>
	/* Page-specific styles only — shared auth styles in global.css */
	.auth-card small {
		display: block; margin-top: 0.25rem;
		color: var(--text-tertiary); font-size: 0.75rem;
		font-family: var(--font-sans);
	}
</style>
