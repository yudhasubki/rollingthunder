<script lang="ts">
	import type { SortingState } from '@tanstack/table-core';
	import {
		stageDataUpdate,
		stageDataDelete,
		stageDataInsert,
		updateStagedInsert,
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
		X,
		Download,
		Columns3
	} from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import DataCellValue from '$lib/components/database/DataCellValue.svelte';
	import RowDetailDrawer from '$lib/components/database/RowDetailDrawer.svelte';
	import { getContextMenuPosition } from '$lib/utils/contextMenu';
	import { getColumnTypeLabel, getDefaultColumnWidth } from '$lib/table/cells';
	import { getForeignRelation } from '$lib/table/relations';
	import { getNextSortingState } from '$lib/table/sorting';
	import { getRowIdentity, STAGED_CHANGED_COLUMNS } from '$lib/table/changes';
	import { updateStatus } from '$lib/stores/status.svelte';
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
		onExport?: (preferredScope?: 'selected') => void;
		onSelectionChange?: (rows: Record<string, any>[], indexes: number[]) => void;
		exporting?: boolean;
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
		onExport,
		onSelectionChange,
		exporting = false,
		detailTitle = 'Table row',
		gridTitle = 'Data rows',
		readonly = false,
		loading = false,
		loadingTitle = 'Loading table data',
		loadingDescription = 'Waiting for the database…'
	}: Props = $props();

	const stagedChanges = $derived(getStagedChanges(tabId));
	const primaryKeyColumns = $derived(
		columns.filter((column) => column.is_primary).map((column) => column.name)
	);
	const stagedChangeCount = $derived(
		stagedChanges.data.added.length +
			stagedChanges.data.updated.length +
			stagedChanges.data.deleted.length
	);

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

	// Merge staged values into the current page so the grid previews what will
	// actually be reviewed and applied.
	const displayData = $derived([
		...stagedChanges.data.added.filter((row: any) => row._isNew),
		...data.map((row) => {
			const identity = getRowIdentity(row, primaryKeyColumns);
			return (
				stagedChanges.data.updated.find(
					(candidate) =>
						identity !== null && getRowIdentity(candidate, primaryKeyColumns) === identity
				) ?? row
			);
		})
	]);

	// Editing state
	let editingCell = $state<{ rowIndex: number; colName: string } | null>(null);
	let editValue = $state<string>('');
	let selectedRowIndex = $state<number | null>(null);
	let exportSelectedRowIndexes = $state<number[]>([]);
	let selectAllCheckbox = $state<HTMLInputElement | null>(null);
	let previousData = data;
	let columnWidths = $state<Record<string, number>>({});
	let resizingColumn = $state<string | null>(null);
	let resizeStartX = 0;
	let resizeStartWidth = 0;
	let resizePointerId: number | null = null;
	let hiddenColumns = $state<Set<string>>(new Set());
	let columnMenuOpen = $state(false);
	let gridSurface: HTMLDivElement | null = null;
	let selectingCells = false;
	let selectionAnchor = $state<{ row: number; column: number } | null>(null);
	let selectionFocus = $state<{ row: number; column: number } | null>(null);

	const rowNumberWidth = 64;
	const actionColumnWidth = 36;
	const visibleColumns = $derived(columns.filter((column) => !hiddenColumns.has(column.name)));
	const selectedCellRange = $derived.by(() => {
		if (!selectionAnchor || !selectionFocus) return null;
		return {
			startRow: Math.min(selectionAnchor.row, selectionFocus.row),
			endRow: Math.max(selectionAnchor.row, selectionFocus.row),
			startColumn: Math.min(selectionAnchor.column, selectionFocus.column),
			endColumn: Math.max(selectionAnchor.column, selectionFocus.column)
		};
	});
	const allDisplayRowsSelected = $derived(
		displayData.length > 0 && exportSelectedRowIndexes.length === displayData.length
	);
	const someDisplayRowsSelected = $derived(
		exportSelectedRowIndexes.length > 0 && !allDisplayRowsSelected
	);
	const selectedRow = $derived(
		selectedRowIndex === null ? null : (displayData[selectedRowIndex] ?? null)
	);
	const canDeleteSelectedRow = $derived(
		Boolean(selectedRow && (selectedRow._isNew || primaryKeyColumns.length > 0))
	);
	const tablePixelWidth = $derived(
		rowNumberWidth +
			actionColumnWidth +
			visibleColumns.reduce((total, column) => total + getColumnWidth(column), 0)
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
		selectionAnchor = null;
		selectionFocus = null;
		updateExportSelection([]);
		closeRowDetails(false);
	});

	$effect(() => {
		if (selectAllCheckbox) {
			selectAllCheckbox.indeterminate = someDisplayRowsSelected;
		}
	});

	function getRowClass(row: Record<string, any>, rowIndex: number): string {
		const identity = getRowIdentity(row, primaryKeyColumns);
		if (row._isNew || stagedChanges.data.added.some((candidate: any) => candidate === row))
			return 'row-added';
		if (
			Array.isArray(row[STAGED_CHANGED_COLUMNS]) ||
			stagedChanges.data.updated.some(
				(candidate) =>
					identity !== null && getRowIdentity(candidate, primaryKeyColumns) === identity
			)
		) {
			return 'row-updated';
		}
		if (
			stagedChanges.data.deleted.some(
				(candidate) =>
					identity !== null && getRowIdentity(candidate, primaryKeyColumns) === identity
			)
		) {
			return 'row-deleted';
		}
		return '';
	}

	function startEdit(row: Record<string, any>, rowIndex: number, colName: string) {
		const column = columns.find((candidate) => candidate.name === colName);
		if (column?.is_generated) {
			updateStatus('Generated columns are computed by the database and cannot be edited.', 'warn');
			return;
		}
		if (!row._isNew && primaryKeyColumns.length === 0) {
			updateStatus(
				'This table has no primary key, so existing rows cannot be edited safely.',
				'warn'
			);
			return;
		}
		if (getRowClass(row, rowIndex) === 'row-deleted') {
			updateStatus('Discard staged changes before editing a deleted row.', 'warn');
			return;
		}
		editingCell = { rowIndex, colName };
		editValue = row[colName]?.toString() ?? '';
	}

	function saveEdit(row: Record<string, any>, rowIndex: number) {
		if (!editingCell) return;

		const { colName } = editingCell;
		const newValue = editValue;

		// Always update the row with new value
		const updatedRow = { ...row, [colName]: newValue };

		// For new rows (_isNew), update the staged insert directly
		if (row._isNew) {
			updateStagedInsert(tabId, row, updatedRow);
		} else {
			// For existing rows, stage as update if value changed
			const oldValue = row[colName];
			if (newValue !== oldValue?.toString()) {
				stageDataUpdate(tabId, row, updatedRow, primaryKeyColumns);
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
			newRow[col.name] = col.is_autoinc || col.is_generated || col.default ? undefined : null;
		});
		updateExportSelection([]);
		stageDataInsert(tabId, newRow);
	}

	function toggleColumnVisibility(columnName: string) {
		const next = new Set(hiddenColumns);
		if (next.has(columnName)) {
			next.delete(columnName);
		} else {
			if (visibleColumns.length <= 1) {
				updateStatus('Keep at least one column visible.', 'warn');
				return;
			}
			next.add(columnName);
		}
		hiddenColumns = next;
		selectionAnchor = null;
		selectionFocus = null;
	}

	function showAllColumns() {
		hiddenColumns = new Set();
	}

	function isCellSelected(row: number, column: number): boolean {
		const range = selectedCellRange;
		return Boolean(
			range &&
				row >= range.startRow &&
				row <= range.endRow &&
				column >= range.startColumn &&
				column <= range.endColumn
		);
	}

	function selectCell(event: PointerEvent, row: number, column: number) {
		if (event.button !== 0) return;
		event.stopPropagation();
		gridSurface?.focus({ preventScroll: true });
		if (event.shiftKey && selectionAnchor) {
			selectionFocus = { row, column };
		} else {
			selectionAnchor = { row, column };
			selectionFocus = { row, column };
		}
		selectedRowIndex = row;
		selectingCells = true;
	}

	function extendCellSelection(row: number, column: number) {
		if (!selectingCells || !selectionAnchor) return;
		selectionFocus = { row, column };
		selectedRowIndex = row;
	}

	function finishCellSelection() {
		selectingCells = false;
	}

	function handleGridKeydown(event: KeyboardEvent) {
		if (editingCell || !selectionFocus || visibleColumns.length === 0 || displayData.length === 0)
			return;
		if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'a') {
			event.preventDefault();
			selectionAnchor = { row: 0, column: 0 };
			selectionFocus = {
				row: displayData.length - 1,
				column: visibleColumns.length - 1
			};
			return;
		}
		const movement: Record<string, [number, number]> = {
			ArrowUp: [-1, 0],
			ArrowDown: [1, 0],
			ArrowLeft: [0, -1],
			ArrowRight: [0, 1]
		};
		const delta = movement[event.key];
		if (delta) {
			event.preventDefault();
			const next = {
				row: Math.max(0, Math.min(displayData.length - 1, selectionFocus.row + delta[0])),
				column: Math.max(0, Math.min(visibleColumns.length - 1, selectionFocus.column + delta[1]))
			};
			if (!event.shiftKey) selectionAnchor = next;
			selectionFocus = next;
			selectedRowIndex = next.row;
			return;
		}
		if (event.key === 'Enter' && !readonly) {
			event.preventDefault();
			const column = visibleColumns[selectionFocus.column];
			const row = displayData[selectionFocus.row];
			if (column && row) startEdit(row, selectionFocus.row, column.name);
		} else if (event.key === 'Escape') {
			selectionAnchor = null;
			selectionFocus = null;
		}
	}

	function clipboardCellValue(value: unknown): string {
		if (value === null || value === undefined) return '';
		const rendered = typeof value === 'object' ? JSON.stringify(value) : String(value);
		return rendered.replaceAll('\t', ' ').replaceAll('\r', ' ').replaceAll('\n', ' ');
	}

	function handleGridCopy(event: ClipboardEvent) {
		if (editingCell || !selectedCellRange || !event.clipboardData) return;
		const range = selectedCellRange;
		const lines: string[] = [];
		for (let rowIndex = range.startRow; rowIndex <= range.endRow; rowIndex++) {
			const row = displayData[rowIndex];
			if (!row) continue;
			const values: string[] = [];
			for (let columnIndex = range.startColumn; columnIndex <= range.endColumn; columnIndex++) {
				const column = visibleColumns[columnIndex];
				values.push(column ? clipboardCellValue(row[column.name]) : '');
			}
			lines.push(values.join('\t'));
		}
		event.preventDefault();
		event.clipboardData.setData('text/plain', lines.join('\n'));
		updateStatus(
			`Copied ${range.endRow - range.startRow + 1} × ${range.endColumn - range.startColumn + 1} cells`,
			'success'
		);
	}

	function createBlankRow(): Record<string, any> {
		const row: Record<string, any> = { _isNew: true };
		for (const column of columns) {
			row[column.name] =
				column.is_autoinc || column.is_generated || column.default ? undefined : null;
		}
		return row;
	}

	function handleGridPaste(event: ClipboardEvent) {
		if (editingCell || !event.clipboardData || !selectionAnchor) return;
		event.preventDefault();
		if (readonly) {
			updateStatus('This result is read-only, so pasted cells were not staged.', 'warn');
			return;
		}
		const text = event.clipboardData.getData('text/plain').replaceAll('\r\n', '\n');
		const lines = text.split('\n');
		if (lines.at(-1) === '') lines.pop();
		const matrix = lines.map((line) => line.split('\t'));
		if (matrix.length === 0) return;
		const start = selectedCellRange
			? { row: selectedCellRange.startRow, column: selectedCellRange.startColumn }
			: selectionAnchor;

		let skippedUnsafeRow = false;
		for (let rowOffset = 0; rowOffset < matrix.length; rowOffset++) {
			const targetRowIndex = start.row + rowOffset;
			const sourceValues = matrix[rowOffset];
			const current = displayData[targetRowIndex];
			if (current && !current._isNew && primaryKeyColumns.length === 0) {
				skippedUnsafeRow = true;
				continue;
			}
			const next = current ? { ...current } : createBlankRow();
			let changed = false;
			for (let columnOffset = 0; columnOffset < sourceValues.length; columnOffset++) {
				const column = visibleColumns[start.column + columnOffset];
				if (!column || column.is_generated) continue;
				next[column.name] = sourceValues[columnOffset];
				changed = true;
			}
			if (!changed) continue;
			if (!current) {
				stageDataInsert(tabId, next);
			} else if (current._isNew) {
				updateStagedInsert(tabId, current, next);
			} else {
				stageDataUpdate(tabId, current, next, primaryKeyColumns);
			}
		}
		selectionFocus = {
			row: start.row + matrix.length - 1,
			column: Math.min(
				visibleColumns.length - 1,
				start.column + Math.max(...matrix.map((row) => row.length)) - 1
			)
		};
		if (skippedUnsafeRow) {
			updateStatus('Existing rows without a primary key were skipped during paste.', 'warn');
			return;
		}
		updateStatus(
			`Pasted ${matrix.length} row${matrix.length === 1 ? '' : 's'} as staged changes`,
			'success'
		);
	}

	function deleteSelectedRow() {
		if (selectedRowIndex !== null && displayData[selectedRowIndex]) {
			const row = displayData[selectedRowIndex];
			if (!row._isNew && primaryKeyColumns.length === 0) {
				updateStatus(
					'This table has no primary key, so existing rows cannot be deleted safely.',
					'warn'
				);
				return;
			}
			stageDataDelete(tabId, row, primaryKeyColumns);
			selectedRowIndex = null;
		}
	}

	function selectRow(rowIndex: number) {
		selectedRowIndex = selectedRowIndex === rowIndex ? null : rowIndex;
	}

	function updateExportSelection(indexes: number[]) {
		exportSelectedRowIndexes = indexes;
		onSelectionChange?.(indexes.map((index) => displayData[index]).filter(Boolean), [...indexes]);
	}

	function toggleExportRow(rowIndex: number) {
		updateExportSelection(
			exportSelectedRowIndexes.includes(rowIndex)
				? exportSelectedRowIndexes.filter((index) => index !== rowIndex)
				: [...exportSelectedRowIndexes, rowIndex].sort((left, right) => left - right)
		);
	}

	function toggleAllDisplayRows() {
		updateExportSelection(allDisplayRowsSelected ? [] : displayData.map((_, index) => index));
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
	onpointerup={(event) => {
		finishColumnResize(event);
		finishCellSelection();
	}}
	onpointercancel={(event) => {
		finishColumnResize(event);
		finishCellSelection();
	}}
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
				{#if stagedChangeCount > 0}
					<span
						class="border-warning-border bg-warning-soft text-warning inline-flex h-6 items-center gap-1.5 rounded-md border px-2 text-[8px] font-semibold"
						title={`${stagedChanges.data.added.length} inserts · ${stagedChanges.data.updated.length} updates · ${stagedChanges.data.deleted.length} deletes`}
					>
						<span class="bg-warning h-1.5 w-1.5 rounded-full"></span>
						{stagedChangeCount} staged
					</span>
				{/if}
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
				{#if onExport}
					<button
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium disabled:pointer-events-none disabled:opacity-40"
						onclick={() => onExport?.(exportSelectedRowIndexes.length > 0 ? 'selected' : undefined)}
						disabled={exporting || loading}
					>
						{#if exporting}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else}
							<Download class="h-3 w-3" />
						{/if}
						Export
						{#if exportSelectedRowIndexes.length > 0}
							<span
								class="bg-primary/10 text-primary inline-flex min-w-4 items-center justify-center rounded px-1 text-[7px] font-bold tabular-nums"
							>
								{exportSelectedRowIndexes.length}
							</span>
						{/if}
					</button>
				{/if}
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
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-medium"
					onclick={() => (columnMenuOpen = !columnMenuOpen)}
					aria-expanded={columnMenuOpen}
					title="Choose visible columns"
				>
					<Columns3 class="h-3 w-3" />
					Columns
					<span class="text-muted-foreground font-mono text-[8px]"
						>{visibleColumns.length}/{columns.length}</span
					>
				</button>
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
						disabled={!canDeleteSelectedRow}
						onclick={deleteSelectedRow}
						title={selectedRow && !selectedRow._isNew && primaryKeyColumns.length === 0
							? 'A primary key is required to delete an existing row'
							: 'Stage selected row for deletion'}
					>
						<Trash2 class="h-3 w-3" />
						Delete
					</button>
				{/if}
			</div>
		</div>

		{#if columnMenuOpen}
			<div class="flex shrink-0 items-center gap-2 overflow-x-auto border-b px-3 py-2">
				<span class="text-muted-foreground shrink-0 text-[8px] font-bold tracking-wide uppercase">
					Visible
				</span>
				{#each columns as column (column.name)}
					<label
						class="flex h-6 shrink-0 cursor-pointer items-center gap-1.5 rounded-md border px-2 text-[8px] font-semibold {hiddenColumns.has(
							column.name
						)
							? 'text-muted-foreground bg-[var(--surface-sunken)]'
							: 'bg-[var(--surface-raised)]'}"
					>
						<input
							type="checkbox"
							class="accent-primary h-3 w-3"
							checked={!hiddenColumns.has(column.name)}
							onchange={() => toggleColumnVisibility(column.name)}
						/>
						<span class="font-mono">{column.name}</span>
					</label>
				{/each}
				{#if hiddenColumns.size > 0}
					<button
						type="button"
						class="rt-toolbar-button h-6 shrink-0 px-2 text-[8px] font-semibold"
						onclick={showAllColumns}
					>
						Show all
					</button>
				{/if}
			</div>
		{/if}

		<!-- Data Grid -->
		<div
			bind:this={gridSurface}
			class="data-grid-scroll min-h-0 flex-1 overflow-auto outline-none"
			tabindex="0"
			role="grid"
			aria-label={`${gridTitle}. Use arrow keys to move, Shift plus arrows to select, and copy or paste tab-separated cells.`}
			onkeydown={handleGridKeydown}
			oncopy={handleGridCopy}
			onpaste={handleGridPaste}
		>
			<table
				class="data-grid-table caption-bottom border-separate border-spacing-0 text-xs"
				style="width: max(100%, {tablePixelWidth}px); table-layout: fixed;"
			>
				<thead>
					<tr>
						<th
							class="column-header-cell frozen-edge frozen-edge--left row-number-cell text-muted-foreground h-9 border-r border-b px-0 text-center align-middle font-mono text-[9px] font-semibold"
							style="width: {rowNumberWidth}px; min-width: {rowNumberWidth}px; max-width: {rowNumberWidth}px;"
							aria-label="Select rows and row number"
						>
							<span class="flex h-full items-center justify-center gap-2 px-2">
								<input
									bind:this={selectAllCheckbox}
									type="checkbox"
									class="accent-primary h-3.5 w-3.5 cursor-pointer"
									checked={allDisplayRowsSelected}
									disabled={displayData.length === 0 || loading}
									onchange={toggleAllDisplayRows}
									aria-label={allDisplayRowsSelected
										? 'Clear selected rows'
										: 'Select all rows on this page'}
								/>
								<span aria-hidden="true">#</span>
							</span>
						</th>
						{#each visibleColumns as col (col.name)}
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
							<td colspan={visibleColumns.length + 2} class="h-44 px-6 text-center">
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
							<td
								colspan={visibleColumns.length + 2}
								class="text-muted-foreground h-32 text-center"
							>
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
								data-export-selected={exportSelectedRowIndexes.includes(rowIndex)}
								aria-selected={selectedRowIndex === rowIndex}
								onclick={() => selectRow(rowIndex)}
								oncontextmenu={(event) => handleContextMenu(event, row, rowIndex)}
							>
								<td
									class="frozen-edge frozen-edge--left row-number-cell text-muted-foreground relative h-10 border-r border-b p-0 text-center align-middle text-[9px]"
									style="width: {rowNumberWidth}px; min-width: {rowNumberWidth}px; max-width: {rowNumberWidth}px;"
								>
									{#if selectedRowIndex === rowIndex}
										<span
											class="bg-primary absolute inset-y-1.5 left-0 w-0.5 rounded-r"
											aria-hidden="true"
										></span>
									{/if}
									<span class="flex h-full items-center justify-center gap-1 px-1">
										<input
											type="checkbox"
											class="accent-primary h-3.5 w-3.5 shrink-0 cursor-pointer"
											checked={exportSelectedRowIndexes.includes(rowIndex)}
											onclick={(event) => event.stopPropagation()}
											onchange={() => toggleExportRow(rowIndex)}
											aria-label={`Select row ${currentPage * pageSize + rowIndex + 1} for export`}
										/>
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
									</span>
								</td>
								{#each visibleColumns as col, columnIndex (col.name)}
									<td
										class="h-10 overflow-hidden border-r border-b p-0 align-middle {isCellSelected(
											rowIndex,
											columnIndex
										)
											? 'cell-selected'
											: ''}"
										style="width: {getColumnWidth(col)}px;"
										onpointerenter={() => extendCellSelection(rowIndex, columnIndex)}
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
												class="hover:bg-accent/45 flex h-10 w-full min-w-0 items-center overflow-hidden px-3 text-left transition-colors {isCellSelected(
													rowIndex,
													columnIndex
												)
													? 'bg-accent/40'
													: ''}"
												onpointerdown={(event) => selectCell(event, rowIndex, columnIndex)}
												onclick={(event) => event.stopPropagation()}
												ondblclick={() =>
													!readonly && !col.is_generated && startEdit(row, rowIndex, col.name)}
												title={readonly
													? 'Read-only result'
													: col.is_generated
														? 'Generated by the database'
														: !row._isNew && primaryKeyColumns.length === 0
															? 'A primary key is required to edit this row safely'
															: getRowClass(row, rowIndex) === 'row-deleted'
																? 'This row is staged for deletion'
																: 'Double-click to edit'}
											>
												{#if row._isNew && row[col.name] === undefined}
													<span
														class="text-muted-foreground inline-flex rounded border border-dashed px-1.5 py-0.5 font-mono text-[8px] italic"
														title="The database will generate this value"
													>
														DEFAULT
													</span>
												{:else}
													<DataCellValue value={row[col.name]} dataType={getColumnTypeLabel(col)} />
												{/if}
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
						stageDataDelete(tabId, contextRow, primaryKeyColumns);
						closeContextMenu();
					}}
					disabled={!contextRow._isNew && primaryKeyColumns.length === 0}
					title={!contextRow._isNew && primaryKeyColumns.length === 0
						? 'A primary key is required to delete this row safely'
						: 'Stage row for deletion'}
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

	.data-grid-table td.cell-selected {
		box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--primary) 72%, transparent);
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
		background: color-mix(in oklab, var(--success) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.row-updated .frozen-edge {
		background: color-mix(in oklab, var(--warning) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.row-deleted .frozen-edge {
		background: color-mix(in oklab, var(--danger) 10%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row:hover {
		background: var(--surface-hover);
	}

	.data-grid-table tbody tr.data-row:hover .frozen-edge {
		background: var(--surface-hover);
	}

	.data-grid-table tbody tr.data-row[data-export-selected='true'] {
		background: color-mix(in srgb, var(--primary) 4%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row[data-export-selected='true'] .frozen-edge {
		background: color-mix(in srgb, var(--primary) 4%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row[data-selected='true'] {
		background: color-mix(in srgb, var(--primary) 8%, var(--surface-raised));
	}

	.data-grid-table tbody tr.data-row[data-selected='true'] .frozen-edge {
		background: color-mix(in srgb, var(--primary) 8%, var(--surface-raised));
	}
</style>
