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
    let loading = false;

    async function connect() {
        loading = true;
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
        } finally {
            loading = false
        }
    }
</script>

<div class="max-w-md mx-auto mt-10 space-y-4">
    <h2 class="text-2xl font-semibold">Connect to Database</h2>

    <fieldset disabled={loading} class="space-y-4">
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
    </fieldset>

    <button 
        on:click={connect} 
        disabled={loading}
        class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700">
        {#if loading}
            <svg class="animate-spin h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor"
                d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
            </svg>
            Connecting...
        {:else}
            Connect
        {/if}
    </button>

    {#if result}
        <div class="mt-4 text-sm font-mono">{result}</div>
    {/if}
</div>