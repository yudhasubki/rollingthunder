<script lang="ts">
	import AppHeader from '$lib/components/layout/AppHeader.svelte';
	import AppSidebar from '$lib/components/layout/AppSidebar.svelte';
	import AppStatusBar from '$lib/components/layout/AppStatusBar.svelte';
	import CreateTableContent from '$lib/components/CreateTableContent.svelte';
	import SchemaDiagramContent from '$lib/components/SchemaDiagramContent.svelte';
	import DatabaseObjectContent from '$lib/components/DatabaseObjectContent.svelte';
	import ConnectionManagerModal from '$lib/components/ConnectionManagerModal.svelte';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import DiagnosticsDialog from '$lib/components/DiagnosticsDialog.svelte';
	import ImportDataDialog from '$lib/components/database/ImportDataDialog.svelte';
	import { createTabs, melt } from '@melt-ui/svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import {
		hasChanges,
		discardStagedChanges,
		getCreateTableSubmit,
		getStagedChanges
	} from '$lib/stores/staged.svelte';
	import {
		updateStatus,
		addConsoleLog,
		updateDatabaseInfo,
		getConsoleLogs,
		getShowConsole,
		toggleConsole,
		clearConsoleLogs
	} from '$lib/stores/status.svelte';
	import {
		ApplyTableChanges,
		GetCollectionStructures,
		GetDatabaseInfo
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { createServiceError } from '$lib/errors/service';
	import {
		describeRow,
		formatChangeValue,
		getChangedColumns,
		getOriginalRow,
		stripInternalRowFields
	} from '$lib/table/changes';
	import type { Tab } from '$lib/models/Tab';
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
		Trash2,
		Plus,
		Pencil,
		TriangleAlert,
		ShieldCheck,
		Loader2,
		Boxes
	} from 'lucide-svelte';

	// Import content components
	import TableContent from '$lib/components/TableContent.svelte';
	import QueryEditorContent from '$lib/components/QueryEditorContent.svelte';
	import ConnectionPanel from '$lib/components/layout/ConnectionPanel.svelte';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { goto } from '$app/navigation';
	import { commandDefinitions, matchesShortcut, type CommandID } from '$lib/commands/shortcuts';
	import { shortcutStore } from '$lib/stores/shortcuts.svelte';
	import { focusTrap } from '$lib/actions/focusTrap';

	const tabs = $derived(tabsStore.tabs);
	const allTabs = $derived(tabsStore.allTabs);
	const activeTabId = $derived(tabsStore.activeTabId);
	const activeTab = $derived(tabsStore.activeTab);
	const activeConnectionId = $derived(connectionStore.activeConnection?.id ?? null);
	const activeCreateTableSubmit = $derived(getCreateTableSubmit(activeTabId));
	const hasUnsavedChanges = $derived(
		hasChanges(activeTabId) ||
			(activeTab?.kind === 'createTable' && activeCreateTableSubmit !== null)
	);
	const showChangeActions = $derived(
		activeTab?.kind === 'table' || activeTab?.kind === 'createTable'
	);
	const canApplyChanges = $derived(showChangeActions && hasUnsavedChanges);
	const activeStagedChanges = $derived(activeTabId ? getStagedChanges(activeTabId) : null);
	const activeStagedCount = $derived(
		activeStagedChanges
			? activeStagedChanges.data.added.length +
					activeStagedChanges.data.updated.length +
					activeStagedChanges.data.deleted.length
			: 0
	);
	const consoleLogs = $derived(getConsoleLogs());
	const showConsole = $derived(getShowConsole());
	const latestConsoleLog = $derived(consoleLogs[0] ?? null);
	const consoleErrorCount = $derived(consoleLogs.filter((log) => log.level === 'error').length);
	const consoleWarningCount = $derived(consoleLogs.filter((log) => log.level === 'warn').length);

	// Guard: redirect to login if no connections (after checking)
	let hasCheckedConnections = $state(false);
	let connectionManagerOpen = $state(false);
	let commandPaletteOpen = $state(false);
	let diagnosticsOpen = $state(false);
	let importDialogOpen = $state(false);
	let tabStripElement = $state<HTMLDivElement | null>(null);
	let reviewOpen = $state(false);
	let reviewTab = $state<Tab | null>(null);
	let reviewPrimaryKeys = $state<string[]>([]);
	let reviewLoading = $state(false);
	let applyingChanges = $state(false);
	let reviewError = $state('');
	let reviewErrorHint = $state('');
	let reviewRequestVersion = 0;
	let discardTarget = $state<Tab | null>(null);
	let discardClosesTab = $state(false);
	const reviewChanges = $derived(reviewTab ? getStagedChanges(reviewTab.id) : null);
	const reviewChangeCount = $derived(
		reviewChanges
			? reviewChanges.data.added.length +
					reviewChanges.data.updated.length +
					reviewChanges.data.deleted.length
			: 0
	);
	const discardChangesSnapshot = $derived(
		discardTarget ? getStagedChanges(discardTarget.id) : null
	);
	const discardChangeCount = $derived(
		discardChangesSnapshot
			? discardChangesSnapshot.data.added.length +
					discardChangesSnapshot.data.updated.length +
					discardChangesSnapshot.data.deleted.length
			: 0
	);

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
		tabValueStore.set(id ?? '');
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

	$effect(() => {
		const connectionId = activeConnectionId;
		if (!connectionId) return;

		let cancelled = false;
		void GetDatabaseInfo(connectionId).then((res) => {
			if (cancelled) return;
			if (res.errors?.length > 0) {
				updateStatus(res.errors[0].detail, 'error');
				return;
			}
			updateDatabaseInfo(res.data);
			updateStatus('', 'info');
		});

		return () => {
			cancelled = true;
		};
	});

	onMount(() => {
		const handleOpenConnectionManager = () => {
			connectionManagerOpen = true;
		};
		const handleOpenCommandPalette = () => {
			commandPaletteOpen = true;
		};
		const handleOpenImportData = () => {
			if (!activeConnectionId) {
				updateStatus('Connect a database before importing data', 'warn');
				return;
			}
			importDialogOpen = true;
		};
		const handleOpenDiagnostics = () => {
			diagnosticsOpen = true;
		};

		// Keyboard shortcuts
		function handleKeydown(e: KeyboardEvent) {
			if (commandPaletteOpen) return;
			for (const command of commandDefinitions) {
				if (!matchesShortcut(e, shortcutStore.get(command.id))) continue;
				e.preventDefault();
				executeWorkspaceCommand(command.id);
				return;
			}

			const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
			const modifier = isMac ? e.metaKey : e.ctrlKey;

			if (modifier && e.key === 's') {
				e.preventDefault();
				if (canApplyChanges) {
					void requestApplyChanges();
				}
			}

			if (modifier && e.key === 'w') {
				e.preventDefault();
				if (activeTabId) {
					requestCloseTab(activeTabId);
				}
			}

			if (e.key === 'Escape') {
				if (reviewOpen && !applyingChanges) {
					closeReview();
				} else if (discardTarget) {
					discardTarget = null;
				}
			}
		}

		document.addEventListener('keydown', handleKeydown);
		window.addEventListener('open-connection-manager', handleOpenConnectionManager);
		window.addEventListener('open-command-palette', handleOpenCommandPalette);
		window.addEventListener('open-import-data', handleOpenImportData);
		window.addEventListener('open-diagnostics', handleOpenDiagnostics);

		return () => {
			document.removeEventListener('keydown', handleKeydown);
			window.removeEventListener('open-connection-manager', handleOpenConnectionManager);
			window.removeEventListener('open-command-palette', handleOpenCommandPalette);
			window.removeEventListener('open-import-data', handleOpenImportData);
			window.removeEventListener('open-diagnostics', handleOpenDiagnostics);
		};
	});

	function cycleTab(direction: 1 | -1): void {
		if (tabs.length < 2) return;
		const currentIndex = tabs.findIndex((tab) => tab.id === activeTabId);
		const nextIndex = (Math.max(currentIndex, 0) + direction + tabs.length) % tabs.length;
		tabsStore.setActive(tabs[nextIndex].id);
	}

	function dispatchQueryCommand(command: CommandID): void {
		if (activeTab?.kind !== 'query') {
			updateStatus('Open a query tab to use this command', 'warn');
			return;
		}
		window.dispatchEvent(
			new CustomEvent('rollingthunder-query-command', {
				detail: { tabId: activeTab.id, command }
			})
		);
	}

	function executeWorkspaceCommand(command: CommandID): void {
		switch (command) {
			case 'commandPalette':
				commandPaletteOpen = true;
				break;
			case 'newQuery':
				if (activeConnectionId) tabsStore.newQueryTab(activeConnectionId);
				break;
			case 'runQuery':
			case 'formatQuery':
			case 'explainQuery':
			case 'saveQuery':
				dispatchQueryCommand(command);
				break;
			case 'importData':
				if (activeConnectionId) importDialogOpen = true;
				break;
			case 'nextTab':
				cycleTab(1);
				break;
			case 'previousTab':
				cycleTab(-1);
				break;
			case 'toggleConsole':
				toggleConsole();
				break;
			case 'manageConnections':
				connectionManagerOpen = true;
				break;
		}
	}

	function handleTableClick(schema: string, table: string) {
		const connectionId = activeConnectionId;
		if (!connectionId) {
			updateStatus('No active connection', 'error');
			return;
		}

		const existingTab = tabsStore.findTableTab(connectionId, schema, table);
		if (existingTab) {
			tabsStore.setActive(existingTab.id);
		} else {
			tabsStore.newTableTab(connectionId, schema, table);
		}
		updateStatus('', 'info');
	}

	async function handleImportedData(schema: string, table: string): Promise<void> {
		const connectionId = activeConnectionId;
		if (!connectionId) return;
		window.dispatchEvent(
			new CustomEvent('database-objects-changed', {
				detail: { connectionId, schema }
			})
		);
		const tabId = tabsStore.newTableTab(connectionId, schema, table);
		tabsStore.updateTab(tabId, {
			activeSubTab: 'data',
			revision: Date.now()
		});
		updateStatus(`Imported data into ${schema}.${table}`, 'success');
		addConsoleLog(`Import committed: ${schema}.${table}`, 'success');
	}

	async function requestApplyChanges() {
		const targetTab = tabsStore.activeTab;
		if (!targetTab) {
			updateStatus('No active tab', 'error');
			return;
		}

		// Handle createTable tab - use registered callback
		if (targetTab.kind === 'createTable') {
			const submit = getCreateTableSubmit(targetTab.id);
			if (submit) {
				await submit();
			} else {
				updateStatus('Create table form not ready', 'error');
			}
			return;
		}

		if (targetTab.kind !== 'table') {
			updateStatus('No active table selected', 'error');
			return;
		}

		const stagedChanges = getStagedChanges(targetTab.id);
		const stagedCount =
			stagedChanges.data.added.length +
			stagedChanges.data.updated.length +
			stagedChanges.data.deleted.length;
		if (stagedCount === 0) {
			updateStatus('There are no staged row changes to review', 'info');
			return;
		}

		reviewTab = { ...targetTab };
		reviewPrimaryKeys = [];
		reviewError = '';
		reviewErrorHint = '';
		reviewLoading = true;
		reviewOpen = true;
		const requestVersion = ++reviewRequestVersion;

		try {
			const table = new database.Table({
				Schema: targetTab.schema,
				Name: targetTab.table
			});
			const response = await GetCollectionStructures(targetTab.connectionId, table);
			if (
				requestVersion !== reviewRequestVersion ||
				!reviewOpen ||
				reviewTab?.id !== targetTab.id
			) {
				return;
			}
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not prepare the change review');
			}
			reviewPrimaryKeys = (response.data || [])
				.filter((column) => column.is_primary)
				.map((column) => column.name);

			if (
				reviewPrimaryKeys.length === 0 &&
				(stagedChanges.data.updated.length > 0 || stagedChanges.data.deleted.length > 0)
			) {
				reviewError =
					'Existing rows cannot be changed safely because this table has no primary key.';
				reviewErrorHint =
					'Discard the staged updates/deletes or add a primary key. New rows can still be inserted separately.';
			}
		} catch (error: any) {
			reviewError = error?.message ?? 'Could not prepare the change review';
			reviewErrorHint = error?.hint ?? '';
		} finally {
			if (requestVersion === reviewRequestVersion) {
				reviewLoading = false;
			}
		}
	}

	function closeReview(force = false) {
		if (applyingChanges && !force) return;
		reviewRequestVersion += 1;
		reviewOpen = false;
		reviewTab = null;
		reviewPrimaryKeys = [];
		reviewLoading = false;
		reviewError = '';
		reviewErrorHint = '';
	}

	async function confirmApplyChanges() {
		const targetTab = reviewTab;
		const stagedChanges = reviewChanges;
		if (
			!targetTab ||
			targetTab.kind !== 'table' ||
			!stagedChanges ||
			reviewLoading ||
			reviewError ||
			applyingChanges
		) {
			return;
		}

		applyingChanges = true;
		reviewError = '';
		reviewErrorHint = '';
		updateStatus(`Applying ${reviewChangeCount} reviewed row changes atomically…`, 'info');

		const changes = new database.TableChangeSet({
			table: new database.Table({
				Schema: targetTab.schema,
				Name: targetTab.table
			}),
			added: stagedChanges.data.added.map(stripInternalRowFields),
			updated: stagedChanges.data.updated.map(
				(row) =>
					new database.RowUpdate({
						original: stripInternalRowFields(getOriginalRow(row)),
						values: stripInternalRowFields(row),
						changedColumns: getChangedColumns(row)
					})
			),
			deleted: stagedChanges.data.deleted.map(stripInternalRowFields)
		});

		try {
			const response = await ApplyTableChanges(targetTab.connectionId, changes);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Failed to apply reviewed changes');
			}

			const result = response.data;
			const summary = `${result?.inserted ?? 0} inserted · ${result?.updated ?? 0} updated · ${result?.deleted ?? 0} deleted`;
			discardStagedChanges(targetTab.id);
			tabsStore.updateTab(targetTab.id, { revision: Date.now() });
			updateStatus(`Changes committed: ${summary}`, 'success');
			addConsoleLog(`Committed ${targetTab.schema}.${targetTab.table}: ${summary}`, 'success');
			applyingChanges = false;
			closeReview(true);
		} catch (error: any) {
			reviewError = error?.message ?? 'Failed to apply reviewed changes';
			reviewErrorHint =
				error?.hint ?? 'Nothing was committed. Review the staged rows and try again.';
			updateStatus(reviewError, 'error');
			addConsoleLog(reviewError, 'error');
		} finally {
			applyingChanges = false;
		}
	}

	function requestDiscardChanges(tabId: string, closeAfter = false) {
		const target = tabsStore.allTabs.find((tab) => tab.id === tabId);
		if (!target) return;
		if (!hasChanges(tabId)) {
			if (closeAfter) tabsStore.closeTab(tabId);
			return;
		}
		discardTarget = target;
		discardClosesTab = closeAfter;
	}

	function confirmDiscardChanges() {
		const target = discardTarget;
		if (!target) return;
		discardStagedChanges(target.id);
		updateStatus(`Discarded ${discardChangeCount} staged changes from ${target.title}`, 'info');
		discardTarget = null;
		if (discardClosesTab) {
			tabsStore.closeTab(target.id);
		}
		discardClosesTab = false;
	}

	function discardChanges() {
		if (activeTabId) requestDiscardChanges(activeTabId);
	}

	function requestCloseTab(tabId: string) {
		requestDiscardChanges(tabId, true);
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
	<CommandPalette
		open={commandPaletteOpen}
		hasConnection={Boolean(activeConnectionId)}
		activeTabKind={activeTab?.kind}
		onClose={() => (commandPaletteOpen = false)}
		onExecute={executeWorkspaceCommand}
	/>
	<DiagnosticsDialog open={diagnosticsOpen} onClose={() => (diagnosticsOpen = false)} />
	{#if activeConnectionId}
		<ImportDataDialog
			open={importDialogOpen}
			connectionId={activeConnectionId}
			initialSchema={activeTab?.schema}
			onClose={() => (importDialogOpen = false)}
			onImported={handleImportedData}
		/>
	{/if}

	<!-- Main Content -->
	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Connection Panel -->
		<ConnectionPanel />

		<!-- Sidebar -->
		{#if activeConnectionId}
			<AppSidebar connectionId={activeConnectionId} onTableClick={handleTableClick} />
		{/if}

		<!-- Workspace -->
		<main
			id="main-content"
			tabindex="-1"
			class="rt-workspace flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[var(--surface-raised)]"
		>
			{#if allTabs.length > 0}
				<div use:melt={$tabsRoot} class="flex min-h-0 flex-1 flex-col overflow-hidden">
					{#if tabs.length > 0}
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
												<Table2
													class="group-data-[state=active]:text-primary h-3.5 w-3.5 shrink-0"
												/>
											{:else if tab.kind === 'schemaDiagram'}
												<Workflow
													class="group-data-[state=active]:text-primary h-3.5 w-3.5 shrink-0"
												/>
											{:else if tab.kind === 'databaseObject'}
												<Boxes
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
													requestCloseTab(tab.id);
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
								class="flex h-8 min-w-[188px] flex-shrink-0 items-center justify-end gap-1 border-l pl-2"
							>
								{#if activeTab?.kind === 'table' && activeStagedCount > 0}
									<span
										class="mr-1 inline-flex h-6 items-center rounded-md bg-amber-500/10 px-2 text-[8px] font-semibold text-amber-700 dark:text-amber-300"
										title={`${activeStagedChanges?.data.added.length ?? 0} inserts · ${activeStagedChanges?.data.updated.length ?? 0} updates · ${activeStagedChanges?.data.deleted.length ?? 0} deletes`}
									>
										{activeStagedCount} pending
									</span>
								{/if}
								<button
									class="rt-primary-button inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-2.5 text-[11px] font-semibold disabled:pointer-events-none disabled:opacity-35 disabled:shadow-none"
									disabled={!canApplyChanges}
									onclick={() => void requestApplyChanges()}
								>
									<Save class="h-3 w-3" />
									{activeTab?.kind === 'table' ? 'Review' : 'Apply'}
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
					{/if}

					<!-- Tab Content -->
					{#each allTabs as tab (tab.id)}
						<div
							use:melt={$tabContent(tab.id)}
							class="min-h-0 flex-1 flex-col overflow-hidden p-0"
							class:flex={tab.connectionId === activeConnectionId && tab.id === activeTabId}
							class:hidden={tab.connectionId !== activeConnectionId || tab.id !== activeTabId}
						>
							{#if tab.kind === 'table'}
								<TableContent {tab} />
							{:else if tab.kind === 'query'}
								<QueryEditorContent {tab} />
							{:else if tab.kind === 'createTable'}
								<CreateTableContent {tab} />
							{:else if tab.kind === 'schemaDiagram'}
								<SchemaDiagramContent
									connectionId={tab.connectionId}
									schema={tab.schema || 'public'}
								/>
							{:else if tab.kind === 'databaseObject'}
								<DatabaseObjectContent {tab} />
							{:else}
								<div class="text-muted-foreground flex flex-1 items-center justify-center">
									Select a table or create a new query
								</div>
							{/if}
						</div>
					{/each}
					{#if tabs.length === 0}
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
									This connection has its own workspace. Open a table or start a query.
								</p>
								{#if activeConnectionId}
									<div class="mt-5 flex items-center justify-center">
										<button
											type="button"
											class="rt-primary-button inline-flex h-8 items-center gap-2 rounded-md px-3 text-xs font-semibold"
											onclick={() => tabsStore.newQueryTab(activeConnectionId)}
										>
											<Code class="h-3.5 w-3.5" />
											New query
										</button>
									</div>
								{/if}
							</div>
						</div>
					{/if}
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
								onclick={() => activeConnectionId && tabsStore.newQueryTab(activeConnectionId)}
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
				aria-expanded={showConsole}
				aria-controls="activity-console-content"
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
			<div
				id="activity-console-content"
				class="rt-code-surface min-h-0 flex-1 overflow-auto border-t"
				role="region"
				aria-label="Activity console events"
			>
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

{#if reviewOpen && reviewTab && reviewChanges}
	<div class="fixed inset-0 z-[100] flex items-center justify-center p-6">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={() => closeReview()}
			aria-label="Close change review"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[86vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="change-review-title"
		>
			<header class="flex shrink-0 items-start gap-3 border-b p-4">
				<span
					class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-300"
				>
					<ShieldCheck class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="change-review-title" class="text-sm font-bold">Review database changes</h2>
					<p class="text-muted-foreground mt-1 text-[10px]">
						Nothing is written until you confirm this reviewed change set.
					</p>
					<div class="mt-2 flex flex-wrap items-center gap-1.5">
						<span class="bg-muted rounded px-2 py-1 font-mono text-[9px] font-semibold"
							>{reviewTab.schema}.{reviewTab.table}</span
						>
						<span class="text-muted-foreground text-[9px]">
							{reviewPrimaryKeys.length > 0
								? `Primary key: ${reviewPrimaryKeys.join(', ')}`
								: 'No primary key detected'}
						</span>
					</div>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-8 w-8 cursor-pointer"
					onclick={() => closeReview()}
					disabled={applyingChanges}
					aria-label="Close change review"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="grid shrink-0 grid-cols-3 divide-x border-b bg-[var(--surface-sunken)]">
				<div class="flex h-14 items-center gap-2.5 px-4">
					<span
						class="flex h-7 w-7 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-500"
					>
						<Plus class="h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[12px] font-bold tabular-nums">
							{reviewChanges.data.added.length}
						</div>
						<div class="text-muted-foreground text-[8px]">Insert</div>
					</div>
				</div>
				<div class="flex h-14 items-center gap-2.5 px-4">
					<span
						class="flex h-7 w-7 items-center justify-center rounded-md bg-amber-500/10 text-amber-500"
					>
						<Pencil class="h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[12px] font-bold tabular-nums">
							{reviewChanges.data.updated.length}
						</div>
						<div class="text-muted-foreground text-[8px]">Update</div>
					</div>
				</div>
				<div class="flex h-14 items-center gap-2.5 px-4">
					<span
						class="flex h-7 w-7 items-center justify-center rounded-md bg-red-500/10 text-red-500"
					>
						<Trash2 class="h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[12px] font-bold tabular-nums">
							{reviewChanges.data.deleted.length}
						</div>
						<div class="text-muted-foreground text-[8px]">Delete</div>
					</div>
				</div>
			</div>

			<div class="min-h-0 flex-1 overflow-auto p-4">
				{#if reviewLoading}
					<div class="text-muted-foreground flex h-40 flex-col items-center justify-center">
						<Loader2 class="h-5 w-5 animate-spin" />
						<p class="mt-2 text-[10px] font-semibold">Validating row identity and metadata…</p>
					</div>
				{:else}
					{#if reviewError}
						<div
							class="mb-3 flex items-start gap-2.5 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-red-600 dark:text-red-400"
						>
							<TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" />
							<div class="min-w-0">
								<p class="text-[10px] font-semibold">{reviewError}</p>
								{#if reviewErrorHint}
									<p class="mt-1 text-[9px] leading-relaxed opacity-85">{reviewErrorHint}</p>
								{/if}
							</div>
						</div>
					{/if}

					<div class="space-y-4">
						{#if reviewChanges.data.added.length > 0}
							<section>
								<div class="mb-1.5 flex items-center gap-2">
									<Plus class="h-3 w-3 text-emerald-500" />
									<h3 class="text-[10px] font-bold">Rows to insert</h3>
								</div>
								<div class="space-y-1.5">
									{#each reviewChanges.data.added.slice(0, 5) as row, index}
										<div class="rounded-lg border bg-[var(--surface-raised)] px-3 py-2">
											<div class="text-[9px] font-semibold">
												{describeRow(row, reviewPrimaryKeys, index)}
											</div>
											<div class="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1">
												{#each Object.entries(stripInternalRowFields(row)).slice(0, 5) as [column, value]}
													<span class="text-[8px]">
														<code>{column}</code> = {formatChangeValue(value)}
													</span>
												{/each}
											</div>
										</div>
									{/each}
									{#if reviewChanges.data.added.length > 5}
										<p class="text-muted-foreground px-1 text-[8px]">
											+{reviewChanges.data.added.length - 5} more inserts
										</p>
									{/if}
								</div>
							</section>
						{/if}

						{#if reviewChanges.data.updated.length > 0}
							<section>
								<div class="mb-1.5 flex items-center gap-2">
									<Pencil class="h-3 w-3 text-amber-500" />
									<h3 class="text-[10px] font-bold">Rows to update</h3>
								</div>
								<div class="space-y-1.5">
									{#each reviewChanges.data.updated.slice(0, 5) as row, index}
										<div class="rounded-lg border bg-[var(--surface-raised)] px-3 py-2">
											<div class="text-[9px] font-semibold">
												{describeRow(row, reviewPrimaryKeys, index)}
											</div>
											<div class="mt-1.5 space-y-1">
												{#each getChangedColumns(row).slice(0, 5) as column}
													<div
														class="grid grid-cols-[minmax(80px,0.7fr)_minmax(0,1fr)_12px_minmax(0,1fr)] items-center gap-2 text-[8px]"
													>
														<code class="truncate font-semibold">{column}</code>
														<span class="text-muted-foreground truncate line-through"
															>{formatChangeValue(getOriginalRow(row)[column])}</span
														>
														<span class="text-muted-foreground">→</span>
														<span class="truncate font-medium"
															>{formatChangeValue(row[column])}</span
														>
													</div>
												{/each}
											</div>
										</div>
									{/each}
									{#if reviewChanges.data.updated.length > 5}
										<p class="text-muted-foreground px-1 text-[8px]">
											+{reviewChanges.data.updated.length - 5} more updates
										</p>
									{/if}
								</div>
							</section>
						{/if}

						{#if reviewChanges.data.deleted.length > 0}
							<section>
								<div class="mb-1.5 flex items-center gap-2">
									<Trash2 class="h-3 w-3 text-red-500" />
									<h3 class="text-[10px] font-bold">Rows to delete permanently</h3>
								</div>
								<div class="space-y-1.5">
									{#each reviewChanges.data.deleted.slice(0, 5) as row, index}
										<div
											class="flex items-center justify-between gap-3 rounded-lg border border-red-500/20 bg-red-500/5 px-3 py-2"
										>
											<span class="truncate text-[9px] font-semibold">
												{describeRow(row, reviewPrimaryKeys, index)}
											</span>
											<span class="shrink-0 text-[8px] font-semibold text-red-500"
												>Permanent delete</span
											>
										</div>
									{/each}
									{#if reviewChanges.data.deleted.length > 5}
										<p class="text-muted-foreground px-1 text-[8px]">
											+{reviewChanges.data.deleted.length - 5} more deletes
										</p>
									{/if}
								</div>
							</section>
						{/if}
					</div>
				{/if}
			</div>

			<footer class="flex shrink-0 items-center justify-between gap-4 border-t p-4">
				<div class="flex min-w-0 items-start gap-2">
					<ShieldCheck class="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
					<p class="text-muted-foreground text-[8px] leading-relaxed">
						All {reviewChangeCount} changes run in one database transaction. If any row fails, the complete
						set is rolled back.
					</p>
				</div>
				<div class="flex shrink-0 gap-2">
					<button
						type="button"
						class="rt-toolbar-button h-8 cursor-pointer px-3 text-[10px] font-semibold"
						onclick={() => closeReview()}
						disabled={applyingChanges}
					>
						Keep editing
					</button>
					<button
						type="button"
						class="{reviewChanges.data.deleted.length > 0
							? 'bg-red-600 text-white hover:bg-red-700'
							: 'rt-primary-button'} inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[10px] font-bold disabled:pointer-events-none disabled:opacity-50"
						onclick={confirmApplyChanges}
						disabled={reviewLoading ||
							Boolean(reviewError) ||
							applyingChanges ||
							reviewChangeCount === 0}
					>
						{#if applyingChanges}
							<Loader2 class="h-3 w-3 animate-spin" />
							Applying…
						{:else}
							<ShieldCheck class="h-3 w-3" />
							Apply {reviewChangeCount} changes
						{/if}
					</button>
				</div>
			</footer>
		</div>
	</div>
{/if}

{#if discardTarget && discardChangesSnapshot}
	<div class="fixed inset-0 z-[110] flex items-center justify-center p-6">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={() => (discardTarget = null)}
			aria-label="Keep staged changes"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative w-full max-w-sm rounded-xl p-4"
			role="dialog"
			aria-modal="true"
			aria-labelledby="discard-changes-title"
		>
			<div class="flex items-start gap-3">
				<span
					class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-300"
				>
					<TriangleAlert class="h-4 w-4" />
				</span>
				<div class="min-w-0">
					<h2 id="discard-changes-title" class="text-sm font-bold">
						Discard {discardChangeCount} staged changes?
					</h2>
					<p class="text-muted-foreground mt-1 text-[10px] leading-relaxed">
						Your database has not been changed, but the local edits in
						<span class="text-foreground font-mono font-semibold">{discardTarget.title}</span>
						will be lost.
					</p>
				</div>
			</div>
			<div class="mt-3 grid grid-cols-3 gap-2">
				<div class="rounded-lg border px-2.5 py-2 text-center">
					<div class="text-[11px] font-bold">{discardChangesSnapshot.data.added.length}</div>
					<div class="text-muted-foreground text-[8px]">Insert</div>
				</div>
				<div class="rounded-lg border px-2.5 py-2 text-center">
					<div class="text-[11px] font-bold">{discardChangesSnapshot.data.updated.length}</div>
					<div class="text-muted-foreground text-[8px]">Update</div>
				</div>
				<div class="rounded-lg border px-2.5 py-2 text-center">
					<div class="text-[11px] font-bold">{discardChangesSnapshot.data.deleted.length}</div>
					<div class="text-muted-foreground text-[8px]">Delete</div>
				</div>
			</div>
			<div class="mt-4 flex justify-end gap-2">
				<button
					type="button"
					class="rt-toolbar-button h-8 cursor-pointer px-3 text-[10px] font-semibold"
					onclick={() => (discardTarget = null)}
				>
					Keep changes
				</button>
				<button
					type="button"
					class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md bg-red-600 px-3 text-[10px] font-bold text-white transition-colors hover:bg-red-700"
					onclick={confirmDiscardChanges}
				>
					<Trash2 class="h-3 w-3" />
					{discardClosesTab ? 'Discard and close' : 'Discard changes'}
				</button>
			</div>
		</div>
	</div>
{/if}
