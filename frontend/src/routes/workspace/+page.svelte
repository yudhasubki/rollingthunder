<script lang="ts">
    import Sidebar from "$lib/components/Sidebar.svelte";
    import { GetCollectionStructures, GetIndices } from "$lib/wailsjs/go/db/Service"
	import type { database } from "$lib/wailsjs/go/models";
	import { LogError } from "$lib/wailsjs/runtime/runtime";

    let selectedTable: string = $state('');
    let selectedSchema: string = $state('');
    let columns: database.Structure[] = $state([]);
    let indices: database.Index[] = $state([]);

    function handleTableClick(schema: string, table: string) {
        selectedSchema = schema;
        selectedTable = table;

        loadColumnsInfo()
        loadIndices()
    }

    async function loadColumnsInfo() {
        try {
            const response = await GetCollectionStructures(selectedSchema, selectedTable)
            columns = response.data
        } catch (e: any) {
            LogError(e)
        }
    }

    async function loadIndices() {
        try {
            const response = await GetIndices(selectedSchema, selectedTable);
            indices = response.data;
        } catch (e: any) {
            LogError(e)
        }
    }
</script>

<div class="flex h-screen">
  <Sidebar onTableClick={handleTableClick} />

  <main class="flex-1 p-4 overflow-y-auto">
    {#if selectedTable}
        <h2 class="text-lg font-bold mb-2"> {selectedSchema}.{selectedTable}</h2>
        
        <div class="overflow-auto max-w-full border rounded shadow-sm mb-4">
            <table class="table-auto w-full min-w-[600px] text-sm">
                <thead class="bg-gray-100">
                    <tr>
                        <th class="px-2 py-1 border">Name</th>
                        <th class="px-2 py-1 border">Type</th>
                        <th class="px-2 py-1 border">Length</th>
                        <th class="px-2 py-1 border">Nullable</th>
                        <th class="px-2 py-1 border">Default</th>
                        <th class="px-2 py-1 border">Primary Key</th>
                        <th class="px-2 py-1 border">Foreign Key</th>
                        
                    </tr>
                </thead>
                <tbody>
                    {#each columns as col}
                        <tr>
                            <td class="px-2 py-1 border">{col.name}</td>
                            <td class="px-2 py-1 border">{col.data_type}</td>
                            <td class="px-2 py-1 border">{col.length}</td>
                            <td class="px-2 py-1 border">{col.nullable}</td>
                            <td class="px-2 py-1 border">{col.default ?? 'NULL'}</td>
                            <td class="px-2 py-1 border">{col.is_primary ?? ''}</td>
                            <td class="px-2 py-1 border">{col.foreign_key ?? ''}</td>

                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
        
        <div class="overflow-auto max-w-full border rounded shadow-sm">
            <table class="table-auto w-full min-w-[600px] text-sm">
                <thead class="bg-gray-100">
                    <tr>
                        <th class="px-2 py-1 border">Name</th>
                        <th class="px-2 py-1 border">Columns</th>
                        <th class="px-2 py-1 border">Is Unique</th>
                        <th class="px-2 py-1 border">Algorithm</th>
                        
                    </tr>
                </thead>
                <tbody>
                    {#each indices as col}
                        <tr>
                            <td class="px-2 py-1 border">{col.name}</td>
                            <td class="px-2 py-1 border">{col.columns}</td>
                            <td class="px-2 py-1 border">{col.is_unique}</td>
                            <td class="px-2 py-1 border">{col.algorithm}</td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
        
    {:else}
        <p class="text-gray-400 italic">Silakan pilih tabel di sebelah kiri untuk melihat detail atau menjalankan query.</p>
    {/if}
  </main>
</div>