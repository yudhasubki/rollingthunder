<script lang="ts">
	import { onMount } from 'svelte';
	import { GetSchemas, GetCollections } from '$lib/wailsjs/go/db/Service';
	import {
		ChevronDown,
		ChevronRight,
		Database,
		Table2,
		Plus,
		RefreshCw,
		Code
	} from 'lucide-svelte';
	import { createDropdownMenu, createContextMenu, melt } from '@melt-ui/svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		onTableClick: (schema: string, table: string) => void;
	}

	let { onTableClick }: Props = $props();

	let schemas: string[] = $state([]);
	let expandedSchemas = $state<string[]>([]);
	let tablesBySchema = $state<Record<string, string[]>>({});
	let loading = $state(false);
	let selectedItem = $state<string | null>(null);

	// Context menu for tables
	let contextTable = $state<{ schema: string; table: string } | null>(null);
	const {
		elements: { trigger: ctxTrigger, menu: ctxMenu, item: ctxItem, separator: ctxSeparator },
		states: { open: ctxOpen }
	} = createContextMenu();

	// Dropdown menu for new actions
	const {
		elements: { trigger: ddTrigger, menu: ddMenu, item: ddItem },
		states: { open: ddOpen }
	} = createDropdownMenu({
		positioning: { placement: 'bottom-end' }
	});

	onMount(async () => {
		await loadSchemas();
	});

	async function loadSchemas() {
		loading = true;
		try {
			const response = await GetSchemas();
			schemas = response.data || [];
			expandedSchemas = [...schemas];
			for (const schema of schemas) {
				await loadTablesForSchema(schema);
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to load schemas', 'error');
		} finally {
			loading = false;
		}
	}

	async function loadTablesForSchema(schema: string) {
		try {
			const response = await GetCollections([schema]);
			tablesBySchema = {
				...tablesBySchema,
				[schema]: (response.data || []).sort((a, b) => a.localeCompare(b))
			};
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to load tables', 'error');
		}
	}

	function toggleSchema(schema: string) {
		if (expandedSchemas.includes(schema)) {
			expandedSchemas = expandedSchemas.filter((s) => s !== schema);
		} else {
			expandedSchemas = [...expandedSchemas, schema];
			if (!tablesBySchema[schema]) {
				loadTablesForSchema(schema);
			}
		}
	}

	function handleTableClick(schema: string, table: string) {
		selectedItem = `${schema}.${table}`;
		onTableClick(schema, table);
		updateStatus('', 'info');
	}

	async function refresh() {
		tablesBySchema = {};
		await loadSchemas();
		updateStatus('Refreshed', 'info');
	}

	function newQuery() {
		tabsStore.newQueryTab();
		updateStatus('New query tab created', 'info');
	}

	function handleTableContextMenu(schema: string, table: string) {
		contextTable = { schema, table };
	}
</script>

<aside class="bg-sidebar flex h-full w-64 min-w-64 flex-col overflow-hidden border-r">
	<!-- Header -->
	<div class="flex flex-shrink-0 items-center justify-between border-b px-3 py-2">
		<span class="text-sidebar-foreground text-sm font-medium">Explorer</span>
		<div class="flex gap-1">
			<button
				class="hover:bg-sidebar-accent inline-flex h-6 w-6 cursor-pointer items-center justify-center rounded-md transition-colors disabled:opacity-50"
				onclick={refresh}
				disabled={loading}
			>
				<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
			</button>
			<!-- New dropdown -->
			<button
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
				use:melt={$ddItem}
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={newQuery}
			>
				<Code class="h-4 w-4" />
				New Query
			</button>
		</div>
	{/if}

	<!-- Schema Tree -->
	<div class="min-h-0 flex-1 overflow-auto p-2">
		{#each schemas as schema (schema)}
			<div class="mb-1">
				<button
					type="button"
					class="hover:bg-sidebar-accent flex w-full items-center gap-1 rounded-md px-2 py-1 text-sm"
					onclick={() => toggleSchema(schema)}
				>
					{#if expandedSchemas.includes(schema)}
						<ChevronDown class="text-muted-foreground h-4 w-4" />
					{:else}
						<ChevronRight class="text-muted-foreground h-4 w-4" />
					{/if}
					<Database class="text-muted-foreground h-4 w-4" />
					<span class="font-medium">{schema}</span>
				</button>

				{#if expandedSchemas.includes(schema) && tablesBySchema[schema]}
					<div class="ml-4 mt-1 space-y-0.5">
						{#each tablesBySchema[schema] as table (table)}
							<button
								type="button"
								use:melt={$ctxTrigger}
								class="hover:bg-sidebar-accent flex w-full items-center gap-2 rounded-md px-2 py-1 text-sm transition-colors {selectedItem ===
								`${schema}.${table}`
									? 'bg-sidebar-accent text-sidebar-accent-foreground'
									: 'text-muted-foreground'}"
								onclick={() => handleTableClick(schema, table)}
								oncontextmenu={() => handleTableContextMenu(schema, table)}
							>
								<Table2 class="h-3.5 w-3.5" />
								<span class="truncate">{table}</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/each}

		{#if schemas.length === 0 && !loading}
			<div class="text-muted-foreground flex flex-col items-center justify-center py-8 text-center">
				<Database class="mb-2 h-8 w-8" />
				<p class="text-sm">No schemas found</p>
			</div>
		{/if}
	</div>

	<!-- Context Menu for tables -->
	{#if $ctxOpen && contextTable}
		<div
			use:melt={$ctxMenu}
			class="bg-popover text-popover-foreground z-50 min-w-48 rounded-md border p-1 shadow-md"
			transition:fly={{ duration: 150, y: -5 }}
		>
			<button
				use:melt={$ctxItem}
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
				onclick={() => contextTable && handleTableClick(contextTable.schema, contextTable.table)}
			>
				Open Table
			</button>
			<div use:melt={$ctxSeparator} class="bg-border -mx-1 my-1 h-px"></div>
			<button
				use:melt={$ctxItem}
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
			>
				Rename
			</button>
			<button
				use:melt={$ctxItem}
				class="hover:bg-destructive/10 text-destructive flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
			>
				Drop Table
			</button>
		</div>
	{/if}

	<!-- Bottom Actions -->
	<div class="flex-shrink-0 border-t p-2">
		<button
			class="hover:bg-accent hover:text-accent-foreground border-input inline-flex h-8 w-full cursor-pointer items-center justify-center gap-2 rounded-md border bg-transparent px-3 text-sm transition-colors"
			onclick={newQuery}
		>
			<Code class="h-4 w-4" />
			New SQL Query
		</button>
	</div>
</aside>
