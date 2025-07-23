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
        <label class="tab group flex flex-row gap-1 items-center">
            <input 
                type="radio" 
                class="tab peer"
                checked={tab.id === active}
                aria-label={tab.title}
                onchange={() => {handleTabChange(tab.id)}}
            />
            <span class="peer-checked:font-semibold">{tab.title}</span>

            <span
                role="button"
                tabindex="0"
                class="opacity-0 group-hover:opacity-100 text-gray-400"
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