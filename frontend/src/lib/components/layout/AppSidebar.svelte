<script lang="ts">
	import { onMount } from 'svelte';
	import { writable } from 'svelte/store';
	import { GetSchemas, GetCollections, DropTable, TruncateTable } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import {
		ChevronDown,
		ChevronRight,
		Database,
		Table2,
		Plus,
		RefreshCw,
		Code,
		Search,
		Trash2,
		Eraser,
		MoreVertical,
		AlertTriangle,
		History,
		Clock,
		Play
	} from 'lucide-svelte';
	import { createDropdownMenu, createDialog, melt } from '@melt-ui/svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import {
		getQueryHistory,
		deleteQueryHistoryItem,
		clearQueryHistory,
		type QueryHistoryItem
	} from '$lib/stores/queryHistory.svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import {
		setSidebarRefresh,
		setSidebarAddTable,
		setSidebarRemoveTable
	} from '$lib/stores/sidebar.svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		onTableClick: (schema: string, table: string) => void;
	}

	let { onTableClick }: Props = $props();

	let schemas: string[] = $state([]);
	let selectedSchema = $state<string>('');
	let tables = $state<string[]>([]);
	let loading = $state(false);
	let loadingTables = $state(false);
	let selectedItem = $state<string | null>(null);
	let searchQuery = $state('');
	let historyExpanded = $state(true);

	// Schema selector dropdown
	const schemaOpenStore = writable(false);
	const {
		elements: { trigger: schemaTrigger, menu: schemaMenu, item: schemaItem },
		states: { open: schemaOpen }
	} = createDropdownMenu({
		open: schemaOpenStore,
		positioning: { placement: 'bottom', sameWidth: true }
	});

	// New actions dropdown menu
	const {
		elements: { trigger: ddTrigger, menu: ddMenu, item: ddItem },
		states: { open: ddOpen }
	} = createDropdownMenu({
		positioning: { placement: 'bottom-end' }
	});

	// Filtered tables based on search
	const filteredTables = $derived(
		searchQuery ? tables.filter((t) => t.toLowerCase().includes(searchQuery.toLowerCase())) : tables
	);

	// Context menu state
	let contextMenuTable = $state<string | null>(null);
	let contextMenuPos = $state({ x: 0, y: 0 });
	let showContextMenu = $state(false);

	// Context menu dropdown
	const {
		elements: { trigger: ctxTrigger, menu: ctxMenu, item: ctxItem },
		states: { open: ctxOpen }
	} = createDropdownMenu({
		positioning: { placement: 'bottom-start' }
	});

	// Confirmation dialog state
	let confirmAction = $state<'drop' | 'truncate' | null>(null);
	let confirmTableName = $state<string | null>(null);

	const confirmOpenStore = writable(false);
	const {
		elements: { trigger: dialogTrigger, overlay, content, title, description, close, portalled },
		states: { open: dialogOpen }
	} = createDialog({
		open: confirmOpenStore,
		forceVisible: true
	});

	function openConfirmDialog(action: 'drop' | 'truncate', tableName: string) {
		confirmAction = action;
		confirmTableName = tableName;
		closeContextMenu();
		confirmOpenStore.set(true);
	}

	function closeConfirmDialog() {
		confirmOpenStore.set(false);
		confirmAction = null;
		confirmTableName = null;
		actionLoading = false;
	}

	let actionLoading = $state(false);

	async function executeConfirmedAction() {
		if (!confirmTableName || !selectedSchema || !confirmAction) return;

		const tableName = confirmTableName;
		const action = confirmAction;
		actionLoading = true;

		if (action === 'drop') {
			try {
				const table = new database.Table({ Schema: selectedSchema, Name: tableName });
				const response = await DropTable(table);
				if (response.errors?.length) {
					updateStatus(response.errors[0].detail, 'error');
				} else {
					updateStatus(`DROP TABLE ${selectedSchema}.${tableName}`, 'info');
					const tabId = tabsStore.tabs.find(
						(t) => t.schema === selectedSchema && t.name === tableName
					)?.id;
					if (tabId) {
						tabsStore.closeTab(tabId);
					}
					tables = tables.filter((t) => t !== tableName);
				}
			} catch (e: any) {
				updateStatus(e?.message ?? 'Failed to drop table', 'error');
			}
		} else if (action === 'truncate') {
			try {
				const table = new database.Table({ Schema: selectedSchema, Name: tableName });
				const response = await TruncateTable(table);
				if (response.errors?.length) {
					updateStatus(response.errors[0].detail, 'error');
				} else {
					updateStatus(`TRUNCATE TABLE ${selectedSchema}.${tableName}`, 'info');
				}
			} catch (e: any) {
				updateStatus(e?.message ?? 'Failed to truncate table', 'error');
			}
		}
		closeConfirmDialog();
	}

	onMount(() => {
		// Register sidebar functions for external access
		setSidebarRefresh(async () => {
			await loadTables();
		});
		setSidebarAddTable((tableName: string) => {
			if (!tables.includes(tableName)) {
				tables = [...tables, tableName].sort((a, b) => a.localeCompare(b));
			}
		});
		setSidebarRemoveTable((tableName: string) => {
			tables = tables.filter((t) => t !== tableName);
		});

		// Load initial data
		loadSchemas();

		// Listen for connection switch events
		const handleConnectionSwitch = () => {
			selectedSchema = ''; // Reset schema selection
			loadSchemas(); // Reload schemas for new connection
		};
		window.addEventListener('connection-switched', handleConnectionSwitch);

		// Cleanup
		return () => {
			setSidebarRefresh(null);
			setSidebarAddTable(null);
			setSidebarRemoveTable(null);
			window.removeEventListener('connection-switched', handleConnectionSwitch);
		};
	});

	async function loadSchemas() {
		loading = true;
		try {
			const response = await GetSchemas();
			schemas = response.data || [];
			// Auto-select first schema
			if (schemas.length > 0 && !selectedSchema) {
				selectedSchema = schemas[0];
				await loadTables();
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to load schemas', 'error');
		} finally {
			loading = false;
		}
	}

	async function loadTables() {
		if (!selectedSchema) return;
		loadingTables = true;
		try {
			const response = await GetCollections([selectedSchema]);
			tables = (response.data || []).sort((a, b) => a.localeCompare(b));
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to load tables', 'error');
		} finally {
			loadingTables = false;
		}
	}

	async function selectSchema(schema: string) {
		selectedSchema = schema;
		schemaOpenStore.set(false);
		await loadTables();
	}

	function handleTableClick(table: string) {
		selectedItem = `${selectedSchema}.${table}`;
		onTableClick(selectedSchema, table);
		updateStatus('', 'info');
	}

	async function refresh() {
		// Refresh schemas first, then reload tables
		loading = true;
		try {
			const response = await GetSchemas();
			schemas = response.data || [];
			// Keep the current schema if it still exists, otherwise select first
			if (selectedSchema && schemas.includes(selectedSchema)) {
				await loadTables();
			} else if (schemas.length > 0) {
				selectedSchema = schemas[0];
				await loadTables();
			} else {
				tables = [];
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to refresh', 'error');
		} finally {
			loading = false;
		}
		updateStatus('Schema refreshed', 'info');
	}

	function newQuery() {
		tabsStore.newQueryTab();
		updateStatus('', 'info');
	}

	function openNewTableTab() {
		if (!selectedSchema) {
			updateStatus('Please select a schema first', 'error');
			return;
		}
		tabsStore.newCreateTableTab(selectedSchema);
	}

	function handleContextMenu(e: MouseEvent, table: string) {
		e.preventDefault();
		contextMenuTable = table;
		contextMenuPos = { x: e.clientX, y: e.clientY };
		showContextMenu = true;
	}

	function closeContextMenu() {
		showContextMenu = false;
		contextMenuTable = null;
	}

	function handleDropTable() {
		if (!contextMenuTable) return;
		openConfirmDialog('drop', contextMenuTable);
	}

	function handleTruncateTable() {
		if (!contextMenuTable) return;
		openConfirmDialog('truncate', contextMenuTable);
	}

	function openQueryFromHistory(item: QueryHistoryItem) {
		tabsStore.newQueryTabWithContent(item.query, 'History Query');
	}

	function formatHistoryTime(date: Date): string {
		const now = new Date();
		const diff = now.getTime() - date.getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'now';
		if (mins < 60) return `${mins}m`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h`;
		return date.toLocaleDateString();
	}
</script>

<aside class="bg-sidebar flex h-full w-64 min-w-64 flex-col overflow-hidden border-r">
	<!-- Header -->
	<div class="flex flex-shrink-0 items-center justify-between border-b px-3 py-2">
		<span class="text-sidebar-foreground text-sm font-medium">Explorer</span>
		<div class="flex gap-1">
			<button
				type="button"
				class="hover:bg-sidebar-accent inline-flex h-6 w-6 cursor-pointer items-center justify-center rounded-md transition-colors disabled:opacity-50"
				onclick={refresh}
				disabled={loading}
			>
				<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
			</button>
			<!-- New dropdown -->
			<button
				type="button"
				use:melt={$ddTrigger}
				class="hover:bg-sidebar-accent inline-flex h-6 w-6 cursor-pointer items-center justify-center rounded-md transition-colors"
			>
				<Plus class="h-3.5 w-3.5" />
			</button>
		</div>
	</div>

	{#if $ddOpen}
		<div
			use:melt={$ddMenu}
			class="bg-popover text-popover-foreground z-50 min-w-40 rounded-md border p-1 shadow-md"
			transition:fly={{ duration: 150, y: -10 }}
		>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={openNewTableTab}
			>
				<Table2 class="h-4 w-4" />
				New Table
			</button>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={newQuery}
			>
				<Code class="h-4 w-4" />
				New Query
			</button>
		</div>
	{/if}

	<!-- Schema Selector -->
	<div class="flex-shrink-0 border-b p-2">
		<button
			type="button"
			use:melt={$schemaTrigger}
			class="border-input bg-background hover:bg-accent/50 flex h-8 w-full cursor-pointer items-center justify-between rounded-md border px-2 text-sm transition-colors"
		>
			<div class="flex items-center gap-2">
				<Database class="text-muted-foreground h-4 w-4" />
				<span class="truncate">{selectedSchema || 'Select schema...'}</span>
			</div>
			<ChevronDown class="text-muted-foreground h-4 w-4 shrink-0" />
		</button>

		{#if $schemaOpen}
			<!-- Backdrop -->
			<button
				type="button"
				class="fixed inset-0 z-40 cursor-default"
				onclick={() => schemaOpenStore.set(false)}
			></button>

			<div
				class="bg-popover text-popover-foreground absolute left-2 right-2 z-50 mt-1 max-h-52 overflow-auto rounded-md border p-1 shadow-lg"
				style="width: calc(100% - 16px);"
				transition:fly={{ duration: 100, y: -5 }}
			>
				{#each schemas as schema (schema)}
					<button
						type="button"
						class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none {selectedSchema ===
						schema
							? 'bg-accent'
							: ''}"
						onclick={() => selectSchema(schema)}
					>
						<Database class="h-4 w-4" />
						{schema}
					</button>
				{/each}
				{#if schemas.length === 0}
					<div class="text-muted-foreground px-2 py-1.5 text-sm">No schemas</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Search -->
	<div class="flex-shrink-0 px-2 py-1.5">
		<div class="relative">
			<Search
				class="text-muted-foreground pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2"
			/>
			<input
				type="text"
				class="border-input bg-background placeholder:text-muted-foreground focus:ring-primary h-7 w-full rounded-md border pl-7 pr-2 text-sm focus:outline-none focus:ring-1"
				placeholder="Filter tables..."
				bind:value={searchQuery}
			/>
		</div>
	</div>

	<!-- Tables List -->
	<div class="min-h-0 flex-1 overflow-auto px-2 pb-2">
		{#if loadingTables}
			<div class="flex flex-col items-center justify-center py-8">
				<div
					class="border-primary h-5 w-5 animate-spin rounded-full border-2 border-t-transparent"
				></div>
				<span class="text-muted-foreground mt-2 text-xs">Loading tables...</span>
			</div>
		{:else if filteredTables.length === 0}
			<div class="text-muted-foreground flex flex-col items-center justify-center py-8 text-center">
				<Table2 class="mb-2 h-8 w-8" />
				<p class="text-sm">{searchQuery ? 'No tables match' : 'No tables'}</p>
			</div>
		{:else}
			<div class="space-y-0.5">
				{#each filteredTables as table (table)}
					<button
						type="button"
						class="hover:bg-sidebar-accent flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors {selectedItem ===
						`${selectedSchema}.${table}`
							? 'bg-sidebar-accent text-sidebar-accent-foreground'
							: 'text-muted-foreground'}"
						onclick={() => handleTableClick(table)}
						oncontextmenu={(e) => handleContextMenu(e, table)}
					>
						<Table2 class="h-3.5 w-3.5 shrink-0" />
						<span class="truncate">{table}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Query History Section -->
	<div class="flex-shrink-0 border-t">
		<button
			type="button"
			class="hover:bg-sidebar-accent flex w-full items-center justify-between px-3 py-2 transition-colors"
			onclick={() => (historyExpanded = !historyExpanded)}
		>
			<div class="flex items-center gap-2">
				{#if historyExpanded}
					<ChevronDown class="h-3.5 w-3.5" />
				{:else}
					<ChevronRight class="h-3.5 w-3.5" />
				{/if}
				<History class="h-3.5 w-3.5" />
				<span class="text-sm font-medium">Query History</span>
			</div>
			{#if getQueryHistory().length > 0}
				<span class="text-muted-foreground text-xs">{getQueryHistory().length}</span>
			{/if}
		</button>

		{#if historyExpanded}
			<div class="max-h-48 overflow-auto px-2 pb-2">
				{#if getQueryHistory().length === 0}
					<div class="text-muted-foreground py-4 text-center text-xs">No query history</div>
				{:else}
					<div class="space-y-0.5">
						{#each getQueryHistory().slice(0, 20) as item (item.id)}
							<div
								role="button"
								tabindex="0"
								class="hover:bg-sidebar-accent group flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors"
								onclick={() => openQueryFromHistory(item)}
								onkeydown={(e) => e.key === 'Enter' && openQueryFromHistory(item)}
							>
								<div class="mt-0.5 shrink-0">
									{#if item.status === 'success'}
										<div class="h-1.5 w-1.5 rounded-full bg-green-500"></div>
									{:else}
										<div class="h-1.5 w-1.5 rounded-full bg-red-500"></div>
									{/if}
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate font-mono text-xs">
										{item.query.substring(0, 40)}{item.query.length > 40 ? '...' : ''}
									</div>
									<div class="text-muted-foreground flex items-center gap-1 text-[10px]">
										<Clock class="h-2.5 w-2.5" />
										{formatHistoryTime(item.timestamp)}
										{#if item.rowCount !== undefined}
											<span>• {item.rowCount} rows</span>
										{/if}
									</div>
								</div>
								<button
									type="button"
									class="invisible shrink-0 rounded p-0.5 hover:bg-red-100 hover:text-red-600 group-hover:visible"
									onclick={(e) => {
										e.stopPropagation();
										deleteQueryHistoryItem(item.id);
									}}
									title="Delete"
								>
									<Trash2 class="h-3 w-3" />
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Context Menu -->
	{#if showContextMenu && contextMenuTable}
		<!-- Backdrop to close menu -->
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			onclick={closeContextMenu}
			aria-label="Close context menu"
		></button>
		<!-- Menu -->
		<div
			class="bg-popover text-popover-foreground fixed z-50 min-w-40 rounded-md border p-1 shadow-lg"
			style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
			transition:fly={{ duration: 100, y: -5 }}
		>
			<div class="text-muted-foreground border-b px-2 py-1 text-xs font-medium">
				{contextMenuTable}
			</div>
			<button
				type="button"
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm"
				onclick={handleTruncateTable}
			>
				<Eraser class="h-4 w-4" />
				Truncate Table
			</button>
			<button
				type="button"
				class="hover:bg-destructive/10 text-destructive flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm"
				onclick={handleDropTable}
			>
				<Trash2 class="h-4 w-4" />
				Drop Table
			</button>
		</div>
	{/if}

	<!-- Footer with table count -->
	<div class="text-muted-foreground flex-shrink-0 border-t px-3 py-1.5 text-xs">
		{tables.length} table{tables.length !== 1 ? 's' : ''}
	</div>
</aside>

<!-- Confirmation Dialog -->
{#if $dialogOpen}
	<div use:melt={$portalled}>
		<div use:melt={$overlay} class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"></div>
		<div
			use:melt={$content}
			class="bg-popover text-popover-foreground fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg"
		>
			<div class="flex items-start gap-4">
				<div
					class="bg-destructive/10 text-destructive flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
				>
					<AlertTriangle class="h-5 w-5" />
				</div>
				<div class="flex-1">
					<h2 use:melt={$title} class="text-lg font-semibold">
						{confirmAction === 'drop' ? 'Drop Table' : 'Truncate Table'}
					</h2>
					<p use:melt={$description} class="text-muted-foreground mt-2 text-sm">
						{#if confirmAction === 'drop'}
							Are you sure you want to drop <strong>"{selectedSchema}.{confirmTableName}"</strong>?
							This action cannot be undone and all data will be permanently lost.
						{:else}
							Are you sure you want to truncate <strong
								>"{selectedSchema}.{confirmTableName}"</strong
							>? All data in the table will be permanently deleted.
						{/if}
					</p>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					use:melt={$close}
					type="button"
					class="border-input bg-background hover:bg-accent hover:text-accent-foreground rounded-md border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					onclick={closeConfirmDialog}
					disabled={actionLoading}
				>
					Cancel
				</button>
				<button
					type="button"
					class="inline-flex items-center justify-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
					onclick={executeConfirmedAction}
					disabled={actionLoading}
				>
					{#if actionLoading}
						<div
							class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
						></div>
						Processing...
					{:else}
						{confirmAction === 'drop' ? 'Drop Table' : 'Truncate'}
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}
