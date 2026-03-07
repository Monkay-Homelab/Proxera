/**
 * Creates a fetch group that auto-aborts the previous in-flight request
 * when a new one is started. Prevents race conditions from overlapping
 * auto-refresh + manual refresh calls.
 */
export function createFetchGroup() {
	let controller: AbortController | null = null;

	return {
		/**
		 * Wraps an API call with automatic abort of the previous in-flight request.
		 * Pass the AbortSignal to your fetch/api call.
		 */
		signal(): AbortSignal {
			if (controller) controller.abort();
			controller = new AbortController();
			return controller.signal;
		},

		/** Abort any in-flight request (e.g., on component destroy). */
		abort() {
			if (controller) {
				controller.abort();
				controller = null;
			}
		},

		/** Check if the error is from an aborted request (should be silently ignored). */
		isAborted(err: unknown): boolean {
			return err instanceof DOMException && err.name === 'AbortError';
		}
	};
}
