<script lang="ts">
    import Sidebar from "$lib/components/Sidebar.svelte";
    import StatusBar from "$lib/components/StatusBar.svelte";
    import TabBar from '$lib/components/TabBar.svelte';
    import { tabsStore } from "$lib/stores/tabs.svelte";
    import { Save, RefreshCcw } from 'lucide-svelte';
    import {
        hasChanges,
        discardStagedChanges
    } from '$lib/stores/staged.svelte';
    import { 
        getLevel,
        getSegments,
        updateStatus,
        updateDatabaseInfo
    } from '$lib/stores/status.svelte';
    import { GetDatabaseInfo } from "$lib/wailsjs/go/db/Service"
    import { onMount } from "svelte";
    
    const segments = $derived(getSegments());

    onMount(() => {
        GetDatabaseInfo().then(res => {
            if(res.errors?.length > 0) {
                updateStatus(res.errors[0].detail, 'error');
                return;
            }
            updateDatabaseInfo(res.data);
            updateStatus('', 'info')
        });
    });

    function handleTableClick(schema: string, table: string) {
        const existingTab = tabsStore.findTableTab(schema, table)

        if (existingTab) {
            tabsStore.setActive(existingTab.id);
        } else {
            tabsStore.newTableTab(schema, table)
        }
        updateStatus('', 'info')
    }

    function applyChanges() {
        updateStatus("Applying changes...", "info");
    }

    function discardChanges() {
        updateStatus("Discarding changes...", "info");
        discardStagedChanges();
    }
</script>

<div class="flex flex-col h-screen">
    <StatusBar {segments} level={getLevel()} />
    
    <div class="flex flex-1 overflow-hidden">
        <Sidebar onTableClick={handleTableClick} />

        <main class="flex-1 p-4 overflow-y-auto">
            <TabBar />

            <div class="flex justify-end mt-4 gap-2">
                <button 
                    class="btn btn-primary"
                    disabled={!hasChanges()}
                    onclick={applyChanges}
                >
                    <Save size="14"/> Apply
                </button>

                <button 
                    class="btn btn-primary"
                    disabled={!hasChanges()}
                    onclick={discardChanges}
                >
                    <RefreshCcw size="14" /> Discard
                </button>
            </div>
        </main>
    </div>
</div>