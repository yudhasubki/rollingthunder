<script lang="ts">
	import {
		createTable,
		getCoreRowModel,
		getPaginationRowModel,
		getSortedRowModel,
		type ColumnDef,
		type SortingState
	} from '@tanstack/table-core';
	import { createContextMenu, melt } from '@melt-ui/svelte';
	import { writable } from 'svelte/store';
	import {
		stageDataUpdate,
		stageDataDelete,
		stageDataInsert,
		stagedChanges
	} from '$lib/stores/staged.svelte';
	import { Plus, Trash2, Copy, ArrowUp, ArrowDown, Filter } from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import { fly } from 'svelte/transition';

	interface Props {
		columns: database.Structure[];
		data: Record<string, any>[];
		totalRows: number;
		currentPage: number;
		pageSize: number;
		onPageChange: (page: number) => void;
		onAddFilter?: () => void;
		readonly?: boolean;
		loading?: boolean;
	}

	let {
		columns,
		data,
		totalRows,
		currentPage,
		pageSize,
		onPageChange,
		onAddFilter,
		readonly = false,
		loading = false
	}: Props = $props();

	// Use writable store for context menu state to avoid Svelte 5 runes conflict
	const ctxOpenStore = writable(false);

	// Track menu position for manual positioning
	let menuPosition = $state({ x: 0, y: 0 });

	// Create melt-ui context menu
	const {
		elements: { menu: ctxMenu, item: ctxItem, separator: ctxSeparator },
		states: { open: ctxOpen }
	} = createContextMenu({
		open: ctxOpenStore,
		forceVisible: true
	});

	// Track which row is being right-clicked
	let contextRow = $state<Record<string, any> | null>(null);

	// Merge staged added rows with existing data for display
	const displayData = $derived([...stagedChanges.data.added.filter((r: any) => r._isNew), ...data]);

	// Editing state
	let editingCell = $state<{ rowIndex: number; colName: string } | null>(null);
	let editValue = $state<string>('');
	let selectedRowIndex = $state<number | null>(null);

	// Sorting state
	let sorting = $state<SortingState>([]);

	function getRowClass(row: Record<string, any>, rowIndex: number): string {
		const rowId = row.id || row._id || rowIndex;
		if (stagedChanges.data.added.some((r: any) => r.id === rowId || r._isNew)) {
			return 'row-added';
		}
		if (stagedChanges.data.updated.some((r: any) => r.id === rowId)) {
			return 'row-updated';
		}
		if (stagedChanges.data.deleted.some((r: any) => r.id === rowId)) {
			return 'row-deleted';
		}
		if (selectedRowIndex === rowIndex) {
			return 'bg-accent';
		}
		return '';
	}

	function startEdit(rowIndex: number, colName: string, currentValue: any) {
		editingCell = { rowIndex, colName };
		editValue = currentValue?.toString() ?? '';
	}

	function saveEdit(row: Record<string, any>, rowIndex: number) {
		if (!editingCell) return;

		const { colName } = editingCell;
		const newValue = editValue;

		// Always update the row with new value
		const updatedRow = { ...row, [colName]: newValue };

		// For new rows (_isNew), update the staged insert directly
		if (row._isNew) {
			// Find and update the row in stagedChanges.data.added
			const addedIndex = stagedChanges.data.added.findIndex((r: any) => r === row);
			if (addedIndex >= 0) {
				stagedChanges.data.added[addedIndex] = updatedRow;
			}
		} else {
			// For existing rows, stage as update if value changed
			const oldValue = row[colName];
			if (newValue !== oldValue?.toString()) {
				stageDataUpdate(updatedRow);
			}
		}

		editingCell = null;
	}

	function cancelEdit() {
		editingCell = null;
		editValue = '';
	}

	function handleKeydown(e: KeyboardEvent, row: Record<string, any>, rowIndex: number) {
		if (e.key === 'Enter') {
			saveEdit(row, rowIndex);
		} else if (e.key === 'Escape') {
			cancelEdit();
		}
	}

	function addNewRow() {
		const newRow: Record<string, any> = { _isNew: true };
		columns.forEach((col) => {
			if (col.defaultValue) {
				newRow[col.name] = col.defaultValue;
			} else {
				newRow[col.name] = null;
			}
		});
		stageDataInsert(newRow);
	}

	function deleteSelectedRow() {
		if (selectedRowIndex !== null && displayData[selectedRowIndex]) {
			stageDataDelete(displayData[selectedRowIndex]);
			selectedRowIndex = null;
		}
	}

	function selectRow(rowIndex: number) {
		selectedRowIndex = selectedRowIndex === rowIndex ? null : rowIndex;
	}

	function handleContextMenu(e: MouseEvent, row: Record<string, any>) {
		e.preventDefault();
		contextRow = row;
		menuPosition = { x: e.clientX, y: e.clientY };
		ctxOpenStore.set(true);
	}

	const totalPages = $derived(Math.ceil(totalRows / pageSize) || 1);
</script>

<div class="flex h-full min-h-0 flex-col">
	<!-- Toolbar -->
	<div class="mb-2 flex flex-shrink-0 items-center justify-between">
		<div class="flex items-center gap-2">
			{#if !readonly}
				<button
					class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border bg-transparent px-3 text-sm transition-colors"
					onclick={addNewRow}
				>
					<Plus class="h-3.5 w-3.5" />
					Add Row
				</button>
				<button
					class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border bg-transparent px-3 text-sm transition-colors disabled:pointer-events-none disabled:opacity-50"
					disabled={selectedRowIndex === null}
					onclick={deleteSelectedRow}
				>
					<Trash2 class="h-3.5 w-3.5" />
					Delete
				</button>
			{/if}
			{#if onAddFilter}
				<button
					class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border bg-transparent px-3 text-sm transition-colors"
					onclick={onAddFilter}
				>
					<Filter class="h-3.5 w-3.5" />
					Filter
				</button>
			{/if}
		</div>
		<span class="text-muted-foreground text-sm">
			{totalRows} rows total
		</span>
	</div>

	<!-- Data Grid -->
	<div class="max-h-[calc(100vh-320px)] min-h-0 flex-1 overflow-auto rounded-md border">
		<table class="w-full caption-bottom text-sm">
			<thead class="[&_tr]:border-b">
				<tr class="hover:bg-muted/50 border-b transition-colors">
					<th class="text-muted-foreground h-10 w-8 px-2 text-left align-middle font-medium">#</th>
					{#each columns as col (col.name)}
						<th
							class="text-muted-foreground h-10 px-2 text-left align-middle font-mono text-xs font-medium"
						>
							<button
								type="button"
								class="hover:text-foreground flex items-center gap-1"
								onclick={() => {
									const existing = sorting.find((s) => s.id === col.name);
									if (existing) {
										if (existing.desc) {
											sorting = sorting.filter((s) => s.id !== col.name);
										} else {
											sorting = sorting.map((s) => (s.id === col.name ? { ...s, desc: true } : s));
										}
									} else {
										sorting = [...sorting, { id: col.name, desc: false }];
									}
								}}
							>
								{col.name}
								{#if sorting.find((s) => s.id === col.name)}
									{#if sorting.find((s) => s.id === col.name)?.desc}
										<ArrowDown class="h-3 w-3" />
									{:else}
										<ArrowUp class="h-3 w-3" />
									{/if}
								{/if}
							</button>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody class="[&_tr:last-child]:border-0">
				{#each displayData as row, rowIndex (rowIndex)}
					<tr
						class="hover:bg-muted/50 border-b transition-colors {getRowClass(
							row,
							rowIndex
						)} cursor-pointer"
						onclick={() => selectRow(rowIndex)}
						oncontextmenu={(e) => handleContextMenu(e, row)}
					>
						<td class="text-muted-foreground w-8 p-2 text-center align-middle text-xs">
							{currentPage * pageSize + rowIndex + 1}
						</td>
						{#each columns as col (col.name)}
							<td class="p-0 align-middle">
								{#if editingCell?.rowIndex === rowIndex && editingCell?.colName === col.name}
									<input
										class="bg-background focus:ring-primary h-full w-full border-0 px-4 py-2 font-mono text-xs outline-none focus:ring-2"
										value={editValue}
										oninput={(e) => (editValue = e.currentTarget.value)}
										onblur={() => saveEdit(row, rowIndex)}
										onkeydown={(e) => handleKeydown(e, row, rowIndex)}
									/>
								{:else}
									<button
										type="button"
										class="hover:bg-accent block w-full px-4 py-2 text-left font-mono text-xs"
										ondblclick={() => startEdit(rowIndex, col.name, row[col.name])}
									>
										<span class="block max-w-48 truncate">
											{#if row[col.name] !== null && row[col.name] !== undefined}
												{row[col.name]}
											{:else}
												<span class="text-muted-foreground italic">NULL</span>
											{/if}
										</span>
									</button>
								{/if}
							</td>
						{/each}
					</tr>
				{/each}

				{#if loading}
					<tr>
						<td colspan={columns.length + 1} class="h-32 text-center">
							<div class="flex flex-col items-center justify-center gap-2">
								<div
									class="border-primary h-6 w-6 animate-spin rounded-full border-2 border-t-transparent"
								></div>
								<span class="text-muted-foreground text-sm">Loading data...</span>
							</div>
						</td>
					</tr>
				{:else if displayData.length === 0}
					<tr>
						<td colspan={columns.length + 1} class="text-muted-foreground h-24 text-center">
							No data
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<!-- Context Menu -->
	{#if $ctxOpen}
		<div
			class="bg-popover text-popover-foreground fixed z-50 min-w-48 rounded-md border p-1 shadow-md"
			style="left: {menuPosition.x}px; top: {menuPosition.y}px;"
			transition:fly={{ duration: 150, y: -5 }}
			role="menu"
		>
			<button
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={() => {
					addNewRow();
					ctxOpenStore.set(false);
				}}
			>
				<Plus class="h-4 w-4" />
				Add row
			</button>
			<button
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={() => {
					if (contextRow) stageDataDelete(contextRow);
					ctxOpenStore.set(false);
				}}
			>
				<Trash2 class="h-4 w-4" />
				Delete row
			</button>
			<div class="bg-border -mx-1 my-1 h-px"></div>
			<button
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={() => {
					if (contextRow) navigator.clipboard.writeText(JSON.stringify(contextRow));
					ctxOpenStore.set(false);
				}}
			>
				<Copy class="h-4 w-4" />
				Copy Row as JSON
			</button>
		</div>
	{/if}

	<!-- Click outside to close -->
	{#if $ctxOpen}
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			onclick={() => ctxOpenStore.set(false)}
			oncontextmenu={(e) => {
				e.preventDefault();
				ctxOpenStore.set(false);
			}}
		></button>
	{/if}

	<!-- Pagination -->
	<div class="mt-3 flex flex-shrink-0 items-center justify-between py-2">
		<span class="text-muted-foreground text-sm">
			showing {currentPage * pageSize + 1}-{Math.min((currentPage + 1) * pageSize, totalRows)} of {totalRows}
		</span>
		<div class="flex items-center gap-2">
			<button
				class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 cursor-pointer items-center rounded-md border bg-transparent px-3 text-sm transition-colors disabled:pointer-events-none disabled:opacity-50"
				disabled={currentPage === 0}
				onclick={() => onPageChange(currentPage - 1)}
			>
				Previous
			</button>
			<span class="text-sm">
				page {currentPage + 1} of {totalPages}
			</span>
			<button
				class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 cursor-pointer items-center rounded-md border bg-transparent px-3 text-sm transition-colors disabled:pointer-events-none disabled:opacity-50"
				disabled={currentPage + 1 >= totalPages}
				onclick={() => onPageChange(currentPage + 1)}
			>
				Next
			</button>
		</div>
	</div>
</div>
