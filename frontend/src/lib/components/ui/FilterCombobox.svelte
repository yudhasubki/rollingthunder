<script lang="ts">
	import { createSelect, melt } from '@melt-ui/svelte';
	import { writable, get } from 'svelte/store';
	import { tick } from 'svelte';
	import { ChevronDown, Check } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	interface Option {
		value: string;
		label: string;
	}

	interface Props {
		options: Option[];
		value: string;
		onChange: (value: string) => void;
		placeholder?: string;
		class?: string;
		disabled?: boolean;
		searchable?: boolean;
		triggerClass?: string;
		searchPlaceholder?: string;
		emptyText?: string;
		id?: string;
	}

	let {
		options,
		value,
		onChange,
		placeholder = 'Select...',
		class: className = '',
		disabled = false,
		searchable = true,
		triggerClass = 'h-8 px-2 text-[11px]',
		searchPlaceholder = 'Search...',
		emptyText = 'No results',
		id = undefined
	}: Props = $props();

	// Find option by value
	const getOption = (val: string) => options.find((o) => o.value === val);

	// State stores
	const selectedStore = writable<Option | undefined>(getOption(value));
	const openStore = writable(false);
	let searchQuery = $state('');
	let searchInputElement = $state<HTMLInputElement | null>(null);

	const {
		elements: { trigger, menu, option: optionEl },
		states: { open },
		helpers: { isSelected }
	} = createSelect<string>({
		selected: selectedStore,
		open: openStore,
		portal: 'body',
		positioning: {
			placement: 'bottom-start',
			strategy: 'fixed',
			sameWidth: true,
			fitViewport: true,
			gutter: 4,
			overflowPadding: 8
		},
		onSelectedChange: ({ next }) => {
			if (next?.value) {
				onChange(next.value);
				openStore.set(false);
			}
			return next;
		}
	});

	// Sync external value to internal state
	$effect(() => {
		const opt = getOption(value);
		if (opt) {
			const currentSelected = get(selectedStore);
			if (!currentSelected || currentSelected.value !== value) {
				selectedStore.set(opt);
			}
		} else if (get(selectedStore)) {
			selectedStore.set(undefined);
		}
	});

	// Display label
	const displayLabel = $derived(getOption(value)?.label ?? '');

	// Filter options
	const filteredOptions = $derived(
		searchable && searchQuery
			? options.filter((opt) => opt.label.toLowerCase().includes(searchQuery.toLowerCase()))
			: options
	);

	$effect(() => {
		if ($open && searchable) {
			tick().then(() => searchInputElement?.focus());
		} else if (!$open && searchQuery) {
			searchQuery = '';
		}
	});
</script>

<div class="relative {className}">
	<!-- Trigger Button -->
	<button
		{id}
		type="button"
		use:melt={$trigger}
		class="rt-input flex w-full cursor-pointer items-center justify-between gap-2 disabled:cursor-not-allowed disabled:opacity-55 {triggerClass}"
		{disabled}
	>
		<span class="truncate {displayLabel ? '' : 'text-muted-foreground'}">
			{displayLabel || placeholder}
		</span>
		<ChevronDown
			class="text-muted-foreground h-4 w-4 shrink-0 transition-transform {$open
				? 'rotate-180'
				: ''}"
		/>
	</button>

	<!-- Dropdown Menu -->
	{#if $open}
		<div
			use:melt={$menu}
			class="rt-popover text-popover-foreground z-[140] max-h-52 min-w-[132px] overflow-hidden rounded-lg p-1.5"
			transition:fly={{ duration: 100, y: -5 }}
			data-filter-dropdown={id ?? 'options'}
		>
			{#if searchable}
				<div class="p-1">
					<input
						bind:this={searchInputElement}
						bind:value={searchQuery}
						class="rt-input placeholder:text-muted-foreground h-7 w-full px-2 text-[11px]"
						placeholder={searchPlaceholder}
						onkeydown={(event) => event.stopPropagation()}
					/>
				</div>
			{/if}

			<!-- Options -->
			<div class="max-h-40 overflow-auto">
				{#if filteredOptions.length === 0}
					<div class="text-muted-foreground px-2 py-1.5 text-[11px]">{emptyText}</div>
				{:else}
					{#each filteredOptions as opt (opt.value)}
						<button
							type="button"
							use:melt={$optionEl({ value: opt.value, label: opt.label })}
							class="hover:bg-accent hover:text-accent-foreground data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[selected]:bg-accent flex w-full cursor-pointer items-center justify-between rounded-md px-2 py-1.5 text-left text-[11px] outline-none"
						>
							<span>{opt.label}</span>
							{#if $isSelected(opt.value)}
								<Check class="h-4 w-4 shrink-0" />
							{/if}
						</button>
					{/each}
				{/if}
			</div>
		</div>
	{/if}
</div>
