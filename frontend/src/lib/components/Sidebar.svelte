<script lang="ts">
    import { onMount } from 'svelte';
    import { GetSchemas, GetCollections } from '$lib/wailsjs/go/db/Service';

    const { onTableClick } = $props<{
        onTableClick: (schema: string, table: string) => void;
    }>()

    let schemas: string[] = $state([]);
    let selectedSchema = $state('public');
    let tables: string[] = $state([]);
    let showSchemaDropdown = $state(false);
    let error: string = $state('');

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
        } catch (e: any) {
            error = e.message || 'Failed to load tables';
        }
    }
</script>

<aside class="w-1/4 p-4 overflow-y-auto bg-gray-100">
    <h2 class="font-bold mb-2">📦 Tables</h2>

    {#if showSchemaDropdown}
        <label for="schema" class="block text-xs mb-1">Schema</label>
        <select
            bind:value={selectedSchema}
            onchange={loadTables}
            class="w-full mb-4 p-1 border rounded text-sm"
        >
        {#each schemas as schema}
            <option value={schema}>{schema}</option>
        {/each}
        </select>
    {/if}

    {#if error}
        <p class="text-red-500 text-sm mb-2">{error}</p>
    {/if}

    <ul class="text-sm space-y-1">
        {#each tables as table}
        <li>
            <div
                tabindex="0"
                role="button"
                onclick={() => onTableClick(selectedSchema, table)}
                onkeydown={(e) => e.key === 'Enter' && onTableClick(selectedSchema, table)}
                class="cursor-pointer hover:bg-gray-200 p-1 rounded focus:outline-none focus:ring-2 focus:ring-blue-400"
            >
                {table}
            </div>
        </li>
        {/each}
    </ul>
</aside>