<script lang="ts">
	import {
		Download,
		FileSpreadsheet,
		Rows3,
		Database,
		Braces,
		FileCode2,
		X,
		Loader2,
		TriangleAlert
	} from 'lucide-svelte';
	import type { ExportFormat, ExportScope, ExportSettings } from '$lib/export/options';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	interface Props {
		open: boolean;
		source: 'table' | 'query';
		sourceName?: string;
		pageRows: number;
		totalRows: number;
		truncated?: boolean;
		exporting?: boolean;
		onClose: () => void;
		onExport: (settings: ExportSettings) => void | Promise<void>;
	}

	let {
		open,
		source,
		sourceName = '',
		pageRows,
		totalRows,
		truncated = false,
		exporting = false,
		onClose,
		onExport
	}: Props = $props();

	let scope = $state<ExportScope>('page');
	let format = $state<ExportFormat>('csv');
	let delimiter = $state<',' | ';' | '\t'>(',');
	let includeHeader = $state(true);
	let nullValue = $state('');
	let prettyJSON = $state(true);
	let sqlBatchSize = $state(100);
	let includeTransaction = $state(true);
	let wasOpen = false;
	const sqlBatchOptions = [
		{ value: '100', label: '100 rows' },
		{ value: '500', label: '500 rows' },
		{ value: '1000', label: '1,000 rows' }
	];

	$effect(() => {
		if (open && !wasOpen) {
			scope = source === 'query' ? 'loaded' : 'page';
			format = 'csv';
			delimiter = ',';
			includeHeader = true;
			nullValue = '';
			prettyJSON = true;
			sqlBatchSize = 100;
			includeTransaction = true;
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
			format,
			delimiter,
			includeHeader,
			nullValue,
			prettyJSON,
			sqlBatchSize,
			includeTransaction
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
					{#if format === 'json'}
						<Braces class="h-4 w-4" />
					{:else if format === 'sql'}
						<FileCode2 class="h-4 w-4" />
					{:else}
						<FileSpreadsheet class="h-4 w-4" />
					{/if}
				</span>
				<div class="min-w-0">
					<h2 id="export-dialog-title" class="text-[13px] font-bold">Export data</h2>
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
					<span class="text-[10px] font-bold">Format</span>
					<span class="text-muted-foreground text-[9px]">UTF-8</span>
				</div>
				<div class="grid gap-2 {source === 'table' ? 'grid-cols-3' : 'grid-cols-2'}">
					<button
						type="button"
						class="flex min-h-14 cursor-pointer items-start gap-2.5 rounded-lg border p-3 text-left transition-colors {format ===
						'csv'
							? 'border-primary/50 bg-primary/5'
							: 'hover:bg-[var(--surface-hover)]'}"
						onclick={() => (format = 'csv')}
						disabled={exporting}
					>
						<FileSpreadsheet class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
						<span>
							<span class="block text-[10px] font-semibold">CSV</span>
							<span class="text-muted-foreground mt-1 block text-[9px]"> Tabular rows </span>
						</span>
					</button>
					<button
						type="button"
						class="flex min-h-14 cursor-pointer items-start gap-2.5 rounded-lg border p-3 text-left transition-colors {format ===
						'json'
							? 'border-primary/50 bg-primary/5'
							: 'hover:bg-[var(--surface-hover)]'}"
						onclick={() => (format = 'json')}
						disabled={exporting}
					>
						<Braces class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
						<span>
							<span class="block text-[10px] font-semibold">JSON</span>
							<span class="text-muted-foreground mt-1 block text-[9px]"> Typed objects </span>
						</span>
					</button>
					{#if source === 'table'}
						<button
							type="button"
							class="flex min-h-14 cursor-pointer items-start gap-2.5 rounded-lg border p-3 text-left transition-colors {format ===
							'sql'
								? 'border-primary/50 bg-primary/5'
								: 'hover:bg-[var(--surface-hover)]'}"
							onclick={() => (format = 'sql')}
							disabled={exporting}
						>
							<FileCode2 class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span>
								<span class="block text-[10px] font-semibold">SQL</span>
								<span class="text-muted-foreground mt-1 block text-[9px]"> INSERT statements </span>
							</span>
						</button>
					{/if}
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<span class="text-[10px] font-bold">Rows</span>
					<span class="text-muted-foreground text-[9px]">{format.toUpperCase()}</span>
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

			{#if format === 'csv'}
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
										disabled={exporting}
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
			{:else if format === 'json'}
				<div>
					<span class="mb-2 block text-[10px] font-bold">JSON options</span>
					<div class="rounded-lg border p-3">
						<label class="flex cursor-pointer items-start gap-2">
							<input
								type="checkbox"
								class="accent-primary mt-0.5 h-3.5 w-3.5"
								checked={prettyJSON}
								onchange={(event) => (prettyJSON = event.currentTarget.checked)}
								disabled={exporting}
							/>
							<span>
								<span class="block text-[9px] font-semibold">Pretty-print JSON</span>
								<span class="text-muted-foreground mt-1 block text-[9px] leading-relaxed">
									Use two-space indentation. Disable it for a smaller compact file.
								</span>
							</span>
						</label>
						<div class="text-muted-foreground mt-3 border-t pt-3 text-[9px] leading-relaxed">
							Exports one valid JSON array. Dates use ISO 8601 and binary values use a
							<code class="font-mono">base64:</code> prefix.
						</div>
					</div>
				</div>
			{:else}
				<div>
					<span class="mb-2 block text-[10px] font-bold">SQL options</span>
					<div class="rounded-lg border p-3">
						<div class="grid grid-cols-2 gap-3">
							<label class="space-y-1.5">
								<span class="text-muted-foreground block text-[9px] font-semibold">
									Rows per INSERT
								</span>
								<FilterCombobox
									id="export-sql-batch-size"
									options={sqlBatchOptions}
									value={String(sqlBatchSize)}
									onChange={(value) => (sqlBatchSize = Number(value))}
									placeholder="Rows per INSERT"
									searchable={false}
									disabled={exporting}
									triggerClass="h-8 px-2.5 text-[9px]"
								/>
							</label>

							<label
								class="flex cursor-pointer items-start gap-2 rounded-md bg-[var(--surface-sunken)] px-2.5 py-2"
							>
								<input
									type="checkbox"
									class="accent-primary mt-0.5 h-3.5 w-3.5"
									checked={includeTransaction}
									onchange={(event) => (includeTransaction = event.currentTarget.checked)}
									disabled={exporting}
								/>
								<span>
									<span class="block text-[9px] font-semibold">Wrap in transaction</span>
									<span class="text-muted-foreground mt-1 block text-[8px] leading-relaxed">
										Add BEGIN and COMMIT.
									</span>
								</span>
							</label>
						</div>

						<div class="text-muted-foreground mt-3 border-t pt-3 text-[9px] leading-relaxed">
							{#if sourceName}
								<span class="text-foreground block truncate font-mono font-semibold">
									Target: {sourceName}
								</span>
							{/if}
							<span class:mt-1={sourceName !== ''} class="block">
								PostgreSQL quotes native values. Generated columns are skipped; sequence state is
								not changed.
							</span>
						</div>
					</div>
				</div>
			{/if}
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
