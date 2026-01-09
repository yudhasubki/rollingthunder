<script lang="ts">
	import { createSelect, melt } from '@melt-ui/svelte';
	import { writable } from 'svelte/store';
	import { ChevronDown, Check } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	interface DataType {
		name: string;
		category: string;
		description: string;
	}

	interface Props {
		dataTypes: DataType[];
		value: string;
		onChange: (value: string) => void;
	}

	let { dataTypes, value, onChange }: Props = $props();

	// Group data types by category
	const groupedTypes = $derived(() => {
		const groups: Record<string, DataType[]> = {};
		for (const dt of dataTypes) {
			if (!groups[dt.category]) {
				groups[dt.category] = [];
			}
			groups[dt.category].push(dt);
		}
		return groups;
	});

	// Find current selected option
	const getSelected = (val: string) => {
		const dt = dataTypes.find((d) => d.name === val);
		return dt ? { value: dt.name, label: dt.name } : undefined;
	};

	const selectedStore = writable(getSelected(value));
	const openStore = writable(false);

	const {
		elements: { trigger, menu, option, group, groupLabel },
		states: { open, selected },
		helpers: { isSelected }
	} = createSelect({
		selected: selectedStore,
		open: openStore,
		forceVisible: true,
		positioning: {
			placement: 'bottom',
			sameWidth: true
		},
		onSelectedChange: ({ next }) => {
			if (next?.value) {
				onChange(next.value as string);
			}
			return next;
		}
	});

	// Sync external value to internal state
	$effect(() => {
		const opt = getSelected(value);
		if (opt && $selected?.value !== value) {
			selectedStore.set(opt);
		}
	});
</script>

<div class="relative">
	<button
		type="button"
		use:melt={$trigger}
		class="border-input bg-background hover:bg-accent/50 focus:ring-primary flex h-8 w-full cursor-pointer items-center justify-between rounded-md border px-2 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-offset-1"
	>
		<span class="truncate">{value || 'Select type...'}</span>
		<ChevronDown class="text-muted-foreground h-4 w-4 shrink-0" />
	</button>

	{#if $open}
		<div
			use:melt={$menu}
			class="bg-popover text-popover-foreground z-[110] max-h-60 overflow-auto rounded-md border p-1 shadow-lg"
			transition:fly={{ duration: 100, y: -5 }}
		>
			{#each Object.entries(groupedTypes()) as [category, types]}
				<div use:melt={$group(category)}>
					<div
						use:melt={$groupLabel(category)}
						class="text-muted-foreground px-2 py-1.5 text-xs font-semibold"
					>
						{category}
					</div>
					{#each types as dt}
						<div
							use:melt={$option({ value: dt.name, label: dt.name })}
							class="data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground flex cursor-pointer items-center justify-between rounded-sm px-2 py-1.5 text-sm outline-none"
						>
							<span>{dt.name}</span>
							{#if $isSelected(dt.name)}
								<Check class="h-4 w-4" />
							{/if}
						</div>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>
