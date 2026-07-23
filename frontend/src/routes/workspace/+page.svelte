<script lang="ts">
	import AppHeader from '$lib/components/layout/AppHeader.svelte';
	import AppSidebar from '$lib/components/layout/AppSidebar.svelte';
	import AppStatusBar from '$lib/components/layout/AppStatusBar.svelte';
	import CreateTableContent from '$lib/components/CreateTableContent.svelte';
	import SchemaDiagramContent from '$lib/components/SchemaDiagramContent.svelte';
	import ConnectionManagerModal from '$lib/components/ConnectionManagerModal.svelte';
	import { createTabs, melt } from '@melt-ui/svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import {
		hasChanges,
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
	import { onMount, tick } from 'svelte';
	import { writable } from 'svelte/store';
	import {
		Save,
		RotateCcw,
		X,
		Table2,
		Code,
		ChevronUp,
		ChevronDown,
		Terminal,
		Workflow,
		CircleCheck,
		CircleAlert,
		CircleX,
		Info,
		Trash2
	} from 'lucide-svelte';

	// Import content components
	import TableContent from '$lib/components/TableContent.svelte';
	import QueryEditorContent from '$lib/components/QueryEditorContent.svelte';
	import ConnectionPanel from '$lib/components/layout/ConnectionPanel.svelte';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { goto } from '$app/navigation';

	const tabs = $derived(tabsStore.tabs);
	const activeTabId = $derived(tabsStore.activeTabId);
	const activeTab = $derived(tabsStore.activeTab);
	const hasUnsavedChanges = $derived(
		hasChanges() ||
			(tabsStore.activeTab?.kind === 'createTable' && createTableState.submit !== null)
	);
	const showChangeActions = $derived(
		activeTab?.kind === 'table' || activeTab?.kind === 'createTable'
	);
	const canApplyChanges = $derived(showChangeActions && hasUnsavedChanges);
	const consoleLogs = $derived(getConsoleLogs());
	const showConsole = $derived(getShowConsole());
	const latestConsoleLog = $derived(consoleLogs[0] ?? null);
	const consoleErrorCount = $derived(consoleLogs.filter((log) => log.level === 'error').length);
	const consoleWarningCount = $derived(consoleLogs.filter((log) => log.level === 'warn').length);

	// Guard: redirect to login if no connections (after checking)
	let hasCheckedConnections = $state(false);
	let connectionManagerOpen = $state(false);
	let tabStripElement = $state<HTMLDivElement | null>(null);

	$effect(() => {
		const checkConnections = async () => {
			await connectionStore.refreshConnections();
			hasCheckedConnections = true;
		};
		checkConnections();
	});

	$effect(() => {
		// Only redirect after we've checked and there are no connections
		if (hasCheckedConnections && connectionStore.connections.length === 0) {
			goto('/');
		}
	});

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
		const activeId = activeTabId;
		const strip = tabStripElement;
		if (!activeId || !strip) return;

		let cancelled = false;
		void tick().then(() => {
			if (cancelled || tabsStore.activeTabId !== activeId) return;

			const activeTrigger = Array.from(strip.querySelectorAll<HTMLElement>('[role="tab"]')).find(
				(trigger) => trigger.dataset.value === activeId
			);

			activeTrigger?.scrollIntoView({
				behavior: 'smooth',
				block: 'nearest',
				inline: 'nearest'
			});
		});

		return () => {
			cancelled = true;
		};
	});

	$effect(() => {
		if (!tabsStore.activeTabId && tabsStore.tabs.length > 0) {
			tabsStore.setActive(tabsStore.tabs[0].id);
		}
	});

	onMount(() => {
		const handleOpenConnectionManager = () => {
			connectionManagerOpen = true;
		};

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
				if (canApplyChanges) {
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
		window.addEventListener('open-connection-manager', handleOpenConnectionManager);

		return () => {
			document.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('open-connection-manager', handleOpenConnectionManager);
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

<div class="rt-shell flex h-screen flex-col">
	<!-- Header -->
	<AppHeader />
	<ConnectionManagerModal
		open={connectionManagerOpen}
		onClose={() => (connectionManagerOpen = false)}
		onConnected={() => connectionStore.refreshConnections()}
	/>

	<!-- Main Content -->
	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Connection Panel -->
		<ConnectionPanel />

		<!-- Sidebar -->
		<AppSidebar onTableClick={handleTableClick} />

		<!-- Workspace -->
		<main
			class="rt-workspace flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--surface-raised)]"
		>
			{#if tabs.length > 0}
				<div use:melt={$tabsRoot} class="flex min-h-0 flex-1 flex-col overflow-hidden">
					<div
						class="flex h-10 shrink-0 items-center justify-between border-b bg-[var(--surface-sunken)] px-2"
					>
						<div bind:this={tabStripElement} class="rt-tab-strip min-w-0 flex-1 overflow-x-auto">
							<div use:melt={$tabsList} class="inline-flex h-10 items-center bg-transparent">
								{#each tabs as tab (tab.id)}
									<div
										use:melt={$tabTrigger(tab.id)}
										class="text-muted-foreground group hover:text-foreground data-[state=active]:border-b-primary data-[state=active]:text-foreground relative inline-flex h-10 max-w-[200px] min-w-[104px] cursor-pointer items-center gap-2 border-b-2 border-b-transparent px-3 text-[11px] font-semibold transition-colors hover:bg-[var(--surface-hover)] data-[state=active]:bg-[var(--surface-raised)]"
									>
										{#if tab.kind === 'table' || tab.kind === 'createTable'}
											<Table2 class="group-data-[state=active]:text-primary h-3.5 w-3.5 shrink-0" />
										{:else if tab.kind === 'schemaDiagram'}
											<Workflow
												class="group-data-[state=active]:text-primary h-3.5 w-3.5 shrink-0"
											/>
										{:else}
											<Code class="group-data-[state=active]:text-primary h-3.5 w-3.5 shrink-0" />
										{/if}
										<span class="min-w-0 flex-1 truncate text-left">{tab.title}</span>
										<button
											type="button"
											class="text-muted-foreground hover:bg-muted hover:text-foreground ml-auto shrink-0 rounded p-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-data-[state=active]:opacity-60"
											onclick={(e) => {
												e.stopPropagation();
												tabsStore.closeTab(tab.id);
											}}
											aria-label="Close {tab.title}"
											title="Close tab"
										>
											<X class="h-3 w-3" />
										</button>
									</div>
								{/each}
							</div>
						</div>

						<div
							class="flex h-8 min-w-[154px] flex-shrink-0 items-center justify-end gap-1 border-l pl-2"
						>
							<button
								class="rt-primary-button inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-2.5 text-[11px] font-semibold disabled:pointer-events-none disabled:opacity-35 disabled:shadow-none"
								disabled={!canApplyChanges}
								onclick={applyChanges}
							>
								<Save class="h-3 w-3" />
								Apply
							</button>
							<button
								class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[11px] disabled:pointer-events-none disabled:opacity-35"
								disabled={!canApplyChanges}
								onclick={discardChanges}
							>
								<RotateCcw class="h-3 w-3" />
								Discard
							</button>
						</div>
					</div>

					<!-- Tab Content -->
					{#each tabs as tab (tab.id)}
						<div
							use:melt={$tabContent(tab.id)}
							class="min-h-0 flex-1 flex-col overflow-hidden p-0"
							class:flex={tab.id === activeTabId}
							class:hidden={tab.id !== activeTabId}
						>
							{#if tab.kind === 'table'}
								<TableContent />
							{:else if tab.kind === 'query'}
								<QueryEditorContent {tab} />
							{:else if tab.kind === 'createTable'}
								<CreateTableContent />
							{:else if tab.kind === 'schemaDiagram'}
								<SchemaDiagramContent schema={tab.schema || 'public'} />
							{:else}
								<div class="text-muted-foreground flex flex-1 items-center justify-center">
									Select a table or create a new query
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<div class="relative flex flex-1 items-center justify-center overflow-hidden">
					<div class="rt-empty-grid pointer-events-none absolute inset-0 opacity-70"></div>
					<div class="relative max-w-sm px-8 text-center">
						<img
							src="/logo.png"
							alt="Rolling Thunder"
							class="rt-brand-logo mx-auto mb-5 h-14 w-14 rounded-2xl"
						/>
						<h2 class="text-base font-bold tracking-[-0.01em]">Your workspace is ready</h2>
						<p class="text-muted-foreground mt-1.5 text-xs leading-relaxed">
							Open a table from the explorer, or start a fresh SQL query.
						</p>
						<div class="mt-5 flex items-center justify-center gap-2">
							<button
								type="button"
								class="rt-primary-button inline-flex h-8 items-center gap-2 rounded-md px-3 text-xs font-semibold"
								onclick={() => tabsStore.newQueryTab()}
							>
								<Code class="h-3.5 w-3.5" />
								New query
							</button>
							<span class="text-muted-foreground rounded-md border px-2 py-1.5 text-[10px]">
								⌘ N
							</span>
						</div>
					</div>
				</div>
			{/if}
		</main>
	</div>

	<!-- Console Panel -->
	<div
		class="flex shrink-0 flex-col border-t bg-[var(--surface-sunken)]"
		style={showConsole ? 'height: 224px' : ''}
	>
		<div class="flex h-9 shrink-0 items-center gap-2 px-3">
			<button
				type="button"
				class="hover:bg-muted/50 flex h-7 min-w-0 flex-1 items-center gap-2 rounded px-1.5 text-left"
				onclick={toggleConsole}
			>
				<span class="bg-foreground/10 flex h-5 w-5 items-center justify-center rounded">
					<Terminal class="h-3 w-3" />
				</span>
				<span class="text-[10px] font-bold tracking-[0.08em] uppercase">Activity console</span>
				{#if consoleLogs.length > 0}
					<span
						class="h-1.5 w-1.5 rounded-full {latestConsoleLog?.level === 'error'
							? 'bg-red-500'
							: latestConsoleLog?.level === 'warn'
								? 'bg-amber-500'
								: latestConsoleLog?.level === 'success'
									? 'bg-emerald-500'
									: 'bg-sky-500'}"
					></span>
				{/if}
				{#if !showConsole && latestConsoleLog}
					<span class="text-muted-foreground min-w-0 flex-1 truncate text-[10px]">
						{latestConsoleLog.message}
					</span>
				{/if}
				<span class="text-muted-foreground ml-auto text-[9px]">
					{consoleLogs.length}
					{consoleLogs.length === 1 ? 'event' : 'events'}
				</span>
				{#if showConsole}
					<ChevronDown class="text-muted-foreground h-3.5 w-3.5" />
				{:else}
					<ChevronUp class="text-muted-foreground h-3.5 w-3.5" />
				{/if}
			</button>

			{#if consoleErrorCount > 0}
				<span class="flex items-center gap-1 text-[9px] font-semibold text-red-500">
					<CircleX class="h-3 w-3" />
					{consoleErrorCount}
				</span>
			{/if}
			{#if consoleWarningCount > 0}
				<span class="flex items-center gap-1 text-[9px] font-semibold text-amber-500">
					<CircleAlert class="h-3 w-3" />
					{consoleWarningCount}
				</span>
			{/if}
			{#if consoleLogs.length > 0}
				<button
					type="button"
					class="rt-toolbar-button h-7 gap-1.5 px-2 text-[9px] font-semibold"
					onclick={clearConsoleLogs}
					title="Clear activity console"
				>
					<Trash2 class="h-3 w-3" />
					Clear
				</button>
			{/if}
		</div>

		{#if showConsole}
			<div class="rt-code-surface min-h-0 flex-1 overflow-auto border-t">
				{#if consoleLogs.length === 0}
					<div
						class="text-muted-foreground flex h-full flex-col items-center justify-center text-center"
					>
						<Terminal class="h-5 w-5 opacity-45" />
						<p class="mt-2 text-[10px] font-semibold">No activity yet</p>
						<p class="mt-1 text-[9px]">
							Queries, data loads, and connection events will appear here.
						</p>
					</div>
				{:else}
					<div
						class="text-muted-foreground sticky top-0 z-10 grid grid-cols-[82px_76px_minmax(0,1fr)] border-b bg-[var(--surface-sunken)] px-3 py-1.5 text-[8px] font-bold tracking-[0.1em] uppercase"
					>
						<span>Time</span>
						<span>Level</span>
						<span>Message</span>
					</div>
					{#each consoleLogs as log}
						<div
							class="grid min-h-7 grid-cols-[82px_76px_minmax(0,1fr)] items-start border-b px-3 py-1.5 font-mono text-[9px] last:border-b-0"
						>
							<span class="text-muted-foreground pt-0.5">
								{log.timestamp.toLocaleTimeString([], {
									hour: '2-digit',
									minute: '2-digit',
									second: '2-digit'
								})}
							</span>
							<span
								class="flex items-center gap-1.5 font-sans text-[8px] font-bold uppercase {log.level ===
								'error'
									? 'text-red-500'
									: log.level === 'warn'
										? 'text-amber-500'
										: log.level === 'success'
											? 'text-emerald-500'
											: 'text-sky-500'}"
							>
								{#if log.level === 'error'}
									<CircleX class="h-3 w-3" />
								{:else if log.level === 'warn'}
									<CircleAlert class="h-3 w-3" />
								{:else if log.level === 'success'}
									<CircleCheck class="h-3 w-3" />
								{:else}
									<Info class="h-3 w-3" />
								{/if}
								{log.level}
							</span>
							<span class="text-foreground leading-relaxed">{log.message}</span>
						</div>
					{/each}
				{/if}
			</div>
		{/if}
	</div>

	<!-- Status Bar -->
	<AppStatusBar />
</div>
