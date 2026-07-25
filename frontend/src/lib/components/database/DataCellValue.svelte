<script lang="ts">
	import { getCellPresentation } from '$lib/table/cells';
	import { Braces, CalendarClock, FileDigit } from 'lucide-svelte';

	interface Props {
		value: any;
		dataType?: string;
	}

	let { value, dataType = '' }: Props = $props();
	const presentation = $derived(getCellPresentation(value, dataType));
</script>

{#if presentation.kind === 'null'}
	<span
		class="text-muted-foreground inline-flex rounded border border-dashed px-1.5 py-0.5 font-mono text-[8px] italic"
	>
		NULL
	</span>
{:else if presentation.kind === 'boolean'}
	<span
		class="inline-flex items-center gap-1.5 font-mono text-[9px] font-semibold"
		title={presentation.title}
	>
		<span
			class="h-1.5 w-1.5 rounded-full {presentation.booleanValue
				? 'bg-success'
				: 'bg-muted-foreground/55'}"
		></span>
		{presentation.text}
	</span>
{:else if presentation.kind === 'json'}
	<span
		class="bg-muted text-muted-foreground inline-flex max-w-full items-center gap-1.5 rounded px-1.5 py-1 font-mono text-[9px]"
		title={presentation.title}
	>
		<Braces class="h-3 w-3 shrink-0" />
		<span class="truncate">{presentation.text}</span>
	</span>
{:else if presentation.kind === 'datetime'}
	<span
		class="text-muted-foreground flex min-w-0 items-center gap-1.5 font-mono text-[9px]"
		title={presentation.title}
	>
		<CalendarClock class="h-3 w-3 shrink-0 opacity-55" />
		<span class="truncate">{presentation.text}</span>
	</span>
{:else if presentation.kind === 'binary'}
	<span
		class="text-muted-foreground flex min-w-0 items-center gap-1.5 font-mono text-[9px]"
		title={presentation.title}
	>
		<FileDigit class="h-3 w-3 shrink-0 opacity-55" />
		<span class="truncate">{presentation.text}</span>
	</span>
{:else}
	<span
		class="block min-w-0 truncate font-mono text-[10px] {presentation.kind === 'number'
			? 'font-medium tabular-nums'
			: ''}"
		title={presentation.title}
	>
		{presentation.text || ' '}
	</span>
{/if}
