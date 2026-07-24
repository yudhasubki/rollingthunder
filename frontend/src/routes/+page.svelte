<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { ConnectSavedConnection, GetSavedConnections } from '$lib/wailsjs/go/db/Service';
	import { db } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import ConnectionManagerModal from '$lib/components/ConnectionManagerModal.svelte';
	import DiagnosticsDialog from '$lib/components/DiagnosticsDialog.svelte';
	import {
		CONNECTION_TIMEOUT_SECONDS,
		cancelConnectionAttempt,
		createConnectionAttemptID,
		startConnectionElapsedTimer
	} from '$lib/connection/attempt';
	import { createServiceError } from '$lib/errors/service';
	import {
		ArrowLeft,
		ArrowRight,
		AlertCircle,
		Check,
		Database,
		Loader2,
		Pencil,
		Plus,
		Search,
		ShieldCheck,
		TableProperties,
		TerminalSquare,
		Workflow,
		X
	} from 'lucide-svelte';

	let profiles = $state<db.SavedConnection[]>([]);
	let searchQuery = $state('');
	let loadingProfiles = $state(false);
	let connectingId = $state<string | null>(null);
	let message = $state('');
	let messageLevel = $state<'info' | 'error' | 'success'>('info');
	let managerOpen = $state(false);
	let managerInitialId = $state<string | null>(null);
	let managerStartNew = $state(false);
	let diagnosticsOpen = $state(false);
	let connectionAttemptID = $state<string | null>(null);
	let connectionElapsedSeconds = $state(0);
	let cancellingConnection = $state(false);
	let stopConnectionElapsedTimer: (() => void) | null = null;

	const filteredProfiles = $derived(
		searchQuery.trim()
			? profiles.filter((profile) => {
					const config = profile.config;
					return `${config.name} ${config.driver || 'postgres'} ${config.host} ${config.db} ${config.user}`
						.toLowerCase()
						.includes(searchQuery.trim().toLowerCase());
				})
			: profiles
	);

	onMount(() => {
		void Promise.all([loadProfiles(), connectionStore.refreshConnections()]);
		return () => {
			stopConnectionElapsedTimer?.();
			if (connectionAttemptID) {
				void cancelConnectionAttempt(connectionAttemptID).catch(() => {});
			}
		};
	});

	async function loadProfiles() {
		loadingProfiles = true;
		try {
			const response = await GetSavedConnections();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load saved profiles');
			}
			profiles = response.data || [];
		} catch (error: any) {
			message = error?.message || 'Could not load saved profiles';
			messageLevel = 'error';
		} finally {
			loadingProfiles = false;
		}
	}

	function isConnected(profile: db.SavedConnection) {
		return connectionStore.connections.some(
			(connection) =>
				connection.profileId === profile.id ||
				(connection.name === profile.config.name &&
					connection.driver === (profile.config.driver || 'postgres') &&
					connection.host === profile.config.host &&
					connection.database === profile.config.db)
		);
	}

	function profileEndpoint(profile: db.SavedConnection): string {
		if ((profile.config.driver || 'postgres') === 'sqlite') return profile.config.db;
		return `${profile.config.host}:${profile.config.port} / ${profile.config.db}`;
	}

	function providerName(profile: db.SavedConnection): string {
		switch (profile.config.driver || 'postgres') {
			case 'mysql':
			case 'mariadb':
				return 'MySQL / MariaDB';
			case 'sqlite':
				return 'SQLite';
			default:
				return 'PostgreSQL';
		}
	}

	function profileSecurity(profile: db.SavedConnection): string {
		if ((profile.config.driver || 'postgres') === 'sqlite') {
			return 'Local file · WAL · foreign keys on';
		}
		return `${profile.config.user || 'No username'} · ${
			profile.hasPassword ? 'Password secured by OS' : 'No stored password'
		} · TLS ${profile.config.sslMode || 'disable'}`;
	}

	async function connectProfile(profile: db.SavedConnection) {
		if (connectingId !== null) return;
		const attemptID = createConnectionAttemptID();
		connectingId = profile.id;
		connectionAttemptID = attemptID;
		connectionElapsedSeconds = 0;
		cancellingConnection = false;
		stopConnectionElapsedTimer?.();
		stopConnectionElapsedTimer = startConnectionElapsedTimer((seconds) => {
			connectionElapsedSeconds = seconds;
		});
		message = `Connecting to ${profileEndpoint(profile)}. Automatic timeout after ${CONNECTION_TIMEOUT_SECONDS} seconds.`;
		messageLevel = 'info';

		try {
			const response = await ConnectSavedConnection(profile.id, attemptID);
			if (response.errors?.length || !response.data?.connected) {
				throw createServiceError(response.errors?.[0], 'Connection failed');
			}
			await connectionStore.refreshConnections();
			goto('/workspace');
		} catch (error: any) {
			const detail = error?.message || 'Could not connect to the database';
			message = detail;
			messageLevel =
				error?.code === 'CONNECTION_CANCELLED' || detail.toLowerCase().includes('cancelled')
					? 'info'
					: 'error';
		} finally {
			if (connectionAttemptID === attemptID) {
				connectionAttemptID = null;
				stopConnectionElapsedTimer?.();
				stopConnectionElapsedTimer = null;
			}
			cancellingConnection = false;
			connectingId = null;
		}
	}

	async function cancelProfileConnection() {
		if (!connectionAttemptID || cancellingConnection) return;
		cancellingConnection = true;
		message = 'Cancelling connection attempt…';
		messageLevel = 'info';

		try {
			await cancelConnectionAttempt(connectionAttemptID);
		} catch (error: any) {
			cancellingConnection = false;
			message = error?.message || 'Could not cancel connection attempt';
			messageLevel = 'error';
		}
	}

	function openNewProfile() {
		managerInitialId = null;
		managerStartNew = true;
		managerOpen = true;
	}

	function editProfile(profile: db.SavedConnection) {
		managerInitialId = profile.id;
		managerStartNew = false;
		managerOpen = true;
	}

	async function closeManager() {
		managerOpen = false;
		await loadProfiles();
	}
</script>

<svelte:head>
	<title>Rolling Thunder · Connections</title>
</svelte:head>

<div class="rt-connection-shell grid h-screen grid-cols-[minmax(400px,0.9fr)_minmax(560px,1.1fr)]">
	<section
		class="relative flex min-w-0 flex-col overflow-hidden border-r bg-[var(--surface-sunken)] p-8"
	>
		<div class="rt-empty-grid pointer-events-none absolute inset-0 opacity-40"></div>

		{#if connectionStore.connections.length > 0}
			<button
				type="button"
				class="rt-toolbar-button border-border absolute top-8 right-8 z-10 h-8 gap-1.5 px-2.5 text-[9px] font-semibold"
				onclick={() => goto('/workspace')}
			>
				<ArrowLeft class="h-3 w-3" />
				Workspace
			</button>
		{/if}

		<div class="relative my-auto max-w-[430px] py-10">
			<img src="/logo.png" alt="Rolling Thunder" class="rt-brand-logo h-24 w-24" />
			<div class="mt-5 text-[14px] font-bold tracking-[-0.02em]">Rolling Thunder</div>
			<p class="text-primary mt-7 text-[9px] font-bold tracking-[0.15em] uppercase">
				One focused workspace
			</p>
			<h1 class="mt-3 text-[28px] leading-[1.18] font-bold tracking-[-0.035em]">
				Move through your databases without losing context.
			</h1>
			<p class="text-muted-foreground mt-4 max-w-sm text-[11px] leading-relaxed">
				Explore objects, inspect data, and run dialect-aware SQL across PostgreSQL, MySQL, MariaDB,
				and SQLite from one desktop workspace.
			</p>

			<div class="mt-8 space-y-4">
				<div class="flex items-center gap-3">
					<span class="flex h-8 w-8 items-center justify-center rounded-lg border">
						<Workflow class="text-muted-foreground h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[10px] font-bold">Map the schema</div>
						<div class="text-muted-foreground mt-0.5 text-[8px]">
							See tables and foreign-key relationships together.
						</div>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<span class="flex h-8 w-8 items-center justify-center rounded-lg border">
						<TableProperties class="text-muted-foreground h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[10px] font-bold">Inspect and edit data</div>
						<div class="text-muted-foreground mt-0.5 text-[8px]">
							Structure, rows, indexes, and DDL in one table view.
						</div>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<span class="flex h-8 w-8 items-center justify-center rounded-lg border">
						<TerminalSquare class="text-muted-foreground h-3.5 w-3.5" />
					</span>
					<div>
						<div class="text-[10px] font-bold">Stay informed</div>
						<div class="text-muted-foreground mt-0.5 text-[8px]">
							Every query and background load reports clear progress.
						</div>
					</div>
				</div>
			</div>
		</div>

		<footer
			class="text-muted-foreground relative flex items-center justify-between gap-3 text-[8px]"
		>
			<span class="flex items-center gap-2">
				<ShieldCheck class="h-3.5 w-3.5" />
				Connection profiles are stored locally on this device.
			</span>
			<button
				type="button"
				class="hover:text-foreground focus-visible:ring-ring cursor-pointer rounded-sm underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:outline-none"
				onclick={() => (diagnosticsOpen = true)}
			>
				Privacy & diagnostics
			</button>
		</footer>
	</section>

	<main
		id="main-content"
		tabindex="-1"
		class="flex min-w-0 flex-col overflow-hidden bg-[var(--surface-raised)]"
	>
		<div class="mx-auto flex h-full w-full max-w-[760px] flex-col px-8 py-8">
			<header class="flex shrink-0 items-end justify-between">
				<div>
					<p class="text-muted-foreground text-[8px] font-bold tracking-[0.14em] uppercase">
						Connection hub
					</p>
					<h2 class="mt-2 text-[20px] font-bold tracking-[-0.025em]">Choose a profile</h2>
					<p class="text-muted-foreground mt-1 text-[9px]">
						Connect to a saved database or add a new one.
					</p>
				</div>
				<span class="text-muted-foreground text-[9px]">
					{profiles.length}
					{profiles.length === 1 ? 'profile' : 'profiles'}
				</span>
			</header>

			<div class="mt-6 flex shrink-0 items-center gap-2">
				<div class="relative min-w-0 flex-1">
					<Search
						class="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2"
					/>
					<input
						type="search"
						class="rt-input h-9 w-full pr-3 pl-9 text-[10px]"
						placeholder="Filter by name, provider, host, database, or user"
						bind:value={searchQuery}
					/>
				</div>
				<button
					type="button"
					class="rt-primary-button inline-flex h-9 shrink-0 items-center gap-2 rounded-md px-3.5 text-[10px] font-bold"
					onclick={openNewProfile}
				>
					<Plus class="h-3.5 w-3.5" />
					Add connection
				</button>
			</div>

			{#if message}
				<div
					class="mt-3 flex shrink-0 items-center gap-2 text-[9px] {messageLevel === 'error'
						? 'text-red-500'
						: 'text-muted-foreground'}"
				>
					{#if connectingId}
						<Loader2 class="h-3 w-3 animate-spin" />
					{:else if messageLevel === 'error'}
						<AlertCircle class="h-3 w-3" />
					{:else}
						<Check class="h-3 w-3" />
					{/if}
					{message}
					{#if connectingId}
						<span class="font-mono text-[8px] tabular-nums">
							{connectionElapsedSeconds}s / {CONNECTION_TIMEOUT_SECONDS}s
						</span>
					{/if}
				</div>
			{/if}

			<div class="mt-5 min-h-0 flex-1 overflow-auto pr-1">
				{#if loadingProfiles}
					<div
						class="text-muted-foreground flex h-full items-center justify-center gap-2 text-[10px]"
					>
						<Loader2 class="h-4 w-4 animate-spin" />
						Loading connection profiles
					</div>
				{:else if filteredProfiles.length === 0}
					<div class="flex h-full min-h-64 items-center justify-center">
						<div class="text-muted-foreground max-w-xs text-center">
							<span class="mx-auto flex h-11 w-11 items-center justify-center rounded-xl border">
								<Database class="h-5 w-5" />
							</span>
							<h3 class="text-foreground mt-3 text-[11px] font-bold">
								{searchQuery ? 'No profiles match that filter' : 'No connections yet'}
							</h3>
							<p class="mt-1 text-[9px] leading-relaxed">
								{searchQuery
									? 'Try a different name, host, database, or user.'
									: 'Add your first database profile to open the workspace.'}
							</p>
							{#if !searchQuery}
								<button
									type="button"
									class="rt-primary-button mt-4 inline-flex h-8 items-center gap-2 rounded-md px-3 text-[9px] font-bold"
									onclick={openNewProfile}
								>
									<Plus class="h-3 w-3" />
									Add connection
								</button>
							{/if}
						</div>
					</div>
				{:else}
					<div class="space-y-2">
						{#each filteredProfiles as profile (profile.id)}
							<article
								class="group flex min-h-[78px] items-center gap-3 rounded-lg border px-3.5 py-3 transition-colors hover:bg-[var(--surface-hover)]"
							>
								<span
									class="text-muted-foreground flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-raised)]"
								>
									<Database class="h-4 w-4" />
								</span>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h3 class="truncate text-[11px] font-bold">
											{profile.config.name || 'Unnamed profile'}
										</h3>
										{#if isConnected(profile)}
											<span
												class="flex items-center gap-1 text-[8px] font-semibold text-emerald-500"
											>
												<span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
												Connected
											</span>
										{/if}
										<span
											class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[7px] font-bold tracking-wide uppercase"
										>
											{providerName(profile)}
										</span>
									</div>
									<p class="text-muted-foreground mt-1 truncate font-mono text-[8px]">
										{profileEndpoint(profile)}
									</p>
									<p class="text-muted-foreground mt-1 text-[8px]">
										{profileSecurity(profile)}
									</p>
								</div>
								<div class="flex shrink-0 items-center gap-1">
									<button
										type="button"
										class="rt-toolbar-button h-8 w-8 opacity-0 group-hover:opacity-100"
										onclick={() => editProfile(profile)}
										disabled={connectingId !== null}
										title="Edit profile"
										aria-label="Edit {profile.config.name || profile.config.db}"
									>
										<Pencil class="h-3.5 w-3.5" />
									</button>
									<button
										type="button"
										class="rt-toolbar-button h-8 gap-1.5 px-3 text-[9px] font-bold {connectingId ===
										profile.id
											? 'border-red-500/25 bg-red-500/8 text-red-500 hover:bg-red-500/15'
											: 'border-border'}"
										onclick={() =>
											connectingId === profile.id
												? cancelProfileConnection()
												: connectProfile(profile)}
										disabled={(connectingId !== null && connectingId !== profile.id) ||
											cancellingConnection}
									>
										{#if connectingId === profile.id}
											{#if cancellingConnection}
												<Loader2 class="h-3 w-3 animate-spin" />
												Cancelling
											{:else}
												<X class="h-3 w-3" />
												Cancel · {connectionElapsedSeconds}s
											{/if}
										{:else}
											Connect
											<ArrowRight class="h-3 w-3" />
										{/if}
									</button>
								</div>
							</article>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</main>

	<ConnectionManagerModal
		open={managerOpen}
		onClose={closeManager}
		onConnected={() => goto('/workspace')}
		initialProfileId={managerInitialId}
		startNew={managerStartNew}
	/>
	<DiagnosticsDialog open={diagnosticsOpen} onClose={() => (diagnosticsOpen = false)} />
</div>
