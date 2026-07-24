<script lang="ts">
	import { onMount } from 'svelte';
	import { writable } from 'svelte/store';
	import {
		GetCapabilities,
		GetSchemas,
		GetCollections,
		GetDatabaseObjects,
		DropTable,
		TruncateTable
	} from '$lib/wailsjs/go/db/Service';
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
		Workflow,
		Boxes,
		Braces,
		Copy,
		FileCode2,
		KeyRound,
		Layers3,
		ListOrdered,
		ListTree,
		PanelsTopLeft,
		Puzzle,
		Zap,
		Import
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
	import { ClipboardSetText } from '$lib/wailsjs/runtime/runtime';
	import {
		countGroupedObjects,
		databaseObjectKey,
		databaseObjectQualifiedName,
		groupDatabaseObjects
	} from '$lib/database/objects';
	import { createServiceError } from '$lib/errors/service';
	import ObjectChangeDialog from '$lib/components/database/ObjectChangeDialog.svelte';
	import type { StructuralChangeIntent } from '$lib/database/changeTemplates';

	interface Props {
		connectionId: string;
		onTableClick: (schema: string, table: string) => void;
	}

	let { connectionId, onTableClick }: Props = $props();

	let schemas: string[] = $state([]);
	let selectedSchema = $state<string>('');
	let tables = $state<string[]>([]);
	let objects = $state<database.DatabaseObject[]>([]);
	let capabilities = $state<database.Capabilities | null>(null);
	let loading = $state(false);
	let loadingTables = $state(false);
	let selectedItem = $state<string | null>(null);
	let searchQuery = $state('');
	let historyExpanded = $state(true);
	let changeIntent = $state<StructuralChangeIntent | null>(null);
	let expandedGroups = $state<Record<string, boolean>>({
		tables: true,
		views: true,
		'materialized-views': true,
		functions: false,
		procedures: false,
		triggers: false,
		sequences: false,
		types: false,
		constraints: false,
		extensions: false,
		indexes: false
	});

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

	const objectGroups = $derived(groupDatabaseObjects(objects, searchQuery));
	const visibleObjectCount = $derived(countGroupedObjects(objectGroups));

	// Context menu state
	let contextMenuObject = $state<database.DatabaseObject | null>(null);
	const contextMenuTable = $derived(
		contextMenuObject?.reference.kind === 'table' ? contextMenuObject.reference.name : null
	);
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
				const response = await DropTable(connectionId, table);
				if (response.errors?.length) {
					updateStatus(response.errors[0].detail, 'error');
				} else {
					updateStatus(`DROP TABLE ${selectedSchema}.${tableName}`, 'info');
					const tabId = tabsStore.tabs.find(
						(t) =>
							t.connectionId === connectionId &&
							t.schema === selectedSchema &&
							t.table === tableName
					)?.id;
					if (tabId) {
						tabsStore.closeTab(tabId);
					}
					tables = tables.filter((t) => t !== tableName);
					objects = objects.filter(
						(object) => object.reference.kind !== 'table' || object.reference.name !== tableName
					);
				}
			} catch (e: any) {
				updateStatus(e?.message ?? 'Failed to drop table', 'error');
			}
		} else if (action === 'truncate') {
			try {
				const table = new database.Table({ Schema: selectedSchema, Name: tableName });
				const response = await TruncateTable(connectionId, table);
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
				objects = [
					...objects,
					new database.DatabaseObject({
						reference: new database.ObjectReference({
							kind: 'table',
							schema: selectedSchema,
							name: tableName
						}),
						displayName: tableName,
						canOpenData: true,
						canManage: true,
						properties: []
					})
				];
			}
		});
		setSidebarRemoveTable((tableName: string) => {
			tables = tables.filter((t) => t !== tableName);
			objects = objects.filter(
				(object) => object.reference.kind !== 'table' || object.reference.name !== tableName
			);
		});

		// Load initial data
		loadSchemas();

		// Listen for connection switch events
		const handleConnectionSwitch = () => {
			selectedSchema = ''; // Reset schema selection
			loadSchemas(); // Reload schemas for new connection
		};
		const handleObjectsChanged = (event: Event) => {
			const detail = (event as CustomEvent<{ connectionId?: string; schema?: string }>).detail;
			if (
				detail?.connectionId === connectionId &&
				(!detail.schema || detail.schema === selectedSchema)
			) {
				void loadTables();
			}
		};
		window.addEventListener('connection-switched', handleConnectionSwitch);
		window.addEventListener('database-objects-changed', handleObjectsChanged);

		// Cleanup
		return () => {
			setSidebarRefresh(null);
			setSidebarAddTable(null);
			setSidebarRemoveTable(null);
			window.removeEventListener('connection-switched', handleConnectionSwitch);
			window.removeEventListener('database-objects-changed', handleObjectsChanged);
		};
	});

	async function loadSchemas() {
		loading = true;
		try {
			const [capabilityResponse, schemaResponse] = await Promise.all([
				GetCapabilities(connectionId),
				GetSchemas(connectionId)
			]);
			if (capabilityResponse.errors?.length) {
				throw createServiceError(
					capabilityResponse.errors[0],
					'Could not load driver capabilities'
				);
			}
			if (schemaResponse.errors?.length) {
				throw createServiceError(schemaResponse.errors[0], 'Could not load namespaces');
			}
			capabilities = capabilityResponse.data || null;
			schemas = schemaResponse.data || [];
			// Auto-select first schema
			if (schemas.length > 0 && !selectedSchema) {
				selectedSchema =
					schemas.find((schema) => schema === 'public') ||
					schemas.find((schema) => schema === 'main') ||
					schemas[0];
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
			const response = await GetDatabaseObjects(
				connectionId,
				new database.ObjectFilter({
					schema: selectedSchema,
					kinds: [],
					search: ''
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load database objects');
			}
			objects = response.data || [];
			tables = objects
				.filter((object) => object.reference.kind === 'table')
				.map((object) => object.reference.name)
				.sort((left, right) => left.localeCompare(right));
		} catch (e: any) {
			// A legacy or restricted driver can still expose its tables even when
			// richer object metadata is unavailable.
			try {
				const fallback = await GetCollections(connectionId, [selectedSchema]);
				if (fallback.errors?.length) {
					throw createServiceError(fallback.errors[0], 'Could not load tables');
				}
				tables = (fallback.data || []).sort((left, right) => left.localeCompare(right));
				objects = tables.map(
					(table) =>
						new database.DatabaseObject({
							reference: new database.ObjectReference({
								kind: 'table',
								schema: selectedSchema,
								name: table
							}),
							displayName: table,
							canOpenData: true,
							canManage: false,
							properties: []
						})
				);
				updateStatus(`${e?.message || 'Object metadata unavailable'} Showing tables only.`, 'warn');
			} catch (fallbackError: any) {
				objects = [];
				tables = [];
				updateStatus(fallbackError?.message ?? 'Failed to load database objects', 'error');
			}
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
		const object = objects.find(
			(candidate) => candidate.reference.kind === 'table' && candidate.reference.name === table
		);
		selectedItem = object
			? databaseObjectKey(object.reference)
			: `table:${selectedSchema}:${table}`;
		onTableClick(selectedSchema, table);
		updateStatus('', 'info');
	}

	function handleObjectClick(object: database.DatabaseObject) {
		selectedItem = databaseObjectKey(object.reference);
		if (object.reference.kind === 'table') {
			onTableClick(object.reference.schema || selectedSchema, object.reference.name);
		} else {
			tabsStore.newDatabaseObjectTab(connectionId, object.reference);
		}
		updateStatus('', 'info');
	}

	function toggleObjectGroup(groupId: string) {
		expandedGroups[groupId] = !expandedGroups[groupId];
	}

	function isObjectGroupExpanded(groupId: string): boolean {
		return Boolean(searchQuery.trim()) || expandedGroups[groupId] !== false;
	}

	function objectSecondaryLabel(object: database.DatabaseObject): string {
		const reference = object.reference;
		if (reference.parentName) {
			return reference.parentSchema
				? `${reference.parentSchema}.${reference.parentName}`
				: reference.parentName;
		}
		if (reference.signature) return reference.signature;
		return object.description || '';
	}

	async function refresh() {
		// Refresh schemas first, then reload tables
		loading = true;
		try {
			const response = await GetSchemas(connectionId);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not refresh namespaces');
			}
			schemas = response.data || [];
			// Keep the current schema if it still exists, otherwise select first
			if (selectedSchema && schemas.includes(selectedSchema)) {
				await loadTables();
			} else if (schemas.length > 0) {
				selectedSchema = schemas[0];
				await loadTables();
			} else {
				tables = [];
				objects = [];
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to refresh', 'error');
		} finally {
			loading = false;
		}
		updateStatus('Schema refreshed', 'info');
	}

	function newQuery() {
		tabsStore.newQueryTab(connectionId);
		updateStatus('', 'info');
	}

	function openImportData() {
		ddOpen.set(false);
		window.dispatchEvent(new CustomEvent('open-import-data'));
	}

	function openNewTableTab() {
		if (!selectedSchema) {
			updateStatus('Please select a schema first', 'error');
			return;
		}
		tabsStore.newCreateTableTab(connectionId, selectedSchema);
	}

	function openSchemaDiagram() {
		if (!selectedSchema) {
			updateStatus('Please select a schema first', 'error');
			return;
		}
		tabsStore.newSchemaDiagramTab(connectionId, selectedSchema);
		updateStatus(`Opening schema diagram for ${selectedSchema}…`, 'info');
	}

	function openStructuralChange(intent: StructuralChangeIntent) {
		ddOpen.set(false);
		changeIntent = intent;
	}

	async function handleStructuralChangeApplied() {
		changeIntent = null;
		await loadTables();
	}

	function handleContextMenu(e: MouseEvent, object: database.DatabaseObject) {
		e.preventDefault();
		contextMenuObject = object;
		contextMenuPos = getContextMenuPosition(e, 248, object.reference.kind === 'table' ? 318 : 224);
		showContextMenu = true;
	}

	function closeContextMenu() {
		showContextMenu = false;
		contextMenuObject = null;
	}

	function handleDropTable() {
		if (!contextMenuTable) return;
		openConfirmDialog('drop', contextMenuTable);
	}

	function handleTruncateTable() {
		if (!contextMenuTable) return;
		openConfirmDialog('truncate', contextMenuTable);
	}

	function openContextObject() {
		if (!contextMenuObject) return;
		const object = contextMenuObject;
		closeContextMenu();
		handleObjectClick(object);
	}

	async function copyContextName(qualified = false) {
		if (!contextMenuObject) return;
		const value = qualified
			? databaseObjectQualifiedName(contextMenuObject.reference)
			: contextMenuObject.reference.name;
		await ClipboardSetText(value);
		updateStatus(`Copied ${qualified ? 'qualified ' : ''}object name`, 'success');
		closeContextMenu();
	}

	function openQueryFromHistory(item: QueryHistoryItem) {
		tabsStore.newQueryTabWithContent(connectionId, item.query, 'History Query');
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
	aria-label="Database explorer"
>
	<div class="flex h-11 shrink-0 items-center justify-between border-b px-3">
		<div class="flex min-w-0 items-center gap-2">
			<span class="bg-primary/10 text-primary flex h-6 w-6 items-center justify-center rounded-md">
				<Database class="h-3.5 w-3.5" />
			</span>
			<div class="min-w-0">
				<div class="truncate text-xs font-bold">Database explorer</div>
				<div class="text-muted-foreground truncate text-[9px]">
					{capabilities?.displayName || 'Database'} · {selectedSchema || 'Select a namespace'}
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
			{#if capabilities?.manageViews}
				<button
					type="button"
					use:melt={$ddItem}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => openStructuralChange('create-view')}
				>
					<PanelsTopLeft class="h-3.5 w-3.5" />
					New view
				</button>
			{/if}
			{#if capabilities?.manageViews && capabilities?.materializedViews}
				<button
					type="button"
					use:melt={$ddItem}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => openStructuralChange('create-materialized-view')}
				>
					<Layers3 class="h-3.5 w-3.5" />
					New materialized view
				</button>
			{/if}
			{#if capabilities?.manageRoutines && capabilities?.functions}
				<button
					type="button"
					use:melt={$ddItem}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => openStructuralChange('create-function')}
				>
					<FileCode2 class="h-3.5 w-3.5" />
					New function
				</button>
			{/if}
			{#if capabilities?.manageRoutines && capabilities?.procedures}
				<button
					type="button"
					use:melt={$ddItem}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => openStructuralChange('create-procedure')}
				>
					<FileCode2 class="h-3.5 w-3.5" />
					New procedure
				</button>
			{/if}
			{#if capabilities?.manageTriggers && capabilities?.triggers}
				<button
					type="button"
					use:melt={$ddItem}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => openStructuralChange('create-trigger')}
				>
					<Zap class="h-3.5 w-3.5" />
					New trigger
				</button>
			{/if}
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
				onclick={newQuery}
			>
				<Code class="h-3.5 w-3.5" />
				New query
			</button>
			<button
				type="button"
				use:melt={$ddItem}
				class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
				onclick={openImportData}
			>
				<Import class="h-3.5 w-3.5" />
				Import CSV / JSON
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
				placeholder="Filter database objects"
				bind:value={searchQuery}
			/>
		</div>
		<span
			class="text-muted-foreground flex h-8 min-w-8 items-center justify-center rounded-md border bg-[var(--surface-sunken)] px-1.5 text-[10px] font-semibold"
			title="Visible database objects"
		>
			{visibleObjectCount}
		</span>
	</div>

	<div class="min-h-0 flex-1 overflow-auto px-2 py-1.5">
		{#if loadingTables}
			<div class="flex flex-col items-center justify-center py-10">
				<div
					class="border-primary h-5 w-5 animate-spin rounded-full border-2 border-t-transparent"
				></div>
				<span class="text-muted-foreground mt-2 text-[11px]">Loading database objects…</span>
				<span class="text-muted-foreground mt-0.5 text-[9px]">{selectedSchema}</span>
			</div>
		{:else if objectGroups.length === 0}
			<div
				class="text-muted-foreground mx-1 flex flex-col items-center justify-center rounded-lg border border-dashed py-10 text-center"
			>
				<span class="bg-muted mb-2 flex h-9 w-9 items-center justify-center rounded-lg">
					<Boxes class="h-4 w-4" />
				</span>
				<p class="text-xs font-medium">
					{searchQuery ? 'No matching objects' : 'No database objects'}
				</p>
				<p class="mt-1 max-w-44 text-[9px]">
					{searchQuery ? 'Try a different name or type.' : 'This namespace is empty.'}
				</p>
			</div>
		{:else}
			<div class="space-y-1">
				{#each objectGroups as group (group.id)}
					<section class="overflow-hidden rounded-md">
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground flex h-7 w-full cursor-pointer items-center gap-1.5 rounded-md px-1.5 text-left transition-colors hover:bg-[var(--surface-hover)]"
							onclick={() => toggleObjectGroup(group.id)}
							aria-expanded={isObjectGroupExpanded(group.id)}
						>
							{#if isObjectGroupExpanded(group.id)}
								<ChevronDown class="h-3 w-3 shrink-0" />
							{:else}
								<ChevronRight class="h-3 w-3 shrink-0" />
							{/if}
							{#if group.id === 'tables'}
								<Table2 class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'views'}
								<PanelsTopLeft class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'materialized-views'}
								<Layers3 class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'functions' || group.id === 'procedures'}
								<FileCode2 class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'triggers'}
								<Zap class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'sequences'}
								<ListOrdered class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'types'}
								<Braces class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'constraints'}
								<KeyRound class="h-3.5 w-3.5 shrink-0" />
							{:else if group.id === 'extensions'}
								<Puzzle class="h-3.5 w-3.5 shrink-0" />
							{:else}
								<ListTree class="h-3.5 w-3.5 shrink-0" />
							{/if}
							<span
								class="min-w-0 flex-1 truncate text-[9px] font-bold tracking-[0.09em] uppercase"
							>
								{group.label}
							</span>
							<span class="text-[9px] tabular-nums">{group.objects.length}</span>
						</button>

						{#if isObjectGroupExpanded(group.id)}
							<div class="mt-0.5 space-y-px pl-3">
								{#each group.objects as object (databaseObjectKey(object.reference))}
									{@const objectKey = databaseObjectKey(object.reference)}
									{@const secondary = objectSecondaryLabel(object)}
									<button
										type="button"
										class="group relative flex min-h-8 w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-left transition-colors {selectedItem ===
										objectKey
											? 'bg-sidebar-accent text-sidebar-accent-foreground'
											: 'text-muted-foreground hover:text-foreground hover:bg-[var(--surface-hover)]'}"
										onclick={() => handleObjectClick(object)}
										oncontextmenu={(event) => handleContextMenu(event, object)}
										title={databaseObjectQualifiedName(object.reference)}
									>
										{#if selectedItem === objectKey}
											<span
												class="bg-primary absolute top-1.5 bottom-1.5 left-0 w-0.5 rounded-r-full"
											></span>
										{/if}
										<span
											class="flex h-5 w-5 shrink-0 items-center justify-center rounded border bg-[var(--surface-raised)]"
										>
											{#if object.reference.kind === 'table'}
												<Table2 class="h-3 w-3" />
											{:else if object.reference.kind === 'view'}
												<PanelsTopLeft class="h-3 w-3" />
											{:else if object.reference.kind === 'materialized_view'}
												<Layers3 class="h-3 w-3" />
											{:else if object.reference.kind === 'function' || object.reference.kind === 'procedure'}
												<FileCode2 class="h-3 w-3" />
											{:else if object.reference.kind === 'trigger'}
												<Zap class="h-3 w-3" />
											{:else if object.reference.kind === 'sequence'}
												<ListOrdered class="h-3 w-3" />
											{:else if object.reference.kind === 'type' || object.reference.kind === 'enum' || object.reference.kind === 'domain'}
												<Braces class="h-3 w-3" />
											{:else if object.reference.kind === 'constraint'}
												<KeyRound class="h-3 w-3" />
											{:else if object.reference.kind === 'extension'}
												<Puzzle class="h-3 w-3" />
											{:else}
												<ListTree class="h-3 w-3" />
											{/if}
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[10px] font-medium"
												>{object.displayName}</span
											>
											{#if secondary}
												<span class="text-muted-foreground block truncate text-[8px]"
													>{secondary}</span
												>
											{/if}
										</span>
									</button>
								{/each}
							</div>
						{/if}
					</section>
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
		<span class="text-muted-foreground shrink-0 text-[9px]">{objects.length} objects</span>

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

	{#if showContextMenu && contextMenuObject}
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
			data-context-menu="database-object"
		>
			<div class="rt-context-header">
				<span class="rt-context-header-icon">
					{#if contextMenuObject.reference.kind === 'table'}
						<Table2 class="h-3.5 w-3.5" />
					{:else}
						<Boxes class="h-3.5 w-3.5" />
					{/if}
				</span>
				<span class="min-w-0">
					<span class="rt-context-title">{contextMenuObject.displayName}</span>
					<span class="rt-context-meta"
						>{contextMenuObject.reference.schema || selectedSchema} · {contextMenuObject.reference.kind.replaceAll(
							'_',
							' '
						)}</span
					>
				</span>
			</div>
			<button type="button" class="rt-context-item" onclick={openContextObject} role="menuitem">
				<span class="rt-context-item-icon">
					<ArrowUpRight class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">
						{contextMenuObject.reference.kind === 'table' ? 'Open table' : 'Inspect object'}
					</span>
					<span class="rt-context-meta">
						{contextMenuObject.reference.kind === 'table'
							? 'Browse structure and data'
							: 'View definition and relationships'}
					</span>
				</span>
			</button>
			<div class="rt-context-divider"></div>
			<button
				type="button"
				class="rt-context-item"
				onclick={() => void copyContextName(false)}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Copy class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Copy name</span>
					<span class="rt-context-meta">{contextMenuObject.reference.name}</span>
				</span>
			</button>
			<button
				type="button"
				class="rt-context-item"
				onclick={() => void copyContextName(true)}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Copy class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Copy qualified name</span>
					<span class="rt-context-meta">
						{databaseObjectQualifiedName(contextMenuObject.reference)}
					</span>
				</span>
			</button>
			{#if contextMenuObject.reference.kind === 'table'}
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
			{/if}
		</div>
	{/if}
</aside>

<ObjectChangeDialog
	open={changeIntent !== null}
	{connectionId}
	intent={changeIntent}
	{capabilities}
	reference={null}
	table={new database.Table({ Schema: selectedSchema, Name: '' })}
	onClose={() => (changeIntent = null)}
	onApplied={handleStructuralChangeApplied}
/>

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
