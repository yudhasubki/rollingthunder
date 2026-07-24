<script lang="ts">
	import { getSegments, getLevel, getDatabaseInfo } from '$lib/stores/status.svelte';
	import { getStagedChanges, hasChanges } from '$lib/stores/staged.svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { CircleCheck, CircleAlert, CircleX, Database } from 'lucide-svelte';

	const segments = $derived(getSegments());
	const level = $derived(getLevel());
	const dbInfo = $derived(getDatabaseInfo());
	const changes = $derived(hasChanges(tabsStore.activeTabId));
	const staged = $derived(tabsStore.activeTabId ? getStagedChanges(tabsStore.activeTabId) : null);

	const statusColor = $derived(
		level === 'error' ? 'text-destructive' : level === 'warn' ? 'text-yellow-500' : 'text-green-500'
	);

	const changeCount = $derived(
		(staged?.data.added.length || 0) +
			(staged?.data.updated.length || 0) +
			(staged?.data.deleted.length || 0)
	);
</script>

<footer
	class="flex h-6 shrink-0 items-center justify-between border-t bg-[var(--surface-raised)] px-3 text-[10px]"
	role="status"
	aria-live="polite"
	aria-atomic="true"
>
	<div class="flex items-center gap-2">
		{#if level === 'error'}
			<CircleX class="h-3 w-3 {statusColor}" />
		{:else if level === 'warn'}
			<CircleAlert class="h-3 w-3 {statusColor}" />
		{:else}
			<CircleCheck class="h-3 w-3 {statusColor}" />
		{/if}
		<span class="text-muted-foreground">
			{#if segments.length > 0}
				{segments.join(' • ')}
			{:else}
				Ready
			{/if}
		</span>
	</div>

	<div class="text-muted-foreground flex items-center gap-3">
		{#if changes}
			<span class="bg-primary/10 text-primary rounded px-1.5 py-0.5 font-semibold">
				{changeCount} pending changes
			</span>
		{/if}

		{#if dbInfo}
			<div class="flex items-center gap-1">
				<Database class="h-2.5 w-2.5" />
				<span class="font-semibold">{dbInfo.engine || 'Unknown'}</span>
			</div>
			<span class="h-3 border-l"></span>
			<span>{dbInfo.database || 'db'}</span>
		{/if}
	</div>
</footer>
