<script>
    import Sidebar from "$lib/components/Sidebar.svelte";

    let selectedTable = '';
    let selectedSchema = '';

    function handleTableClick(table, schema) {
        selectedTable = table;
        selectedSchema = schema;
    }
</script>

<div class="flex h-screen">
  <Sidebar onTableClick={handleTableClick} />

  <main class="flex-1 p-4 overflow-y-auto">
    {#if selectedTable}
      <h2 class="text-lg font-bold mb-2"> {selectedSchema}.{selectedTable}</h2>
      <p class="text-gray-600 text-sm">Klik tombol "Run SQL" untuk mulai query ke tabel ini.</p>
    {:else}
      <p class="text-gray-400 italic">Silakan pilih tabel di sebelah kiri untuk melihat detail atau menjalankan query.</p>
    {/if}
  </main>
</div>