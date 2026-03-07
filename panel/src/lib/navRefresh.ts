import { writable } from 'svelte/store';

// Increment this to trigger a nav-context re-fetch from the layout.
// Pages call: navRefresh.update(n => n + 1)
export const navRefresh = writable(0);
