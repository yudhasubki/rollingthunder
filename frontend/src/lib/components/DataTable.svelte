<script>
    import { Grid, Willow } from "wx-svelte-grid";
    import { Pager } from "wx-svelte-core";
	import { page } from "$app/state";

    const { structures = [], rows = [], pageSize = 8, total = 0, onchange } = $props()
    
    const headers = $derived(
        structures.map(col => ({
            id: col.name,
            header: col.name,
            type: col.data_type,
            sort: true,
            resize: true,
            flexgrow: 1,
        }))
    );
</script>

<Willow>
    <Grid data={rows} columns={headers} />
    <Pager total={total} pageSize={pageSize} onchange={onchange} />
</Willow>