/* Proxera Panel — shared utility functions */

/**
 * Relative time string: "5s ago", "3m ago", "2h ago", "4d ago".
 * Falls back to locale date for anything older than 7 days.
 */
export function formatRelativeTime(dateString: string): string {
	if (!dateString) return 'Never';
	const diffMs = Date.now() - new Date(dateString).getTime();
	const s = Math.floor(diffMs / 1000);
	const m = Math.floor(s / 60);
	const h = Math.floor(m / 60);
	const d = Math.floor(h / 24);
	if (s < 60) return `${s}s ago`;
	if (m < 60) return `${m}m ago`;
	if (h < 24) return `${h}h ago`;
	if (d < 7) return `${d}d ago`;
	return new Date(dateString).toLocaleString();
}

/** Locale date only — "2/5/2026" */
export function formatDate(dateString: string): string {
	if (!dateString) return '—';
	return new Date(dateString).toLocaleDateString();
}

/** Full locale date + time — "2/5/2026, 2:30:05 PM" */
export function formatDateTime(dateString: string): string {
	if (!dateString) return '—';
	return new Date(dateString).toLocaleString();
}

/** Short date + time — "Feb 5 14:30:05" (for metrics) */
export function formatShortDateTime(dateString: string): string {
	if (!dateString) return '—';
	const d = new Date(dateString);
	const month = d.toLocaleString('en', { month: 'short' });
	const day = d.getDate();
	const hh = String(d.getHours()).padStart(2, '0');
	const mm = String(d.getMinutes()).padStart(2, '0');
	const ss = String(d.getSeconds()).padStart(2, '0');
	return `${month} ${day} ${hh}:${mm}:${ss}`;
}

/** Human-readable bytes — "1.5 MB", "320 B" */
export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024));
	const idx = Math.min(i, units.length - 1);
	const val = bytes / Math.pow(1024, idx);
	return `${val < 10 ? val.toFixed(1) : Math.round(val)} ${units[idx]}`;
}

/** Compact number — "1.2k", "3.4M" */
export function formatNumber(n: number): string {
	if (n < 1000) return String(n);
	if (n < 1_000_000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
	return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
}

/** Milliseconds with unit — "12ms", "1.2s" */
export function formatMs(ms: number): string {
	if (ms < 1000) return `${Math.round(ms)}ms`;
	return `${(ms / 1000).toFixed(1)}s`;
}

/** Copy text to clipboard (no-op on failure) */
export async function copyToClipboard(text: string): Promise<void> {
	try {
		await navigator.clipboard.writeText(text);
	} catch {
		// Silently fail — clipboard API may be unavailable
	}
}
