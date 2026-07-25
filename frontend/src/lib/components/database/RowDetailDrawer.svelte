<script lang="ts">
	import type { database } from '$lib/wailsjs/go/models';
	import { getColumnTypeLabel } from '$lib/table/cells';
	import { Check, Copy, Database, FileJson2, Rows3, Search, X } from 'lucide-svelte';
	import { onDestroy } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { focusTrap } from '$lib/actions/focusTrap';
	import { UI_RUNTIME } from '$lib/config/application';

	interface Props {
		open: boolean;
		row: Record<string, any> | null;
		columns: database.Structure[];
		rowNumber: number | null;
		title?: string;
		onClose: () => void;
	}

	let { open, row, columns, rowNumber, title = 'Table row', onClose }: Props = $props();

	let search = $state('');
	let copiedField = $state<string | null>(null);
	let closeButton = $state<HTMLButtonElement>();
	let copyResetTimer: ReturnType<typeof setTimeout> | null = null;
	let wasOpen = false;

	const fields = $derived.by(() => {
		if (!row) return [];

		const columnMap = new Map(columns.map((column) => [column.name, column]));
		const orderedNames = [
			...columns.map((column) => column.name),
			...Object.keys(row).filter((key) => !columnMap.has(key) && !key.startsWith('_'))
		];
		const normalizedSearch = search.trim().toLowerCase();

		return orderedNames
			.filter((name, index, names) => names.indexOf(name) === index)
			.filter((name) => !normalizedSearch || name.toLowerCase().includes(normalizedSearch))
			.map((name) => {
				const column = columnMap.get(name);
				return {
					name,
					type: column ? getColumnTypeLabel(column) : inferValueType(row[name]),
					value: row[name]
				};
			});
	});

	$effect(() => {
		if (open && !wasOpen) {
			search = '';
			requestAnimationFrame(() => closeButton?.focus());
		}
		wasOpen = open;
	});

	onDestroy(() => {
		if (copyResetTimer) clearTimeout(copyResetTimer);
	});

	function inferValueType(value: any): string {
		if (value === null || value === undefined) return 'null';
		if (Array.isArray(value)) return 'array';
		return typeof value === 'object' ? 'object' : typeof value;
	}

	function valueAsText(value: any): string {
		if (value === null || value === undefined) return 'NULL';
		if (typeof value === 'string') return value;
		if (typeof value === 'object') {
			try {
				return JSON.stringify(value, null, 2);
			} catch {
				return String(value);
			}
		}
		return String(value);
	}

	async function copyText(value: string, key: string) {
		try {
			await navigator.clipboard.writeText(value);
			copiedField = key;
			if (copyResetTimer) clearTimeout(copyResetTimer);
			copyResetTimer = setTimeout(() => {
				copiedField = null;
			}, UI_RUNTIME.copyFeedbackMs);
		} catch {
			copiedField = null;
		}
	}

	function handleWindowKeydown(event: KeyboardEvent) {
		if (!open) return;

		if (event.key === 'Escape') {
			event.preventDefault();
			onClose();
			return;
		}
	}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if open && row}
	<button
		type="button"
		class="bg-overlay/25 fixed inset-0 z-[60] cursor-default backdrop-blur-[1px]"
		aria-label="Close row details"
		onclick={onClose}
		transition:fade={{ duration: 120 }}
	></button>

	<div
		use:focusTrap
		class="border-border bg-background fixed inset-y-0 right-0 z-[70] flex w-[min(430px,calc(100vw-24px))] flex-col border-l shadow-2xl"
		role="dialog"
		aria-modal="true"
		aria-label={`${title} details`}
		transition:fly={{ x: 42, duration: 180 }}
	>
		<header class="bg-[var(--surface-raised)]">
			<div class="flex min-h-16 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
				>
					<Rows3 class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<p
						class="text-muted-foreground truncate text-[8px] font-semibold tracking-wide uppercase"
					>
						{title}
					</p>
					<h2 class="mt-0.5 text-[12px] font-bold">
						{rowNumber ? `Row ${rowNumber.toLocaleString()}` : 'Row details'}
					</h2>
				</div>
				<button
					bind:this={closeButton}
					type="button"
					class="rt-toolbar-button h-8 w-8"
					onclick={onClose}
					title="Close details"
					aria-label="Close row details"
				>
					<X class="h-4 w-4" />
				</button>
			</div>

			<div class="flex h-11 items-center gap-2 border-b px-4">
				<div class="relative min-w-0 flex-1">
					<Search
						class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2"
					/>
					<input
						type="search"
						class="rt-input h-8 w-full pr-3 pl-8 text-[10px]"
						placeholder="Find a field"
						bind:value={search}
						aria-label="Find a field"
					/>
				</div>
				<span class="text-muted-foreground shrink-0 text-[8px] font-semibold">
					{fields.length}
					{fields.length === 1 ? 'field' : 'fields'}
				</span>
			</div>
		</header>

		<div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
			{#each fields as field (field.name)}
				<section class="group border-b px-4 py-3 last:border-b-0">
					<div class="flex min-w-0 items-center gap-2">
						<Database class="text-muted-foreground h-3 w-3 shrink-0" />
						<span class="min-w-0 flex-1 truncate font-mono text-[10px] font-bold">
							{field.name}
						</span>
						<span
							class="bg-muted text-muted-foreground max-w-28 truncate rounded px-1.5 py-0.5 font-mono text-[7px] font-semibold"
							title={field.type}
						>
							{field.type}
						</span>
						<button
							type="button"
							class="rt-toolbar-button h-6 w-6 opacity-50 group-hover:opacity-100"
							onclick={() => copyText(valueAsText(field.value), field.name)}
							title={`Copy ${field.name}`}
							aria-label={`Copy ${field.name}`}
						>
							{#if copiedField === field.name}
								<Check class="text-success h-3 w-3" />
							{:else}
								<Copy class="h-3 w-3" />
							{/if}
						</button>
					</div>

					{#if field.value === null || field.value === undefined}
						<div
							class="text-muted-foreground mt-2 rounded-md border border-dashed px-3 py-2 font-mono text-[9px] italic"
						>
							NULL
						</div>
					{:else}
						<pre
							class="rt-code-surface mt-2 max-h-64 overflow-auto rounded-md px-3 py-2.5 font-mono text-[10px] leading-relaxed break-words whitespace-pre-wrap">{valueAsText(
								field.value
							)}</pre>
					{/if}
				</section>
			{:else}
				<div
					class="text-muted-foreground flex h-48 flex-col items-center justify-center px-6 text-center"
				>
					<Search class="h-5 w-5 opacity-40" />
					<p class="mt-2 text-[10px] font-semibold">No matching fields</p>
					<p class="mt-1 text-[8px]">Try another field name.</p>
				</div>
			{/each}
		</div>

		<footer
			class="flex min-h-12 items-center justify-between border-t bg-[var(--surface-raised)] px-4"
		>
			<span class="text-muted-foreground text-[8px]">Read-only row inspector</span>
			<button
				type="button"
				class="rt-toolbar-button h-8 gap-1.5 px-2.5 text-[9px] font-semibold"
				onclick={() => copyText(JSON.stringify(row, null, 2), '__row__')}
			>
				{#if copiedField === '__row__'}
					<Check class="text-success h-3.5 w-3.5" />
					Copied
				{:else}
					<FileJson2 class="h-3.5 w-3.5" />
					Copy JSON
				{/if}
			</button>
		</footer>
	</div>
{/if}
