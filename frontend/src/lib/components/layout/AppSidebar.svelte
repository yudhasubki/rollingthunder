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
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import * as ContextMenu from '$lib/components/ui/context-menu';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';

	interface Props {
		onTableClick: (schema: string, table: string) => void;
	}

	let { onTableClick }: Props = $props();

	let schemas: string[] = $state([]);
	let expandedSchemas = $state<Set<string>>(new Set(['public']));
	let tablesBySchema = $state<Record<string, string[]>>({});
	let loading = $state(false);
	let selectedItem = $state<string | null>(null);

	onMount(async () => {
		await loadSchemas();
	});

	async function loadSchemas() {
		loading = true;
		try {
			const response = await GetSchemas();
			schemas = response.data || [];
			// Load tables for expanded schemas
			for (const schema of expandedSchemas) {
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
			// Sort tables alphabetically
			tablesBySchema[schema] = (response.data || []).sort((a, b) => a.localeCompare(b));
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to load tables', 'error');
		}
	}

	function toggleSchema(schema: string) {
		if (expandedSchemas.has(schema)) {
			expandedSchemas.delete(schema);
			expandedSchemas = new Set(expandedSchemas);
		} else {
			expandedSchemas.add(schema);
			expandedSchemas = new Set(expandedSchemas);
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
</script>

<aside class="bg-sidebar flex h-full w-64 min-w-64 flex-col overflow-hidden border-r">
	<!-- Header -->
	<div class="flex flex-shrink-0 items-center justify-between border-b px-3 py-2">
		<span class="text-sidebar-foreground text-sm font-medium">Explorer</span>
		<div class="flex gap-1">
			<Button variant="ghost" size="icon" class="h-6 w-6" onclick={refresh} disabled={loading}>
				<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
			</Button>
			<!-- New dropdown with Query option -->
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon" class="h-6 w-6" disabled={false} {...props}>
							<Plus class="h-3.5 w-3.5" />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content class="w-40" align="end">
					<DropdownMenu.Item class="" inset={false} onclick={newQuery}>
						<Code class="mr-2 h-4 w-4" />
						New Query
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>

	<!-- Schema Tree - This needs to be scrollable -->
	<div class="min-h-0 flex-1 overflow-auto p-2">
		{#each schemas as schema (schema)}
			<div class="mb-1">
				<!-- Schema Header -->
				<button
					type="button"
					class="hover:bg-sidebar-accent flex w-full items-center gap-1 rounded-md px-2 py-1 text-sm"
					onclick={() => toggleSchema(schema)}
				>
					{#if expandedSchemas.has(schema)}
						<ChevronDown class="text-muted-foreground h-4 w-4" />
					{:else}
						<ChevronRight class="text-muted-foreground h-4 w-4" />
					{/if}
					<Database class="text-muted-foreground h-4 w-4" />
					<span class="font-medium">{schema}</span>
				</button>

				<!-- Tables -->
				{#if expandedSchemas.has(schema) && tablesBySchema[schema]}
					<div class="ml-4 mt-1 space-y-0.5">
						{#each tablesBySchema[schema] as table (table)}
							<ContextMenu.Root>
								<ContextMenu.Trigger class="w-full">
									<button
										type="button"
										class="hover:bg-sidebar-accent flex w-full items-center gap-2 rounded-md px-2 py-1 text-sm transition-colors {selectedItem ===
										`${schema}.${table}`
											? 'bg-sidebar-accent text-sidebar-accent-foreground'
											: 'text-muted-foreground'}"
										onclick={() => handleTableClick(schema, table)}
									>
										<Table2 class="h-3.5 w-3.5" />
										<span class="truncate">{table}</span>
									</button>
								</ContextMenu.Trigger>
								<ContextMenu.Content class="w-48">
									<ContextMenu.Item
										class=""
										inset={false}
										onclick={() => handleTableClick(schema, table)}
									>
										Open Table
									</ContextMenu.Item>
									<ContextMenu.Separator class="" />
									<ContextMenu.Item class="" inset={false}>Rename</ContextMenu.Item>
									<ContextMenu.Item class="text-destructive" inset={false}
										>Drop Table</ContextMenu.Item
									>
								</ContextMenu.Content>
							</ContextMenu.Root>
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

	<!-- Bottom Actions -->
	<div class="flex-shrink-0 border-t p-2">
		<Button variant="outline" size="sm" class="w-full gap-2" onclick={newQuery}>
			<Code class="h-4 w-4" />
			New SQL Query
		</Button>
	</div>
</aside>
