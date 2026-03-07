<script lang="ts">
	import { confirmState } from './confirm';
	import type { ConfirmState } from './confirm';

	let state: ConfirmState = { open: false, title: '', message: '', confirmLabel: 'Confirm', danger: false, resolve: null };
	confirmState.subscribe(v => state = v);

	let dialog: HTMLDivElement | undefined;

	function answer(value: boolean) {
		if (state.resolve) state.resolve(value);
		confirmState.update(s => ({ ...s, open: false, resolve: null }));
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!state.open) return;
		if (e.key === 'Escape') { answer(false); return; }
		if (e.key === 'Tab' && dialog) {
			const focusable = dialog.querySelectorAll<HTMLElement>('button');
			if (focusable.length === 0) return;
			const first = focusable[0];
			const last = focusable[focusable.length - 1];
			if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
			else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
		}
	}

	$: if (state.open && dialog) {
		// Auto-focus first button on open
		requestAnimationFrame(() => {
			const btn = dialog?.querySelector<HTMLElement>('button');
			btn?.focus();
		});
	}
</script>

<svelte:window on:keydown={handleKeydown} />

{#if state.open}
	<div class="confirm-overlay" on:click={() => answer(false)} on:keydown={() => {}} role="presentation">
		<div class="confirm-dialog" bind:this={dialog} on:click|stopPropagation on:keydown|stopPropagation role="dialog" aria-modal="true" aria-labelledby="confirm-title" tabindex="-1">
			<h3 id="confirm-title">{state.title}</h3>
			<p>{state.message}</p>
			<div class="confirm-actions">
				<button class="btn-ghost" on:click={() => answer(false)}>Cancel</button>
				<button class="confirm-btn" class:confirm-danger={state.danger} on:click={() => answer(true)}>{state.confirmLabel}</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.confirm-overlay {
		position: fixed; inset: 0;
		background: rgba(0, 0, 0, 0.6); backdrop-filter: blur(4px);
		display: flex; align-items: center; justify-content: center;
		z-index: 10000;
		animation: fade-in 0.15s ease;
	}
	@keyframes fade-in { from { opacity: 0; } to { opacity: 1; } }

	.confirm-dialog {
		background: var(--surface-raised); border: 1px solid var(--border);
		border-radius: var(--radius-lg); padding: 1.75rem 2rem;
		max-width: 440px; width: 90%;
		animation: dialog-in 0.2s ease;
	}
	@keyframes dialog-in {
		from { opacity: 0; transform: scale(0.96) translateY(4px); }
		to { opacity: 1; transform: scale(1) translateY(0); }
	}

	h3 { margin: 0 0 0.5rem; font-size: var(--text-base); font-weight: 600; color: var(--text-primary); }
	p { margin: 0 0 1.5rem; color: var(--text-secondary); font-size: var(--text-sm); line-height: 1.5; }

	.confirm-actions { display: flex; gap: 0.625rem; justify-content: flex-end; }

	.confirm-btn {
		background: var(--accent); color: #fff; border: none;
		padding: 0.5rem 1.125rem; border-radius: var(--radius);
		font-size: var(--text-sm); font-weight: 600; cursor: pointer;
		transition: background var(--transition);
	}
	.confirm-btn:hover { background: var(--accent-bright); }
	.confirm-danger { background: var(--danger); }
	.confirm-danger:hover { background: #e04550; }

	@media (max-width: 768px) {
		.confirm-dialog { width: 95%; padding: 1.25rem 1.5rem; }
	}
</style>
