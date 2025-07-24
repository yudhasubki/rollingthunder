<script lang="ts">
    import { tabsStore } from '$lib/stores/tabs.svelte';
    import { LayoutGrid, Text } from 'lucide-svelte';
    import DataTable from "$lib/components/DataTable.svelte";
    import Table from "$lib/components/Table.svelte";
    import { database } from "$lib/wailsjs/go/models";
    import { updateStatus, setDatabaseInfo, updateDatabaseInfo } from '$lib/stores/status.svelte';
    import { CountCollectionData, GetCollectionData, GetCollectionStructures, GetDatabaseInfo, GetIndices } from '$lib/wailsjs/go/db/Service';

    let columns = $state<database.Structure[]>([]);
    let indices = $state<database.Index[]>([]);
    let tableTotalData = $state<number>(0);
    let tableData = $state<database.TableData>(database.TableData.createFrom({
        structures: [],
        data: []
    }));
    
    const tableLimit = 300
    let columnsHeader = [
        {id: "name", header: "Name", editor: "text", sort: true},
        {id: "data_type", header: "Type", sort: true},
        {id: "length", header: "Length", sort: true},
        {id: "nullable", header: "Nullable", sort: true},
        {id: "default", header: "Default", sort: true},
        {id: "is_primary_label", header: "Primary Key", sort: true},
        {id: "foreign_key", header: "Foreign Key", sort: true},
    ]

    let indicesHeader = [
        {id: "name", header: "Name", flexgrow: 1, sort: true},
        {id: "columns", header: "Columns", sort: true},
        {id: "is_unique", header: "Unique", sort: true},
        {id: "algorithm", header: "Algorithm", sort: true},
    ];

    $effect(() => {
        if (!tabsStore.activeTab || tabsStore.activeTab.kind !== 'table') return;

        updateStatus('', 'info');

        (async() => {
            try {
                let reqTable = new database.Table();
                reqTable.Name = tabsStore.activeTab.table;
                reqTable.Schema = tabsStore.activeTab.schema;

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
                updateDatabaseInfo(db.data)
            } catch(e: any) {
                updateStatus(e?.message ?? 'Unknown Error', 'error')
            }
        })();
    })
    
    $effect(() => {
        if (tabsStore.activeSubTab !== 'data' || !tabsStore.activeTab || tabsStore.activeTab.kind !== 'table') {
            return;
        }
        
        updateStatus('Loading data...', 'info');
        (async() => {
            try {
                let reqTable = new database.Table();
                reqTable.Name = tabsStore.activeTab.table;
                reqTable.Schema = tabsStore.activeTab.schema;
                reqTable.Limit = tableLimit;

                loadCollectionData({from: 0, to: 300});
                const total = await CountCollectionData(reqTable);
                tableTotalData = total.data;
                updateStatus('', 'info'); // Clear status on success
            } catch(e: any) {
                updateStatus(e?.message ?? 'Failed fetching data', 'error');
            }
        })();
    });

    function loadCollectionData(ev) {
        const {from, to} = ev;
        let reqTable = new database.Table();
        reqTable.Name = tabsStore.activeTab.table;
        reqTable.Schema = tabsStore.activeTab.schema;
        reqTable.Limit = tableLimit;
        reqTable.Offset = from;

        updateStatus('', 'info');
        GetCollectionData(reqTable)
            .then(res => {
                if(res.errors?.length > 0) {
                    updateStatus(res.errors?.[0].detail ?? 'Failed fetching data', 'error');
                    return;
                }
                tableData = res.data;
                updateStatus(''); // Clear status on success
            })
            .catch(e => {
                updateStatus(e?.message ?? 'Failed fetching data', 'error');
            });
    }

    function handleTabChange(tabType: 'structure' | 'data') {
        tabsStore.setActiveSubTab(tabType);
    }
</script>
<div class="tabs tabs-border">
    <label class="tab">
        <input 
            type="radio" 
            class="[&:checked]:font-semibold [&:checked]:text-inherit [&:checked]:bg-transparent" 
            checked={tabsStore.activeSubTab === "structure"}
            onchange={() => handleTabChange('structure')}
        />
        <LayoutGrid class="w-4 h-4" />&nbsp;Structure
    </label>
    
    <div class="tab-content bg-base-100 p-2">
        <div class="max-w-full rounded shadow-sm mb-2" style="height: 30vh;">
            <Table header={columnsHeader} rows={columns}/>
        </div>

        <div class="max-w-full rounded shadow-sm" style="height: 20vh;">
            <Table header={indicesHeader} rows={indices}/>
        </div>
    </div>

    <label class="tab">
            <input 
            type="radio"  
            class="[&:checked]:font-semibold [&:checked]:text-inherit [&:checked]:bg-transparent    " 
            aria-label="Data"
            checked={tabsStore.activeSubTab === "data"} 
            onchange={() => handleTabChange('data')}
        />
        <Text class="w-4 h-4" />&nbsp;Data
    </label>
    
    <div class="tab-content bg-base-100 p-2">
        <div class="max-w-full rounded shadow-sm mb-10" style="height: 50vh;">
            <DataTable 
                structures={tableData.structures} 
                pageSize={tableLimit} 
                total={tableTotalData} 
                rows={tableData.data} 
                onchange={loadCollectionData}
            />
        </div>
    </div>
</div>