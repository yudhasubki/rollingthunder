<script lang="ts">
	import { getSegments, getLevel, getDatabaseInfo } from '$lib/stores/status.svelte';
	import { hasChanges, stagedChanges } from '$lib/stores/staged.svelte';
	import { CircleCheck, CircleAlert, CircleX, Database, Clock } from 'lucide-svelte';

	const segments = $derived(getSegments());
	const level = $derived(getLevel());
	const dbInfo = $derived(getDatabaseInfo());
	const changes = $derived(hasChanges());
	const staged = $derived(stagedChanges);

	const statusColor = $derived(
		level === 'error' ? 'text-destructive' : level === 'warn' ? 'text-yellow-500' : 'text-green-500'
	);

	const changeCount = $derived(
		(staged.data?.added?.length || 0) +
			(staged.data?.updated?.length || 0) +
			(staged.data?.deleted?.length || 0)
	);
</script>

<footer class="bg-sidebar flex h-7 items-center justify-between border-t px-3 text-xs">
	<!-- Left: Status -->
	<div class="flex items-center gap-2">
		{#if level === 'error'}
			<CircleX class="h-3.5 w-3.5 {statusColor}" />
		{:else if level === 'warn'}
			<CircleAlert class="h-3.5 w-3.5 {statusColor}" />
		{:else}
			<CircleCheck class="h-3.5 w-3.5 {statusColor}" />
		{/if}
		<span class="text-muted-foreground">
			{#if segments.length > 0}
				{segments.join(' • ')}
			{:else}
				Ready
			{/if}
		</span>
	</div>

	<!-- Right: Info -->
	<div class="text-muted-foreground flex items-center gap-3">
		{#if changes}
			<span class="bg-secondary rounded-md px-1.5 py-0.5 text-xs">
				{changeCount} pending changes
			</span>
		{/if}

		{#if dbInfo}
			<div class="flex items-center gap-1">
				<Database class="h-3 w-3" />
				<span>{dbInfo.engine || 'Unknown'}</span>
			</div>
			<span class="opacity-50">|</span>
			<span>{dbInfo.database || 'db'}</span>
		{/if}
	</div>
</footer>
