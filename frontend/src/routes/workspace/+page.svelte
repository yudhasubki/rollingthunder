<script>
    import Sidebar from "$lib/components/Sidebar.svelte";
    import { GetCollectionStructures, GetIndices } from "$lib/wailsjs/go/db/Service"
	import { database } from "$lib/wailsjs/go/models";
	import { LogInfo } from "$lib/wailsjs/runtime/runtime";

    let selectedTable = '';
    let selectedSchema = '';
    let columns = [];
    let indices = [];

    function handleTableClick(table, schema) {
        selectedTable = table;
        selectedSchema = schema;

        loadColumnsInfo()
        loadIndices()
    }

    async function loadColumnsInfo() {
        try {
            const response = await GetCollectionStructures(selectedSchema, selectedTable)
            columns = response.data
        } catch (e) {

        }
    }

    async function loadIndices() {
        try {
            const response = await GetIndices(selectedSchema, selectedTable);
            indices = response.data;
        } catch (e) {

        }
    }
</script>

<div class="flex h-screen">
  <Sidebar onTableClick={handleTableClick} />

  <main class="flex-1 p-4 overflow-y-auto">
    {#if selectedTable}
        <h2 class="text-lg font-bold mb-2"> {selectedSchema}.{selectedTable}</h2>
        
        <div class="mb-4">
            <table class="table-auto w-full text-sm border border-gray-300">
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
        

        <table class="table-auto w-full text-sm border border-gray-300">
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
    {:else}
        <p class="text-gray-400 italic">Silakan pilih tabel di sebelah kiri untuk melihat detail atau menjalankan query.</p>
    {/if}
  </main>
</div>