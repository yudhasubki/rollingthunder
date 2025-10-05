<script>
    import { Grid, Willow, HeaderMenu } from "@svar-ui/svelte-grid";
    import { Pager } from "@svar-ui/svelte-core";
	import { page } from "$app/state";
    import { ContextMenu } from "@svar-ui/svelte-menu";
    import { stageDataDelete, stagedChanges } from '$lib/stores/staged.svelte';
    const { structures = [], rows = [], pageSize = 8, total = 0, onchange} = $props()
    
    const headers = $derived(
        structures.map(col => ({
            id: col.name,
            header: col.name,
            type: col.data_type,
            sort: true,
            resize: true,
        }))
    );
    let grid = $state();

    const options = [
        { id: "add", text: "Add Row", icon: "wxi-table-row-plus" },
        { type: "separator" },
        { id: "delete", text: "Delete", icon: "wxi-delete-outline" },
    ];

    const handleClicks = ev => {
        const option = ev.action;
        if (option) {
            const state = grid.getState();
            let selectedRow = state.data.find(r => r.id === ev.context) ?? null;
            switch (option.id) {
                case "add":
                    console.log("add")
                    break;
                case "delete":
                    stageDataDelete(selectedRow);
                    break;
            }
        }
    };

    function resolver(id) {
        if (id) grid.exec("select-row", { id: id });
        return id;
    }

</script>

<Willow>
    <ContextMenu {options} api={grid} at="point" onclick={handleClicks} resolver={resolver}>
        <HeaderMenu api={grid}>
            <Grid 
                data={rows} 
                columns={headers} 
                bind:this={grid}
                rowStyle={row => stagedChanges.data.deleted.find(r => r.id === row.id) ? "row-deleted" : ""}
            />
        </HeaderMenu>
    </ContextMenu>

    <Pager total={total} pageSize={pageSize} onchange={onchange} />
</Willow>

<style>
    :global(.row-deleted:not(.wx-selected) .wx-cell) {
        background: rgba(239, 68, 68, 0.15);
        color: #dc2626;
    }
</style>