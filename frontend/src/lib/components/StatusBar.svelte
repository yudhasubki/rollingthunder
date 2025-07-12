<script lang="ts">
    const {segments, level = 'info'} = $props<{
        segments: string[];
        level?: 'info' | 'warn' | 'error';
    }>()

    const bg = $derived(
        level === 'error'
        ? 'bg-red-50 border-red-300 text-red-800'
        : level === 'warn'
        ? 'bg-yellow-50 border-yellow-300 text-yellow-800'
        : 'bg-green-100 border-green-300 text-green-800'
    )

</script>

<div class={`flex items-center gap-2 px-3 py-1 rounded border text-sm font-mono ${bg}`}>
    {#each segments as seg, i}
        <span class="truncate max-w-[12rem]">{seg}</span>
        {#if i < segments.length - 1}
            <span class="opacity-40">|</span>
        {/if}
    {/each}
</div>