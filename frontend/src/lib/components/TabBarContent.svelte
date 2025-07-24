<script lang="ts">
    import { tabsStore } from '$lib/stores/tabs.svelte';
	import QueryEditor from './QueryEditorContent.svelte';
    import TableContent from './TableContent.svelte';
</script>

{#if tabsStore.activeTab }
    {#if tabsStore.activeTab.kind === 'table'}
        <TableContent />
    {:else if tabsStore.activeTab.kind === 'query'}
        <QueryEditor />
    {:else}
        <main class="flex-1 flex items-center justify-center text-gray-400">
            Klik + buat tab baru
        </main>
    {/if}
{:else}
    <p class="text-gray-400 italic">
        Isi
    </p>
{/if}