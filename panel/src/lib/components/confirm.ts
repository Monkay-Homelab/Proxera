/* Promise-based confirm dialog */
import { writable } from 'svelte/store';

export interface ConfirmState {
	open: boolean;
	title: string;
	message: string;
	confirmLabel: string;
	danger: boolean;
	resolve: ((value: boolean) => void) | null;
}

const initial: ConfirmState = {
	open: false,
	title: '',
	message: '',
	confirmLabel: 'Confirm',
	danger: false,
	resolve: null,
};

export const confirmState = writable<ConfirmState>(initial);

export interface ConfirmOptions {
	title?: string;
	confirmLabel?: string;
	danger?: boolean;
}

/**
 * Show a confirm dialog and return a promise that resolves to true/false.
 * Usage: `if (await confirmDialog('Delete this item?')) { ... }`
 */
export function confirmDialog(
	message: string,
	options: ConfirmOptions = {}
): Promise<boolean> {
	return new Promise((resolve) => {
		confirmState.set({
			open: true,
			title: options.title || 'Confirm',
			message,
			confirmLabel: options.confirmLabel || 'Confirm',
			danger: options.danger ?? false,
			resolve,
		});
	});
}
