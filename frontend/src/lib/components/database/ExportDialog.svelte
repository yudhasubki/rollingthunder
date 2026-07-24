<script lang="ts">
	import {
		Download,
		FileSpreadsheet,
		Rows3,
		Database,
		X,
		Loader2,
		TriangleAlert
	} from 'lucide-svelte';
	import type { CSVExportSettings, ExportScope } from '$lib/export/csv';

	interface Props {
		open: boolean;
		source: 'table' | 'query';
		pageRows: number;
		totalRows: number;
		truncated?: boolean;
		exporting?: boolean;
		onClose: () => void;
		onExport: (settings: CSVExportSettings) => void | Promise<void>;
	}

	let {
		open,
		source,
		pageRows,
		totalRows,
		truncated = false,
		exporting = false,
		onClose,
		onExport
	}: Props = $props();

	let scope = $state<ExportScope>('page');
	let delimiter = $state<',' | ';' | '\t'>(',');
	let includeHeader = $state(true);
	let nullValue = $state('');
	let wasOpen = false;

	$effect(() => {
		if (open && !wasOpen) {
			scope = source === 'query' ? 'loaded' : 'page';
			delimiter = ',';
			includeHeader = true;
			nullValue = '';
		}
		wasOpen = open;
	});

	function close() {
		if (!exporting) onClose();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (open && event.key === 'Escape') close();
	}

	function submit() {
		void onExport({
			scope,
			delimiter,
			includeHeader,
			nullValue
		});
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<button
		type="button"
		class="fixed inset-0 z-[80] cursor-default bg-black/45 backdrop-blur-[1px]"
		onclick={close}
		aria-label="Close export dialog"
	></button>
	<dialog
		open
		class="bg-popover text-popover-foreground fixed top-1/2 left-1/2 z-[81] m-0 flex w-[min(520px,calc(100vw-32px))] max-w-none -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-xl border p-0 shadow-2xl"
		aria-modal="true"
		aria-labelledby="export-dialog-title"
	>
		<header class="flex items-start justify-between border-b px-4 py-3.5">
			<div class="flex min-w-0 items-start gap-3">
				<span
					class="bg-primary/10 text-primary flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
				>
					<FileSpreadsheet class="h-4 w-4" />
				</span>
				<div class="min-w-0">
					<h2 id="export-dialog-title" class="text-[13px] font-bold">Export as CSV</h2>
					<p class="text-muted-foreground mt-1 text-[10px]">
						Choose what to export, then select the destination file.
					</p>
				</div>
			</div>
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={close}
				disabled={exporting}
				aria-label="Close export dialog"
			>
				<X class="h-3.5 w-3.5" />
			</button>
		</header>

		<div class="space-y-4 p-4">
			<div>
				<div class="mb-2 flex items-center justify-between">
					<span class="text-[10px] font-bold">Rows</span>
					<span class="text-muted-foreground text-[9px]">CSV · UTF-8</span>
				</div>

				{#if source === 'table'}
					<div class="grid grid-cols-2 gap-2">
						<button
							type="button"
							class="flex min-h-16 cursor-pointer items-start gap-2.5 rounded-lg border p-3 text-left transition-colors {scope ===
							'page'
								? 'border-primary/50 bg-primary/5'
								: 'hover:bg-[var(--surface-hover)]'}"
							onclick={() => (scope = 'page')}
						>
							<Rows3 class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span>
								<span class="block text-[10px] font-semibold">Current page</span>
								<span class="text-muted-foreground mt-1 block text-[9px]"
									>{pageRows.toLocaleString()} loaded rows</span
								>
							</span>
						</button>
						<button
							type="button"
							class="flex min-h-16 cursor-pointer items-start gap-2.5 rounded-lg border p-3 text-left transition-colors {scope ===
							'all'
								? 'border-primary/50 bg-primary/5'
								: 'hover:bg-[var(--surface-hover)]'}"
							onclick={() => (scope = 'all')}
						>
							<Database class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span>
								<span class="block text-[10px] font-semibold">All filtered rows</span>
								<span class="text-muted-foreground mt-1 block text-[9px]"
									>{totalRows.toLocaleString()} database rows</span
								>
							</span>
						</button>
					</div>
				{:else}
					<div class="rounded-lg border bg-[var(--surface-sunken)] p-3">
						<div class="flex items-center gap-2">
							<Rows3 class="text-muted-foreground h-3.5 w-3.5" />
							<span class="text-[10px] font-semibold">Loaded query result</span>
							<span class="text-muted-foreground ml-auto text-[9px]"
								>{totalRows.toLocaleString()} rows</span
							>
						</div>
						{#if truncated}
							<div
								class="mt-2 flex items-start gap-2 rounded-md bg-amber-500/10 px-2.5 py-2 text-[9px] text-amber-700 dark:text-amber-300"
							>
								<TriangleAlert class="mt-0.5 h-3 w-3 shrink-0" />
								<span>
									The interactive result was capped. This export contains only the rows currently
									loaded in the query tab.
								</span>
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<div>
				<span class="mb-2 block text-[10px] font-bold">CSV options</span>
				<div class="grid grid-cols-[1fr_1fr] gap-3 rounded-lg border p-3">
					<label class="space-y-1.5">
						<span class="text-muted-foreground block text-[9px] font-semibold">Delimiter</span>
						<span class="flex rounded-md border bg-[var(--surface-sunken)] p-0.5">
							{#each [{ value: ',', label: 'Comma' }, { value: ';', label: 'Semicolon' }, { value: '\t', label: 'Tab' }] as option}
								<button
									type="button"
									class="h-7 flex-1 cursor-pointer rounded text-[8px] font-semibold transition-colors {delimiter ===
									option.value
										? 'text-foreground bg-[var(--surface-raised)] shadow-sm'
										: 'text-muted-foreground hover:text-foreground'}"
									onclick={() => (delimiter = option.value as ',' | ';' | '\t')}
								>
									{option.label}
								</button>
							{/each}
						</span>
					</label>

					<label class="space-y-1.5">
						<span class="text-muted-foreground block text-[9px] font-semibold">NULL value</span>
						<input
							class="rt-input h-8 w-full px-2.5 font-mono text-[9px]"
							value={nullValue}
							oninput={(event) => (nullValue = event.currentTarget.value)}
							placeholder="Empty field"
							disabled={exporting}
						/>
					</label>

					<label class="col-span-2 flex cursor-pointer items-center gap-2 border-t pt-3">
						<input
							type="checkbox"
							class="accent-primary h-3.5 w-3.5"
							checked={includeHeader}
							onchange={(event) => (includeHeader = event.currentTarget.checked)}
							disabled={exporting}
						/>
						<span class="text-[9px] font-semibold">Include column names as the first row</span>
					</label>
				</div>
			</div>
		</div>

		<footer class="flex items-center justify-between border-t bg-[var(--surface-sunken)] px-4 py-3">
			<span class="text-muted-foreground text-[9px]">
				The destination is written only after export completes.
			</span>
			<div class="flex items-center gap-2">
				<button
					type="button"
					class="rt-toolbar-button h-8 cursor-pointer px-3 text-[10px] font-semibold"
					onclick={close}
					disabled={exporting}
				>
					Cancel
				</button>
				<button
					type="button"
					class="rt-primary-button inline-flex h-8 cursor-pointer items-center gap-2 rounded-md px-3 text-[10px] font-bold disabled:pointer-events-none disabled:opacity-60"
					onclick={submit}
					disabled={exporting}
				>
					{#if exporting}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
						Exporting…
					{:else}
						<Download class="h-3.5 w-3.5" />
						Choose destination
					{/if}
				</button>
			</div>
		</footer>
	</dialog>
{/if}
