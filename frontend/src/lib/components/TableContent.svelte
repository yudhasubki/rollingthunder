<script lang="ts">
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import DataGrid from '$lib/components/database/DataGrid.svelte';
	import { database } from '$lib/wailsjs/go/models';
	import { updateStatus, updateDatabaseInfo } from '$lib/stores/status.svelte';
	import {
		CountCollectionData,
		GetCollectionData,
		GetCollectionStructures,
		GetDatabaseInfo,
		GetIndices
	} from '$lib/wailsjs/go/db/Service';
	import { LayoutGrid, Table2, Plus, Minus, Filter, Search } from 'lucide-svelte';

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
	let isLoadingData = false; // Prevent infinite loop
	let filters = $state<FilterCondition[]>([]);
	let appliedFilters = $state<FilterCondition[]>([]); // Filters that are actually applied

	const tableLimit = 100;
	let currentPage = $state(0);
	let subTab = $state<'structure' | 'data'>('structure');

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
		currentPage = 0; // Reset to first page when filters change
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
		currentPage = 0; // Reset page when tab changes

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
		// Capture dependencies
		const tab = subTab;
		const page = currentPage;
		const activeTab = tabsStore.activeTab;
		const currentFilters = appliedFilters; // Track applied filters

		if (tab !== 'data' || !activeTab || activeTab.kind !== 'table') {
			return;
		}

		// Prevent infinite loop - skip if already loading
		if (isLoadingData) {
			return;
		}

		// Use captured values in loadTableData
		const tableName = activeTab.table;
		const schemaName = activeTab.schema;

		// Build filter clause
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
						return `"${f.column}" ILIKE '%${f.value.replace(/'/g, "''")}%'`;
					} else {
						return `"${f.column}" ${f.operator} '${f.value.replace(/'/g, "''")}'`;
					}
				});

			return conditions.length > 0 ? conditions.join(' AND ') : '';
		};

		const doLoadData = async () => {
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
			} finally {
				isLoadingData = false;
			}
		};

		doLoadData();
	});

	function handlePageChange(page: number) {
		currentPage = page;
		// Effect will auto-trigger when currentPage changes
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
	<!-- Sub-tabs: Structure / Data -->
	<Tabs.Root bind:value={subTab} class="flex min-h-0 flex-1 flex-col">
		<div class="px-4">
			<Tabs.List variant="underline" class="h-9 bg-transparent">
				<Tabs.Trigger variant="underline" value="structure" class="gap-1.5 text-xs">
					<LayoutGrid class="h-3.5 w-3.5" />
					Structure
				</Tabs.Trigger>
				<Tabs.Trigger variant="underline" value="data" class="gap-1.5 text-xs">
					<Table2 class="h-3.5 w-3.5" />
					Data
				</Tabs.Trigger>
			</Tabs.List>
		</div>

		<!-- Structure Tab -->
		<Tabs.Content value="structure" class="flex-1 overflow-auto p-4">
			<div class="space-y-4">
				<!-- Columns -->
				<div>
					<h3 class="mb-2 text-sm font-medium">columns</h3>
					<div class="rounded-md border">
						<ScrollArea class="h-[35vh]">
							<Table.Root>
								<Table.Header>
									<Table.Row>
										<Table.Head class="w-48">name</Table.Head>
										<Table.Head>type</Table.Head>
										<Table.Head>length</Table.Head>
										<Table.Head>nullable</Table.Head>
										<Table.Head>default</Table.Head>
										<Table.Head>primary</Table.Head>
										<Table.Head>foreign key</Table.Head>
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each columns as col (col.name)}
										<Table.Row>
											<Table.Cell class="font-mono text-sm">{col.name}</Table.Cell>
											<Table.Cell class="text-muted-foreground font-mono text-xs"
												>{col.data_type}</Table.Cell
											>
											<Table.Cell>{col.length || '-'}</Table.Cell>
											<Table.Cell>{col.nullable ? 'yes' : 'no'}</Table.Cell>
											<Table.Cell class="max-w-32 truncate font-mono text-xs"
												>{col.default || '-'}</Table.Cell
											>
											<Table.Cell>{col.is_primary_label || '-'}</Table.Cell>
											<Table.Cell>{col.foreign_key || '-'}</Table.Cell>
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						</ScrollArea>
					</div>
				</div>

				<!-- Indices -->
				<div>
					<h3 class="mb-2 text-sm font-medium">indices</h3>
					<div class="rounded-md border">
						<ScrollArea class="h-[20vh]">
							<Table.Root>
								<Table.Header>
									<Table.Row>
										<Table.Head>name</Table.Head>
										<Table.Head>columns</Table.Head>
										<Table.Head>unique</Table.Head>
										<Table.Head>algorithm</Table.Head>
									</Table.Row>
								</Table.Header>
								<Table.Body>
									{#each indices as idx (idx.name)}
										<Table.Row>
											<Table.Cell class="font-mono text-sm">{idx.name}</Table.Cell>
											<Table.Cell class="font-mono text-xs">{idx.columns}</Table.Cell>
											<Table.Cell>{idx.is_unique ? 'yes' : 'no'}</Table.Cell>
											<Table.Cell>{idx.algorithm || '-'}</Table.Cell>
										</Table.Row>
									{/each}
								</Table.Body>
							</Table.Root>
						</ScrollArea>
					</div>
				</div>
			</div>
		</Tabs.Content>

		<!-- Data Tab -->
		<Tabs.Content value="data" class="flex min-h-0 flex-1 flex-col overflow-hidden p-4">
			<!-- Filter Section -->
			{#if filters.length > 0}
				<div class="mb-3 space-y-2">
					{#each filters as filter (filter.id)}
						<div class="flex items-center gap-2">
							<!-- Checkbox -->
							<input
								type="checkbox"
								class="border-input h-4 w-4 rounded"
								checked={filter.enabled}
								onchange={() => {
									filter.enabled = !filter.enabled;
									filters = [...filters];
								}}
							/>

							<!-- Column Select -->
							<Select.Root
								type="single"
								bind:value={filter.column}
								onValueChange={(v) => updateFilter(filter.id, 'column', v ?? '')}
							>
								<Select.Trigger class="h-8 w-32" size="sm">
									{filter.column || 'Column'}
								</Select.Trigger>
								<Select.Content>
									{#each columns as col}
										<Select.Item value={col.name}>{col.name}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>

							<!-- Operator Select -->
							<Select.Root
								type="single"
								bind:value={filter.operator}
								onValueChange={(v) => updateFilter(filter.id, 'operator', v ?? '=')}
							>
								<Select.Trigger class="h-8 w-28" size="sm">
									{FILTER_OPERATORS.find((op) => op.value === filter.operator)?.label || 'equals'}
								</Select.Trigger>
								<Select.Content>
									{#each FILTER_OPERATORS as op}
										<Select.Item value={op.value}>{op.label}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>

							<!-- Value Input (full width) -->
							{#if filter.operator !== 'IS NULL' && filter.operator !== 'IS NOT NULL'}
								<Input
									type="text"
									class="h-8 flex-1"
									placeholder="Value..."
									value={filter.value}
									oninput={(e) =>
										updateFilter(filter.id, 'value', (e.target as HTMLInputElement).value)}
								/>
							{/if}

							<!-- Remove Button -->
							<Button
								variant="ghost"
								size="icon"
								class="h-8 w-8 shrink-0"
								onclick={() => removeFilter(filter.id)}
							>
								<Minus class="h-4 w-4" />
							</Button>

							<!-- Add Button -->
							<Button variant="ghost" size="icon" class="h-8 w-8 shrink-0" onclick={addFilter}>
								<Plus class="h-4 w-4" />
							</Button>
						</div>
					{/each}

					<!-- Action Row -->
					<div class="flex items-center justify-end gap-2 pt-1">
						<Button variant="ghost" size="sm" class="h-8" onclick={clearFilters}>Clear</Button>
						<Button variant="default" size="sm" class="h-8" onclick={applyFilters}>Apply</Button>
					</div>
				</div>
			{/if}

			<!-- Data Grid -->
			<div class="min-h-0 flex-1 overflow-hidden">
				<DataGrid
					{columns}
					data={tableData}
					totalRows={tableTotalData}
					{currentPage}
					pageSize={tableLimit}
					onPageChange={handlePageChange}
					onAddFilter={addFilter}
				/>
			</div>
		</Tabs.Content>
	</Tabs.Root>
</div>
