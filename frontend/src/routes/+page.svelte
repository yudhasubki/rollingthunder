<script lang="ts">
	import { Connect } from '$lib/wailsjs/go/db/Service';
	import { database as driver, db as service } from '$lib/wailsjs/go/models';
	import { goto } from '$app/navigation';
	import { createSelect, melt } from '@melt-ui/svelte';
	import { Database, Loader2, ChevronDown } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let dbtype = $state('postgres');
	let host = $state('127.0.0.1');
	let port = $state('5432');
	let user = $state('');
	let password = $state('');
	let dbname = $state('');
	let result = $state<string | null>(null);
	let loading = $state(false);

	const dbTypes = [
		{ value: 'postgres', label: 'PostgreSQL' },
		{ value: 'mysql', label: 'MySQL' },
		{ value: 'sqlite', label: 'SQLite' }
	];

	// Melt-UI Select
	const {
		elements: { trigger: selectTrigger, menu: selectMenu, option },
		states: { open: selectOpen, selected }
	} = createSelect({
		defaultSelected: { value: 'postgres', label: 'PostgreSQL' },
		positioning: { placement: 'bottom', sameWidth: true }
	});

	// Sync selected value to dbtype
	$effect(() => {
		if ($selected?.value) {
			dbtype = $selected.value as string;
		}
	});

	async function connect() {
		loading = true;
		result = null;

		try {
			const config = new driver.Config({
				host,
				port,
				user,
				password,
				db: dbname
			});

			const req = new service.ConnectRequest({
				driver: dbtype,
				config
			});

			const res = await Connect(req);

			if (res.data?.connected) {
				goto('/workspace');
			} else {
				result = `Failed: ${res.errors?.[0]?.detail || 'Unknown error'}`;
			}
		} catch (e: any) {
			console.error('Caught error:', e);
			result = `Error: ${e.message}`;
		} finally {
			loading = false;
		}
	}
</script>

<div class="bg-background flex min-h-screen">
	<!-- Left Panel: Branding -->
	<div class="bg-muted/30 hidden w-1/2 items-center justify-center border-r lg:flex">
		<div class="text-center">
			<Database class="text-primary mx-auto mb-6 h-24 w-24" />
			<h1 class="text-4xl font-bold">RollingThunder</h1>
			<p class="text-muted-foreground mt-2">Database Desktop Manager</p>
			<p class="text-muted-foreground mt-1 text-sm">Beta Version</p>
		</div>
	</div>

	<!-- Right Panel: Connection Form -->
	<div class="flex flex-1 items-center justify-center p-8">
		<div class="w-full max-w-md space-y-6">
			<!-- Mobile Logo -->
			<div class="mb-8 text-center lg:hidden">
				<Database class="text-primary mx-auto mb-4 h-16 w-16" />
				<h1 class="text-2xl font-bold">RollingThunder</h1>
			</div>

			<div>
				<h2 class="text-2xl font-semibold tracking-tight">Connect to Database</h2>
				<p class="text-muted-foreground mt-1 text-sm">
					Enter your database credentials to get started
				</p>
			</div>

			<form
				onsubmit={(e) => {
					e.preventDefault();
					connect();
				}}
				class="space-y-4"
			>
				<!-- Database Type -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="dbtype">Database Type</label>
					<button
						use:melt={$selectTrigger}
						class="border-input bg-background hover:bg-accent hover:text-accent-foreground inline-flex h-10 w-full cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-sm"
					>
						{$selected?.label || 'Select type'}
						<ChevronDown class="h-4 w-4 opacity-50" />
					</button>
					{#if $selectOpen}
						<div
							use:melt={$selectMenu}
							class="bg-popover text-popover-foreground z-50 rounded-md border p-1 shadow-md"
							transition:fly={{ duration: 150, y: -10 }}
						>
							{#each dbTypes as db}
								<div
									use:melt={$option({ value: db.value, label: db.label })}
									class="hover:bg-accent hover:text-accent-foreground data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground cursor-pointer rounded-sm px-2 py-1.5 text-sm outline-none"
								>
									{db.label}
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Host -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="host">Host</label>
					<input
						id="host"
						bind:value={host}
						placeholder="127.0.0.1"
						disabled={loading}
						class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<!-- Port -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="port">Port</label>
					<input
						id="port"
						bind:value={port}
						placeholder="5432"
						disabled={loading}
						class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<!-- Username -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="user">Username</label>
					<input
						id="user"
						bind:value={user}
						placeholder="postgres"
						disabled={loading}
						class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<!-- Password -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="password">Password</label>
					<input
						id="password"
						type="password"
						bind:value={password}
						placeholder="••••••••"
						disabled={loading}
						class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<!-- Database Name -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="dbname">Database Name</label>
					<input
						id="dbname"
						bind:value={dbname}
						placeholder="myapp_db"
						disabled={loading}
						class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>

				<!-- Error Message -->
				{#if result}
					<div class="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
						{result}
					</div>
				{/if}

				<!-- Submit Button -->
				<button
					type="submit"
					class="bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-10 w-full cursor-pointer items-center justify-center rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50"
					disabled={loading}
				>
					{#if loading}
						<Loader2 class="mr-2 h-4 w-4 animate-spin" />
						Connecting...
					{:else}
						Connect
					{/if}
				</button>
			</form>
		</div>
	</div>
</div>
