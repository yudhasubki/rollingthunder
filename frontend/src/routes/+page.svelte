<script lang="ts">
    import { Connect } from '$lib/wailsjs/go/db/Service';
    import { database as driver, db as service } from '$lib/wailsjs/go/models';
    import { goto } from '$app/navigation';
    
    let dbtype : string = $state('postgres');
    let host : string = $state('10.8.0.1');
    let port : string = $state('5432');
    let user : string = $state('mahabbah');
    let password : string = $state('7ps9AJGACrS3qY');
    let dbname : string = $state('mahabbahinvite');
    let result = $state(null);
    let loading : boolean = $state(false);

    async function connect() {
        loading = true;
        try {
            const config = new driver.Config({
                host:host,
                port:port,
                user:user,
                password:password,
                db:dbname,
            })

            const req = new service.ConnectRequest({
                driver: dbtype,
                config,
            })

            const res = await Connect(req);

            if(res.data?.connected) {
                goto('/workspace')
            } else {
                result = `Failed: ${res.errors?.[0]?.detail || 'Unknown error'}`;
            }

        } catch (e) {
            console.error('Caught error:', e);
            result = `Error: ${e.message}`;
        } finally {
            loading = false
        }
    }
</script>

<div class="flex flex-col md:flex-row h-screen">
    <div class="md:w-1/2 bg-gray-50 flex items-center justify-center border-b md:border-b-0 md:border-r p-8">
        <div class="text-center space-y-4">
            <img src="/appicon.png" alt="Logo" class="w-24 h-24 mx-auto" />
            <h1 class="text-4xl font-bold text-blue-600">RollingThunder</h1>
            <p class="text-gray-600 text-sm">Version Beta.</p>
        </div>
    </div>

    <div class="md:w-1/2 flex items-center justify-center p-8">
        <div class="w-full max-w-md space-y-4">
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
                onclick={connect}
                disabled={loading}
                class="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700 disabled:opacity-50 flex items-center gap-2"
            >
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
    </div>
</div>