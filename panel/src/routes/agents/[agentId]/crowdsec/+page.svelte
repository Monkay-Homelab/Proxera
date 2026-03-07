<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	const agentId = $page.params.agentId;

	onMount(() => {
		goto(`/crowdsec/${agentId}`, { replaceState: true });
	});
</script>

<div style="text-align: center; padding: 3rem; color: var(--text-tertiary, #5c5c66);">Redirecting...</div>
