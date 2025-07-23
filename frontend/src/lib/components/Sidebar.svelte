<script lang="ts">
    import { onMount } from 'svelte';
    import { GetSchemas, GetCollections } from '$lib/wailsjs/go/db/Service';
	import { Sheet } from 'lucide-svelte';
	import { updateStatus } from '$lib/stores/status.svelte';

    const { onTableClick } = $props<{
        onTableClick: (schema: string, table: string) => void;
    }>()

    let schemas: string[] = $state([]);
    let selectedSchema = $state('public');
    let tables: string[] = $state([]);
    let showSchemaDropdown = $state(false);

    onMount(async () => {
        try {
            const response = await GetSchemas();
            schemas = response.data
            showSchemaDropdown = true;
        } catch {
            showSchemaDropdown = false;
        }

        await loadTables();
    });

    async function loadTables() {
        try {
            const response = await GetCollections([selectedSchema]);
            tables = response.data;
            updateStatus('', 'info')
        } catch (e: any) {
            updateStatus(e?.message ?? 'Failed to load tables', 'error');
        }
    }
</script>

<aside class="w-1/4 p-4 flex gap-2 flex-col border-zinc-300 border-r ">
    {#if showSchemaDropdown}
        <div>
            <select
                bind:value={selectedSchema}
                onchange={loadTables}
                class="select"
            >
                {#each schemas as schema}
                    <option value={schema}>{schema}</option>
                {/each}
            </select>
        </div>
        
    {/if}

    <div>
        <ul class="text-sm space-y-1">
            {#each tables as table}
            <li>
                <div
                    tabindex="0"
                    role="button"
                    onclick={() => onTableClick(selectedSchema, table)}
                    onkeydown={(e) => e.key === 'Enter' && onTableClick(selectedSchema, table)}
                    class="cursor-pointer flex flex-row items-center hover:bg-gray-200 p-1 rounded focus:outline-none focus:ring-2 focus:ring-blue-400"
                >
                    <Sheet class="w-4 h-4" />&nbsp{table}
                </div>
            </li>
            {/each}
        </ul>
    </div>
    
</aside>