<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Connect,
		ChooseSQLiteDatabaseFile,
		DeleteConnection,
		GetSavedConnections,
		SaveConnection,
		UpdateConnection
	} from '$lib/wailsjs/go/db/Service';
	import {
		CONNECTION_TIMEOUT_SECONDS,
		cancelConnectionAttempt,
		createConnectionAttemptID,
		startConnectionElapsedTimer
	} from '$lib/connection/attempt';
	import { createServiceError } from '$lib/errors/service';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { database, db } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import {
		AlertCircle,
		ArrowLeft,
		Check,
		ChevronRight,
		Database,
		Eye,
		EyeOff,
		FilePlus2,
		FolderOpen,
		Loader2,
		Lock,
		Play,
		Plus,
		Save,
		Search,
		Server,
		Trash2,
		X
	} from 'lucide-svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
		onConnected?: () => void;
		initialProfileId?: string | null;
		startNew?: boolean;
	}

	let {
		open,
		onClose,
		onConnected = () => {},
		initialProfileId = null,
		startNew = false
	}: Props = $props();

	type ProviderId = 'postgres' | 'mysql' | 'sqlite';

	const profileColors = ['#ef5b50', '#f59e0b', '#22c55e', '#0ea5e9', '#6366f1', '#a855f7'];
	const providers: Array<{
		id: ProviderId;
		name: string;
		description: string;
		defaultPort: string;
		available: boolean;
		mark: string;
	}> = [
		{
			id: 'postgres',
			name: 'PostgreSQL',
			description: 'Schemas, relations, indexes, and SQL tools.',
			defaultPort: '5432',
			available: true,
			mark: 'PG'
		},
		{
			id: 'mysql',
			name: 'MySQL',
			description: 'MySQL and compatible server connections.',
			defaultPort: '3306',
			available: true,
			mark: 'MY'
		},
		{
			id: 'sqlite',
			name: 'SQLite',
			description: 'Open a local SQLite database file.',
			defaultPort: '',
			available: true,
			mark: 'SQ'
		}
	];
	const sslOptions = [
		{ value: 'disable', label: 'Disable' },
		{ value: 'require', label: 'Require' },
		{ value: 'verify-ca', label: 'Verify CA' },
		{ value: 'verify-full', label: 'Verify full' }
	];

	let profiles = $state<db.SavedConnection[]>([]);
	let searchQuery = $state('');
	let loadingProfiles = $state(false);
	let editingId = $state<string | null>(null);
	let action = $state<'save' | 'connect' | 'delete' | null>(null);
	let message = $state('');
	let messageLevel = $state<'info' | 'error' | 'success'>('info');
	let deleteConfirm = $state(false);
	let showPassword = $state(false);
	let loadedForOpen = $state(false);
	let connectionAttemptID = $state<string | null>(null);
	let connectionElapsedSeconds = $state(0);
	let cancellingConnection = $state(false);
	let stopConnectionElapsedTimer: (() => void) | null = null;

	let connectionName = $state('');
	let connectionColor = $state('#ef5b50');
	let host = $state('127.0.0.1');
	let port = $state('5432');
	let username = $state('');
	let password = $state('');
	let databaseName = $state('');
	let sslMode = $state('disable');
	let sslRootCert = $state('');
	let sslCert = $state('');
	let sslKey = $state('');
	let provider = $state<ProviderId | ''>('');

	const filteredProfiles = $derived(
		searchQuery.trim()
			? profiles.filter((profile) => {
					const config = profile.config;
					return `${config.name} ${config.driver || 'postgres'} ${config.host} ${config.db}`
						.toLowerCase()
						.includes(searchQuery.trim().toLowerCase());
				})
			: profiles
	);

	const endpoint = $derived(
		provider === 'sqlite'
			? databaseName || 'Choose a local database file'
			: `${host || 'host'}:${port || 'port'} / ${databaseName || 'database'}`
	);
	const selectedProvider = $derived(providers.find((item) => item.id === provider) ?? null);

	$effect(() => {
		if (open && !loadedForOpen) {
			loadedForOpen = true;
			void loadProfiles(initialProfileId, startNew);
		} else if (!open) {
			loadedForOpen = false;
			deleteConfirm = false;
			message = '';
		}
	});

	onMount(() => {
		const handleKeydown = (event: KeyboardEvent) => {
			if (open && event.key === 'Escape' && !action) onClose();
		};
		window.addEventListener('keydown', handleKeydown);
		return () => {
			window.removeEventListener('keydown', handleKeydown);
			stopConnectionElapsedTimer?.();
			if (connectionAttemptID) {
				void cancelConnectionAttempt(connectionAttemptID).catch(() => {});
			}
		};
	});

	async function loadProfiles(selectId?: string | null, createNew = false) {
		loadingProfiles = true;
		try {
			const response = await GetSavedConnections();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load saved connections');
			}
			profiles = response.data || [];

			if (createNew) {
				newProfile();
				return;
			}

			const requestedProfile = profiles.find((profile) => profile.id === selectId);
			const currentProfile = profiles.find((profile) => profile.id === editingId);
			if (requestedProfile) {
				selectProfile(requestedProfile);
			} else if (currentProfile) {
				selectProfile(currentProfile);
			} else if (profiles.length > 0 && editingId === null && !connectionName) {
				selectProfile(profiles[0]);
			}
		} catch (error: any) {
			showMessage(error?.message || 'Could not load saved connections', 'error');
		} finally {
			loadingProfiles = false;
		}
	}

	function showMessage(text: string, level: 'info' | 'error' | 'success' = 'info') {
		message = text;
		messageLevel = level;
	}

	function closeModal() {
		if (!action) onClose();
	}

	function selectProfile(profile: db.SavedConnection) {
		const config = profile.config;
		editingId = profile.id;
		connectionName = config.name || '';
		connectionColor = config.color || '#ef5b50';
		const profileProvider = (config.driver as ProviderId) || 'postgres';
		host = profileProvider === 'sqlite' ? '' : config.host || '127.0.0.1';
		port =
			profileProvider === 'sqlite'
				? ''
				: config.port ||
					providers.find((item) => item.id === profileProvider)?.defaultPort ||
					'5432';
		username = config.user || '';
		password = config.password || '';
		databaseName = config.db || '';
		sslMode = config.sslMode || 'disable';
		sslRootCert = config.sslRootCert || '';
		sslCert = config.sslCert || '';
		sslKey = config.sslKey || '';
		provider = profileProvider;
		deleteConfirm = false;
		message = '';
	}

	function newProfile() {
		editingId = null;
		connectionName = '';
		connectionColor = '#ef5b50';
		host = '127.0.0.1';
		port = '5432';
		username = '';
		password = '';
		databaseName = '';
		sslMode = 'disable';
		sslRootCert = '';
		sslCert = '';
		sslKey = '';
		provider = '';
		deleteConfirm = false;
		showPassword = false;
		message = '';
	}

	function isConnected(profile: db.SavedConnection) {
		return connectionStore.connections.some(
			(connection) =>
				connection.name === profile.config.name &&
				connection.driver === (profile.config.driver || 'postgres') &&
				connection.host === profile.config.host &&
				connection.database === profile.config.db
		);
	}

	function buildConfig() {
		const sqlite = provider === 'sqlite';
		return new database.Config({
			name: connectionName.trim(),
			color: connectionColor,
			driver: provider || 'postgres',
			host: sqlite ? '' : host.trim(),
			port: sqlite ? '' : port.trim(),
			user: sqlite ? '' : username.trim(),
			password: sqlite ? '' : password,
			db: databaseName.trim(),
			sslMode: sqlite ? 'disable' : sslMode,
			sslRootCert: sqlite ? '' : sslRootCert.trim(),
			sslCert: sqlite ? '' : sslCert.trim(),
			sslKey: sqlite ? '' : sslKey.trim()
		});
	}

	function validate(requireName = true) {
		if (requireName && !connectionName.trim()) {
			showMessage('Add a profile name before saving.', 'error');
			return false;
		}
		if (!provider) {
			showMessage('Choose a database provider first.', 'error');
			return false;
		}
		if (provider === 'sqlite' && !databaseName.trim()) {
			showMessage('Choose an existing SQLite file or a path for a new database.', 'error');
			return false;
		}
		if (provider !== 'sqlite' && (!host.trim() || !port.trim() || !databaseName.trim())) {
			showMessage('Host, port, and database are required.', 'error');
			return false;
		}
		return true;
	}

	function selectProvider(nextProvider: (typeof providers)[number]) {
		if (!nextProvider.available) return;

		const currentProvider = providers.find((item) => item.id === provider);
		if (!port || port === currentProvider?.defaultPort) {
			port = nextProvider.defaultPort;
		}
		provider = nextProvider.id;
		if (nextProvider.id === 'sqlite') {
			host = '';
			port = '';
			username = '';
			password = '';
			sslMode = 'disable';
		} else {
			host ||= '127.0.0.1';
		}
		message = '';
	}

	function profileEndpoint(profile: db.SavedConnection): string {
		const config = profile.config;
		if ((config.driver || 'postgres') === 'sqlite') return config.db;
		return `${config.host}:${config.port}/${config.db}`;
	}

	async function chooseSQLiteFile(create: boolean) {
		if (action !== null) return;
		try {
			const response = await ChooseSQLiteDatabaseFile(create);
			if (response.errors?.length) {
				throw createServiceError(
					response.errors[0],
					create ? 'Could not choose a new SQLite file' : 'Could not open SQLite file'
				);
			}
			if (response.data) {
				databaseName = response.data;
				if (!connectionName.trim()) {
					const leaf = response.data.split(/[\\/]/).pop() || 'SQLite';
					connectionName = leaf.replace(/\.(sqlite3?|db)$/i, '') || 'SQLite';
				}
				showMessage(
					create
						? 'The database file will be created when you connect.'
						: 'SQLite database selected.',
					'info'
				);
			}
		} catch (error: any) {
			showMessage(error?.message || 'Could not choose SQLite database', 'error');
		}
	}

	async function saveProfile() {
		if (!validate()) return;
		action = 'save';
		showMessage(editingId ? 'Updating connection profile…' : 'Saving new connection profile…');

		try {
			const response = editingId
				? await UpdateConnection(editingId, buildConfig())
				: await SaveConnection(buildConfig());
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not save connection profile');
			}

			const savedId = response.data?.id || editingId;
			await loadProfiles(savedId);
			showMessage('Connection profile saved.', 'success');
			updateStatus(`Saved connection profile “${connectionName}”`, 'success');
		} catch (error: any) {
			showMessage(error?.message || 'Could not save connection profile', 'error');
		} finally {
			action = null;
		}
	}

	async function connectProfile() {
		if (!validate(false)) return;
		const attemptID = createConnectionAttemptID();
		action = 'connect';
		connectionAttemptID = attemptID;
		connectionElapsedSeconds = 0;
		cancellingConnection = false;
		stopConnectionElapsedTimer?.();
		stopConnectionElapsedTimer = startConnectionElapsedTimer((seconds) => {
			connectionElapsedSeconds = seconds;
		});
		showMessage(
			`Connecting to ${endpoint}. Automatic timeout after ${CONNECTION_TIMEOUT_SECONDS} seconds.`
		);

		try {
			const request = new db.ConnectRequest({
				driver: provider || 'postgres',
				config: buildConfig(),
				attemptId: attemptID
			});
			const response = await Connect(request);
			if (response.errors?.length || !response.data?.connected) {
				throw createServiceError(response.errors?.[0], 'Connection failed');
			}

			await connectionStore.refreshConnections();
			window.dispatchEvent(new CustomEvent('connection-switched'));
			updateStatus(`Connected to ${connectionName || databaseName}`, 'success');
			onConnected();
			onClose();
		} catch (error: any) {
			const detail = error?.message || 'Could not connect to the database';
			showMessage(
				detail,
				error?.code === 'CONNECTION_CANCELLED' || detail.toLowerCase().includes('cancelled')
					? 'info'
					: 'error'
			);
		} finally {
			if (connectionAttemptID === attemptID) {
				connectionAttemptID = null;
				stopConnectionElapsedTimer?.();
				stopConnectionElapsedTimer = null;
			}
			cancellingConnection = false;
			action = null;
		}
	}

	async function cancelConnection() {
		if (!connectionAttemptID || cancellingConnection) return;
		const attemptID = connectionAttemptID;
		cancellingConnection = true;
		showMessage(`Cancelling connection to ${endpoint}…`);

		try {
			await cancelConnectionAttempt(attemptID);
		} catch (error: any) {
			cancellingConnection = false;
			showMessage(error?.message || 'Could not cancel connection attempt', 'error');
		}
	}

	async function deleteProfile() {
		if (!editingId) return;
		if (!deleteConfirm) {
			deleteConfirm = true;
			return;
		}

		action = 'delete';
		try {
			const response = await DeleteConnection(editingId);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not delete connection profile');
			}
			const deletedName = connectionName;
			newProfile();
			await loadProfiles();
			showMessage(`Deleted “${deletedName}”.`, 'success');
		} catch (error: any) {
			showMessage(error?.message || 'Could not delete connection profile', 'error');
		} finally {
			action = null;
			deleteConfirm = false;
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[100] flex items-center justify-center p-6">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			aria-label="Close connection manager"
			onclick={closeModal}
		></button>

		<div
			class="rt-popover relative flex h-[min(640px,calc(100vh-48px))] w-[min(960px,calc(100vw-48px))] flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="connection-manager-title"
		>
			<header class="flex h-14 shrink-0 items-center justify-between border-b px-4">
				<div class="flex min-w-0 items-center gap-2.5">
					<img src="/logo.png" alt="" class="rt-brand-logo h-8 w-8 rounded-lg" />
					<div class="min-w-0">
						<h2 id="connection-manager-title" class="text-[13px] font-bold">Manage connections</h2>
						<p class="text-muted-foreground mt-0.5 text-[9px]">
							Add, edit, or connect a database profile in one place.
						</p>
					</div>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-8 w-8"
					onclick={closeModal}
					disabled={action !== null}
					aria-label="Close connection manager"
				>
					<X class="h-4 w-4" />
				</button>
			</header>

			<div class="grid min-h-0 flex-1 grid-cols-[286px_minmax(0,1fr)]">
				<aside class="flex min-h-0 flex-col border-r bg-[var(--surface-sunken)]">
					<div class="border-b p-3">
						<div class="relative">
							<Search
								class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2"
							/>
							<input
								type="search"
								class="rt-input h-8 w-full pr-3 pl-8 text-[10px]"
								placeholder="Search profiles"
								bind:value={searchQuery}
							/>
						</div>
						<button
							type="button"
							class="mt-2 flex h-8 w-full items-center justify-center gap-2 rounded-md border bg-[var(--surface-raised)] text-[10px] font-bold hover:bg-[var(--surface-hover)]"
							onclick={newProfile}
						>
							<Plus class="h-3.5 w-3.5" />
							New connection
						</button>
					</div>

					<div class="min-h-0 flex-1 overflow-auto p-2">
						<div
							class="text-muted-foreground flex items-center justify-between px-2 py-1.5 text-[8px] font-bold tracking-[0.12em] uppercase"
						>
							<span>Saved profiles</span>
							<span>{filteredProfiles.length}</span>
						</div>

						{#if loadingProfiles}
							<div
								class="text-muted-foreground flex items-center justify-center gap-2 py-10 text-[10px]"
							>
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
								Loading profiles
							</div>
						{:else if filteredProfiles.length === 0}
							<div class="text-muted-foreground px-5 py-10 text-center">
								<Database class="mx-auto h-5 w-5 opacity-50" />
								<p class="mt-2 text-[10px] font-semibold">
									{searchQuery ? 'No matching profiles' : 'No saved profiles'}
								</p>
							</div>
						{:else}
							<div class="space-y-0.5">
								{#each filteredProfiles as profile (profile.id)}
									{@const profileProvider =
										providers.find((item) => item.id === (profile.config.driver || 'postgres')) ??
										providers[0]}
									<button
										type="button"
										class="group relative flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left transition-colors {editingId ===
										profile.id
											? 'text-foreground bg-[var(--surface-raised)] shadow-sm'
											: 'text-muted-foreground hover:text-foreground hover:bg-[var(--surface-hover)]'}"
										onclick={() => selectProfile(profile)}
									>
										{#if editingId === profile.id}
											<span class="bg-foreground absolute top-2 bottom-2 left-0 w-0.5 rounded-r"
											></span>
										{/if}
										<span
											class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border font-mono text-[8px] font-bold"
										>
											{profileProvider.mark}
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[10px] font-bold">
												{profile.config.name || 'Unnamed profile'}
											</span>
											<span class="mt-0.5 block truncate font-mono text-[8px]">
												{profileEndpoint(profile)}
											</span>
										</span>
										{#if isConnected(profile)}
											<span
												class="h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-500"
												title="Connected"
											></span>
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				</aside>

				<form
					class="rt-connection-form flex min-h-0 min-w-0 flex-col bg-[var(--surface-raised)]"
					onsubmit={(event) => {
						event.preventDefault();
						void connectProfile();
					}}
				>
					<div class="flex h-[66px] shrink-0 items-center justify-between border-b px-5">
						<div class="flex min-w-0 items-center gap-3">
							{#if selectedProvider}
								<button
									type="button"
									class="rt-toolbar-button h-8 shrink-0 gap-1.5 px-2.5 text-[9px] font-semibold"
									onclick={() => {
										provider = '';
										message = '';
									}}
									disabled={action !== null}
									aria-label="Back to database providers"
								>
									<ArrowLeft class="h-3 w-3" />
									Back
								</button>
								<span class="h-8 border-l"></span>
							{/if}
							<div class="min-w-0">
								<p class="text-muted-foreground text-[8px] font-bold tracking-[0.12em] uppercase">
									{selectedProvider
										? editingId
											? 'Saved profile'
											: 'New profile'
										: 'New connection'}
								</p>
								<h3 class="mt-1 truncate text-[13px] font-bold">
									{selectedProvider
										? connectionName || 'Untitled connection'
										: 'Choose a database provider'}
								</h3>
							</div>
						</div>
						{#if selectedProvider}
							<div class="text-right">
								<p class="text-muted-foreground font-mono text-[8px]">{endpoint}</p>
								<p class="mt-1 text-[8px] font-semibold">{selectedProvider.name}</p>
							</div>
						{:else}
							<span class="text-muted-foreground text-[9px] font-semibold">Step 1 of 2</span>
						{/if}
					</div>

					<div class="min-h-0 flex-1 overflow-auto p-5">
						{#if message}
							<div
								class="mb-4 flex items-center gap-2 rounded-md border px-3 py-2 text-[9px] font-semibold {messageLevel ===
								'error'
									? 'border-red-500/25 bg-red-500/8 text-red-500'
									: messageLevel === 'success'
										? 'border-emerald-500/25 bg-emerald-500/8 text-emerald-600'
										: 'text-muted-foreground bg-[var(--surface-sunken)]'}"
							>
								{#if action}
									<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin" />
								{:else if messageLevel === 'success'}
									<Check class="h-3.5 w-3.5 shrink-0" />
								{:else}
									<AlertCircle class="h-3.5 w-3.5 shrink-0" />
								{/if}
								<span class="min-w-0 flex-1">{message}</span>
								{#if action === 'connect'}
									<span class="shrink-0 font-mono text-[8px] tabular-nums">
										{connectionElapsedSeconds}s / {CONNECTION_TIMEOUT_SECONDS}s
									</span>
								{/if}
							</div>
						{/if}

						{#if !selectedProvider}
							<div class="mx-auto max-w-[520px] py-2">
								<p class="text-muted-foreground text-[9px] font-bold tracking-[0.12em] uppercase">
									Database engine
								</p>
								<h4 class="mt-2 text-base font-bold tracking-[-0.02em]">Choose your provider</h4>
								<p class="text-muted-foreground mt-1 text-[10px] leading-relaxed">
									The connection form will adapt to the selected database engine.
								</p>

								<div class="mt-5 space-y-2">
									{#each providers as item (item.id)}
										<button
											type="button"
											class="group flex w-full items-center gap-3 rounded-lg border px-3.5 py-3 text-left transition-colors {item.available
												? 'cursor-pointer bg-[var(--surface-raised)] hover:border-[var(--brand-border)] hover:bg-[var(--brand-soft)]'
												: 'cursor-not-allowed bg-[var(--surface-sunken)] opacity-55'}"
											onclick={() => selectProvider(item)}
											disabled={!item.available}
										>
											<span
												class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)] font-mono text-[10px] font-bold"
											>
												{item.mark}
											</span>
											<span class="min-w-0 flex-1">
												<span class="block text-[11px] font-bold">{item.name}</span>
												<span class="text-muted-foreground mt-0.5 block text-[9px]">
													{item.description}
												</span>
											</span>
											{#if item.available}
												<span class="text-primary text-[8px] font-bold tracking-wide uppercase">
													Available
												</span>
												<ChevronRight class="text-muted-foreground h-3.5 w-3.5" />
											{:else}
												<span
													class="text-muted-foreground rounded-full border px-2 py-1 text-[8px] font-semibold"
												>
													Coming soon
												</span>
											{/if}
										</button>
									{/each}
								</div>
							</div>
						{:else}
							<div class="grid grid-cols-2 gap-x-4 gap-y-4">
								<div class="col-span-2 grid grid-cols-[minmax(0,1fr)_180px] gap-4">
									<div>
										<label for="modal-connection-name">Profile name</label>
										<input
											id="modal-connection-name"
											bind:value={connectionName}
											placeholder="Production database"
											disabled={action !== null}
										/>
									</div>
									<div>
										<span class="text-muted-foreground mb-2 block text-[10px] font-bold">
											Profile color
										</span>
										<div class="flex h-9 items-center gap-2">
											{#each profileColors as color}
												<button
													type="button"
													class="h-4 w-4 rounded-full transition-transform hover:scale-110 {connectionColor ===
													color
														? 'ring-foreground ring-1 ring-offset-2'
														: ''}"
													style="background-color: {color}"
													onclick={() => (connectionColor = color)}
													disabled={action !== null}
													aria-label="Set profile color to {color}"
												></button>
											{/each}
										</div>
									</div>
								</div>

								{#if provider === 'sqlite'}
									<div class="col-span-2">
										<div class="mt-1 flex items-center gap-2 border-b pb-2">
											<Database class="text-muted-foreground h-3.5 w-3.5" />
											<span class="text-[10px] font-bold">Local database file</span>
										</div>
									</div>
									<div class="col-span-2">
										<label for="modal-database">SQLite file path</label>
										<input
											id="modal-database"
											bind:value={databaseName}
											placeholder="/path/to/database.sqlite3"
											disabled={action !== null}
										/>
									</div>
									<div class="col-span-2 grid grid-cols-2 gap-3">
										<button
											type="button"
											class="rt-toolbar-button h-9 gap-2 px-3 text-[10px] font-bold"
											onclick={() => void chooseSQLiteFile(false)}
											disabled={action !== null}
										>
											<FolderOpen class="h-3.5 w-3.5" />
											Open existing file
										</button>
										<button
											type="button"
											class="rt-toolbar-button h-9 gap-2 px-3 text-[10px] font-bold"
											onclick={() => void chooseSQLiteFile(true)}
											disabled={action !== null}
										>
											<FilePlus2 class="h-3.5 w-3.5" />
											Create new file
										</button>
									</div>
									<div class="col-span-2 rounded-lg border bg-[var(--surface-sunken)] px-3.5 py-3">
										<div class="text-[9px] font-bold">Safe local defaults</div>
										<div class="text-muted-foreground mt-2 grid gap-1.5 text-[8px]">
											<span>• Foreign-key enforcement is enabled for every session.</span>
											<span>• WAL mode improves reader/writer concurrency.</span>
											<span
												>• Locked files wait up to five seconds and return an actionable error.</span
											>
										</div>
									</div>
								{:else}
									<div class="col-span-2 mt-1 flex items-center gap-2 border-b pb-2">
										<Server class="text-muted-foreground h-3.5 w-3.5" />
										<span class="text-[10px] font-bold">{selectedProvider.name} connection</span>
									</div>
									<div>
										<label for="modal-host">Host</label>
										<input
											id="modal-host"
											bind:value={host}
											placeholder="127.0.0.1"
											disabled={action !== null}
										/>
									</div>
									<div class="grid grid-cols-[112px_minmax(0,1fr)] gap-3">
										<div>
											<label for="modal-port">Port</label>
											<input
												id="modal-port"
												bind:value={port}
												placeholder={selectedProvider.defaultPort}
												disabled={action !== null}
											/>
										</div>
										<div>
											<label for="modal-database">Database name</label>
											<input
												id="modal-database"
												bind:value={databaseName}
												placeholder={provider === 'mysql' ? 'app' : 'postgres'}
												disabled={action !== null}
											/>
										</div>
									</div>

									<div class="col-span-2 mt-1 flex items-center gap-2 border-b pb-2">
										<Lock class="text-muted-foreground h-3.5 w-3.5" />
										<span class="text-[10px] font-bold">Authentication & TLS</span>
									</div>
									<div>
										<label for="modal-username">Username</label>
										<input
											id="modal-username"
											bind:value={username}
											placeholder={provider === 'mysql' ? 'root' : 'postgres'}
											disabled={action !== null}
										/>
									</div>
									<div>
										<label for="modal-password">Password</label>
										<div class="relative">
											<input
												id="modal-password"
												type={showPassword ? 'text' : 'password'}
												bind:value={password}
												placeholder="••••••••"
												class="!pr-10"
												disabled={action !== null}
											/>
											<button
												type="button"
												class="rt-toolbar-button absolute top-1/2 right-1.5 h-7 w-7 -translate-y-1/2"
												onclick={() => (showPassword = !showPassword)}
												aria-label={showPassword ? 'Hide password' : 'Show password'}
											>
												{#if showPassword}
													<EyeOff class="h-3.5 w-3.5" />
												{:else}
													<Eye class="h-3.5 w-3.5" />
												{/if}
											</button>
										</div>
									</div>
									<div class="col-span-2">
										<label for="modal-ssl">TLS mode</label>
										<FilterCombobox
											id="modal-ssl"
											options={sslOptions}
											value={sslMode}
											onChange={(value) => (sslMode = value)}
											searchable={false}
											disabled={action !== null}
											triggerClass="h-9 px-3 text-xs"
											placeholder="Select TLS mode"
										/>
									</div>

									{#if sslMode === 'verify-ca' || sslMode === 'verify-full'}
										<div class="col-span-2">
											<label for="modal-root-cert">CA certificate path</label>
											<input
												id="modal-root-cert"
												bind:value={sslRootCert}
												placeholder="/path/to/root.crt"
												disabled={action !== null}
											/>
										</div>
										<div>
											<label for="modal-client-cert">Client certificate path</label>
											<input
												id="modal-client-cert"
												bind:value={sslCert}
												placeholder="/path/to/client.crt"
												disabled={action !== null}
											/>
										</div>
										<div>
											<label for="modal-client-key">Client key path</label>
											<input
												id="modal-client-key"
												bind:value={sslKey}
												placeholder="/path/to/client.key"
												disabled={action !== null}
											/>
										</div>
									{/if}
								{/if}
							</div>
						{/if}
					</div>

					<footer class="flex h-[58px] shrink-0 items-center justify-between border-t px-5">
						<div>
							{#if editingId}
								<button
									type="button"
									class="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[10px] font-semibold {deleteConfirm
										? 'bg-red-500 text-white'
										: 'text-muted-foreground hover:bg-red-500/10 hover:text-red-500'}"
									onclick={deleteProfile}
									disabled={action !== null}
								>
									{#if action === 'delete'}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Trash2 class="h-3 w-3" />
									{/if}
									{deleteConfirm ? 'Confirm delete' : 'Delete'}
								</button>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							{#if selectedProvider}
								<button
									type="button"
									class="rt-toolbar-button border-border h-8 gap-1.5 px-3 text-[10px] font-bold"
									onclick={saveProfile}
									disabled={action !== null}
								>
									{#if action === 'save'}
										<Loader2 class="h-3 w-3 animate-spin" />
									{:else}
										<Save class="h-3 w-3" />
									{/if}
									Save profile
								</button>
								{#if action === 'connect'}
									<button
										type="button"
										class="inline-flex h-8 items-center gap-1.5 rounded-md border border-red-500/25 bg-red-500/8 px-3 text-[10px] font-bold text-red-500 transition-colors hover:bg-red-500/15 disabled:opacity-50"
										onclick={cancelConnection}
										disabled={cancellingConnection}
									>
										{#if cancellingConnection}
											<Loader2 class="h-3 w-3 animate-spin" />
											Cancelling
										{:else}
											<X class="h-3 w-3" />
											Cancel · {connectionElapsedSeconds}s
										{/if}
									</button>
								{:else}
									<button
										type="submit"
										class="rt-primary-button inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-[10px] font-bold disabled:opacity-50"
										disabled={action !== null}
									>
										<Play class="h-3 w-3" fill="currentColor" />
										Connect
									</button>
								{/if}
							{:else}
								<span class="text-muted-foreground text-[9px] font-semibold">
									Select a provider to continue
								</span>
							{/if}
						</div>
					</footer>
				</form>
			</div>
		</div>
	</div>
{/if}
