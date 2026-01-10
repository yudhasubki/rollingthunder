<script lang="ts">
	import {
		Connect,
		GetSavedConnections,
		SaveConnection,
		UpdateConnection,
		DeleteConnection
	} from '$lib/wailsjs/go/db/Service';
	import { database as driver, db as service } from '$lib/wailsjs/go/models';
	import { goto } from '$app/navigation';
	import { createSelect, createDialog, melt } from '@melt-ui/svelte';
	import { writable } from 'svelte/store';
	import {
		Database,
		Loader2,
		ChevronDown,
		Plus,
		Trash2,
		Edit2,
		Lock,
		Server,
		Palette,
		Save,
		AlertTriangle,
		ArrowLeft
	} from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { onMount } from 'svelte';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';

	// Connection form state
	let connectionName = $state('');
	let connectionColor = $state('#3B82F6');
	let dbtype = $state('postgres');
	let host = $state('127.0.0.1');
	let port = $state('5432');
	let user = $state('');
	let password = $state('');
	let dbname = $state('');
	let sslMode = $state('disable');
	let sslRootCert = $state('');
	let sslCert = $state('');
	let sslKey = $state('');

	let result = $state<string | null>(null);
	let loading = $state(false);
	let saving = $state(false);
	let editingId = $state<string | null>(null);

	// Saved connections
	let savedConnections = $state<any[]>([]);

	// Context menu state
	let contextMenuConn = $state<any>(null);
	let contextMenuPos = $state({ x: 0, y: 0 });
	let showContextMenu = $state(false);

	// Delete confirmation modal
	let deleteTargetId = $state<string | null>(null);
	let deleteTargetName = $state<string>('');
	let deleting = $state(false);

	const deleteOpenStore = writable(false);
	const {
		elements: { overlay, content, title, description, close, portalled },
		states: { open: deleteDialogOpen }
	} = createDialog({
		open: deleteOpenStore,
		forceVisible: true
	});

	function handleContextMenu(e: MouseEvent, conn: any) {
		e.preventDefault();
		contextMenuConn = conn;
		contextMenuPos = { x: e.clientX, y: e.clientY };
		showContextMenu = true;
	}

	function closeContextMenu() {
		showContextMenu = false;
		contextMenuConn = null;
	}

	function openDeleteModal() {
		if (!contextMenuConn) return;
		deleteTargetId = contextMenuConn.id;
		deleteTargetName = contextMenuConn.config?.name || 'Unnamed';
		closeContextMenu();
		deleteOpenStore.set(true);
	}

	function closeDeleteModal() {
		deleteOpenStore.set(false);
		deleteTargetId = null;
		deleteTargetName = '';
		deleting = false;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		deleting = true;
		try {
			await DeleteConnection(deleteTargetId);
			await loadSavedConnections();
			if (editingId === deleteTargetId) {
				newConnection();
			}
		} catch (e: any) {
			result = e.message;
		}
		closeDeleteModal();
	}

	const dbTypes = [
		{ value: 'postgres', label: 'PostgreSQL' },
		{ value: 'mysql', label: 'MySQL' },
		{ value: 'sqlite', label: 'SQLite' }
	];

	const sslModes = [
		{ value: 'disable', label: 'Disable' },
		{ value: 'require', label: 'Require' },
		{ value: 'verify-ca', label: 'Verify CA' },
		{ value: 'verify-full', label: 'Verify Full' }
	];

	const colors = [
		'#EF4444',
		'#F97316',
		'#EAB308',
		'#22C55E',
		'#14B8A6',
		'#3B82F6',
		'#6366F1',
		'#8B5CF6',
		'#EC4899',
		'#6B7280'
	];

	// Melt-UI Select for DB Type
	const {
		elements: { trigger: selectTrigger, menu: selectMenu, option },
		states: { open: selectOpen, selected }
	} = createSelect({
		defaultSelected: { value: 'postgres', label: 'PostgreSQL' },
		positioning: { placement: 'bottom', sameWidth: true }
	});

	// Melt-UI Select for SSL Mode
	const {
		elements: { trigger: sslTrigger, menu: sslMenu, option: sslOption },
		states: { open: sslOpen, selected: sslSelected }
	} = createSelect({
		defaultSelected: { value: 'disable', label: 'Disable' },
		positioning: { placement: 'bottom', sameWidth: true }
	});

	$effect(() => {
		if ($selected?.value) {
			dbtype = $selected.value as string;
		}
	});

	$effect(() => {
		if ($sslSelected?.value) {
			sslMode = $sslSelected.value as string;
		}
	});

	onMount(async () => {
		await loadSavedConnections();
		connectionStore.refreshConnections();
	});

	async function loadSavedConnections() {
		try {
			const res = await GetSavedConnections();
			savedConnections = res.data || [];
		} catch (e) {
			console.error('Failed to load connections:', e);
		}
	}

	function selectConnection(conn: any) {
		editingId = conn.id;
		const cfg = conn.config;
		connectionName = cfg.name || '';
		connectionColor = cfg.color || '#3B82F6';
		host = cfg.host || '127.0.0.1';
		port = cfg.port || '5432';
		user = cfg.user || '';
		password = cfg.password || '';
		dbname = cfg.db || '';
		sslMode = cfg.sslMode || 'disable';
		sslRootCert = cfg.sslRootCert || '';
		sslCert = cfg.sslCert || '';
		sslKey = cfg.sslKey || '';
	}

	function newConnection() {
		editingId = null;
		connectionName = '';
		connectionColor = '#3B82F6';
		host = '127.0.0.1';
		port = '5432';
		user = '';
		password = '';
		dbname = '';
		sslMode = 'disable';
		sslRootCert = '';
		sslCert = '';
		sslKey = '';
		result = null;
	}

	async function saveCurrentConnection() {
		if (!connectionName.trim()) {
			result = 'Connection name is required to save';
			return;
		}

		saving = true;
		result = null;
		try {
			const config = new driver.Config({
				name: connectionName,
				color: connectionColor,
				host,
				port,
				user,
				password,
				db: dbname,
				sslMode,
				sslRootCert,
				sslCert,
				sslKey
			});

			let res;
			if (editingId) {
				// Update existing connection
				res = await UpdateConnection(editingId, config);
			} else {
				// Create new connection
				res = await SaveConnection(config);
			}

			if (res.errors?.length) {
				result = res.errors[0].detail;
			} else {
				await loadSavedConnections();
				editingId = res.data?.id || editingId;
			}
		} catch (e: any) {
			result = e.message;
		} finally {
			saving = false;
		}
	}

	async function connect() {
		loading = true;
		result = null;

		try {
			const config = new driver.Config({
				name: connectionName,
				color: connectionColor,
				host,
				port,
				user,
				password,
				db: dbname,
				sslMode,
				sslRootCert,
				sslCert,
				sslKey
			});

			const req = new service.ConnectRequest({
				driver: dbtype,
				config
			});

			const res = await Connect(req);

			if (res.data?.connected) {
				goto('/workspace');
			} else {
				result = `${res.errors?.[0]?.detail || 'Unknown error'}`;
			}
		} catch (e: any) {
			console.error('Caught error:', e);
			result = `${e.message}`;
		} finally {
			loading = false;
		}
	}
</script>

<div class="bg-background flex h-screen">
	<!-- Left Sidebar: Saved Connections -->
	<div class="bg-muted/30 flex w-72 flex-col border-r">
		<!-- Header -->
		<div class="flex items-center justify-between border-b p-4">
			<div class="flex items-center gap-2">
				{#if connectionStore.connections.length > 0}
					<button
						type="button"
						class="hover:bg-accent -ml-1 mr-1 rounded-md p-1.5 transition-colors"
						onclick={() => goto('/workspace')}
						title="Back to workspace"
					>
						<ArrowLeft class="h-4 w-4" />
					</button>
				{/if}
				<img src="/logo.png" alt="RollingThunder" class="h-6 w-6" />
				<span class="font-semibold">Connections</span>
			</div>
			<button
				type="button"
				class="hover:bg-accent rounded-md p-1.5 transition-colors"
				onclick={newConnection}
				title="New Connection"
			>
				<Plus class="h-4 w-4" />
			</button>
		</div>

		<!-- Connection List -->
		<div class="flex-1 overflow-auto p-2">
			{#if savedConnections.length === 0}
				<div class="text-muted-foreground py-8 text-center text-sm">
					<Database class="mx-auto mb-2 h-8 w-8 opacity-50" />
					<p>No saved connections</p>
					<p class="mt-1 text-xs">Create one to get started</p>
				</div>
			{:else}
				<div class="space-y-1">
					{#each savedConnections as conn (conn.id)}
						<button
							type="button"
							class="hover:bg-accent flex w-full items-center gap-3 rounded-lg p-3 text-left transition-colors {editingId ===
							conn.id
								? 'bg-accent'
								: ''}"
							onclick={() => selectConnection(conn)}
							oncontextmenu={(e) => handleContextMenu(e, conn)}
						>
							<div
								class="h-3 w-3 shrink-0 rounded-full"
								style="background-color: {conn.config?.color || '#3B82F6'}"
							></div>
							<div class="flex-1 truncate">
								<div class="truncate text-sm font-medium">
									{conn.config?.name || 'Unnamed'}
								</div>
								<div class="text-muted-foreground truncate text-xs">
									{conn.config?.host || 'localhost'}:{conn.config?.port || '5432'}/{conn.config
										?.db || ''}
								</div>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Footer -->
		<div class="text-muted-foreground border-t px-4 py-2 text-xs">
			{savedConnections.length} connection{savedConnections.length !== 1 ? 's' : ''}
		</div>
	</div>

	<!-- Right Panel: Connection Form -->
	<div class="flex flex-1 flex-col overflow-hidden">
		<!-- Form Header -->
		<div class="flex items-center justify-between border-b px-6 py-4">
			<div>
				<h2 class="text-xl font-semibold">
					{editingId ? 'Edit Connection' : 'New Connection'}
				</h2>
				<p class="text-muted-foreground text-sm">Configure your database connection</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					type="button"
					class="border-input bg-background hover:bg-accent inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors disabled:opacity-50"
					onclick={saveCurrentConnection}
					disabled={saving}
				>
					{#if saving}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<Save class="h-4 w-4" />
					{/if}
					Save
				</button>
			</div>
		</div>

		<!-- Form Content -->
		<div class="flex-1 overflow-auto p-6">
			<form
				onsubmit={(e) => {
					e.preventDefault();
					connect();
				}}
				class="mx-auto max-w-2xl space-y-6"
			>
				<!-- Connection Info Section -->
				<div class="space-y-4">
					<div class="flex items-center gap-2 text-sm font-medium">
						<Server class="h-4 w-4" />
						Connection Info
					</div>

					<div class="grid grid-cols-2 gap-4">
						<!-- Connection Name -->
						<div class="space-y-2">
							<label class="text-sm font-medium" for="connName">Name</label>
							<input
								id="connName"
								bind:value={connectionName}
								placeholder="My Database"
								disabled={loading}
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
							/>
						</div>

						<!-- Color Picker -->
						<div class="space-y-2">
							<label class="text-sm font-medium">Color</label>
							<div class="flex gap-1.5">
								{#each colors as color}
									<button
										type="button"
										class="h-8 w-8 rounded-md transition-transform hover:scale-110 {connectionColor ===
										color
											? 'ring-primary ring-2 ring-offset-2'
											: ''}"
										style="background-color: {color}"
										onclick={() => (connectionColor = color)}
									></button>
								{/each}
							</div>
						</div>
					</div>

					<div class="grid grid-cols-2 gap-4">
						<!-- Database Type -->
						<div class="space-y-2">
							<label class="text-sm font-medium" for="dbtype">Database Type</label>
							<button
								use:melt={$selectTrigger}
								class="border-input bg-background hover:bg-accent inline-flex h-10 w-full cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-sm"
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
											class="hover:bg-accent data-[highlighted]:bg-accent cursor-pointer rounded-sm px-2 py-1.5 text-sm outline-none"
										>
											{db.label}
										</div>
									{/each}
								</div>
							{/if}
						</div>

						<!-- Database Name -->
						<div class="space-y-2">
							<label class="text-sm font-medium" for="dbname">Database</label>
							<input
								id="dbname"
								bind:value={dbname}
								placeholder="myapp_db"
								disabled={loading}
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
							/>
						</div>
					</div>

					<div class="grid grid-cols-4 gap-4">
						<!-- Host -->
						<div class="col-span-2 space-y-2">
							<label class="text-sm font-medium" for="host">Host</label>
							<input
								id="host"
								bind:value={host}
								placeholder="127.0.0.1"
								disabled={loading}
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
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
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
							/>
						</div>

						<!-- SSL Mode -->
						<div class="space-y-2">
							<label class="text-sm font-medium">SSL</label>
							<button
								use:melt={$sslTrigger}
								class="border-input bg-background hover:bg-accent inline-flex h-10 w-full cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-sm"
							>
								{$sslSelected?.label || 'Disable'}
								<ChevronDown class="h-4 w-4 opacity-50" />
							</button>
							{#if $sslOpen}
								<div
									use:melt={$sslMenu}
									class="bg-popover text-popover-foreground z-50 rounded-md border p-1 shadow-md"
									transition:fly={{ duration: 150, y: -10 }}
								>
									{#each sslModes as mode}
										<div
											use:melt={$sslOption({ value: mode.value, label: mode.label })}
											class="hover:bg-accent data-[highlighted]:bg-accent cursor-pointer rounded-sm px-2 py-1.5 text-sm outline-none"
										>
											{mode.label}
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</div>

					<div class="grid grid-cols-2 gap-4">
						<!-- Username -->
						<div class="space-y-2">
							<label class="text-sm font-medium" for="user">Username</label>
							<input
								id="user"
								bind:value={user}
								placeholder="postgres"
								disabled={loading}
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
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
								class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
							/>
						</div>
					</div>
				</div>

				<!-- SSL Certificates Section (only if SSL enabled) -->
				{#if sslMode !== 'disable'}
					<div class="space-y-4">
						<div class="flex items-center gap-2 text-sm font-medium">
							<Lock class="h-4 w-4" />
							SSL Certificates
						</div>

						<div class="space-y-4">
							<div class="space-y-2">
								<label class="text-sm font-medium" for="sslRootCert">CA Certificate Path</label>
								<input
									id="sslRootCert"
									bind:value={sslRootCert}
									placeholder="/path/to/ca-certificate.crt"
									disabled={loading}
									class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
								/>
							</div>

							<div class="grid grid-cols-2 gap-4">
								<div class="space-y-2">
									<label class="text-sm font-medium" for="sslCert">Client Certificate Path</label>
									<input
										id="sslCert"
										bind:value={sslCert}
										placeholder="/path/to/client-cert.crt"
										disabled={loading}
										class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
									/>
								</div>

								<div class="space-y-2">
									<label class="text-sm font-medium" for="sslKey">Client Key Path</label>
									<input
										id="sslKey"
										bind:value={sslKey}
										placeholder="/path/to/client-key.key"
										disabled={loading}
										class="border-input bg-background placeholder:text-muted-foreground focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2"
									/>
								</div>
							</div>
						</div>
					</div>
				{/if}

				<!-- Error Message -->
				{#if result}
					<div class="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
						{result}
					</div>
				{/if}

				<!-- Submit Button -->
				<button
					type="submit"
					class="bg-primary text-primary-foreground hover:bg-primary/90 inline-flex h-11 w-full cursor-pointer items-center justify-center rounded-md px-4 py-2 text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50"
					disabled={loading}
				>
					{#if loading}
						<Loader2 class="mr-2 h-4 w-4 animate-spin" />
						Connecting...
					{:else}
						<Database class="mr-2 h-4 w-4" />
						Connect
					{/if}
				</button>
			</form>
		</div>

		<!-- Footer -->
		<div class="text-muted-foreground border-t px-6 py-3 text-center text-xs">
			RollingThunder Database Manager · Beta
		</div>
	</div>
</div>

<!-- Context Menu -->
{#if showContextMenu}
	<button
		type="button"
		class="fixed inset-0 z-40 cursor-default"
		onclick={closeContextMenu}
		onkeydown={(e) => e.key === 'Escape' && closeContextMenu()}
	></button>
	<div
		class="bg-popover text-popover-foreground fixed z-50 min-w-[160px] rounded-md border p-1 shadow-md"
		style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px"
	>
		<button
			type="button"
			class="hover:bg-destructive/10 text-destructive flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm"
			onclick={openDeleteModal}
		>
			<Trash2 class="h-4 w-4" />
			Delete Connection
		</button>
	</div>
{/if}

<!-- Delete Confirmation Modal -->
{#if $deleteDialogOpen}
	<div use:melt={$portalled}>
		<div use:melt={$overlay} class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"></div>
		<div
			use:melt={$content}
			class="bg-popover text-popover-foreground fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg"
		>
			<div class="flex items-start gap-4">
				<div
					class="bg-destructive/10 text-destructive flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
				>
					<AlertTriangle class="h-5 w-5" />
				</div>
				<div class="flex-1">
					<h2 use:melt={$title} class="text-lg font-semibold">Delete Connection</h2>
					<p use:melt={$description} class="text-muted-foreground mt-2 text-sm">
						Are you sure you want to delete <strong>"{deleteTargetName}"</strong>? This action
						cannot be undone.
					</p>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					use:melt={$close}
					type="button"
					class="border-input bg-background hover:bg-accent hover:text-accent-foreground rounded-md border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					onclick={closeDeleteModal}
					disabled={deleting}
				>
					Cancel
				</button>
				<button
					type="button"
					class="inline-flex items-center justify-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
					onclick={executeDelete}
					disabled={deleting}
				>
					{#if deleting}
						<div
							class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"
						></div>
						Deleting...
					{:else}
						Delete
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}
