<script lang="ts">
	import AppHeader from '$lib/components/layout/AppHeader.svelte';
	import AppSidebar from '$lib/components/layout/AppSidebar.svelte';
	import AppStatusBar from '$lib/components/layout/AppStatusBar.svelte';
	import CreateTableContent from '$lib/components/CreateTableContent.svelte';
	import { createTabs, melt } from '@melt-ui/svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import {
		hasChanges,
		hasCreateTableChanges,
		discardStagedChanges,
		stagedChanges,
		createTableState
	} from '$lib/stores/staged.svelte';
	import {
		updateStatus,
		updateDatabaseInfo,
		getConsoleLogs,
		getShowConsole,
		toggleConsole,
		clearConsoleLogs
	} from '$lib/stores/status.svelte';
	import {
		GetDatabaseInfo,
		InsertRow,
		UpdateRow,
		DeleteRow,
		CreateTable
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { onMount } from 'svelte';
	import { writable } from 'svelte/store';
	import {
		Save,
		RotateCcw,
		X,
		LayoutGrid,
		Table2,
		Code,
		ChevronUp,
		ChevronDown,
		Terminal
	} from 'lucide-svelte';

	// Import content components
	import TableContent from '$lib/components/TableContent.svelte';
	import QueryEditorContent from '$lib/components/QueryEditorContent.svelte';

	const tabs = $derived(tabsStore.tabs);
	const activeTabId = $derived(tabsStore.activeTabId);
	const activeTab = $derived(tabsStore.activeTab);
	const hasUnsavedChanges = $derived(
		hasChanges() ||
			(tabsStore.activeTab?.kind === 'createTable' && createTableState.submit !== null)
	);
	const consoleLogs = $derived(getConsoleLogs());
	const showConsole = $derived(getShowConsole());

	const tabValueStore = writable(tabsStore.activeTabId ?? '');

	// Melt-UI Tabs
	const {
		elements: { root: tabsRoot, list: tabsList, trigger: tabTrigger, content: tabContent }
	} = createTabs({
		value: tabValueStore,
		autoSet: false,
		defaultValue: tabsStore.activeTabId ?? '',
		onValueChange: ({ next }) => {
			if (next && next !== tabsStore.activeTabId) {
				tabsStore.setActive(next);
			}
			return next;
		}
	});

	// Sync store -> melt-ui
	$effect(() => {
		const id = activeTabId;
		if (id) {
			tabValueStore.set(id);
		}
	});

	$effect(() => {
		if (!tabsStore.activeTabId && tabsStore.tabs.length > 0) {
			tabsStore.setActive(tabsStore.tabs[0].id);
		}
	});

	onMount(() => {
		GetDatabaseInfo().then((res) => {
			if (res.errors?.length > 0) {
				updateStatus(res.errors[0].detail, 'error');
				return;
			}
			updateDatabaseInfo(res.data);
			updateStatus('', 'info');
		});

		// Keyboard shortcuts
		function handleKeydown(e: KeyboardEvent) {
			const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
			const modifier = isMac ? e.metaKey : e.ctrlKey;

			if (modifier && e.key === 's') {
				e.preventDefault();
				if (hasUnsavedChanges) {
					applyChanges();
				}
			}

			if (modifier && e.key === 'w') {
				e.preventDefault();
				if (activeTabId) {
					tabsStore.closeTab(activeTabId);
				}
			}
		}

		document.addEventListener('keydown', handleKeydown);

		return () => {
			document.removeEventListener('keydown', handleKeydown);
		};
	});

	function handleTableClick(schema: string, table: string) {
		const existingTab = tabsStore.findTableTab(schema, table);
		if (existingTab) {
			tabsStore.setActive(existingTab.id);
		} else {
			tabsStore.newTableTab(schema, table);
		}
		updateStatus('', 'info');
	}

	async function applyChanges() {
		if (!tabsStore.activeTab) {
			updateStatus('No active tab', 'error');
			return;
		}

		// Handle createTable tab - use registered callback
		if (tabsStore.activeTab.kind === 'createTable') {
			if (createTableState.submit) {
				await createTableState.submit();
			} else {
				updateStatus('Create table form not ready', 'error');
			}
			return;
		}

		if (tabsStore.activeTab.kind !== 'table') {
			updateStatus('No active table selected', 'error');
			return;
		}

		updateStatus('Applying changes...', 'info');

		const table = new database.Table();
		table.Schema = tabsStore.activeTab.schema;
		table.Name = tabsStore.activeTab.table;

		const primaryKey = 'id';

		try {
			for (const row of stagedChanges.data.added) {
				const cleanData: Record<string, any> = {};
				for (const [key, value] of Object.entries(row)) {
					if (key !== '_isNew' && !key.startsWith('temp_')) {
						cleanData[key] = value;
					}
				}
				const result = await InsertRow(table, cleanData);
				if (result.errors?.length) {
					throw new Error(result.errors[0].detail);
				}
			}

			for (const row of stagedChanges.data.updated) {
				const result = await UpdateRow(table, row, primaryKey);
				if (result.errors?.length) {
					throw new Error(result.errors[0].detail);
				}
			}

			for (const row of stagedChanges.data.deleted) {
				const primaryValue = row[primaryKey];
				if (primaryValue !== undefined) {
					const result = await DeleteRow(table, primaryKey, primaryValue);
					if (result.errors?.length) {
						throw new Error(result.errors[0].detail);
					}
				}
			}

			discardStagedChanges();
			updateStatus('Changes applied successfully', 'info');

			const currentTab = tabsStore.activeTab;
			if (currentTab) {
				tabsStore.updateTab(currentTab.id, { ...currentTab });
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to apply changes', 'error');
		}
	}

	// Types that require a size/length parameter
	const typesWithSize = ['varchar', 'char', 'numeric', 'decimal'];
	function typeNeedsSize(type: string): boolean {
		return typesWithSize.some((t) => type.toLowerCase().startsWith(t));
	}

	async function applyCreateTable() {
		const { schema, tableName, columns } = stagedChanges.createTable;

		if (!tableName.trim()) {
			updateStatus('Table name is required', 'error');
			return;
		}

		const validColumns = columns.filter((c) => c.name.trim() && c.type);
		if (validColumns.length === 0) {
			updateStatus('At least one column with name and type is required', 'error');
			return;
		}

		updateStatus('Creating table...', 'info');

		try {
			const table = new database.Table({ schema, name: tableName.trim() });
			const columnDefs = validColumns.map((c) => {
				let finalType = c.type;
				if (typeNeedsSize(c.type) && c.size) {
					finalType = `${c.type}(${c.size})`;
				}
				return {
					name: c.name.trim(),
					type: finalType,
					nullable: c.nullable,
					default: c.defaultValue,
					primaryKey: c.primaryKey,
					unique: c.unique
				};
			});

			const response = await CreateTable(table, columnDefs);
			if (response.errors?.length) {
				updateStatus(response.errors[0].detail, 'error');
			} else {
				updateStatus(`Table "${tableName}" created successfully`, 'info');
				// Close this tab and open the new table
				const currentTabId = tabsStore.activeTabId;
				if (currentTabId) {
					tabsStore.closeTab(currentTabId);
				}
				tabsStore.newTableTab(schema, tableName.trim());
				discardStagedChanges();
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to create table', 'error');
		}
	}

	function discardChanges() {
		updateStatus('Discarding changes...', 'info');
		discardStagedChanges();
	}
</script>

<div class="bg-background flex h-screen flex-col">
	<!-- Header -->
	<AppHeader />

	<!-- Main Content -->
	<div class="flex flex-1 overflow-hidden">
		<!-- Sidebar -->
		<AppSidebar onTableClick={handleTableClick} />

		<!-- Workspace -->
		<main class="flex min-h-0 flex-1 flex-col overflow-hidden">
			{#if tabs.length > 0}
				<!-- Tab Bar -->
				<div use:melt={$tabsRoot} class="flex min-h-0 flex-1 flex-col">
					<div class="bg-muted/30 flex items-center justify-between border-b px-2">
						<div class="min-w-0 flex-1 overflow-x-auto">
							<div use:melt={$tabsList} class="inline-flex h-10 items-center gap-0 bg-transparent">
								{#each tabs as tab (tab.id)}
									<div
										use:melt={$tabTrigger(tab.id)}
										class="data-[state=active]:bg-background text-muted-foreground data-[state=active]:text-foreground data-[state=active]:border-primary group inline-flex h-9 cursor-pointer items-center gap-2 rounded-t-md border-b-2 border-transparent px-3 text-sm font-medium transition-colors"
									>
										{#if tab.kind === 'table'}
											<Table2 class="h-3.5 w-3.5" />
										{:else}
											<Code class="h-3.5 w-3.5" />
										{/if}
										<span class="max-w-32 truncate">{tab.title}</span>
										<button
											type="button"
											class="hover:bg-muted ml-1 rounded p-0.5 opacity-0 group-hover:opacity-100"
											onclick={(e) => {
												e.stopPropagation();
												tabsStore.closeTab(tab.id);
											}}
										>
											<X class="h-3 w-3" />
										</button>
									</div>
								{/each}
							</div>
						</div>

						<!-- Actions -->
						<div class="flex flex-shrink-0 items-center gap-2 py-1 pl-2">
							<button
								class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border bg-transparent px-2 text-xs transition-colors disabled:pointer-events-none disabled:opacity-50"
								disabled={!hasUnsavedChanges}
								onclick={applyChanges}
							>
								<Save class="h-3.5 w-3.5" />
								Apply
							</button>
							<button
								class="hover:bg-accent hover:text-accent-foreground inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-2 text-xs transition-colors disabled:pointer-events-none disabled:opacity-50"
								disabled={!hasUnsavedChanges}
								onclick={discardChanges}
							>
								<RotateCcw class="h-3.5 w-3.5" />
								Discard
							</button>
						</div>
					</div>

					<!-- Tab Content -->
					{#each tabs as tab (tab.id)}
						<div
							use:melt={$tabContent(tab.id)}
							class="min-h-0 flex-1 p-0 data-[state=active]:flex data-[state=active]:flex-col"
						>
							{#if tab.kind === 'table'}
								<TableContent />
							{:else if tab.kind === 'query'}
								<QueryEditorContent />
							{:else if tab.kind === 'createTable'}
								<CreateTableContent />
							{:else}
								<div class="text-muted-foreground flex flex-1 items-center justify-center">
									Select a table or create a new query
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<!-- Empty State -->
				<div class="flex flex-1 items-center justify-center">
					<div class="text-center">
						<LayoutGrid class="text-muted-foreground mx-auto mb-4 h-12 w-12" />
						<h2 class="text-lg font-medium">No tables open</h2>
						<p class="text-muted-foreground mt-1 text-sm">
							Select a table from the sidebar to get started
						</p>
					</div>
				</div>
			{/if}
		</main>
	</div>

	<!-- Console Panel -->
	<div class="bg-muted/50 flex flex-col border-t" style={showConsole ? 'height: 200px' : ''}>
		<div class="flex items-center justify-between px-3 py-1.5">
			<div
				class="hover:bg-muted/50 flex flex-1 cursor-pointer items-center gap-2 rounded px-1 py-0.5"
				role="button"
				tabindex="0"
				onclick={toggleConsole}
				onkeydown={(e) => e.key === 'Enter' && toggleConsole()}
			>
				<Terminal class="h-3.5 w-3.5" />
				<span class="text-xs font-medium">Console</span>
				{#if consoleLogs.length > 0}
					<span class="bg-muted-foreground/20 rounded-full px-1.5 py-0.5 text-[10px]">
						{consoleLogs.length}
					</span>
				{/if}
				{#if !showConsole && consoleLogs.length > 0}
					<span class="text-muted-foreground max-w-xs truncate text-xs">
						— {consoleLogs[consoleLogs.length - 1]?.message}
					</span>
				{/if}
				{#if showConsole}
					<ChevronDown class="h-4 w-4" />
				{:else}
					<ChevronUp class="h-4 w-4" />
				{/if}
			</div>
			{#if showConsole && consoleLogs.length > 0}
				<button
					type="button"
					class="text-muted-foreground hover:text-foreground text-xs"
					onclick={clearConsoleLogs}
				>
					Clear
				</button>
			{/if}
		</div>

		{#if showConsole}
			<div class="flex-1 overflow-auto border-t p-2">
				{#if consoleLogs.length === 0}
					<p class="text-muted-foreground text-xs">No messages</p>
				{:else}
					{#each consoleLogs as log}
						<div class="mb-1 flex gap-2 font-mono text-xs">
							<span class="text-muted-foreground flex-shrink-0">
								{log.timestamp.toLocaleTimeString()}
							</span>
							<span
								class={log.level === 'error'
									? 'text-red-500'
									: log.level === 'warn'
										? 'text-yellow-500'
										: 'text-foreground'}
							>
								{log.message}
							</span>
						</div>
					{/each}
				{/if}
			</div>
		{/if}
	</div>

	<!-- Status Bar -->
	<AppStatusBar />
</div>
