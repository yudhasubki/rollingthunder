<script>
    import { Grid, Willow } from "wx-svelte-grid"; 
    const { header = [], rows } = $props()
</script>

<Willow>
    <Grid  data={rows} columns={header} />
</Willow>