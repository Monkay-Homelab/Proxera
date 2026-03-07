/* Toast notification store */
import { writable } from 'svelte/store';

export interface ToastItem {
	id: number;
	message: string;
	type: 'success' | 'error' | 'info';
}

let nextId = 0;
export const toasts = writable<ToastItem[]>([]);

function addToast(message: string, type: ToastItem['type'], duration = 5000) {
	const id = nextId++;
	toasts.update(t => [...t, { id, message, type }]);
	setTimeout(() => {
		toasts.update(t => t.filter(i => i.id !== id));
	}, duration);
}

export function toast(message: string, type: ToastItem['type'] = 'info') {
	addToast(message, type);
}

export function toastSuccess(message: string) {
	addToast(message, 'success');
}

export function toastError(message: string) {
	addToast(message, 'error', 7000);
}
