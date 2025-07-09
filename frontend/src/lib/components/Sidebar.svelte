<script lang="ts">
  import { onMount } from 'svelte';
  import { GetSchemas, GetCollections } from '$lib/wailsjs/go/db/Service';
	import { LogInfo } from '$lib/wailsjs/runtime/runtime';

  export let onTableClick: (table: string) => void;

  let schemas: string[] = [];
  let selectedSchema = 'public';
  let tables: string[] = [];
  let showSchemaDropdown = false;
  let error = '';

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
      LogInfo("response : "+response)
      tables = response.data;
    } catch (e: any) {
      error = e.message || 'Failed to load tables';
    }
  }
</script>

<aside class="w-1/4 border-r p-4 overflow-y-auto bg-gray-100">
  <h2 class="font-bold mb-2">📦 Tables</h2>

  {#if showSchemaDropdown}
    <label class="block text-xs mb-1">Schema</label>
    <select
      bind:value={selectedSchema}
      on:change={loadTables}
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
      <li
        class="cursor-pointer hover:bg-gray-200 p-1 rounded"
        on:click={() => onTableClick(table)}
      >
        {table}
      </li>
    {/each}
  </ul>
</aside>