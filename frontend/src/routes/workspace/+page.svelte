<script lang="ts">
	import AppHeader from '$lib/components/layout/AppHeader.svelte';
	import AppSidebar from '$lib/components/layout/AppSidebar.svelte';
	import AppStatusBar from '$lib/components/layout/AppStatusBar.svelte';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Button } from '$lib/components/ui/button';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { hasChanges, discardStagedChanges, stagedChanges } from '$lib/stores/staged.svelte';
	import {
		updateStatus,
		updateDatabaseInfo,
		getConsoleLogs,
		getShowConsole,
		toggleConsole,
		clearConsoleLogs
	} from '$lib/stores/status.svelte';
	import { GetDatabaseInfo, InsertRow, UpdateRow, DeleteRow } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { onMount } from 'svelte';
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
	const hasUnsavedChanges = $derived(hasChanges());
	const consoleLogs = $derived(getConsoleLogs());
	const showConsole = $derived(getShowConsole());

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
		if (!tabsStore.activeTab || tabsStore.activeTab.kind !== 'table') {
			updateStatus('No active table selected', 'error');
			return;
		}

		updateStatus('Applying changes...', 'info');

		const table = new database.Table();
		table.Schema = tabsStore.activeTab.schema;
		table.Name = tabsStore.activeTab.table;

		// Get primary key column (assume first column with 'id' or isPrimary flag)
		// For now we'll use 'id' as default primary key
		const primaryKey = 'id';

		try {
			// Process inserts
			for (const row of stagedChanges.data.added) {
				// Remove internal fields before sending
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

			// Process updates
			for (const row of stagedChanges.data.updated) {
				const result = await UpdateRow(table, row, primaryKey);
				if (result.errors?.length) {
					throw new Error(result.errors[0].detail);
				}
			}

			// Process deletes
			for (const row of stagedChanges.data.deleted) {
				const primaryValue = row[primaryKey];
				if (primaryValue !== undefined) {
					const result = await DeleteRow(table, primaryKey, primaryValue);
					if (result.errors?.length) {
						throw new Error(result.errors[0].detail);
					}
				}
			}

			// Clear staged changes after successful apply
			discardStagedChanges();
			updateStatus('Changes applied successfully', 'info');

			// Refresh the current tab to show updated data
			// This will trigger the $effect in TableContent
			const currentTab = tabsStore.activeTab;
			if (currentTab) {
				tabsStore.updateTab(currentTab.id, { ...currentTab });
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to apply changes', 'error');
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
				<Tabs.Root
					value={activeTabId || ''}
					onValueChange={(v) => tabsStore.setActive(v)}
					class="flex min-h-0 flex-1 flex-col"
				>
					<div class="bg-muted/30 flex items-center justify-between border-b px-2">
						<div class="min-w-0 flex-1 overflow-x-auto">
							<Tabs.List class="h-10 gap-0 bg-transparent">
								{#each tabs as tab (tab.id)}
									<Tabs.Trigger
										value={tab.id}
										class="data-[state=active]:bg-background group gap-2"
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
									</Tabs.Trigger>
								{/each}
							</Tabs.List>
						</div>

						<!-- Actions -->
						<div class="flex flex-shrink-0 items-center gap-2 py-1 pl-2">
							<Button
								variant="outline"
								size="sm"
								class="h-7 gap-1.5 text-xs"
								disabled={!hasUnsavedChanges}
								onclick={applyChanges}
							>
								<Save class="h-3.5 w-3.5" />
								Apply
							</Button>
							<Button
								variant="ghost"
								size="sm"
								class="h-7 gap-1.5 text-xs"
								disabled={!hasUnsavedChanges}
								onclick={discardChanges}
							>
								<RotateCcw class="h-3.5 w-3.5" />
								Discard
							</Button>
						</div>
					</div>

					<!-- Tab Content -->
					{#each tabs as tab (tab.id)}
						<Tabs.Content
							value={tab.id}
							class="min-h-0 flex-1 overflow-hidden p-0 data-[state=active]:flex data-[state=active]:flex-col"
						>
							{#if tab.kind === 'table'}
								<TableContent />
							{:else if tab.kind === 'query'}
								<QueryEditorContent />
							{:else}
								<div class="text-muted-foreground flex flex-1 items-center justify-center">
									Select a table or create a new query
								</div>
							{/if}
						</Tabs.Content>
					{/each}
				</Tabs.Root>
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

	<!-- Console Panel (Always visible, collapsible) -->
	<div class="bg-muted/50 flex flex-col border-t" style={showConsole ? 'height: 200px' : ''}>
		<!-- Console Header -->
		<div class="flex items-center justify-between px-3 py-1.5">
			<!-- Clickable toggle area -->
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
			<!-- Clear button (separate from toggle) -->
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

		<!-- Console Content (Expandable) -->
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
