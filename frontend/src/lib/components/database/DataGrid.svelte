<script lang="ts">
	import type { SortingState } from '@tanstack/table-core';
	import {
		stageDataUpdate,
		stageDataDelete,
		stageDataInsert,
		getStagedChanges
	} from '$lib/stores/staged.svelte';
	import {
		Plus,
		Trash2,
		Copy,
		ArrowUp,
		ArrowDown,
		ArrowUpDown,
		ChevronLeft,
		ChevronRight,
		Filter,
		Loader2,
		PanelRightOpen,
		Rows3,
		X
	} from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import DataCellValue from '$lib/components/database/DataCellValue.svelte';
	import RowDetailDrawer from '$lib/components/database/RowDetailDrawer.svelte';
	import { getContextMenuPosition } from '$lib/utils/contextMenu';
	import { getColumnTypeLabel, getDefaultColumnWidth } from '$lib/table/cells';
	import { getForeignRelation } from '$lib/table/relations';
	import { getNextSortingState } from '$lib/table/sorting';
	import { fly } from 'svelte/transition';

	interface Props {
		tabId: string;
		columns: database.Structure[];
		data: Record<string, any>[];
		totalRows: number;
		currentPage: number;
		pageSize: number;
		onPageChange: (page: number) => void;
		sorting?: SortingState;
		onSortingChange?: (sorting: SortingState) => void;
		onAddFilter?: () => void;
		detailTitle?: string;
		gridTitle?: string;
		readonly?: boolean;
		loading?: boolean;
		loadingTitle?: string;
		loadingDescription?: string;
	}

	let {
		tabId,
		columns,
		data,
		totalRows,
		currentPage,
		pageSize,
		onPageChange,
		sorting = [],
		onSortingChange,
		onAddFilter,
		detailTitle = 'Table row',
		gridTitle = 'Data rows',
		readonly = false,
		loading = false,
		loadingTitle = 'Loading table data',
		loadingDescription = 'Waiting for the database…'
	}: Props = $props();

	const stagedChanges = $derived(getStagedChanges(tabId));

	// Track menu position for manual positioning
	let menuPosition = $state({ x: 0, y: 0 });
	let contextMenuOpen = $state(false);

	// Track which row is being right-clicked
	let contextRow = $state<Record<string, any> | null>(null);
	let contextRowIndex = $state<number | null>(null);
	let detailOpen = $state(false);
	let detailRow = $state<Record<string, any> | null>(null);
	let detailRowIndex = $state<number | null>(null);
	let detailTrigger: HTMLElement | null = null;

	// Merge staged added rows with existing data for display
	const displayData = $derived([...stagedChanges.data.added.filter((r: any) => r._isNew), ...data]);

	// Editing state
	let editingCell = $state<{ rowIndex: number; colName: string } | null>(null);
	let editValue = $state<string>('');
	let selectedRowIndex = $state<number | null>(null);
	let previousData = data;
	let columnWidths = $state<Record<string, number>>({});
	let resizingColumn = $state<string | null>(null);
	let resizeStartX = 0;
	let resizeStartWidth = 0;
	let resizePointerId: number | null = null;

	const rowNumberWidth = 40;
	const actionColumnWidth = 36;
	const tablePixelWidth = $derived(
		rowNumberWidth +
			actionColumnWidth +
			columns.reduce((total, column) => total + getColumnWidth(column), 0)
	);

	function handleSortClick(event: MouseEvent, column: string) {
		if (!onSortingChange) return;
		onSortingChange(getNextSortingState(sorting, column, event.shiftKey));
	}

	function getSortIndex(column: string): number {
		return sorting.findIndex((sort) => sort.id === column);
	}

	function getColumnWidth(column: database.Structure): number {
		return (
			columnWidths[column.name] ?? getDefaultColumnWidth(column.name, getColumnTypeLabel(column))
		);
	}

	function getColumnMetadataTitle(column: database.Structure): string {
		const relation = getForeignRelation(column);
		const metadata = [getColumnTypeLabel(column)];

		if (column.is_primary) metadata.push('Primary key');
		if (relation) {
			const target = `${relation.schema ? `${relation.schema}.` : ''}${relation.table}${
				relation.column ? `.${relation.column}` : ''
			}`;
			metadata.push(`Foreign key → ${target}`);
		}
		if (!column.nullable) metadata.push('Required');

		return `${column.name}\n${metadata.join(' · ')}`;
	}

	function setColumnWidth(column: database.Structure, width: number) {
		columnWidths = {
			...columnWidths,
			[column.name]: Math.min(560, Math.max(96, Math.round(width)))
		};
	}

	function startColumnResize(event: PointerEvent, column: database.Structure) {
		event.preventDefault();
		event.stopPropagation();
		resizingColumn = column.name;
		resizeStartX = event.clientX;
		resizeStartWidth = getColumnWidth(column);
		resizePointerId = event.pointerId;
		(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId);
	}

	function handleResizePointerMove(event: PointerEvent) {
		if (!resizingColumn || event.pointerId !== resizePointerId) return;
		const column = columns.find((candidate) => candidate.name === resizingColumn);
		if (!column) return;
		setColumnWidth(column, resizeStartWidth + event.clientX - resizeStartX);
	}

	function finishColumnResize(event: PointerEvent) {
		if (event.pointerId !== resizePointerId) return;
		resizingColumn = null;
		resizePointerId = null;
	}

	function resetColumnWidth(event: MouseEvent, column: database.Structure) {
		event.preventDefault();
		event.stopPropagation();
		const nextWidths = { ...columnWidths };
		delete nextWidths[column.name];
		columnWidths = nextWidths;
	}

	function handleResizeKeydown(event: KeyboardEvent, column: database.Structure) {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
		event.preventDefault();
		event.stopPropagation();
		setColumnWidth(column, getColumnWidth(column) + (event.key === 'ArrowRight' ? 16 : -16));
	}

	$effect(() => {
		const currentData = data;
		if (currentData === previousData) return;

		previousData = currentData;
		selectedRowIndex = null;
		closeRowDetails(false);
	});

	function getRowClass(row: Record<string, any>, rowIndex: number): string {
		const rowId = row.id ?? row._id ?? rowIndex;
		if (
			row._isNew ||
			stagedChanges.data.added.some(
				(candidate: any) =>
					candidate === row || (!candidate._isNew && (candidate.id ?? candidate._id) === rowId)
			)
		)
			return 'row-added';
		if (stagedChanges.data.updated.some((r: any) => r.id === rowId)) {
			return 'row-updated';
		}
		if (stagedChanges.data.deleted.some((r: any) => r.id === rowId)) {
			return 'row-deleted';
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
				stageDataUpdate(tabId, updatedRow);
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
		stageDataInsert(tabId, newRow);
	}

	function deleteSelectedRow() {
		if (selectedRowIndex !== null && displayData[selectedRowIndex]) {
			stageDataDelete(tabId, displayData[selectedRowIndex]);
			selectedRowIndex = null;
		}
	}

	function selectRow(rowIndex: number) {
		selectedRowIndex = selectedRowIndex === rowIndex ? null : rowIndex;
	}

	function openRowDetails(
		row: Record<string, any>,
		rowIndex: number,
		trigger?: HTMLElement | null
	) {
		detailRow = row;
		detailRowIndex = rowIndex;
		detailTrigger = trigger || null;
		detailOpen = true;
		closeContextMenu();
	}

	function openSelectedRowDetails(trigger: HTMLElement) {
		if (selectedRowIndex === null || !displayData[selectedRowIndex]) return;
		openRowDetails(displayData[selectedRowIndex], selectedRowIndex, trigger);
	}

	function closeRowDetails(restoreFocus = true) {
		detailOpen = false;
		detailRow = null;
		detailRowIndex = null;

		const trigger = detailTrigger;
		detailTrigger = null;
		if (restoreFocus && trigger?.isConnected) {
			requestAnimationFrame(() => trigger.focus());
		}
	}

	function handleContextMenu(e: MouseEvent, row: Record<string, any>, rowIndex: number) {
		e.preventDefault();
		contextRow = row;
		contextRowIndex = rowIndex;
		selectedRowIndex = rowIndex;
		menuPosition = getContextMenuPosition(e, 236, readonly ? 164 : 248);
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

<svelte:window
	onpointermove={handleResizePointerMove}
	onpointerup={finishColumnResize}
	onpointercancel={finishColumnResize}
/>

<div class="flex h-full min-h-0 flex-col">
	<div
		class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border bg-[var(--surface-raised)] shadow-[0_1px_2px_rgba(0,0,0,0.03)]"
	>
		<!-- Toolbar -->
		<div
			class="flex min-h-12 flex-shrink-0 items-center justify-between gap-4 overflow-x-auto border-b px-3"
		>
			<div class="flex shrink-0 items-center gap-2.5">
				<span
					class="bg-muted text-muted-foreground flex h-7 w-7 items-center justify-center rounded-md"
				>
					<Rows3 class="h-3.5 w-3.5" />
				</span>
				<span class="flex flex-col">
					<span class="text-[10px] leading-tight font-semibold">{gridTitle}</span>
					<span class="text-muted-foreground mt-0.5 text-[8px] leading-tight"
						>{totalRows.toLocaleString()} rows</span
					>
				</span>
				{#if onSortingChange && sorting.length > 0}
					<span
						class="bg-muted/70 text-muted-foreground inline-flex h-6 max-w-52 items-center gap-1.5 rounded-md px-2 text-[8px] font-medium"
						title={sorting
							.map((sort, index) => `${index + 1}. ${sort.id} ${sort.desc ? 'DESC' : 'ASC'}`)
							.join('\n')}
					>
						<span class="truncate">
							{sorting.length === 1
								? `${sorting[0].id} · ${sorting[0].desc ? 'DESC' : 'ASC'}`
								: `${sorting.length} sort levels`}
						</span>
						<button
							type="button"
							class="hover:text-foreground -mr-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded"
							onclick={() => onSortingChange?.([])}
							title="Clear sorting"
							aria-label="Clear sorting"
						>
							<X class="h-2.5 w-2.5" />
						</button>
					</span>
				{/if}
			</div>

			<div class="flex shrink-0 items-center gap-1">
				{#if onAddFilter}
					<button
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium"
						onclick={onAddFilter}
					>
						<Filter class="h-3 w-3" />
						Filter
					</button>
				{/if}
				<button
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium disabled:pointer-events-none disabled:opacity-40"
					disabled={selectedRowIndex === null}
					onclick={(event) => openSelectedRowDetails(event.currentTarget)}
					title={selectedRowIndex === null ? 'Select a row to inspect it' : 'Open row details'}
				>
					<PanelRightOpen class="h-3 w-3" />
					Details
				</button>
				{#if !readonly}
					<button
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium"
						onclick={addNewRow}
					>
						<Plus class="h-3 w-3" />
						Add row
					</button>
					<button
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium disabled:pointer-events-none disabled:opacity-40"
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
		<div class="data-grid-scroll min-h-0 flex-1 overflow-auto">
			<table
				class="data-grid-table caption-bottom border-separate border-spacing-0 text-xs"
				style="width: max(100%, {tablePixelWidth}px); table-layout: fixed;"
			>
				<thead>
					<tr>
						<th
							class="column-header-cell frozen-edge frozen-edge--left row-number-cell text-muted-foreground h-9 border-r border-b px-0 text-center align-middle font-mono text-[9px] font-semibold"
							style="width: {rowNumberWidth}px; min-width: {rowNumberWidth}px; max-width: {rowNumberWidth}px;"
							aria-label="Row number"
						>
							#
						</th>
						{#each columns as col (col.name)}
							{@const sortIndex = getSortIndex(col.name)}
							<th
								class="column-header-cell text-muted-foreground relative h-9 border-r border-b p-0 text-left align-middle tracking-normal normal-case"
								style="width: {getColumnWidth(col)}px;"
								aria-sort={sortIndex < 0
									? 'none'
									: sorting[sortIndex]?.desc
										? 'descending'
										: 'ascending'}
							>
								<div class="flex h-full min-w-0 items-center px-3 pr-4">
									{#if onSortingChange}
										<button
											type="button"
											class="group/sort flex h-full min-w-0 flex-1 items-center gap-1.5 text-left"
											onclick={(event) => handleSortClick(event, col.name)}
											title={`${getColumnMetadataTitle(col)}\nClick to sort · Shift-click to add another column`}
											aria-label={`Sort by ${col.name}`}
											aria-pressed={sortIndex >= 0}
										>
											<span
												class="text-foreground min-w-0 truncate font-mono text-[9px] font-semibold"
											>
												{col.name}
											</span>
											{#if sortIndex >= 0}
												{#if sorting[sortIndex]?.desc}
													<ArrowDown class="text-primary h-3 w-3 shrink-0" />
												{:else}
													<ArrowUp class="text-primary h-3 w-3 shrink-0" />
												{/if}
												{#if sorting.length > 1}
													<span
														class="bg-primary/10 text-primary inline-flex h-3.5 min-w-3.5 shrink-0 items-center justify-center rounded px-1 font-sans text-[7px] font-semibold"
													>
														{sortIndex + 1}
													</span>
												{/if}
											{:else}
												<ArrowUpDown
													class="h-3 w-3 shrink-0 opacity-0 transition-opacity group-hover/sort:opacity-35"
												/>
											{/if}
										</button>
									{:else}
										<span
											class="text-foreground min-w-0 flex-1 truncate font-mono text-[9px] font-semibold"
											title={getColumnMetadataTitle(col)}
										>
											{col.name}
										</span>
									{/if}
								</div>
								<button
									type="button"
									class="hover:bg-primary/35 focus:bg-primary/45 absolute inset-y-1 right-0 z-10 w-1.5 cursor-col-resize rounded-full transition-colors focus:outline-none {resizingColumn ===
									col.name
										? 'bg-primary/55'
										: 'bg-transparent'}"
									onpointerdown={(event) => startColumnResize(event, col)}
									ondblclick={(event) => resetColumnWidth(event, col)}
									onkeydown={(event) => handleResizeKeydown(event, col)}
									aria-label={`Resize ${col.name} column`}
									title="Drag to resize · Double-click to reset"
								></button>
							</th>
						{/each}
						<th
							class="column-header-cell frozen-edge frozen-edge--right detail-action-cell text-muted-foreground h-9 border-b px-0 text-center align-middle"
							style="width: {actionColumnWidth}px; min-width: {actionColumnWidth}px; max-width: {actionColumnWidth}px;"
							aria-label="Row actions"
						>
							<PanelRightOpen class="mx-auto h-3 w-3 opacity-65" />
						</th>
					</tr>
				</thead>
				<tbody>
					{#if loading}
						<tr class="hover:!bg-transparent">
							<td colspan={columns.length + 2} class="h-44 px-6 text-center">
								<div class="mx-auto flex max-w-sm flex-col items-center">
									<span
										class="bg-primary/10 text-primary flex h-10 w-10 items-center justify-center rounded-lg"
									>
										<Loader2 class="h-5 w-5 animate-spin" />
									</span>
									<p class="mt-3 text-[11px] font-semibold">{loadingTitle}</p>
									<p class="text-muted-foreground mt-1 text-[9px]">{loadingDescription}</p>
									<div
										class="rt-loading-progress bg-muted mt-4 h-1 w-full max-w-56 overflow-hidden rounded-full"
									></div>
								</div>
							</td>
						</tr>
					{:else if displayData.length === 0}
						<tr>
							<td colspan={columns.length + 2} class="text-muted-foreground h-32 text-center">
								<div class="mx-auto flex max-w-xs flex-col items-center">
									<span
										class="bg-muted flex h-8 w-8 items-center justify-center rounded-md opacity-80"
									>
										<Rows3 class="h-3.5 w-3.5" />
									</span>
									<p class="text-foreground mt-2.5 text-[10px] font-semibold">No rows to display</p>
									<p class="mt-1 text-[8px]">The current result set is empty.</p>
								</div>
							</td>
						</tr>
					{:else}
						{#each displayData as row, rowIndex (rowIndex)}
							<tr
								class="data-row group cursor-pointer transition-colors {getRowClass(row, rowIndex)}"
								data-selected={selectedRowIndex === rowIndex}
								aria-selected={selectedRowIndex === rowIndex}
								onclick={() => selectRow(rowIndex)}
								oncontextmenu={(event) => handleContextMenu(event, row, rowIndex)}
							>
								<td
									class="frozen-edge frozen-edge--left row-number-cell text-muted-foreground h-10 border-r border-b p-0 text-center align-middle text-[9px]"
									style="width: {rowNumberWidth}px; min-width: {rowNumberWidth}px; max-width: {rowNumberWidth}px;"
								>
									{#if selectedRowIndex === rowIndex}
										<span
											class="bg-primary absolute inset-y-1.5 left-0 w-0.5 rounded-r"
											aria-hidden="true"
										></span>
									{/if}
									<button
										type="button"
										class="hover:bg-accent hover:text-foreground inline-flex h-6 min-w-6 items-center justify-center rounded px-1 font-mono tabular-nums transition-colors"
										onclick={(event) => {
											event.stopPropagation();
											selectedRowIndex = rowIndex;
											openRowDetails(row, rowIndex, event.currentTarget);
										}}
										title="Open row details"
										aria-label={`Open details for row ${currentPage * pageSize + rowIndex + 1}`}
									>
										{currentPage * pageSize + rowIndex + 1}
									</button>
								</td>
								{#each columns as col (col.name)}
									<td
										class="h-10 overflow-hidden border-r border-b p-0 align-middle"
										style="width: {getColumnWidth(col)}px;"
									>
										{#if editingCell?.rowIndex === rowIndex && editingCell?.colName === col.name}
											<input
												class="bg-background focus:ring-primary h-10 w-full border-0 px-3 font-mono text-[10px] outline-none focus:ring-1 focus:ring-inset"
												value={editValue}
												oninput={(event) => (editValue = event.currentTarget.value)}
												onblur={() => saveEdit(row, rowIndex)}
												onkeydown={(event) => handleKeydown(event, row, rowIndex)}
											/>
										{:else}
											<button
												type="button"
												class="hover:bg-accent/45 flex h-10 w-full min-w-0 items-center overflow-hidden px-3 text-left transition-colors"
												ondblclick={() => !readonly && startEdit(rowIndex, col.name, row[col.name])}
												title={readonly ? 'Read-only result' : 'Double-click to edit'}
											>
												<DataCellValue value={row[col.name]} dataType={getColumnTypeLabel(col)} />
											</button>
										{/if}
									</td>
								{/each}
								<td
									class="frozen-edge frozen-edge--right detail-action-cell h-10 border-b p-0 text-center align-middle"
									style="width: {actionColumnWidth}px; min-width: {actionColumnWidth}px; max-width: {actionColumnWidth}px;"
								>
									<button
										type="button"
										class="hover:bg-accent hover:text-foreground inline-flex h-6 w-6 items-center justify-center rounded transition-all {selectedRowIndex ===
										rowIndex
											? 'opacity-100'
											: 'opacity-35 group-hover:opacity-100 focus:opacity-100'}"
										onclick={(event) => {
											event.stopPropagation();
											selectedRowIndex = rowIndex;
											openRowDetails(row, rowIndex, event.currentTarget);
										}}
										title="Open row details"
										aria-label={`Open details for row ${currentPage * pageSize + rowIndex + 1}`}
									>
										<PanelRightOpen class="h-3 w-3" />
									</button>
								</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			</table>
		</div>

		<!-- Pagination -->
		<div class="flex min-h-10 flex-shrink-0 items-center justify-between gap-3 border-t px-3">
			<span class="text-muted-foreground text-[8px]">
				Showing
				<span class="text-foreground font-medium tabular-nums"
					>{firstVisibleRow.toLocaleString()}–{lastVisibleRow.toLocaleString()}</span
				>
				of <span class="tabular-nums">{totalRows.toLocaleString()}</span>
			</span>
			<div class="flex items-center gap-1">
				<span class="text-muted-foreground mr-1 text-[8px] tabular-nums">
					Page <span class="text-foreground font-medium">{currentPage + 1}</span> of {totalPages}
				</span>
				<button
					type="button"
					class="rt-toolbar-button h-6 w-6 cursor-pointer p-0 disabled:pointer-events-none disabled:opacity-35"
					disabled={currentPage === 0}
					onclick={() => onPageChange(currentPage - 1)}
					title="Previous page"
					aria-label="Previous page"
				>
					<ChevronLeft class="h-3 w-3" />
				</button>
				<button
					type="button"
					class="rt-toolbar-button h-6 w-6 cursor-pointer p-0 disabled:pointer-events-none disabled:opacity-35"
					disabled={currentPage + 1 >= totalPages}
					onclick={() => onPageChange(currentPage + 1)}
					title="Next page"
					aria-label="Next page"
				>
					<ChevronRight class="h-3 w-3" />
				</button>
			</div>
		</div>
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
				onclick={(event) => openRowDetails(contextRow, contextRowIndex, event.currentTarget)}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<PanelRightOpen class="h-3.5 w-3.5" />
				</span>
				<span class="rt-context-label">View row details</span>
				<span class="text-muted-foreground text-[9px] font-semibold">Drawer</span>
			</button>
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
						stageDataDelete(tabId, contextRow);
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

	<RowDetailDrawer
		open={detailOpen}
		row={detailRow}
		{columns}
		rowNumber={detailRowIndex === null ? null : currentPage * pageSize + detailRowIndex + 1}
		title={detailTitle}
		onClose={closeRowDetails}
	/>
</div>

<style>
	.data-grid-scroll {
		isolation: isolate;
		scrollbar-gutter: stable;
	}

	.data-grid-table th {
		letter-spacing: normal;
		text-transform: none;
	}

	.data-grid-table thead {
		position: -webkit-sticky;
		position: sticky;
		top: 0;
		z-index: 30;
		background: color-mix(in oklab, var(--surface-sunken) 62%, var(--surface-raised));
	}

	.data-grid-table .column-header-cell {
		background: color-mix(in oklab, var(--surface-sunken) 62%, var(--surface-raised));
	}

	.data-grid-table .frozen-edge {
		position: -webkit-sticky;
		position: sticky;
		background: var(--surface-raised);
		background-clip: padding-box;
	}

	.data-grid-table .frozen-edge--left {
		left: 0;
		box-shadow:
			1px 0 0 var(--border),
			8px 0 14px -13px rgb(0 0 0 / 55%);
	}

	.data-grid-table .frozen-edge--right {
		right: 0;
		box-shadow:
			-1px 0 0 var(--border),
			-8px 0 14px -13px rgb(0 0 0 / 55%);
	}

	.data-grid-table thead .frozen-edge {
		z-index: 2;
		background: color-mix(in oklab, var(--surface-sunken) 62%, var(--surface-raised));
	}

	.data-grid-table tbody .frozen-edge {
		z-index: 3;
	}

	.data-grid-table tbody tr.data-row:not(.row-added):not(.row-updated):not(.row-deleted) {
		background: var(--surface-raised);
	}

	.data-grid-table
		tbody
		tr.data-row:not(.row-added):not(.row-updated):not(.row-deleted)
		.frozen-edge {
		background: var(--surface-raised);
	}

	.data-grid-table tbody tr.row-added .frozen-edge {
		background: color-mix(in oklab, var(--color-green-500) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.row-updated .frozen-edge {
		background: color-mix(in oklab, var(--color-yellow-500) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.row-deleted .frozen-edge {
		background: color-mix(in oklab, var(--color-red-500) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row:hover {
		background: var(--surface-hover);
	}

	.data-grid-table tbody tr.data-row:hover .frozen-edge {
		background: var(--surface-hover);
	}

	.data-grid-table tbody tr.data-row[data-selected='true'] {
		background: color-mix(in srgb, var(--primary) 6%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row[data-selected='true'] .frozen-edge {
		background: color-mix(in srgb, var(--primary) 6%, var(--surface-raised));
	}
</style>
