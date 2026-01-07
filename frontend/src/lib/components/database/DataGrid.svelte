<script lang="ts">
	import {
		createTable,
		getCoreRowModel,
		getPaginationRowModel,
		getSortedRowModel,
		type ColumnDef,
		type SortingState
	} from '@tanstack/table-core';
	import * as Table from '$lib/components/ui/table';
	import * as ContextMenu from '$lib/components/ui/context-menu';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import {
		stageDataUpdate,
		stageDataDelete,
		stageDataInsert,
		stagedChanges
	} from '$lib/stores/staged.svelte';
	import { Plus, Trash2, Copy, ArrowUp, ArrowDown, Filter } from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';

	interface Props {
		columns: database.Structure[];
		data: Record<string, any>[];
		totalRows: number;
		currentPage: number;
		pageSize: number;
		onPageChange: (page: number) => void;
		onAddFilter?: () => void;
		readonly?: boolean; // Hide edit/add/delete buttons for query results
	}

	let {
		columns,
		data,
		totalRows,
		currentPage,
		pageSize,
		onPageChange,
		onAddFilter,
		readonly = false
	}: Props = $props();

	// Merge staged added rows with existing data for display
	const displayData = $derived([...stagedChanges.data.added.filter((r: any) => r._isNew), ...data]);

	// Editing state
	let editingCell = $state<{ rowIndex: number; colName: string } | null>(null);
	let editValue = $state<string>('');
	let selectedRowIndex = $state<number | null>(null);

	// Sorting state
	let sorting = $state<SortingState>([]);

	// Get row class based on staged changes
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
		const oldValue = row[colName];

		if (oldValue !== editValue) {
			// Check if this is a staged new row
			if (row._isNew) {
				// Update the staged row in-place
				const stagedIndex = stagedChanges.data.added.findIndex((r: any) => r === row);
				if (stagedIndex >= 0) {
					stagedChanges.data.added[stagedIndex][colName] = editValue;
				}
			} else {
				// It's an existing row from DB - add to updated list
				const updatedRow = { ...row, [colName]: editValue };
				stageDataUpdate(updatedRow);
			}
		}

		editingCell = null;
		editValue = '';
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

	function selectRow(rowIndex: number) {
		selectedRowIndex = selectedRowIndex === rowIndex ? null : rowIndex;
	}

	function addNewRow() {
		// Create a new empty row - leave auto-increment columns empty
		const newRow: Record<string, any> = {};
		columns.forEach((col) => {
			// Skip auto-increment columns (let DB handle them)
			if (col.is_autoinc || col.is_primary) {
				newRow[col.name] = null;
			} else {
				// Only use default if it's a simple value, not an expression
				const defaultVal = col.default;
				if (defaultVal && !defaultVal.includes('(') && !defaultVal.includes('::')) {
					newRow[col.name] = defaultVal;
				} else {
					newRow[col.name] = null;
				}
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

	function copyRowValue(row: Record<string, any>, colName: string) {
		const value = row[colName]?.toString() ?? '';
		navigator.clipboard.writeText(value);
	}

	const totalPages = $derived(Math.ceil(totalRows / pageSize) || 1);
</script>

<div class="flex h-full min-h-0 flex-col">
	<!-- Toolbar -->
	<div class="mb-2 flex flex-shrink-0 items-center justify-between">
		<div class="flex items-center gap-2">
			{#if !readonly}
				<Button variant="outline" size="sm" class="gap-1.5" onclick={addNewRow}>
					<Plus class="h-3.5 w-3.5" />
					Add Row
				</Button>
				<Button
					variant="outline"
					size="sm"
					class="gap-1.5"
					disabled={selectedRowIndex === null}
					onclick={deleteSelectedRow}
				>
					<Trash2 class="h-3.5 w-3.5" />
					Delete
				</Button>
			{/if}
			{#if onAddFilter}
				<Button variant="outline" size="sm" class="gap-1.5" onclick={onAddFilter}>
					<Filter class="h-3.5 w-3.5" />
					Filter
				</Button>
			{/if}
		</div>
		<span class="text-muted-foreground text-sm">
			{totalRows} rows total
		</span>
	</div>

	<!-- Data Grid - scrollable container -->
	<div class="min-h-0 flex-1 overflow-auto rounded-md border">
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head class="w-8">#</Table.Head>
					{#each columns as col (col.name)}
						<Table.Head class="font-mono text-xs">
							<button
								type="button"
								class="hover:text-foreground flex items-center gap-1"
								onclick={() => {
									// Toggle sorting
									const existingSort = sorting.find((s) => s.id === col.name);
									if (existingSort) {
										if (existingSort.desc) {
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
						</Table.Head>
					{/each}
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each displayData as row, rowIndex (rowIndex)}
					<ContextMenu.Root>
						<ContextMenu.Trigger class="contents">
							<Table.Row
								class="{getRowClass(row, rowIndex)} cursor-pointer"
								onclick={() => selectRow(rowIndex)}
							>
								<Table.Cell class="text-muted-foreground text-xs">
									{currentPage * pageSize + rowIndex + 1}
								</Table.Cell>
								{#each columns as col (col.name)}
									<Table.Cell class="p-0">
										{#if editingCell?.rowIndex === rowIndex && editingCell?.colName === col.name}
											<Input
												class="h-8 rounded-none border-0 font-mono text-xs focus-visible:ring-1"
												bind:value={editValue}
												onblur={() => saveEdit(row, rowIndex)}
												onkeydown={(e) => handleKeydown(e, row, rowIndex)}
												autofocus
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
									</Table.Cell>
								{/each}
							</Table.Row>
						</ContextMenu.Trigger>
						<ContextMenu.Content class="w-48">
							<ContextMenu.Item class="" inset={false} onclick={addNewRow}>
								<Plus class="mr-2 h-4 w-4" />
								Add row
							</ContextMenu.Item>
							<ContextMenu.Item class="" inset={false} onclick={() => stageDataDelete(row)}>
								<Trash2 class="mr-2 h-4 w-4" />
								Delete row
							</ContextMenu.Item>
							<ContextMenu.Separator class="" />
							<ContextMenu.Item
								class=""
								inset={false}
								onclick={() => navigator.clipboard.writeText(JSON.stringify(row))}
							>
								<Copy class="mr-2 h-4 w-4" />
								Copy Row as JSON
							</ContextMenu.Item>
						</ContextMenu.Content>
					</ContextMenu.Root>
				{/each}

				{#if displayData.length === 0}
					<Table.Row>
						<Table.Cell colspan={columns.length + 1} class="text-muted-foreground h-24 text-center">
							no data
						</Table.Cell>
					</Table.Row>
				{/if}
			</Table.Body>
		</Table.Root>
	</div>

	<!-- Pagination -->
	<div class="mt-2 flex items-center justify-between">
		<span class="text-muted-foreground text-sm">
			showing {currentPage * pageSize + 1}-{Math.min((currentPage + 1) * pageSize, totalRows)} of {totalRows}
		</span>
		<div class="flex items-center gap-2">
			<Button
				variant="outline"
				size="sm"
				disabled={currentPage === 0}
				onclick={() => onPageChange(currentPage - 1)}
			>
				Previous
			</Button>
			<span class="text-sm">
				page {currentPage + 1} of {totalPages}
			</span>
			<Button
				variant="outline"
				size="sm"
				disabled={currentPage + 1 >= totalPages}
				onclick={() => onPageChange(currentPage + 1)}
			>
				Next
			</Button>
		</div>
	</div>
</div>
