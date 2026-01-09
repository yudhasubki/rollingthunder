<script lang="ts">
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { createTabs, melt } from '@melt-ui/svelte';
	import DataGrid from '$lib/components/database/DataGrid.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { database } from '$lib/wailsjs/go/models';
	import { updateStatus, updateDatabaseInfo } from '$lib/stores/status.svelte';
	import {
		CountCollectionData,
		GetCollectionData,
		GetCollectionStructures,
		GetDatabaseInfo,
		GetIndices,
		GetTableDDL
	} from '$lib/wailsjs/go/db/Service';
	import { LayoutGrid, Table2, Plus, Minus, Filter, Search, Code } from 'lucide-svelte';

	// Filter types
	interface FilterCondition {
		id: string;
		column: string;
		operator: string;
		value: string;
		enabled: boolean;
	}

	const FILTER_OPERATORS = [
		{ value: '=', label: 'equals' },
		{ value: '!=', label: 'not equals' },
		{ value: '>', label: 'greater than' },
		{ value: '<', label: 'less than' },
		{ value: '>=', label: 'greater or equal' },
		{ value: '<=', label: 'less or equal' },
		{ value: 'LIKE', label: 'contains' },
		{ value: 'IS NULL', label: 'is null' },
		{ value: 'IS NOT NULL', label: 'is not null' }
	];

	let columns = $state<database.Structure[]>([]);
	let indices = $state<database.Index[]>([]);
	let tableTotalData = $state<number>(0);
	let tableData = $state<Record<string, any>[]>([]);
	let isLoadingData = $state(false);
	let filters = $state<FilterCondition[]>([]);
	let appliedFilters = $state<FilterCondition[]>([]);

	// DDL state
	let tableDDL = $state<string>('');
	let isLoadingDDL = $state(false);

	// Track last loaded state to prevent duplicate loads
	let lastLoadKey = '';

	const tableLimit = 100;
	let currentPage = $state(0);

	// Melt-UI Tabs
	const {
		elements: { root: tabsRoot, list: tabsList, trigger: tabTrigger, content: tabContent },
		states: { value: tabValue }
	} = createTabs({
		defaultValue: 'structure'
	});

	// Filter management functions
	function addFilter() {
		const firstCol = columns.length > 0 ? columns[0].name : '';
		filters = [
			...filters,
			{
				id: crypto.randomUUID(),
				column: firstCol,
				operator: '=',
				value: '',
				enabled: true
			}
		];
	}

	function removeFilter(id: string) {
		filters = filters.filter((f) => f.id !== id);
	}

	function updateFilter(id: string, field: keyof FilterCondition, value: string) {
		filters = filters.map((f) => (f.id === id ? { ...f, [field]: value } : f));
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

	$effect(() => {
		const activeTab = tabsStore.activeTab;
		if (!activeTab || activeTab.kind !== 'table') return;

		updateStatus('', 'info');
		currentPage = 0;

		const loadStructure = async () => {
			try {
				let reqTable = new database.Table();
				reqTable.Name = activeTab.table;
				reqTable.Schema = activeTab.schema;

				const [cols, idxs, db] = await Promise.all([
					GetCollectionStructures(reqTable),
					GetIndices(reqTable),
					GetDatabaseInfo()
				]);

				if (cols.errors?.length) throw new Error(cols.errors[0].detail);
				if (idxs.errors?.length) throw new Error(idxs.errors[0].detail);
				if (db.errors?.length) throw new Error(db.errors[0].detail);

				columns = cols.data || [];
				indices = idxs.data || [];
				updateDatabaseInfo(db.data);
			} catch (e: any) {
				updateStatus(e?.message ?? 'Unknown Error', 'error');
			}
		};

		loadStructure();
	});

	$effect(() => {
		const tab = $tabValue;
		const page = currentPage;
		const activeTab = tabsStore.activeTab;
		const currentFilters = appliedFilters;

		if (tab !== 'data' || !activeTab || activeTab.kind !== 'table') {
			return;
		}

		const tableName = activeTab.table;
		const schemaName = activeTab.schema;

		// Create a key from current load parameters
		const filterKey = JSON.stringify(currentFilters.filter((f) => f.enabled));
		const loadKey = `${schemaName}.${tableName}:${page}:${filterKey}`;

		// Skip if we already loaded this exact state
		if (loadKey === lastLoadKey) {
			return;
		}

		const buildFilterClause = (): string => {
			if (currentFilters.length === 0) return '';

			const conditions = currentFilters
				.filter(
					(f) =>
						f.enabled &&
						f.column &&
						(f.operator === 'IS NULL' || f.operator === 'IS NOT NULL' || f.value)
				)
				.map((f) => {
					if (f.operator === 'IS NULL' || f.operator === 'IS NOT NULL') {
						return `"${f.column}" ${f.operator}`;
					} else if (f.operator === 'LIKE') {
						return `"${f.column}" ILIKE '%${f.value.replace(/'/g, "''")}'`;
					} else {
						return `"${f.column}" ${f.operator} '${f.value.replace(/'/g, "''")}'`;
					}
				});

			return conditions.length > 0 ? conditions.join(' AND ') : '';
		};

		const doLoadData = async () => {
			lastLoadKey = loadKey; // Set before async to prevent re-entry
			isLoadingData = true;
			updateStatus('loading data...', 'info');
			try {
				let reqTable = new database.Table();
				reqTable.Name = tableName;
				reqTable.Schema = schemaName;
				reqTable.Limit = tableLimit;
				reqTable.Offset = page * tableLimit;
				reqTable.Filter = buildFilterClause();

				const totalRes = await CountCollectionData(reqTable);
				const dataRes = await GetCollectionData(reqTable);

				if (dataRes.errors?.length) throw new Error(dataRes.errors[0].detail);

				tableData = dataRes.data?.data || [];
				tableTotalData = totalRes.data || 0;
				updateStatus('', 'info');
			} catch (e: any) {
				console.error('[TableContent] Error loading data:', e);
				updateStatus(e?.message ?? 'Failed fetching data', 'error');
				lastLoadKey = ''; // Reset on error to allow retry
			} finally {
				isLoadingData = false;
			}
		};

		doLoadData();
	});

	function handlePageChange(page: number) {
		currentPage = page;
	}

	// Load DDL when DDL tab is selected
	$effect(() => {
		const tab = $tabValue;
		const activeTab = tabsStore.activeTab;

		if (tab !== 'ddl' || !activeTab || activeTab.kind !== 'table') {
			return;
		}

		const tableName = activeTab.table;
		const schemaName = activeTab.schema;

		const loadDDL = async () => {
			isLoadingDDL = true;
			try {
				let reqTable = new database.Table();
				reqTable.Name = tableName;
				reqTable.Schema = schemaName;

				const res = await GetTableDDL(reqTable);
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

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
	<div use:melt={$tabsRoot} class="flex min-h-0 flex-1 flex-col">
		<div class="px-4">
			<div use:melt={$tabsList} class="border-border inline-flex h-9 items-center gap-1 border-b">
				<button
					use:melt={$tabTrigger('structure')}
					class="data-[state=active]:border-primary data-[state=active]:text-foreground text-muted-foreground inline-flex items-center justify-center gap-1.5 border-b-2 border-transparent px-3 py-1.5 text-xs font-medium transition-colors"
				>
					<LayoutGrid class="h-3.5 w-3.5" />
					Structure
				</button>
				<button
					use:melt={$tabTrigger('data')}
					class="data-[state=active]:border-primary data-[state=active]:text-foreground text-muted-foreground inline-flex items-center justify-center gap-1.5 border-b-2 border-transparent px-3 py-1.5 text-xs font-medium transition-colors"
				>
					<Table2 class="h-3.5 w-3.5" />
					Data
				</button>
				<button
					use:melt={$tabTrigger('ddl')}
					class="data-[state=active]:border-primary data-[state=active]:text-foreground text-muted-foreground inline-flex items-center justify-center gap-1.5 border-b-2 border-transparent px-3 py-1.5 text-xs font-medium transition-colors"
				>
					<Code class="h-3.5 w-3.5" />
					DDL
				</button>
			</div>
		</div>

		<!-- Structure Tab -->
		<div use:melt={$tabContent('structure')} class="flex-1 overflow-auto p-4">
			<div class="space-y-4">
				<!-- Columns -->
				<div>
					<h3 class="mb-2 text-sm font-medium">columns</h3>
					<div class="max-h-[35vh] overflow-auto rounded-md border">
						<table class="w-full caption-bottom text-sm">
							<thead class="[&_tr]:border-b">
								<tr class="hover:bg-muted/50 border-b transition-colors">
									<th
										class="text-muted-foreground h-10 w-48 px-4 text-left align-middle font-medium"
										>name</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>type</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>length</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>nullable</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>default</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>primary</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>foreign key</th
									>
								</tr>
							</thead>
							<tbody class="[&_tr:last-child]:border-0">
								{#each columns as col (col.name)}
									<tr class="hover:bg-muted/50 border-b transition-colors">
										<td class="p-4 align-middle font-mono text-sm">{col.name}</td>
										<td class="text-muted-foreground p-4 align-middle font-mono text-xs"
											>{col.data_type}</td
										>
										<td class="p-4 align-middle">{col.length || '-'}</td>
										<td class="p-4 align-middle">{col.nullable ? 'yes' : 'no'}</td>
										<td class="max-w-32 truncate p-4 align-middle font-mono text-xs"
											>{col.default || '-'}</td
										>
										<td class="p-4 align-middle">{col.is_primary_label || '-'}</td>
										<td class="max-w-48 truncate p-4 align-middle font-mono text-xs"
											>{col.foreign_key || '-'}</td
										>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>

				<!-- Indices -->
				<div>
					<h3 class="mb-2 text-sm font-medium">indices</h3>
					<div class="max-h-[25vh] overflow-auto rounded-md border">
						<table class="w-full caption-bottom text-sm">
							<thead class="[&_tr]:border-b">
								<tr class="hover:bg-muted/50 border-b transition-colors">
									<th
										class="text-muted-foreground h-10 w-64 px-4 text-left align-middle font-medium"
										>name</th
									>
									<th class="text-muted-foreground h-10 px-4 text-left align-middle font-medium"
										>definition</th
									>
								</tr>
							</thead>
							<tbody class="[&_tr:last-child]:border-0">
								{#each indices as idx (idx.name)}
									<tr class="hover:bg-muted/50 border-b transition-colors">
										<td class="p-4 align-middle font-mono text-sm">{idx.name}</td>
										<td class="text-muted-foreground p-4 align-middle font-mono text-xs"
											>{idx.definition}</td
										>
									</tr>
								{:else}
									<tr>
										<td colspan="2" class="text-muted-foreground p-4 text-center">no indices</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			</div>
		</div>

		<!-- Data Tab -->
		<div use:melt={$tabContent('data')} class="flex min-h-0 flex-1 flex-col p-4">
			<!-- Filters Panel -->
			{#if filters.length > 0}
				<div class="bg-muted/30 mb-3 space-y-2 rounded-lg border p-3">
					{#each filters as filter (filter.id)}
						<div class="flex items-center gap-2">
							<!-- Enabled Checkbox -->
							<div class="flex items-center">
								<input
									type="checkbox"
									id="filter-{filter.id}"
									class="border-input bg-background focus:ring-primary accent-primary h-4 w-4 rounded border focus:ring-2 focus:ring-offset-2"
									checked={filter.enabled}
									onchange={() => {
										filter.enabled = !filter.enabled;
										filters = [...filters];
									}}
								/>
							</div>

							<!-- Column Select -->
							<FilterCombobox
								options={columns.map((col) => ({ value: col.name, label: col.name }))}
								value={filter.column}
								onChange={(v) => updateFilter(filter.id, 'column', v)}
								placeholder="Column..."
								class="w-40"
							/>

							<!-- Operator Select -->
							<FilterCombobox
								options={FILTER_OPERATORS}
								value={filter.operator}
								onChange={(v) => updateFilter(filter.id, 'operator', v)}
								placeholder="Operator..."
								class="w-36"
							/>

							<!-- Value Input -->
							{#if filter.operator !== 'IS NULL' && filter.operator !== 'IS NOT NULL'}
								<input
									type="text"
									class="border-input bg-background placeholder:text-muted-foreground focus:ring-primary h-8 flex-1 rounded-md border px-3 text-sm focus:outline-none focus:ring-2 focus:ring-offset-1"
									placeholder="Enter value..."
									value={filter.value}
									oninput={(e) => updateFilter(filter.id, 'value', e.currentTarget.value)}
								/>
							{/if}

							<!-- Remove Filter Button -->
							<button
								type="button"
								class="hover:bg-destructive/10 hover:text-destructive inline-flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors"
								onclick={() => removeFilter(filter.id)}
								title="Remove filter"
							>
								<Minus class="h-4 w-4" />
							</button>

							<!-- Add Filter Button -->
							<button
								type="button"
								class="hover:bg-primary/10 hover:text-primary inline-flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors"
								onclick={addFilter}
								title="Add another filter"
							>
								<Plus class="h-4 w-4" />
							</button>
						</div>
					{/each}

					<div class="flex items-center justify-end gap-2 pt-1">
						<button
							class="hover:bg-accent inline-flex h-8 cursor-pointer items-center rounded-md px-3 text-sm transition-colors"
							onclick={clearFilters}
						>
							Clear
						</button>
						<button
							class="bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-8 cursor-pointer items-center rounded-md px-3 text-sm transition-colors"
							onclick={applyFilters}
						>
							Apply
						</button>
					</div>
				</div>
			{/if}

			<!-- Data Grid -->
			<div class="min-h-0 flex-1 overflow-auto">
				<DataGrid
					{columns}
					data={tableData}
					totalRows={tableTotalData}
					{currentPage}
					pageSize={tableLimit}
					onPageChange={handlePageChange}
					onAddFilter={addFilter}
					loading={isLoadingData}
				/>
			</div>
		</div>

		<!-- DDL Tab -->
		<div use:melt={$tabContent('ddl')} class="flex-1 overflow-auto p-4">
			{#if isLoadingDDL}
				<div class="flex h-32 items-center justify-center">
					<div
						class="h-6 w-6 animate-spin rounded-full border-2 border-current border-t-transparent"
					></div>
				</div>
			{:else if tableDDL}
				<div class="rounded-md border">
					<pre
						class="bg-muted/30 overflow-auto whitespace-pre-wrap p-4 font-mono text-sm">{tableDDL}</pre>
				</div>
			{:else}
				<div class="text-muted-foreground py-8 text-center">No DDL available</div>
			{/if}
		</div>
	</div>
</div>
