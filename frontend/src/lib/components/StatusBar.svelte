<script lang="ts">
    import { Database, Braces, Plug } from 'lucide-svelte';
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
    import { onDestroy } from 'svelte';

</script>

<div class={`flex items-center gap-2 px-3 py-1 text-sm font-mono ${bg}`}>
    {#each segments as seg, i}
        <span class="truncate max-w-[12rem]">{seg}</span>
        {#if i < segments.length - 1}
            <span class="opacity-40">|</span>
        {/if}
    {/each}
</div>
<div class="flex gap-4 px-3 py-2 border-t">
    <button class="flex items-center gap-1 text-sm">
        <Plug size="14"/>
        Connect
    </button>
    <button class="flex items-center gap-1 text-sm">
        <Database size="14" />
        Database
    </button>
    <button class="flex items-center gap-1 text-sm">
        <Braces size="14" />
        SQL Editor
    </button>
</div>
