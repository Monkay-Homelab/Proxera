<script lang="ts">
	import { page } from '$app/stores';
</script>

<div class="error-page">
	<div class="error-content">
		<h1 class="error-code">{$page.status}</h1>
		<p class="error-message">{$page.error?.message || 'An unexpected error occurred'}</p>
		<a href="/home" class="error-link">Go Home</a>
	</div>
</div>

<style>
	.error-page {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 80vh;
		padding: 2rem;
	}

	.error-content {
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
	}

	.error-code {
		font-size: 6rem;
		font-weight: 700;
		color: var(--accent);
		letter-spacing: -0.04em;
		line-height: 1;
		margin: 0;
	}

	.error-message {
		font-size: var(--text-base);
		color: var(--text-secondary);
		margin: 0.5rem 0 1.5rem;
		max-width: 400px;
		line-height: 1.5;
	}

	.error-link {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		background: var(--accent);
		color: #fff;
		border: none;
		padding: 0.5rem 1.125rem;
		border-radius: var(--radius);
		font-size: var(--text-sm);
		font-weight: 600;
		text-decoration: none;
		transition: background var(--transition);
	}

	.error-link:hover {
		background: var(--accent-bright);
	}
</style>
