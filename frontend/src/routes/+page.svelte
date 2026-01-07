<script lang="ts">
	import { Connect } from '$lib/wailsjs/go/db/Service';
	import { database as driver, db as service } from '$lib/wailsjs/go/models';
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Database, Loader2 } from 'lucide-svelte';

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
					<Select.Root type="single" name="dbtype">
						<Select.Trigger class="w-full">
							{dbTypes.find((d) => d.value === dbtype)?.label || 'Select type'}
						</Select.Trigger>
						<Select.Content class="w-full">
							{#each dbTypes as db}
								<Select.Item
									value={db.value}
									label={db.label}
									class=""
									onclick={() => (dbtype = db.value)}
								>
									{db.label}
								</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>

				<!-- Host -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="host">Host</label>
					<Input id="host" bind:value={host} placeholder="127.0.0.1" disabled={loading} />
				</div>

				<!-- Port -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="port">Port</label>
					<Input id="port" bind:value={port} placeholder="5432" disabled={loading} />
				</div>

				<!-- Username -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="user">Username</label>
					<Input id="user" bind:value={user} placeholder="postgres" disabled={loading} />
				</div>

				<!-- Password -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="password">Password</label>
					<Input
						id="password"
						type="password"
						bind:value={password}
						placeholder="••••••••"
						disabled={loading}
					/>
				</div>

				<!-- Database Name -->
				<div class="space-y-2">
					<label class="text-sm font-medium" for="dbname">Database Name</label>
					<Input id="dbname" bind:value={dbname} placeholder="myapp_db" disabled={loading} />
				</div>

				<!-- Error Message -->
				{#if result}
					<div class="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
						{result}
					</div>
				{/if}

				<!-- Submit Button -->
				<Button type="submit" class="w-full" disabled={loading}>
					{#if loading}
						<Loader2 class="mr-2 h-4 w-4 animate-spin" />
						Connecting...
					{:else}
						Connect
					{/if}
				</Button>
			</form>
		</div>
	</div>
</div>
