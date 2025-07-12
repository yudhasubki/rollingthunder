<script lang="ts">
  import { tabs, activeTabId, closeTab, setActive } from '$lib/stores/tabs';
  let list = $derived($tabs);
  let active = $derived($activeTabId);
</script>

<div class="flex items-center border-b bg-gray-50 select-none overflow-x-auto">
    <div class="flex flex-nowrap">
        {#each list as tab}
            <button
                class={
                        `px-3 py-1 border-r text-sm whitespace-nowrap
                        ${tab.id === active ? 'bg-white font-semibold' : 'hover:bg-gray-100'}`
                    }
                onclick={() => setActive(tab.id)}>
                {tab.title}
                <span 
                    role="button"
                    tabindex="0"
                    aria-label="Close tab"
                    class="ml-1 opacity-40 hover:opacity-80 focus:outline-none"
                    onclick={(e) => {
                        e.stopPropagation();
                        closeTab(tab.id);
                    }}
                    onkeydown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            e.stopPropagation();
                            closeTab(tab.id);
                        }
                    }}
                >x
                </span>
            </button>
        {/each}
    </div>
    
</div>