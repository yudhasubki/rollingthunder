<script lang="ts">
    import { tabsStore } from "$lib/stores/tabs.svelte";
    import TabBarContent from './TabBarContent.svelte';
    import { X } from 'lucide-svelte';
    let list = $derived(tabsStore.tabs);
    let active = $derived(tabsStore.activeTabId);

    function handleTabChange(tabId) {
        tabsStore.setActive(tabId)
    }
</script>

{#if list.length > 0}
<div role="tablist" class="tabs tabs-lift">
    {#each list as tab (tab.id)}
        <label class="tab group relative pr-6">
            <input 
                type="radio" 
                class="tab"
                checked={tab.id === active}
                aria-label={tab.title}
                onchange={() => {handleTabChange(tab.id)}}
            />
            {tab.title}

            <span
                role="button"
                tabindex="0"
                class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 transition text-gray-400"
                onclick={(e) => {
                tabsStore.closeTab(tab.id);
                    e.stopPropagation();
                }}
                onkeydown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        tabsStore.closeTab(tab.id);
                        e.stopPropagation();
                        e.preventDefault();
                    }
                }}
            >
                <X class="w-3 h-3" />
            </span>
        </label>
        <div class="tab-content bg-base-100 border-base-300">
            <TabBarContent />
        </div>
    {/each}
</div>
{/if}