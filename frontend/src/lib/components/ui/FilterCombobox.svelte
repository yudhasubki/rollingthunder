<script lang="ts">
	import { createCombobox, melt } from '@melt-ui/svelte';
	import { writable, get } from 'svelte/store';
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
	}

	let {
		options,
		value,
		onChange,
		placeholder = 'Select...',
		class: className = ''
	}: Props = $props();

	// Find option by value
	const getOption = (val: string) => options.find((o) => o.value === val);

	// State stores
	const selectedStore = writable<Option | undefined>(getOption(value));
	const openStore = writable(false);

	const {
		elements: { menu, input, option: optionEl },
		states: { open, inputValue, selected },
		helpers: { isSelected }
	} = createCombobox<string>({
		selected: selectedStore,
		open: openStore,
		forceVisible: true,
		positioning: {
			placement: 'bottom',
			sameWidth: true
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
		}
	});

	// Display label
	const displayLabel = $derived(getOption(value)?.label ?? '');

	// Filter options
	const filteredOptions = $derived(
		$inputValue && $inputValue !== displayLabel
			? options.filter((opt) => opt.label.toLowerCase().includes($inputValue.toLowerCase()))
			: options
	);

	function toggleOpen() {
		openStore.update((v) => !v);
	}
</script>

<div class="relative {className}">
	<!-- Trigger Button -->
	<button
		type="button"
		class="border-input bg-background hover:bg-accent/50 focus:ring-primary flex h-8 w-full cursor-pointer items-center justify-between rounded-md border px-2 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-offset-1"
		onclick={toggleOpen}
	>
		<span class={displayLabel ? '' : 'text-muted-foreground'}>
			{displayLabel || placeholder}
		</span>
		<ChevronDown class="text-muted-foreground h-4 w-4 shrink-0" />
	</button>

	<!-- Dropdown Menu -->
	{#if $open}
		<!-- Backdrop -->
		<button
			type="button"
			class="fixed inset-0 z-[100] cursor-default"
			onclick={() => openStore.set(false)}
		></button>

		<div
			class="bg-popover text-popover-foreground absolute left-0 top-full z-[110] mt-1 max-h-52 w-full min-w-max overflow-auto rounded-md border p-1 shadow-lg"
			transition:fly={{ duration: 100, y: -5 }}
		>
			<!-- Search Input -->
			<div class="p-1">
				<input
					use:melt={$input}
					class="border-input bg-background placeholder:text-muted-foreground h-7 w-full rounded border px-2 text-sm focus:outline-none"
					placeholder="Search..."
				/>
			</div>

			<!-- Options -->
			<div class="max-h-40 overflow-auto">
				{#if filteredOptions.length === 0}
					<div class="text-muted-foreground px-2 py-1.5 text-sm">No results</div>
				{:else}
					{#each filteredOptions as opt (opt.value)}
						<button
							type="button"
							use:melt={$optionEl({ value: opt.value, label: opt.label })}
							class="hover:bg-accent hover:text-accent-foreground data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[selected]:bg-accent flex w-full cursor-pointer items-center justify-between rounded px-2 py-1.5 text-left text-sm outline-none"
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
