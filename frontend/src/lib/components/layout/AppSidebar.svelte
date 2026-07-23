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
		AlertTriangle,
		History,
		Clock,
		ArrowUpRight,
		Workflow
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
	import { getContextMenuPosition } from '$lib/utils/contextMenu';
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

	function openSchemaDiagram() {
		if (!selectedSchema) {
			updateStatus('Please select a schema first', 'error');
			return;
		}
		tabsStore.newSchemaDiagramTab(selectedSchema);
		updateStatus(`Opening schema diagram for ${selectedSchema}…`, 'info');
	}

	function handleContextMenu(e: MouseEvent, table: string) {
		e.preventDefault();
		contextMenuTable = table;
		contextMenuPos = getContextMenuPosition(e, 236, 230);
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

	function openContextTable() {
		if (!contextMenuTable) return;
		const table = contextMenuTable;
		closeContextMenu();
		handleTableClick(table);
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

<aside
	class="rt-panel relative flex h-full w-[272px] max-w-[286px] min-w-[248px] flex-col overflow-hidden border-r"
>
	<div class="flex h-11 shrink-0 items-center justify-between border-b px-3">
		<div class="flex min-w-0 items-center gap-2">
			<span class="bg-primary/10 text-primary flex h-6 w-6 items-center justify-center rounded-md">
				<Database class="h-3.5 w-3.5" />
			</span>
			<div class="min-w-0">
				<div class="truncate text-xs font-bold">Database explorer</div>
				<div class="text-muted-foreground truncate text-[9px]">
					{selectedSchema || 'Select a schema'}
				</div>
			</div>
		</div>
		<div class="flex items-center gap-0.5">
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7 cursor-pointer disabled:opacity-50"
				onclick={refresh}
				disabled={loading}
				title="Refresh schemas"
			>
				<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
			</button>
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7 cursor-pointer disabled:opacity-50"
				onclick={openSchemaDiagram}
				disabled={!selectedSchema}
				title="Open schema diagram"
				aria-label="Open schema diagram"
			>
				<Workflow class="h-3.5 w-3.5" />
			</button>
			<button
				type="button"
				use:melt={$ddTrigger}
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				title="Create new"
			>
				<Plus class="h-3.5 w-3.5" />
			</button>
		</div>
	</div>

	{#if $ddOpen}
		<div
			use:melt={$ddMenu}
			class="rt-popover text-popover-foreground z-50 min-w-40 rounded-lg p-1.5"
			transition:fly={{ duration: 130, y: -6 }}
		>
			<div class="text-muted-foreground px-2 py-1 text-[10px] font-bold tracking-[0.1em] uppercase">
				Create
			</div>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
				onclick={openNewTableTab}
			>
				<Table2 class="h-3.5 w-3.5" />
				New table
			</button>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
				onclick={newQuery}
			>
				<Code class="h-3.5 w-3.5" />
				New query
			</button>
			<div class="bg-border my-1 h-px"></div>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
				onclick={openSchemaDiagram}
			>
				<Workflow class="h-3.5 w-3.5" />
				Schema diagram
			</button>
		</div>
	{/if}

	<div class="flex shrink-0 items-center gap-1.5 border-b p-2">
		<div class="relative min-w-0 flex-1">
			<Search
				class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2"
			/>
			<input
				type="text"
				class="rt-input placeholder:text-muted-foreground h-8 w-full pr-2 pl-8 text-xs"
				placeholder="Filter tables"
				bind:value={searchQuery}
			/>
		</div>
		<span
			class="text-muted-foreground flex h-8 min-w-8 items-center justify-center rounded-md border bg-[var(--surface-sunken)] px-1.5 text-[10px] font-semibold"
			title="Visible tables"
		>
			{filteredTables.length}
		</span>
	</div>

	<div class="min-h-0 flex-1 overflow-auto px-2 py-2">
		<div class="mb-1.5 flex items-center justify-between px-1">
			<span class="text-muted-foreground text-[9px] font-bold tracking-[0.13em] uppercase">
				Tables
			</span>
			<span class="text-muted-foreground text-[9px]">Right-click for actions</span>
		</div>

		{#if loadingTables}
			<div class="flex flex-col items-center justify-center py-10">
				<div
					class="border-primary h-5 w-5 animate-spin rounded-full border-2 border-t-transparent"
				></div>
				<span class="text-muted-foreground mt-2 text-[11px]">Loading tables…</span>
			</div>
		{:else if filteredTables.length === 0}
			<div
				class="text-muted-foreground mx-1 flex flex-col items-center justify-center rounded-lg border border-dashed py-10 text-center"
			>
				<span class="bg-muted mb-2 flex h-9 w-9 items-center justify-center rounded-lg">
					<Table2 class="h-4 w-4" />
				</span>
				<p class="text-xs font-medium">{searchQuery ? 'No matching tables' : 'No tables yet'}</p>
			</div>
		{:else}
			<div class="space-y-0.5">
				{#each filteredTables as table (table)}
					<button
						type="button"
						class="group relative flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs transition-colors {selectedItem ===
						`${selectedSchema}.${table}`
							? 'bg-sidebar-accent text-sidebar-accent-foreground font-semibold'
							: 'text-muted-foreground hover:text-foreground hover:bg-[var(--surface-hover)]'}"
						onclick={() => handleTableClick(table)}
						oncontextmenu={(e) => handleContextMenu(e, table)}
					>
						{#if selectedItem === `${selectedSchema}.${table}`}
							<span class="bg-primary absolute left-0 h-4 w-0.5 rounded-r-full"></span>
						{/if}
						<span
							class="flex h-5 w-5 shrink-0 items-center justify-center rounded border bg-[var(--surface-raised)]"
						>
							<Table2
								class="h-3 w-3 {selectedItem === `${selectedSchema}.${table}`
									? 'text-primary'
									: ''}"
							/>
						</span>
						<span class="truncate">{table}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<div class="max-h-[38%] shrink-0 border-t">
		<button
			type="button"
			class="flex h-9 w-full items-center justify-between px-3 transition-colors hover:bg-[var(--surface-hover)]"
			onclick={() => (historyExpanded = !historyExpanded)}
		>
			<div class="flex items-center gap-2">
				{#if historyExpanded}
					<ChevronDown class="text-muted-foreground h-3 w-3" />
				{:else}
					<ChevronRight class="text-muted-foreground h-3 w-3" />
				{/if}
				<History class="h-3.5 w-3.5" />
				<span class="text-[11px] font-bold">Query history</span>
			</div>
			{#if getQueryHistory().length > 0}
				<span
					class="bg-muted text-muted-foreground rounded-full px-1.5 py-0.5 text-[9px] font-semibold"
				>
					{getQueryHistory().length}
				</span>
			{/if}
		</button>

		{#if historyExpanded}
			<div class="max-h-40 overflow-auto px-2 pb-2">
				{#if getQueryHistory().length === 0}
					<div class="text-muted-foreground py-3 text-center text-[10px]">No recent queries</div>
				{:else}
					<div class="space-y-0.5">
						{#each getQueryHistory().slice(0, 20) as item (item.id)}
							<div
								role="button"
								tabindex="0"
								class="group flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-[var(--surface-hover)]"
								onclick={() => openQueryFromHistory(item)}
								onkeydown={(e) => e.key === 'Enter' && openQueryFromHistory(item)}
							>
								<span
									class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full {item.status === 'success'
										? 'bg-emerald-500'
										: 'bg-red-500'}"
								></span>
								<div class="min-w-0 flex-1">
									<div class="truncate font-mono text-[10px]">
										{item.query.substring(0, 40)}{item.query.length > 40 ? '…' : ''}
									</div>
									<div class="text-muted-foreground mt-0.5 flex items-center gap-1 text-[9px]">
										<Clock class="h-2.5 w-2.5" />
										{formatHistoryTime(item.timestamp)}
										{#if item.rowCount !== undefined}
											<span>· {item.rowCount} rows</span>
										{/if}
									</div>
								</div>
								<button
									type="button"
									class="text-muted-foreground invisible shrink-0 rounded p-0.5 group-hover:visible hover:bg-red-100 hover:text-red-600"
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

	<div class="relative flex h-11 shrink-0 items-center gap-2 border-t px-2">
		<button
			type="button"
			use:melt={$schemaTrigger}
			class="rt-input hover:bg-accent/40 flex h-8 min-w-0 flex-1 cursor-pointer items-center justify-between px-2.5 text-xs"
		>
			<div class="flex min-w-0 items-center gap-2">
				<Database class="text-primary h-3.5 w-3.5 shrink-0" />
				<span class="truncate">{selectedSchema || 'Select schema'}</span>
			</div>
			<ChevronDown class="text-muted-foreground h-3.5 w-3.5 shrink-0" />
		</button>
		<span class="text-muted-foreground shrink-0 text-[9px]">{tables.length} tables</span>

		{#if $schemaOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 cursor-default"
				onclick={() => schemaOpenStore.set(false)}
				aria-label="Close schema selector"
			></button>
			<div
				class="rt-popover text-popover-foreground absolute right-2 bottom-10 left-2 z-50 max-h-52 overflow-auto rounded-lg p-1.5"
				transition:fly={{ duration: 100, y: 5 }}
			>
				<div
					class="text-muted-foreground px-2 py-1 text-[10px] font-bold tracking-[0.1em] uppercase"
				>
					Schemas
				</div>
				{#each schemas as schema (schema)}
					<button
						type="button"
						class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs outline-none {selectedSchema ===
						schema
							? 'bg-accent text-accent-foreground font-semibold'
							: ''}"
						onclick={() => selectSchema(schema)}
					>
						<Database class="h-3.5 w-3.5" />
						{schema}
					</button>
				{/each}
				{#if schemas.length === 0}
					<div class="text-muted-foreground px-2 py-2 text-xs">No schemas</div>
				{/if}
			</div>
		{/if}
	</div>

	{#if showContextMenu && contextMenuTable}
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			onclick={closeContextMenu}
			aria-label="Close context menu"
		></button>
		<div
			class="rt-context-menu fixed z-50"
			style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
			transition:fly={{ duration: 100, y: -5 }}
			role="menu"
			data-context-menu="table"
		>
			<div class="rt-context-header">
				<span class="rt-context-header-icon">
					<Table2 class="h-3.5 w-3.5" />
				</span>
				<span class="min-w-0">
					<span class="rt-context-title">{contextMenuTable}</span>
					<span class="rt-context-meta">{selectedSchema} · table actions</span>
				</span>
			</div>
			<button type="button" class="rt-context-item" onclick={openContextTable} role="menuitem">
				<span class="rt-context-item-icon">
					<ArrowUpRight class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Open table</span>
					<span class="rt-context-meta">Browse structure and data</span>
				</span>
			</button>
			<div class="rt-context-divider"></div>
			<button
				type="button"
				class="rt-context-item rt-context-item--warning"
				onclick={handleTruncateTable}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Eraser class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Clear all rows</span>
					<span class="rt-context-meta">Keep the table structure</span>
				</span>
			</button>
			<button
				type="button"
				class="rt-context-item rt-context-item--danger"
				onclick={handleDropTable}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Trash2 class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Delete table</span>
					<span class="rt-context-meta">Remove structure and data</span>
				</span>
			</button>
		</div>
	{/if}
</aside>

<!-- Confirmation Dialog -->
{#if $dialogOpen}
	<div use:melt={$portalled}>
		<div use:melt={$overlay} class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"></div>
		<div
			use:melt={$content}
			class="bg-popover text-popover-foreground fixed top-1/2 left-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg"
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
