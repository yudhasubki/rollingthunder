<script lang="ts">
	import { onDestroy } from 'svelte';
	import { writable } from 'svelte/store';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import type { Tab } from '$lib/models/Tab';
	import { createTabs, melt } from '@melt-ui/svelte';
	import type { SortingState } from '@tanstack/table-core';
	import DataGrid from '$lib/components/database/DataGrid.svelte';
	import ExportDialog from '$lib/components/database/ExportDialog.svelte';
	import ObjectChangeDialog from '$lib/components/database/ObjectChangeDialog.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import {
		buildExportOptions,
		formatExportBytes,
		getExportExtension,
		type ExportScope,
		type ExportSettings
	} from '$lib/export/options';
	import {
		cancelExportJob,
		createInitialExportProgress,
		startExportProgressPolling
	} from '$lib/export/progress';
	import { database } from '$lib/wailsjs/go/models';
	import { getColumnTypeLabel } from '$lib/table/cells';
	import { getForeignRelation } from '$lib/table/relations';
	import {
		buildDatabaseFilters,
		filterNeedsValue,
		FILTER_OPERATORS,
		type FilterCondition,
		type FilterOperator
	} from '$lib/table/filters';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import {
		CountCollectionData,
		GetCollectionData,
		GetCollectionStructures,
		GetIndices,
		GetTableDDL,
		ExportQueryResults,
		ExportTableData,
		GetCapabilities
	} from '$lib/wailsjs/go/db/Service';
	import {
		LayoutGrid,
		Table2,
		Plus,
		Filter,
		Code,
		Loader2,
		KeyRound,
		Link2,
		X,
		Settings2,
		Trash2
	} from 'lucide-svelte';
	import type { StructuralChangeIntent } from '$lib/database/changeTemplates';

	interface Props {
		tab: Tab;
	}

	let { tab }: Props = $props();

	let columns = $state<database.Structure[]>([]);
	let indices = $state<database.Index[]>([]);
	let tableTotalData = $state<number>(0);
	let tableData = $state<Record<string, any>[]>([]);
	let isLoadingData = $state(false);
	let exportDialogOpen = $state(false);
	let exporting = $state(false);
	let exportCancelling = $state(false);
	let exportProgress = $state<database.ExportProgress | null>(null);
	let exportJobID = $state('');
	let stopExportProgressPolling: (() => void) | null = null;
	let exportInitialScope = $state<ExportScope>('page');
	let selectedRows = $state<Record<string, any>[]>([]);
	let selectedRowIndexes = $state<number[]>([]);
	let dataLoadingTitle = $state('Preparing table data');
	let dataLoadingDescription = $state('Waiting for the database');
	let isLoadingStructure = $state(false);
	let filters = $state<FilterCondition[]>([]);
	let appliedFilters = $state<FilterCondition[]>([]);
	let sorting = $state<SortingState>([]);
	let capabilities = $state<database.Capabilities | null>(null);
	let changeIntent = $state<StructuralChangeIntent | null>(null);
	let changeReference = $state<database.ObjectReference | null>(null);
	const writeBlocked = $derived(
		Boolean(
			connectionStore.connections.find((connection) => connection.id === tab.connectionId)
				?.readOnly &&
				!connectionStore.connections.find((connection) => connection.id === tab.connectionId)
					?.writeUnlocked
		)
	);
	const primaryKeyCount = $derived(columns.filter((column) => column.is_primary).length);
	const relationCount = $derived(
		columns.filter((column) => getColumnRelation(column) !== null).length
	);
	const nullableCount = $derived(columns.filter((column) => column.nullable).length);

	// DDL state
	let tableDDL = $state<string>('');
	let isLoadingDDL = $state(false);

	// Track last loaded state to prevent duplicate loads
	let lastLoadKey = '';
	let activeTableKey = '';
	let dataRequestVersion = 0;

	const tableLimit = 100;
	let currentPage = $state(0);

	// Melt-UI Tabs
	const tableTabValueStore = writable(tab.activeSubTab || 'structure');
	const {
		elements: { root: tabsRoot, list: tabsList, trigger: tabTrigger, content: tabContent },
		states: { value: tabValue }
	} = createTabs({
		value: tableTabValueStore,
		autoSet: false,
		defaultValue: tab.activeSubTab || 'structure',
		onValueChange: ({ next }) => {
			if (next === 'structure' || next === 'data' || next === 'ddl') {
				tabsStore.updateTab(tab.id, { activeSubTab: next });
			}
			return next;
		}
	});

	$effect(() => {
		tableTabValueStore.set(tab.activeSubTab || 'structure');
	});

	// Filter management functions
	function addFilter() {
		const firstCol = columns.length > 0 ? columns[0].name : '';
		filters = [
			...filters,
			{
				id: crypto.randomUUID(),
				column: firstCol,
				operator: 'eq',
				value: '',
				enabled: true
			}
		];
	}

	function removeFilter(id: string) {
		filters = filters.filter((f) => f.id !== id);
	}

	function updateFilter(id: string, field: keyof FilterCondition, value: string | FilterOperator) {
		filters = filters.map((filter) =>
			filter.id === id ? ({ ...filter, [field]: value } as FilterCondition) : filter
		);
	}

	function applyFilters() {
		appliedFilters = [...filters];
		currentPage = 0;
	}

	function clearFilters() {
		filters = [];
		appliedFilters = [];
		currentPage = 0;
	}

	function buildDatabaseSorts(currentSorting: SortingState): database.Sort[] {
		return currentSorting.map(
			(sort) =>
				new database.Sort({
					Column: sort.id,
					Direction: sort.desc ? 'desc' : 'asc',
					Nulls: 'last'
				})
		);
	}

	$effect(() => {
		if (tab.kind !== 'table' || !tab.schema || !tab.table) return;
		const structureRevision = tab.revision ?? 0;
		void structureRevision;

		updateStatus('', 'info');
		const nextTableKey = `${tab.connectionId}:${tab.schema}.${tab.table}`;
		if (activeTableKey !== nextTableKey) {
			activeTableKey = nextTableKey;
			currentPage = 0;
			sorting = [];
			lastLoadKey = '';
		}

		const loadStructure = async () => {
			isLoadingStructure = true;
			try {
				let reqTable = new database.Table();
				reqTable.Name = tab.table;
				reqTable.Schema = tab.schema;

				const [cols, idxs, capabilityResponse] = await Promise.all([
					GetCollectionStructures(tab.connectionId, reqTable),
					GetIndices(tab.connectionId, reqTable),
					GetCapabilities(tab.connectionId)
				]);

				if (cols.errors?.length) throw new Error(cols.errors[0].detail);
				if (idxs.errors?.length) throw new Error(idxs.errors[0].detail);
				if (!capabilityResponse.errors?.length) {
					capabilities = capabilityResponse.data || null;
				}

				indices = idxs.data || [];
				const primaryColumns = new Set(
					indices
						.filter((index) => index.is_primary || /_pkey$/i.test(index.name))
						.flatMap((index) => index.columns || [])
				);
				columns = (cols.data || []).map((column) =>
					column.is_primary || !primaryColumns.has(column.name)
						? column
						: new database.Structure({
								...column,
								is_primary: true,
								is_primary_label: 'PRI'
							})
				);
			} catch (e: any) {
				updateStatus(e?.message ?? 'Unknown Error', 'error');
			} finally {
				isLoadingStructure = false;
			}
		};

		loadStructure();
	});

	$effect(() => {
		const subTab = $tabValue;
		const page = currentPage;
		const currentFilters = appliedFilters;
		const currentSorting = sorting;
		const revision = tab.revision ?? 0;

		if (subTab !== 'data' || !tab.schema || !tab.table) {
			dataRequestVersion += 1;
			isLoadingData = false;
			return;
		}

		const tableName = tab.table;
		const schemaName = tab.schema;
		const connectionId = tab.connectionId;

		// Create a key from current load parameters
		const filterKey = JSON.stringify(currentFilters.filter((f) => f.enabled));
		const sortKey = JSON.stringify(currentSorting);
		const loadKey = `${connectionId}:${schemaName}.${tableName}:${page}:${filterKey}:${sortKey}:${revision}`;

		// Skip if we already loaded this exact state
		if (loadKey === lastLoadKey) {
			return;
		}

		const doLoadData = async () => {
			const requestVersion = ++dataRequestVersion;
			lastLoadKey = loadKey; // Set before async to prevent re-entry
			isLoadingData = true;
			const startedAt = performance.now();
			const offset = page * tableLimit;
			dataLoadingTitle = `Loading ${schemaName}.${tableName}`;
			dataLoadingDescription = 'Counting rows that match the current filters…';
			updateStatus(`Loading ${schemaName}.${tableName}: counting matching rows…`, 'info');
			try {
				let reqTable = new database.Table();
				reqTable.Name = tableName;
				reqTable.Schema = schemaName;
				reqTable.Limit = tableLimit;
				reqTable.Offset = offset;
				reqTable.Filters = buildDatabaseFilters(currentFilters).map(
					(filter) => new database.Filter(filter)
				);
				reqTable.Sorts = buildDatabaseSorts(currentSorting);

				const totalRes = await CountCollectionData(connectionId, reqTable);
				if (requestVersion !== dataRequestVersion) return;
				if (totalRes.errors?.length) throw new Error(totalRes.errors[0].detail);

				tableTotalData = totalRes.data || 0;
				const firstRow = tableTotalData > 0 ? offset + 1 : 0;
				const lastRow = Math.min(offset + tableLimit, tableTotalData);
				dataLoadingDescription =
					tableTotalData > 0
						? `Fetching rows ${firstRow.toLocaleString()}–${lastRow.toLocaleString()} of ${tableTotalData.toLocaleString()}…`
						: 'The table contains no matching rows.';
				updateStatus(
					tableTotalData > 0
						? `Fetching ${schemaName}.${tableName} rows ${firstRow}–${lastRow} of ${tableTotalData}…`
						: `${schemaName}.${tableName} has no matching rows`,
					'info'
				);

				const dataRes = await GetCollectionData(connectionId, reqTable);
				if (requestVersion !== dataRequestVersion) return;

				if (dataRes.errors?.length) throw new Error(dataRes.errors[0].detail);

				tableData = dataRes.data?.data || [];
				const duration = Math.round(performance.now() - startedAt);
				updateStatus(
					`Loaded ${tableData.length} ${tableData.length === 1 ? 'row' : 'rows'} from ${schemaName}.${tableName} in ${duration}ms`,
					'success'
				);
			} catch (e: any) {
				if (requestVersion !== dataRequestVersion) return;
				console.error('[TableContent] Error loading data:', e);
				updateStatus(e?.message ?? 'Failed fetching data', 'error');
				lastLoadKey = ''; // Reset on error to allow retry
			} finally {
				if (requestVersion === dataRequestVersion) {
					isLoadingData = false;
				}
			}
		};

		doLoadData();
	});

	function handlePageChange(page: number) {
		currentPage = page;
	}

	function handleSortingChange(nextSorting: SortingState) {
		sorting = nextSorting;
		currentPage = 0;
	}

	function openExportDialog(preferredScope?: 'selected') {
		exportInitialScope =
			preferredScope === 'selected' && selectedRows.length > 0 ? 'selected' : 'page';
		exportDialogOpen = true;
	}

	function handleExportSelection(rows: Record<string, any>[], indexes: number[]) {
		selectedRows = rows;
		selectedRowIndexes = indexes;
	}

	function beginExportProgress(jobID: string, expectedRows: number) {
		stopExportProgressPolling?.();
		exportJobID = jobID;
		exportProgress = createInitialExportProgress(jobID, expectedRows);
		stopExportProgressPolling = startExportProgressPolling(jobID, (progress) => {
			exportProgress = progress;
		});
	}

	function finishExportProgress() {
		stopExportProgressPolling?.();
		stopExportProgressPolling = null;
		exportJobID = '';
		exportProgress = null;
		exportCancelling = false;
	}

	async function cancelRunningExport() {
		if (!exportJobID || !exporting || exportCancelling) return;

		exportCancelling = true;
		if (exportProgress) {
			exportProgress = new database.ExportProgress({
				...exportProgress,
				status: 'cancelling',
				cancellable: false
			});
		}

		try {
			await cancelExportJob(exportJobID);
			updateStatus('Stopping export safely…', 'info');
		} catch (error: any) {
			exportCancelling = false;
			updateStatus(error?.message ?? 'Failed to cancel export', 'error');
		}
	}

	async function handleExport(settings: ExportSettings) {
		if (!tab.schema || !tab.table || exporting) return;

		const expectedRows =
			settings.scope === 'selected'
				? selectedRowIndexes.length
				: settings.scope === 'all'
					? tableTotalData
					: tableData.length;
		if (expectedRows === 0) {
			updateStatus('There are no rows to export', 'warn');
			return;
		}

		const persistedSelectedIndexes =
			settings.scope === 'selected'
				? selectedRows.map((row) => tableData.indexOf(row)).filter((index) => index >= 0)
				: [];
		if (
			settings.scope === 'selected' &&
			settings.format === 'sql' &&
			persistedSelectedIndexes.length !== selectedRows.length
		) {
			updateStatus(
				'Apply or discard newly added rows before exporting the selection as SQL',
				'warn'
			);
			return;
		}

		exporting = true;
		exportCancelling = false;
		const extension = getExportExtension(settings.format);
		const jobID = crypto.randomUUID();
		beginExportProgress(jobID, expectedRows);
		const table = new database.Table({
			Schema: tab.schema,
			Name: tab.table,
			Limit: tableLimit,
			Offset: currentPage * tableLimit,
			Filters: buildDatabaseFilters(appliedFilters).map((filter) => new database.Filter(filter)),
			Sorts: buildDatabaseSorts(sorting)
		});

		try {
			updateStatus(
				settings.scope === 'selected'
					? `Exporting ${selectedRows.length.toLocaleString()} selected rows as ${settings.format.toUpperCase()}…`
					: settings.scope === 'all'
						? `Exporting ${tableTotalData.toLocaleString()} filtered rows as ${settings.format.toUpperCase()}…`
						: `Exporting page ${currentPage + 1} as ${settings.format.toUpperCase()}…`,
				'info'
			);
			const options = new database.ExportOptions(buildExportOptions(settings));
			const response =
				settings.scope === 'selected' && settings.format !== 'sql'
					? await ExportQueryResults(
							new database.RowsExportRequest({
								columns: columns.map((column) => column.name),
								rows: selectedRows,
								jobId: jobID,
								expectedRows,
								suggestedName: `${tab.schema}-${tab.table}-selected.${extension}`,
								options
							})
						)
					: await ExportTableData(
							tab.connectionId,
							new database.TableExportRequest({
								table,
								scope:
									settings.scope === 'selected'
										? 'selected'
										: settings.scope === 'all'
											? 'all'
											: 'page',
								selectedRowIndexes: persistedSelectedIndexes,
								jobId: jobID,
								expectedRows,
								suggestedName: `${tab.schema}-${tab.table}.${extension}`,
								options
							})
						);
			if (response.errors?.length) throw new Error(response.errors[0].detail);

			if (response.data?.cancelled) {
				updateStatus('Export cancelled', 'info');
			} else if (response.data) {
				updateStatus(
					`Exported ${response.data.rows.toLocaleString()} rows (${formatExportBytes(response.data.bytes)}) to ${response.data.path}`,
					'success'
				);
			}
			exportDialogOpen = false;
		} catch (error: any) {
			updateStatus(error?.message ?? 'Failed to export table data', 'error');
		} finally {
			exporting = false;
			finishExportProgress();
		}
	}

	onDestroy(() => {
		stopExportProgressPolling?.();
	});

	function getColumnRelation(column: database.Structure) {
		return getForeignRelation(column, tab.schema || '');
	}

	function openForeignReference(column: database.Structure) {
		const relation = getColumnRelation(column);
		if (!relation) return;

		const existing = tabsStore.findTableTab(tab.connectionId, relation.schema, relation.table);
		if (existing) {
			tabsStore.setActive(existing.id);
		} else {
			tabsStore.newTableTab(tab.connectionId, relation.schema, relation.table);
		}
		updateStatus(`Opened ${relation.schema}.${relation.table}`, 'info');
	}

	function openTableChange(
		intent: StructuralChangeIntent,
		reference: database.ObjectReference | null = null
	) {
		if (writeBlocked) {
			updateStatus(
				'This connection is read-only. Temporarily unlock writes from the connection menu first.',
				'info'
			);
			return;
		}
		changeReference = reference;
		changeIntent = intent;
	}

	function openDropIndex(index: database.Index) {
		openTableChange(
			'drop',
			new database.ObjectReference({
				kind: 'index',
				schema: tab.schema || '',
				name: index.name,
				parentSchema: tab.schema || '',
				parentName: tab.table || ''
			})
		);
	}

	function handleStructureChangeApplied() {
		changeIntent = null;
		changeReference = null;
		tabsStore.updateTab(tab.id, { revision: Date.now() });
		window.dispatchEvent(
			new CustomEvent('database-objects-changed', {
				detail: { connectionId: tab.connectionId, schema: tab.schema }
			})
		);
	}

	// Load DDL when DDL tab is selected
	$effect(() => {
		const subTab = $tabValue;

		if (subTab !== 'ddl' || !tab.schema || !tab.table) {
			return;
		}

		const tableName = tab.table;
		const schemaName = tab.schema;

		const loadDDL = async () => {
			isLoadingDDL = true;
			try {
				let reqTable = new database.Table();
				reqTable.Name = tableName;
				reqTable.Schema = schemaName;

				const res = await GetTableDDL(tab.connectionId, reqTable);
				if (res.errors?.length) throw new Error(res.errors[0].detail);
				tableDDL = res.data || '';
			} catch (e: any) {
				console.error('[TableContent] Error loading DDL:', e);
				tableDDL = `-- Error: ${e?.message ?? 'Failed to generate DDL'}`;
			} finally {
				isLoadingDDL = false;
			}
		};

		loadDDL();
	});
</script>

<div class="flex h-full min-h-0 w-full flex-1 flex-col overflow-hidden">
	<div use:melt={$tabsRoot} class="flex h-full min-h-0 flex-1 flex-col overflow-hidden">
		<div class="flex h-11 shrink-0 items-center justify-between border-b px-4">
			<div use:melt={$tabsList} class="inline-flex h-full items-center gap-5">
				<button
					use:melt={$tabTrigger('structure')}
					class="text-muted-foreground data-[state=active]:border-b-primary data-[state=active]:text-foreground inline-flex h-full items-center justify-center gap-1.5 border-0 border-b-2 border-b-transparent px-0.5 text-[10px] font-semibold transition-colors"
				>
					<LayoutGrid class="h-3 w-3" />
					Structure
				</button>
				<button
					use:melt={$tabTrigger('data')}
					class="text-muted-foreground data-[state=active]:border-b-primary data-[state=active]:text-foreground inline-flex h-full items-center justify-center gap-1.5 border-0 border-b-2 border-b-transparent px-0.5 text-[10px] font-semibold transition-colors"
				>
					<Table2 class="h-3 w-3" />
					Data
				</button>
				<button
					use:melt={$tabTrigger('ddl')}
					class="text-muted-foreground data-[state=active]:border-b-primary data-[state=active]:text-foreground inline-flex h-full items-center justify-center gap-1.5 border-0 border-b-2 border-b-transparent px-0.5 text-[10px] font-semibold transition-colors"
				>
					<Code class="h-3 w-3" />
					DDL
				</button>
			</div>
			<div class="text-muted-foreground flex items-center gap-2 text-[10px]">
				{#if tab.schema && tab.table}
					<span class="font-mono">{tab.schema}.{tab.table}</span>
					<span class="h-3 border-l"></span>
				{/if}
				<span>{columns.length} columns</span>
			</div>
		</div>

		<!-- Structure Tab -->
		<div
			use:melt={$tabContent('structure')}
			data-structure-scroll
			class="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-[var(--background)] p-3"
		>
			{#if isLoadingStructure}
				<div class="flex h-full items-center justify-center py-20">
					<Loader2 class="text-muted-foreground h-8 w-8 animate-spin" />
				</div>
			{:else}
				<div class="space-y-3 pr-1 pb-3">
					{#if capabilities?.manageIndexes || capabilities?.alterTableStructure}
						<div
							class="flex min-h-10 items-center justify-between gap-3 rounded-lg border bg-[var(--surface-raised)] px-3 py-1.5"
						>
							<div class="flex min-w-0 items-center gap-2">
								<Settings2 class="text-muted-foreground h-3.5 w-3.5 shrink-0" />
								<div class="min-w-0">
									<div class="text-[9px] font-bold">Reviewed structure changes</div>
									<div class="text-muted-foreground truncate text-[8px]">
										Every action opens an exact SQL preview before apply.
									</div>
								</div>
							</div>
							<div class="flex shrink-0 items-center gap-1">
								{#if capabilities?.manageIndexes}
									<button
										type="button"
										class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
										onclick={() => openTableChange('create-index')}
									>
										<Plus class="h-3 w-3" />
										New index
									</button>
								{/if}
								{#if capabilities?.alterTableStructure}
									<button
										type="button"
										class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
										onclick={() => openTableChange('add-column')}
									>
										<Plus class="h-3 w-3" />
										Add column
									</button>
									<button
										type="button"
										class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
										onclick={() => openTableChange('alter-column')}
										disabled={columns.length === 0}
									>
										<Settings2 class="h-3 w-3" />
										Alter column
									</button>
									<button
										type="button"
										class="rt-toolbar-button text-danger hover:bg-danger-soft h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
										onclick={() => openTableChange('drop-column')}
										disabled={columns.length <= 1}
									>
										<Trash2 class="h-3 w-3" />
										Drop column
									</button>
									<button
										type="button"
										class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
										onclick={() => openTableChange('add-constraint')}
									>
										<KeyRound class="h-3 w-3" />
										Add constraint
									</button>
								{/if}
							</div>
						</div>
					{/if}
					<section
						class="grid shrink-0 grid-cols-4 divide-x overflow-hidden rounded-lg border bg-[var(--surface-raised)]"
					>
						<div class="flex min-h-12 items-center gap-2.5 px-3">
							<span
								class="bg-muted text-muted-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
							>
								<Table2 class="h-3.5 w-3.5" />
							</span>
							<div class="min-w-0">
								<div class="text-[11px] font-semibold tabular-nums">{columns.length}</div>
								<div class="text-muted-foreground truncate text-[8px]">
									Columns · {nullableCount} nullable
								</div>
							</div>
						</div>
						<div class="flex min-h-12 items-center gap-2.5 px-3">
							<span
								class="bg-muted text-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
							>
								<KeyRound class="h-3.5 w-3.5" />
							</span>
							<div class="min-w-0">
								<div class="text-[11px] font-semibold tabular-nums">{primaryKeyCount}</div>
								<div class="text-muted-foreground truncate text-[8px]">Primary keys</div>
							</div>
						</div>
						<div class="flex min-h-12 items-center gap-2.5 px-3">
							<span
								class="bg-info-soft text-info flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
							>
								<Link2 class="h-3.5 w-3.5" />
							</span>
							<div class="min-w-0">
								<div class="text-[11px] font-semibold tabular-nums">{relationCount}</div>
								<div class="text-muted-foreground truncate text-[8px]">Foreign keys</div>
							</div>
						</div>
						<div class="flex min-h-12 items-center gap-2.5 px-3">
							<span
								class="bg-muted text-muted-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
							>
								<KeyRound class="h-3.5 w-3.5" />
							</span>
							<div class="min-w-0">
								<div class="text-[11px] font-semibold tabular-nums">{indices.length}</div>
								<div class="text-muted-foreground truncate text-[8px]">Indexes</div>
							</div>
						</div>
					</section>

					<div class="space-y-3">
						<section class="min-w-0 overflow-hidden rounded-lg border bg-[var(--surface-raised)]">
							<div class="flex h-9 items-center justify-between border-b px-3">
								<div class="flex items-center gap-2">
									<Table2 class="text-muted-foreground h-3.5 w-3.5" />
									<h3 class="text-[10px] font-semibold">Columns</h3>
								</div>
								<span
									class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[8px] tabular-nums"
									>{columns.length}</span
								>
							</div>
							<div class="overflow-x-auto">
								<table class="w-full min-w-[840px] table-fixed caption-bottom">
									<thead class="bg-muted/35">
										<tr class="border-b">
											<th class="h-7 w-[28%] px-3 text-left text-[8px]">Column</th>
											<th class="h-7 w-[18%] px-3 text-left text-[8px]">Data type</th>
											<th class="h-7 w-[28%] px-3 text-left text-[8px]">Properties</th>
											<th class="h-7 px-3 text-left text-[8px]">Default / reference</th>
										</tr>
									</thead>
									<tbody>
										{#each columns as col (col.name)}
											{@const relation = getColumnRelation(col)}
											<tr
												class="h-11 border-b transition-colors last:border-b-0 hover:bg-[var(--surface-hover)]"
											>
												<td class="h-11 px-3">
													<div class="flex min-w-0 items-center gap-2">
														<span
															class="bg-muted flex h-6 w-6 shrink-0 items-center justify-center rounded-md"
														>
															{#if col.is_primary}
																<KeyRound class="text-foreground h-3 w-3" />
															{:else if relation}
																<Link2 class="text-info h-3 w-3" />
															{:else}
																<span class="bg-muted-foreground/40 h-1.5 w-1.5 rounded-full"
																></span>
															{/if}
														</span>
														<span class="truncate font-mono text-[10px] font-semibold"
															>{col.name}</span
														>
													</div>
												</td>
												<td class="h-11 px-3">
													<div
														class="flex min-w-0 items-center gap-1.5"
														title={getColumnTypeLabel(col)}
													>
														<span
															class="inline-flex max-w-full shrink-0 rounded px-1.5 py-1 font-mono text-[8px] {col.is_enum
																? 'bg-muted text-muted-foreground dark:text-muted-foreground font-semibold'
																: 'bg-muted text-muted-foreground'}"
														>
															{col.is_enum
																? 'ENUM'
																: `${col.data_type}${col.length ? `(${col.length})` : ''}`}
														</span>
														{#if col.is_enum && col.type_name}
															<span class="text-muted-foreground truncate font-mono text-[7px]">
																{col.type_schema ? `${col.type_schema}.` : ''}{col.type_name}
															</span>
														{:else if col.affinity && col.affinity !== col.data_type}
															<span
																class="text-muted-foreground truncate font-mono text-[7px]"
																title={`SQLite ${col.affinity} affinity`}
															>
																{col.affinity}
															</span>
														{/if}
													</div>
												</td>
												<td class="h-11 px-3">
													<div class="flex min-w-0 items-center gap-1.5 overflow-hidden">
														{#if col.is_primary}
															<span
																class="bg-muted text-foreground inline-flex h-5 shrink-0 items-center gap-1 rounded px-1.5 text-[7px] font-semibold"
															>
																<KeyRound class="h-2.5 w-2.5" />
																PK
															</span>
														{/if}
														{#if relation}
															<span
																class="bg-info-soft text-info inline-flex h-5 shrink-0 items-center gap-1 rounded px-1.5 text-[7px] font-semibold"
															>
																<Link2 class="h-2.5 w-2.5" />
																FK
															</span>
														{/if}
														{#if col.is_unique && !col.is_primary}
															<span
																class="bg-muted text-muted-foreground inline-flex h-5 shrink-0 items-center rounded px-1.5 text-[7px] font-medium"
																>UNIQUE</span
															>
														{/if}
														{#if col.is_autoinc}
															<span
																class="bg-muted text-muted-foreground inline-flex h-5 shrink-0 items-center rounded px-1.5 text-[7px] font-medium"
																>AUTO</span
															>
														{/if}
														{#if col.is_generated}
															<span
																class="bg-muted text-muted-foreground dark:text-muted-foreground inline-flex h-5 shrink-0 items-center rounded px-1.5 text-[7px] font-medium"
																title={col.generation || 'Generated column'}>GENERATED</span
															>
														{/if}
														{#if col.is_rowid}
															<span
																class="bg-muted text-muted-foreground inline-flex h-5 shrink-0 items-center rounded px-1.5 text-[7px] font-medium"
																>ROWID</span
															>
														{/if}
														<span class="text-muted-foreground truncate text-[8px]">
															{col.nullable ? 'Nullable' : 'Required'}
														</span>
													</div>
												</td>
												<td class="h-11 px-3">
													<div
														class="text-muted-foreground flex min-w-0 items-center gap-2 overflow-hidden font-mono text-[8px]"
													>
														{#if relation}
															<button
																type="button"
																class="text-info hover:text-info inline-flex min-w-0 cursor-pointer items-center gap-1 transition-colors hover:underline"
																title={`Open ${relation.schema}.${relation.table}`}
																aria-label={`Open referenced table ${relation.schema}.${relation.table}`}
																onclick={() => openForeignReference(col)}
															>
																<Link2 class="h-3 w-3 shrink-0" />
																<span class="truncate"
																	>{relation.schema}.{relation.table}{relation.column
																		? `.${relation.column}`
																		: ''}</span
																>
															</button>
														{/if}
														{#if col.default}
															{#if relation}
																<span class="h-3 shrink-0 border-l"></span>
															{/if}
															<span class="truncate" title={`Default ${col.default}`}>
																default {col.default}
															</span>
														{:else if !relation}
															<span>-</span>
														{/if}
													</div>
												</td>
											</tr>
										{:else}
											<tr>
												<td colspan="4" class="text-muted-foreground h-28 text-center text-[10px]">
													No column metadata available
												</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						</section>

						<section class="overflow-hidden rounded-lg border bg-[var(--surface-raised)]">
							<div class="flex h-9 items-center justify-between border-b px-3">
								<div class="flex items-center gap-2">
									<KeyRound class="text-muted-foreground h-3.5 w-3.5" />
									<h3 class="text-[10px] font-semibold">Indexes</h3>
								</div>
								<span
									class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[8px] tabular-nums"
									>{indices.length}</span
								>
							</div>
							<div>
								{#each indices as idx (idx.name)}
									<div class="flex min-h-14 items-start gap-2.5 border-b px-3 py-3 last:border-b-0">
										<KeyRound
											class="mt-0.5 h-3.5 w-3.5 shrink-0 {idx.is_primary
												? 'text-foreground'
												: 'text-muted-foreground'}"
										/>
										<div class="min-w-0 flex-1">
											<div class="truncate font-mono text-[9px] font-semibold">{idx.name}</div>
											<div class="text-muted-foreground mt-1 truncate font-mono text-[8px]">
												{idx.columns?.join(', ') || 'No columns'}
											</div>
											<div class="text-muted-foreground mt-1 text-[8px]">
												{idx.algorithm || 'default'} · {idx.is_primary
													? 'primary'
													: idx.is_unique
														? 'unique'
														: 'non-unique'}
											</div>
										</div>
										{#if capabilities?.manageIndexes && !idx.is_primary}
											<button
												type="button"
												class="rt-toolbar-button text-danger hover:bg-danger-soft h-7 w-7 shrink-0 cursor-pointer"
												onclick={() => openDropIndex(idx)}
												title={`Drop index ${idx.name}`}
												aria-label={`Drop index ${idx.name}`}
											>
												<Trash2 class="h-3 w-3" />
											</button>
										{/if}
									</div>
								{:else}
									<div class="text-muted-foreground px-4 py-10 text-center">
										<KeyRound class="mx-auto h-4 w-4 opacity-45" />
										<p class="mt-2 text-[9px]">No indexes defined</p>
									</div>
								{/each}
							</div>
						</section>
					</div>
				</div>
			{/if}
		</div>

		<!-- Data Tab -->
		<div
			use:melt={$tabContent('data')}
			class="flex min-h-0 flex-1 flex-col bg-[var(--background)] p-3"
		>
			<!-- Filters Panel -->
			{#if filters.length > 0}
				<section class="mb-2 overflow-hidden rounded-lg border bg-[var(--surface-raised)]">
					<div class="flex h-9 items-center justify-between border-b px-3">
						<div class="flex items-center gap-2">
							<Filter class="text-muted-foreground h-3.5 w-3.5" />
							<span class="text-[10px] font-bold">Filters</span>
							<span class="text-muted-foreground text-[8px]">
								{filters.filter((filter) => filter.enabled).length} active
							</span>
						</div>
						<button
							type="button"
							class="rt-toolbar-button h-7 gap-1.5 px-2 text-[9px] font-semibold"
							onclick={addFilter}
						>
							<Plus class="h-3 w-3" />
							Add condition
						</button>
					</div>

					<div class="space-y-1.5 p-2.5">
						{#each filters as filter (filter.id)}
							<div
								class="grid grid-cols-[22px_168px_142px_minmax(140px,1fr)_30px] items-center gap-2"
							>
								<input
									type="checkbox"
									id="filter-{filter.id}"
									class="border-input bg-background focus:ring-primary accent-primary h-3.5 w-3.5 rounded border focus:ring-2"
									checked={filter.enabled}
									onchange={() => {
										filter.enabled = !filter.enabled;
										filters = [...filters];
									}}
									aria-label="Enable filter"
								/>

								<FilterCombobox
									options={columns.map((col) => ({ value: col.name, label: col.name }))}
									value={filter.column}
									onChange={(v) => updateFilter(filter.id, 'column', v)}
									placeholder="Column"
									class="w-full"
								/>

								<FilterCombobox
									options={FILTER_OPERATORS}
									value={filter.operator}
									onChange={(v) => updateFilter(filter.id, 'operator', v)}
									placeholder="Operator"
									class="w-full"
								/>

								{#if filterNeedsValue(filter.operator)}
									<input
										type="text"
										class="rt-input placeholder:text-muted-foreground h-8 w-full px-3 text-[10px]"
										placeholder="Value"
										value={filter.value}
										oninput={(e) => updateFilter(filter.id, 'value', e.currentTarget.value)}
										onkeydown={(event) => event.key === 'Enter' && applyFilters()}
									/>
								{:else}
									<span class="text-muted-foreground px-2 text-[9px]">No value required</span>
								{/if}

								<button
									type="button"
									class="rt-toolbar-button hover:text-destructive h-7 w-7"
									onclick={() => removeFilter(filter.id)}
									title="Remove condition"
									aria-label="Remove filter condition"
								>
									<X class="h-3.5 w-3.5" />
								</button>
							</div>
						{/each}
					</div>

					<div class="flex h-10 items-center justify-between border-t px-3">
						<span class="text-muted-foreground text-[8px]">
							Filters are applied when you run them.
						</span>
						<div class="flex items-center gap-1.5">
							<button
								type="button"
								class="rt-toolbar-button h-7 px-2.5 text-[9px] font-semibold"
								onclick={clearFilters}
							>
								Reset
							</button>
							<button
								type="button"
								class="rt-primary-button inline-flex h-7 items-center rounded-md px-3 text-[9px] font-bold"
								onclick={applyFilters}
							>
								Run filters
							</button>
						</div>
					</div>
				</section>
			{/if}

			<!-- Data Grid -->
			<div class="min-h-0 flex-1 overflow-hidden">
				<DataGrid
					tabId={tab.id}
					{columns}
					data={tableData}
					totalRows={tableTotalData}
					{currentPage}
					pageSize={tableLimit}
					{sorting}
					onPageChange={handlePageChange}
					onSortingChange={handleSortingChange}
					onAddFilter={addFilter}
					onExport={openExportDialog}
					onSelectionChange={handleExportSelection}
					{exporting}
					readonly={writeBlocked}
					detailTitle={tab.schema && tab.table ? `${tab.schema}.${tab.table}` : 'Table row'}
					loading={isLoadingData}
					loadingTitle={dataLoadingTitle}
					loadingDescription={dataLoadingDescription}
				/>
			</div>
		</div>

		<!-- DDL Tab -->
		<div use:melt={$tabContent('ddl')} class="flex-1 overflow-auto bg-[var(--background)] p-3">
			{#if isLoadingDDL}
				<div class="flex h-32 items-center justify-center">
					<div
						class="h-6 w-6 animate-spin rounded-full border-2 border-current border-t-transparent"
					></div>
				</div>
			{:else if tableDDL}
				<div class="overflow-hidden rounded-lg border bg-[var(--surface-raised)] shadow-sm">
					<div class="flex h-9 items-center justify-between border-b px-3">
						<span class="text-xs font-bold">Table definition</span>
						<span class="text-muted-foreground font-mono text-[9px]">SQL</span>
					</div>
					<pre
						class="rt-code-surface overflow-auto p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap">{tableDDL}</pre>
				</div>
			{:else}
				<div class="text-muted-foreground py-8 text-center">No DDL available</div>
			{/if}
		</div>
	</div>
</div>

<ExportDialog
	open={exportDialogOpen}
	source="table"
	sourceName={tab.schema && tab.table ? `${tab.schema}.${tab.table}` : ''}
	engine={capabilities?.displayName || capabilities?.engine || ''}
	pageRows={tableData.length}
	totalRows={tableTotalData}
	selectedRows={selectedRows.length}
	initialScope={exportInitialScope}
	{exporting}
	cancelling={exportCancelling}
	progress={exportProgress}
	onClose={() => (exportDialogOpen = false)}
	onCancelExport={cancelRunningExport}
	onExport={handleExport}
/>

<ObjectChangeDialog
	open={changeIntent !== null}
	connectionId={tab.connectionId}
	intent={changeIntent}
	{capabilities}
	reference={changeReference ||
		new database.ObjectReference({
			kind: 'table',
			schema: tab.schema || '',
			name: tab.table || ''
		})}
	table={new database.Table({
		Schema: tab.schema || '',
		Name: tab.table || ''
	})}
	{columns}
	onClose={() => {
		changeIntent = null;
		changeReference = null;
	}}
	onApplied={handleStructureChangeApplied}
/>
