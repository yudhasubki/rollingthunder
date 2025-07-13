<script lang="ts">
    import Sidebar from "$lib/components/Sidebar.svelte";
	import StatusBar from "$lib/components/StatusBar.svelte";
    import TabBar   from '$lib/components/TabBar.svelte';
    import Table from "$lib/components/Table.svelte";
    import { CountCollectionData, GetCollectionData, GetCollectionStructures, GetDatabaseInfo, GetIndices } from "$lib/wailsjs/go/db/Service"
	import { database } from "$lib/wailsjs/go/models";
	import { onMount } from "svelte";
    import { tabs, activeTabId, newTableTab, activeSubTab } from '$lib/stores/tabs';
	import DataTable from "$lib/components/DataTable.svelte";

    let databaseInfo= $state<database.Info | null>();
    let columns = $state<database.Structure[]>([]);
    let status : string = $state('');
    let level = $state<'info' | 'warn' | 'error'>('info');
    let activeTab = $derived(
        $tabs.find(t => t.id === $activeTabId) ?? null,
    );

    let tableData = $state<database.TableData>(database.TableData.createFrom({
        structures: [],
        data: []
    }));
    let tableTotalData = $state<number>(0);
    const tableLimit = 300
    let isLoadingData = $state(false);

    const segments = $derived(
        databaseInfo
        ? [databaseInfo.engine, databaseInfo.version, databaseInfo.database, ...(status ? [status] : [])]
        : []
    );

    onMount(() => {
        GetDatabaseInfo().then(res => {
            if(res.errors?.length > 0) {
                status = res.errors[0].detail
                return
            }

            databaseInfo = res.data
            status = ''
            level = 'info'
        });
    })

    let columnsHeader = [
        {id: "name", header: "Name", editor: "text", sort: true},
        {id: "data_type", header: "Type", sort: true},
        {id: "length", header: "Length", sort: true},
        {id: "nullable", header: "Nullable", sort: true},
        {id: "default", header: "Default", sort: true},
        {id: "is_primary_label", header: "Primary Key", sort: true},
        {id: "foreign_key", header: "Foreign Key", sort: true},
    ]

    let indices = $state<database.Index[]>([]);
    let indicesHeader = [
        {id: "name", header: "Name", flexgrow: 1, sort: true},
        {id: "columns", header: "Columns", sort: true},
        {id: "is_unique", header: "Unique", sort: true},
        {id: "algorithm", header: "Algorithm", sort: true},
    ];

    function handleTableClick(schema: string, table: string) {
        const existingTab = $tabs.find(t => 
            t.kind === 'table' && 
            t.schema === schema && 
            t.table === table
        );

        if (existingTab) {
            activeTabId.set(existingTab.id);
        } else {
            newTableTab(schema, table);
        }
    }

    $effect(() => {
        if (!activeTab || activeTab.kind !== 'table') return;

        status = '';
        level = 'info';

        (async() => {
            try {
                let reqTable = new database.Table();
                reqTable.Name = activeTab.table;
                reqTable.Schema = activeTab.schema;

                const [cols, idxs, db] = await Promise.all([
                    GetCollectionStructures(reqTable),
                    GetIndices(reqTable),
                    GetDatabaseInfo(),
                ])
                
                if (cols.errors?.length)  throw new Error(cols.errors[0].detail);
                if (idxs.errors?.length)  throw new Error(idxs.errors[0].detail);
                if (db.errors?.length)   throw new Error(db.errors[0].detail);

                columns = cols.data
                indices = idxs.data
                databaseInfo = db.data
            } catch(e: any) {
                level = 'error'
                status = e?.message ?? 'Unknown Error';
            }
        })();
    })

    $effect(() => {
        if ($activeSubTab !== 'data' || !activeTab || activeTab.kind !== 'table') return;
        
        isLoadingData = true;
        status = '';

        (async() => {
            try {
                let reqTable = new database.Table();
                reqTable.Name = activeTab.table;
                reqTable.Schema = activeTab.schema;
                reqTable.Limit = tableLimit

                loadCollectionData({from: 0, to: 300})

                const total = await CountCollectionData(reqTable)
                tableTotalData = total.data
                
            } catch(e: any) {
                level = 'error';
                status = e?.message ?? 'failed fetching data';
            } finally {
                isLoadingData = false
            }
        })();
    });

    function loadCollectionData(ev) {
        const {from, to} = ev;
        let reqTable = new database.Table();
        reqTable.Name = activeTab.table;
        reqTable.Schema = activeTab.schema;
        reqTable.Limit = tableLimit
        reqTable.Offset = from

        GetCollectionData(reqTable).then(res => {
            if(res.errors?.length > 0) {
                level = 'error';
                status = res.errors?.[0].detail ?? 'failed fetching data'
                return
            }

            tableData = res.data
        }).catch(e => {
            level = 'error';
            status = e?.message ?? 'failed fetching data';
        }).finally(() => {
            isLoadingData = false
        })
    }
</script>

<div class="flex flex-col h-screen">
    <StatusBar segments={segments} level={level} />
    
    <div class="flex flex-1 overflow-hidden">
        <Sidebar onTableClick={handleTableClick} />

        <main class="flex-1 p-4 overflow-y-auto">
            <TabBar />
            {#if activeTab}
                {#if activeTab?.kind === 'table'}
                    <div class="flex border-b mb-4">
                        <button
                            class={`px-4 py-2 font-medium ${
                                $activeSubTab === 'structure' 
                                    ? 'text-blue-600 border-b-2 border-blue-500' 
                                    : 'text-gray-500 hover:text-gray-700'
                            }`}
                            onclick={(e) => {e.stopPropagation(); e.preventDefault(); activeSubTab.set('structure')}}
                        >
                            Structure
                        </button>
                        <button
                            class={`px-4 py-2 font-medium ${
                                $activeSubTab === 'data'
                                    ? 'text-blue-600 border-b-2 border-blue-500'
                                    : 'text-gray-500 hover:text-gray-700'
                            }`}
                            onclick={(e) => {e.stopPropagation(); e.preventDefault(); activeSubTab.set('data')}}
                        >
                            Data
                        </button>
                    </div>

                    {#if $activeSubTab === 'structure'}
                        <h2 class="text-lg font-bold mb-2">
                            {activeTab.schema}.{activeTab.table}
                        </h2>

                        <div class="max-w-full rounded shadow-sm mb-2" style="height: 30vh;">
                            <Table header={columnsHeader} rows={columns}/>
                        </div>

                        <div class="max-w-full rounded shadow-sm" style="height: 20vh;">
                            <Table header={indicesHeader} rows={indices}/>
                        </div>
                    {:else}
                        {#if isLoadingData}
                            <div class="flex justify-center py-8">
                                <svg class="animate-spin h-5 w-5 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                                </svg>
                            </div>
                        {:else}
                            <div class="max-w-full rounded shadow-sm" style="height: 50vh;">
                                <DataTable 
                                    structures={tableData.structures} 
                                    pageSize={tableLimit} 
                                    total={tableTotalData} 
                                    rows={tableData.data} 
                                    onchange={loadCollectionData}
                                />
                            </div>
                        {/if}
                    {/if}
                {:else}
                    <main class="flex-1 flex items-center justify-center text-gray-400">
                        Klik + buat tab baru
                    </main>
                {/if}
            {:else}
                <p class="text-gray-400 italic">
                    Silakan pilih tabel di sebelah kiri untuk melihat detail atau menjalankan query.
                </p>
            {/if}
        </main>
    </div>
</div>