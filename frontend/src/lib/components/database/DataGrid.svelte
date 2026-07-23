<script lang="ts">
	import type { SortingState } from '@tanstack/table-core';
	import {
		stageDataUpdate,
		stageDataDelete,
		stageDataInsert,
		stagedChanges
	} from '$lib/stores/staged.svelte';
	import { Plus, Trash2, Copy, ArrowUp, ArrowDown, Filter, Loader2, Rows3 } from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import { getContextMenuPosition } from '$lib/utils/contextMenu';
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
		loadingTitle?: string;
		loadingDescription?: string;
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
		loading = false,
		loadingTitle = 'Loading table data',
		loadingDescription = 'Waiting for the database…'
	}: Props = $props();

	// Track menu position for manual positioning
	let menuPosition = $state({ x: 0, y: 0 });
	let contextMenuOpen = $state(false);

	// Track which row is being right-clicked
	let contextRow = $state<Record<string, any> | null>(null);
	let contextRowIndex = $state<number | null>(null);

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

	function handleContextMenu(e: MouseEvent, row: Record<string, any>, rowIndex: number) {
		e.preventDefault();
		contextRow = row;
		contextRowIndex = rowIndex;
		selectedRowIndex = rowIndex;
		menuPosition = getContextMenuPosition(e, 236, readonly ? 126 : 210);
		contextMenuOpen = true;
	}

	function closeContextMenu() {
		contextMenuOpen = false;
		contextRow = null;
		contextRowIndex = null;
	}

	const totalPages = $derived(Math.ceil(totalRows / pageSize) || 1);
	const firstVisibleRow = $derived(totalRows === 0 ? 0 : currentPage * pageSize + 1);
	const lastVisibleRow = $derived(Math.min((currentPage + 1) * pageSize, totalRows));
</script>

<div class="flex h-full min-h-0 flex-col">
	<!-- Toolbar -->
	<div class="mb-2 flex h-8 flex-shrink-0 items-center justify-between">
		<div class="flex items-center gap-2">
			<span class="text-[10px] font-bold">Data rows</span>
			<span class="text-muted-foreground text-[9px]">{totalRows.toLocaleString()} total</span>
		</div>
		<div class="flex items-center gap-1">
			{#if onAddFilter}
				<button
					class="rt-toolbar-button border-border h-7 cursor-pointer gap-1.5 px-2.5 text-[10px] font-semibold"
					onclick={onAddFilter}
				>
					<Filter class="h-3 w-3" />
					Filter
				</button>
			{/if}
			{#if !readonly}
				<button
					class="rt-toolbar-button border-border h-7 cursor-pointer gap-1.5 px-2.5 text-[10px] font-semibold"
					onclick={addNewRow}
				>
					<Plus class="h-3 w-3" />
					Add row
				</button>
				<button
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[10px] disabled:pointer-events-none disabled:opacity-40"
					disabled={selectedRowIndex === null}
					onclick={deleteSelectedRow}
				>
					<Trash2 class="h-3 w-3" />
					Delete
				</button>
			{/if}
		</div>
	</div>

	<!-- Data Grid -->
	<div
		class="max-h-[calc(100vh-300px)] min-h-0 flex-1 overflow-auto rounded-lg border bg-[var(--surface-raised)]"
	>
		<table class="w-full caption-bottom text-xs">
			<thead class="[&_tr]:border-b">
				<tr class="border-b">
					<th class="text-muted-foreground h-9 w-8 px-2 text-left align-middle font-medium">#</th>
					{#each columns as col (col.name)}
						<th
							class="text-muted-foreground h-9 px-3 text-left align-middle font-mono text-[10px] font-medium"
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
				{#if loading}
					<tr class="hover:!bg-transparent">
						<td colspan={columns.length + 1} class="h-44 px-6 text-center">
							<div class="mx-auto flex max-w-sm flex-col items-center">
								<span
									class="bg-primary/10 text-primary flex h-10 w-10 items-center justify-center rounded-lg"
								>
									<Loader2 class="h-5 w-5 animate-spin" />
								</span>
								<p class="mt-3 text-[11px] font-bold">{loadingTitle}</p>
								<p class="text-muted-foreground mt-1 text-[9px]">{loadingDescription}</p>
								<div
									class="rt-loading-progress bg-muted mt-4 h-1 w-full max-w-56 overflow-hidden rounded-full"
								></div>
							</div>
						</td>
					</tr>
				{:else if displayData.length === 0}
					<tr>
						<td colspan={columns.length + 1} class="text-muted-foreground h-28 text-center">
							<p class="text-[10px] font-semibold">No rows to display</p>
							<p class="mt-1 text-[9px]">Try adjusting the current filters.</p>
						</td>
					</tr>
				{:else}
					{#each displayData as row, rowIndex (rowIndex)}
						<tr
							class="border-b transition-colors {getRowClass(row, rowIndex)} cursor-pointer"
							onclick={() => selectRow(rowIndex)}
							oncontextmenu={(e) => handleContextMenu(e, row, rowIndex)}
						>
							<td class="text-muted-foreground w-8 p-2 text-center align-middle text-[10px]">
								{currentPage * pageSize + rowIndex + 1}
							</td>
							{#each columns as col (col.name)}
								<td class="p-0 align-middle">
									{#if editingCell?.rowIndex === rowIndex && editingCell?.colName === col.name}
										<input
											class="bg-background focus:ring-primary h-full w-full border-0 px-3 py-2 font-mono text-[11px] outline-none focus:ring-2"
											value={editValue}
											oninput={(e) => (editValue = e.currentTarget.value)}
											onblur={() => saveEdit(row, rowIndex)}
											onkeydown={(e) => handleKeydown(e, row, rowIndex)}
										/>
									{:else}
										<button
											type="button"
											class="hover:bg-accent/70 block w-full px-3 py-2 text-left font-mono text-[11px]"
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
				{/if}
			</tbody>
		</table>
	</div>

	<!-- Context Menu -->
	{#if contextMenuOpen && contextRow && contextRowIndex !== null}
		<div
			class="rt-context-menu fixed z-50"
			style="left: {menuPosition.x}px; top: {menuPosition.y}px;"
			transition:fly={{ duration: 100, y: -5 }}
			role="menu"
			data-context-menu="row"
		>
			<div class="rt-context-header">
				<span class="rt-context-header-icon">
					<Rows3 class="h-3.5 w-3.5" />
				</span>
				<span class="min-w-0">
					<span class="rt-context-title">Row {currentPage * pageSize + contextRowIndex + 1}</span>
					<span class="rt-context-meta">Data row actions</span>
				</span>
			</div>
			<button
				type="button"
				class="rt-context-item"
				onclick={() => {
					navigator.clipboard.writeText(JSON.stringify(contextRow));
					closeContextMenu();
				}}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Copy class="h-3.5 w-3.5" />
				</span>
				<span class="rt-context-label">Copy as JSON</span>
				<span class="text-muted-foreground text-[9px] font-semibold">JSON</span>
			</button>
			{#if !readonly}
				<button
					type="button"
					class="rt-context-item"
					onclick={() => {
						addNewRow();
						closeContextMenu();
					}}
					role="menuitem"
				>
					<span class="rt-context-item-icon">
						<Plus class="h-3.5 w-3.5" />
					</span>
					<span class="rt-context-label">Add new row</span>
				</button>
				<div class="rt-context-divider"></div>
				<button
					type="button"
					class="rt-context-item rt-context-item--danger"
					onclick={() => {
						stageDataDelete(contextRow);
						closeContextMenu();
					}}
					role="menuitem"
				>
					<span class="rt-context-item-icon">
						<Trash2 class="h-3.5 w-3.5" />
					</span>
					<span class="rt-context-label">Delete row</span>
				</button>
			{/if}
		</div>
	{/if}

	<!-- Click outside to close -->
	{#if contextMenuOpen}
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			aria-label="Close row menu"
			onclick={closeContextMenu}
			oncontextmenu={(e) => {
				e.preventDefault();
				closeContextMenu();
			}}
		></button>
	{/if}

	<!-- Pagination -->
	<div class="mt-2 flex h-8 flex-shrink-0 items-center justify-between">
		<span class="text-muted-foreground text-[10px]">
			Showing <span class="text-foreground font-semibold"
				>{firstVisibleRow.toLocaleString()}–{lastVisibleRow.toLocaleString()}</span
			>
			of {totalRows.toLocaleString()}
		</span>
		<div class="flex items-center gap-1">
			<button
				class="rt-toolbar-button h-7 cursor-pointer px-2.5 text-[10px] disabled:pointer-events-none disabled:opacity-40"
				disabled={currentPage === 0}
				onclick={() => onPageChange(currentPage - 1)}
			>
				Previous
			</button>
			<span class="text-muted-foreground px-2 text-[10px]">
				Page <span class="text-foreground font-semibold">{currentPage + 1}</span> / {totalPages}
			</span>
			<button
				class="rt-toolbar-button h-7 cursor-pointer px-2.5 text-[10px] disabled:pointer-events-none disabled:opacity-40"
				disabled={currentPage + 1 >= totalPages}
				onclick={() => onPageChange(currentPage + 1)}
			>
				Next
			</button>
		</div>
	</div>
</div>
