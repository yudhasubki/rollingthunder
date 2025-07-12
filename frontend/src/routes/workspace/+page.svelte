<script lang="ts">
    import Sidebar from "$lib/components/Sidebar.svelte";
	import StatusBar from "$lib/components/StatusBar.svelte";
    import TabBar   from '$lib/components/TabBar.svelte';
    import Table from "$lib/components/Table.svelte";
    import { GetCollectionStructures, GetDatabaseInfo, GetIndices } from "$lib/wailsjs/go/db/Service"
	import { database } from "$lib/wailsjs/go/models";
	import { onMount } from "svelte";
    import { tabs, activeTabId, newTableTab, updateTab } from '$lib/stores/tabs';

    let databaseInfo= $state<database.Info | null>();
    let columns = $state<database.Structure[]>([]);
    let status : string = $state('');
    let level = $state<'info' | 'warn' | 'error'>('info');
    let activeTab = $derived(
        $tabs.find(t => t.id === $activeTabId) ?? null,
    );

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
        {id: "name", header: "Name", editor: "text"},
        {id: "data_type", header: "Type"},
        {id: "length", header: "Length"},
        {id: "nullable", header: "Nullable"},
        {id: "default", header: "Default"},
        {id: "is_primary_label", header: "Primary Key"},
        {id: "foreign_key", header: "Foreign Key"},
    ]

    let indices = $state<database.Index[]>([]);
    let indicesHeader = [
        {id: "name", header: "Name", flexgrow: 1},
        {id: "columns", header: "Columns"},
        {id: "is_unique", header: "Unique"},
        {id: "algorithm", header: "Algorithm"},
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
</script>

<div class="flex flex-col h-screen">
    <StatusBar segments={segments} level={level} />
    
    <div class="flex flex-1 overflow-hidden">
        <Sidebar onTableClick={handleTableClick} />

        <main class="flex-1 p-4 overflow-y-auto">
            <TabBar />
            {#if activeTab}
                {#if activeTab?.kind === 'table'}
                    <h2 class="text-lg font-bold mb-2">
                        {activeTab.schema}.{activeTab.table}
                    </h2>

                    <div class="max-w-full rounded shadow-sm mb-2">
                        <Table header={columnsHeader} rows={columns}/>
                    </div>

                    <div class="max-w-full rounded shadow-sm">
                        <Table header={indicesHeader} rows={indices}/>
                    </div>
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