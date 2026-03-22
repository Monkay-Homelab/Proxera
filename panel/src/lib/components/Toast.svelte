<script lang="ts">
	import { toasts } from './toast';
	import type { ToastItem } from './toast';

	let items: ToastItem[] = $state([]);

	$effect(() => {
		const unsubscribe = toasts.subscribe(v => items = v);
		return unsubscribe;
	});

	function dismiss(id: number) {
		toasts.update(t => t.filter(i => i.id !== id));
	}
</script>

{#if items.length > 0}
	<div class="toast-stack" aria-live="polite">
		{#each items as item (item.id)}
			<div class="toast toast-{item.type}" role="alert">
				<span class="toast-icon">
					{#if item.type === 'success'}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
					{:else if item.type === 'error'}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
					{:else}
						<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
					{/if}
				</span>
				<span class="toast-msg">{item.message}</span>
				<button class="toast-close" onclick={() => dismiss(item.id)} aria-label="Dismiss">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
				</button>
			</div>
		{/each}
	</div>
{/if}

<style>
	.toast-stack {
		position: fixed; bottom: 1.25rem; right: 1.25rem;
		display: flex; flex-direction: column-reverse; gap: 0.5rem;
		z-index: 9999; max-width: 420px; width: 100%;
		pointer-events: none;
	}

	.toast {
		display: flex; align-items: center; gap: 0.625rem;
		padding: 0.75rem 1rem; border-radius: var(--radius-lg);
		background: var(--surface-raised); border: 1px solid var(--border);
		box-shadow: var(--shadow-md);
		font-size: var(--text-sm); color: var(--text-primary);
		pointer-events: auto;
		animation: toast-in 0.25s ease;
	}

	@keyframes toast-in {
		from { opacity: 0; transform: translateY(8px) scale(0.97); }
		to { opacity: 1; transform: translateY(0) scale(1); }
	}

	.toast-success { border-color: var(--success); }
	.toast-success .toast-icon { color: var(--success); }
	.toast-error { border-color: var(--danger); }
	.toast-error .toast-icon { color: var(--danger); }
	.toast-info { border-color: var(--accent); }
	.toast-info .toast-icon { color: var(--accent); }

	.toast-icon { display: flex; flex-shrink: 0; }
	.toast-msg { flex: 1; line-height: 1.4; }

	.toast-close {
		background: none; border: none; color: var(--text-muted);
		cursor: pointer; padding: 0.125rem; display: flex; flex-shrink: 0;
		border-radius: 4px; transition: color var(--transition);
	}
	.toast-close:hover { color: var(--text-primary); }

	@media (max-width: 768px) {
		.toast-stack { left: 0.75rem; right: 0.75rem; max-width: none; bottom: 0.75rem; }
	}
</style>
