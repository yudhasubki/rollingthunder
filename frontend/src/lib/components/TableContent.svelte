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
	import {
		LayoutGrid,
		Table2,
		Plus,
		Filter,
		Code,
		Loader2,
		KeyRound,
		Link2,
		X
	} from 'lucide-svelte';

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
	let dataLoadingTitle = $state('Preparing table data');
	let dataLoadingDescription = $state('Waiting for the database');
	let isLoadingStructure = $state(false);
	let filters = $state<FilterCondition[]>([]);
	let appliedFilters = $state<FilterCondition[]>([]);
	const primaryKeyCount = $derived(columns.filter((column) => column.is_primary).length);
	const relationCount = $derived(columns.filter((column) => column.foreign_key).length);
	const nullableCount = $derived(columns.filter((column) => column.nullable).length);

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
			isLoadingStructure = true;
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
			} finally {
				isLoadingStructure = false;
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
				reqTable.Filter = buildFilterClause();

				const totalRes = await CountCollectionData(reqTable);
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

				const dataRes = await GetCollectionData(reqTable);

				if (dataRes.errors?.length) throw new Error(dataRes.errors[0].detail);

				tableData = dataRes.data?.data || [];
				const duration = Math.round(performance.now() - startedAt);
				updateStatus(
					`Loaded ${tableData.length} ${tableData.length === 1 ? 'row' : 'rows'} from ${schemaName}.${tableName} in ${duration}ms`,
					'success'
				);
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
				{#if tabsStore.activeTab?.kind === 'table'}
					<span class="font-mono">{tabsStore.activeTab.schema}.{tabsStore.activeTab.table}</span>
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
					<section
						class="grid shrink-0 grid-cols-4 divide-x overflow-hidden rounded-lg border bg-[var(--surface-raised)]"
					>
						<div class="flex items-center gap-3 px-4 py-3">
							<Table2 class="text-muted-foreground h-4 w-4" />
							<div>
								<div class="text-[13px] font-bold">{columns.length}</div>
								<div class="text-muted-foreground mt-0.5 text-[8px]">
									Columns · {nullableCount} nullable
								</div>
							</div>
						</div>
						<div class="flex items-center gap-3 px-4 py-3">
							<KeyRound class="text-muted-foreground h-4 w-4" />
							<div>
								<div class="text-[13px] font-bold">{primaryKeyCount}</div>
								<div class="text-muted-foreground mt-0.5 text-[8px]">Primary keys</div>
							</div>
						</div>
						<div class="flex items-center gap-3 px-4 py-3">
							<Link2 class="text-muted-foreground h-4 w-4" />
							<div>
								<div class="text-[13px] font-bold">{relationCount}</div>
								<div class="text-muted-foreground mt-0.5 text-[8px]">Relations</div>
							</div>
						</div>
						<div class="flex items-center gap-3 px-4 py-3">
							<KeyRound class="text-muted-foreground h-4 w-4" />
							<div>
								<div class="text-[13px] font-bold">{indices.length}</div>
								<div class="text-muted-foreground mt-0.5 text-[8px]">Indexes</div>
							</div>
						</div>
					</section>

					<div class="space-y-3">
						<section class="min-w-0 overflow-hidden rounded-lg border bg-[var(--surface-raised)]">
							<div class="flex h-11 items-center justify-between border-b px-3.5">
								<div>
									<h3 class="text-[11px] font-bold">Columns</h3>
									<p class="text-muted-foreground mt-0.5 text-[8px]">
										Types, constraints, defaults, and relationships
									</p>
								</div>
								<span class="text-muted-foreground text-[9px]">{columns.length} total</span>
							</div>
							<div class="overflow-x-auto">
								<table class="w-full caption-bottom">
									<thead>
										<tr class="border-b">
											<th class="h-8 w-[28%] px-3.5 text-left">Column</th>
											<th class="h-8 w-[20%] px-3.5 text-left">Type</th>
											<th class="h-8 w-[24%] px-3.5 text-left">Constraints</th>
											<th class="h-8 px-3.5 text-left">Default / relation</th>
										</tr>
									</thead>
									<tbody>
										{#each columns as col (col.name)}
											<tr class="border-b last:border-b-0">
												<td class="px-3.5 py-2.5">
													<div class="flex items-center gap-2">
														{#if col.is_primary}
															<KeyRound class="h-3.5 w-3.5 shrink-0 text-amber-500" />
														{:else if col.foreign_key}
															<Link2 class="text-muted-foreground h-3.5 w-3.5 shrink-0" />
														{:else}
															<span class="bg-muted-foreground/35 h-1.5 w-1.5 shrink-0 rounded-full"
															></span>
														{/if}
														<span class="truncate font-mono text-[10px] font-semibold"
															>{col.name}</span
														>
													</div>
												</td>
												<td class="px-3.5 py-2.5 font-mono text-[9px]">
													{col.data_type}{col.length ? `(${col.length})` : ''}
												</td>
												<td class="text-muted-foreground px-3.5 py-2.5 text-[8px]">
													{#if col.is_primary}<span class="text-foreground font-semibold"
															>primary key</span
														><span class="px-1">·</span>{/if}
													<span>{col.nullable ? 'nullable' : 'required'}</span>
													{#if col.foreign_key}<span class="px-1">·</span><span>foreign key</span
														>{/if}
												</td>
												<td class="px-3.5 py-2.5">
													{#if col.foreign_key}
														<div
															class="text-muted-foreground flex items-center gap-1.5 truncate font-mono text-[8px]"
														>
															<Link2 class="h-3 w-3 shrink-0" />
															<span class="truncate">{col.foreign_key}</span>
														</div>
													{/if}
													{#if col.default}
														<div class="text-muted-foreground mt-0.5 truncate font-mono text-[8px]">
															default {col.default}
														</div>
													{:else if !col.foreign_key}
														<span class="text-muted-foreground text-[9px]">—</span>
													{/if}
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
							<div class="flex h-11 items-center justify-between border-b px-3.5">
								<div>
									<h3 class="text-[11px] font-bold">Indexes</h3>
									<p class="text-muted-foreground mt-0.5 text-[8px]">Lookup and uniqueness rules</p>
								</div>
								<span class="text-muted-foreground text-[9px]">{indices.length}</span>
							</div>
							<div>
								{#each indices as idx (idx.name)}
									<div class="flex min-h-14 items-start gap-2.5 border-b px-3 py-3 last:border-b-0">
										<KeyRound class="text-muted-foreground mt-0.5 h-3.5 w-3.5 shrink-0" />
										<div class="min-w-0 flex-1">
											<div class="truncate font-mono text-[9px] font-semibold">{idx.name}</div>
											<div class="text-muted-foreground mt-1 truncate font-mono text-[8px]">
												{idx.columns?.join(', ') || 'No columns'}
											</div>
											<div class="text-muted-foreground mt-1 text-[8px]">
												{idx.algorithm || 'default'} · {idx.is_unique ? 'unique' : 'non-unique'}
											</div>
										</div>
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

								{#if filter.operator !== 'IS NULL' && filter.operator !== 'IS NOT NULL'}
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
