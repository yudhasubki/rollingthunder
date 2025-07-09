<script>
    import { Connect } from '$lib/wailsjs/go/db/Service';
    import { db as model } from '$lib/wailsjs/go/models';
    import { goto } from '$app/navigation';
    
    let dbtype = 'postgres';
    let host = 'localhost';
    let port = '5432';
    let user = '';
    let password = '';
    let dbname = '';
    let result = null;

    async function connect() {
        try {
            const config = new model.Config({
                host,
                port,
                user,
                password,
                dbname
            })

            const req = new model.ConnectRequest({
                driver: dbtype,
                config,
            })

            const res = await Connect(req);

            if(res.data?.connected) {
                goto('/workspace')
            } else {
                result = `❌ Failed: ${res.errors?.[0]?.detail || 'Unknown error'}`;
            }

        } catch (e) {
            console.error('Caught error:', e);
            result = `❌ Error: ${e.message}`;
        }
    }
</script>

<div class="max-w-md mx-auto mt-10 space-y-4">
  <h2 class="text-2xl font-semibold">Connect to Database</h2>

  <label class="block">DB Type:
    <select bind:value={dbtype} class="w-full border rounded px-2 py-1">
      <option value="postgres">PostgreSQL</option>
      <option value="mysql">MySQL</option>
      <option value="sqlite">SQLite</option>
    </select>
  </label>

  <label class="block">Host:
    <input bind:value={host} class="w-full border rounded px-2 py-1" />
  </label>

  <label class="block">Port:
    <input bind:value={port} class="w-full border rounded px-2 py-1" />
  </label>

  <label class="block">Username:
    <input bind:value={user} class="w-full border rounded px-2 py-1" />
  </label>

  <label class="block">Password:
    <input type="password" bind:value={password} class="w-full border rounded px-2 py-1" />
  </label>

  <label class="block">DB Name:
    <input bind:value={dbname} class="w-full border rounded px-2 py-1" />
  </label>

  <button on:click={connect} class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">
    Connect
  </button>

  {#if result}
    <div class="mt-4 text-sm font-mono">{result}</div>
  {/if}
</div>