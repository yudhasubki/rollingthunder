<script lang="ts">
    import Sidebar from "$lib/components/Sidebar.svelte";
    import Table from "$lib/components/Table.svelte";
    import { GetCollectionStructures, GetIndices } from "$lib/wailsjs/go/db/Service"
	import type { database } from "$lib/wailsjs/go/models";
	import { LogError } from "$lib/wailsjs/runtime/runtime";

    let selectedTable: string = $state('');
    let selectedSchema: string = $state('');
    let columns: database.Structure[] = $state([]);
    let columnsHeader = [
        {id: "name", header: "Name"},
        {id: "data_type", header: "Type"},
        {id: "length", header: "Length"},
        {id: "nullable", header: "Nullable"},
        {id: "default", header: "Default"},
        {id: "is_primary_label", header: "Primary Key"},
        {id: "foreign_key", header: "Foreign Key"},
    ]

    let indices: database.Index[] = $state([]);
    let indicesHeader = [
        {id: "name", header: "Name", flexgrow: 1},
        {id: "columns", header: "Columns"},
        {id: "is_unique", header: "Unique"},
        {id: "algorithm", header: "Algorithm"},
    ];

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
        <div>
            
        </div>
        {#if selectedTable}
            <h2 class="text-lg font-bold mb-2"> {selectedSchema}.{selectedTable}</h2>
            
            <div class="max-w-full rounded shadow-sm mb-2">
                <Table header={columnsHeader} rows={columns} />
            </div>
            
            <div class="max-w-full rounded shadow-sm">
                <Table header={indicesHeader} rows={indices} />
            </div>
            
        {:else}
            <p class="text-gray-400 italic">Silakan pilih tabel di sebelah kiri untuk melihat detail atau menjalankan query.</p>
        {/if}
    </main>
</div>