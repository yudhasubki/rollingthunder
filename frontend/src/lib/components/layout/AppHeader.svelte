<script lang="ts">
	import {
		Moon,
		Sun,
		Monitor,
		TerminalSquare,
		ChevronDown,
		Check,
		Database,
		Plug,
		Settings2,
		Unplug,
		Search,
		HeartPulse,
		RefreshCw,
		ShieldCheck,
		ArchiveRestore,
		AlertTriangle,
		Lock,
		Unlock,
		X
	} from 'lucide-svelte';
	import { createDropdownMenu, melt } from '@melt-ui/svelte';
	import { onMount } from 'svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { fly } from 'svelte/transition';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import {
		CheckConnection,
		ReconnectConnection,
		SetConnectionWriteAccess
	} from '$lib/wailsjs/go/db/Service';
	import { db } from '$lib/wailsjs/go/models';
	import { createServiceError } from '$lib/errors/service';
	import { updateStatus } from '$lib/stores/status.svelte';
	import {
		APPLICATION,
		UI_RUNTIME,
		connectionEnvironmentOption,
		providerOption
	} from '$lib/config/application';

	interface Props {
		onOpenCommandPalette: () => void;
	}

	let { onOpenCommandPalette }: Props = $props();
	let theme: 'light' | 'dark' | 'system' = $state('system');
	let healthBusy = $state(false);
	let writeGuardOpen = $state(false);
	let writeGuardBusy = $state(false);
	let writeGuardConfirmation = $state('');
	let writeGuardError = $state('');

	// Create melt-ui dropdown menu for theme
	const {
		elements: { trigger, menu, item },
		states: { open }
	} = createDropdownMenu({
		positioning: {
			placement: 'bottom-end'
		}
	});

	// Create melt-ui dropdown menu for connections
	const {
		elements: { trigger: connTrigger, menu: connMenu, item: connItem },
		states: { open: connOpen }
	} = createDropdownMenu({
		positioning: {
			placement: 'bottom-start'
		}
	});

	onMount(() => {
		const stored = localStorage.getItem('theme');
		if (stored === 'dark' || stored === 'light') {
			theme = stored;
		}
		// Refresh connections on mount
		void connectionStore.refreshConnections();
		const healthRefreshTimer = globalThis.setInterval(() => {
			void connectionStore.refreshConnections();
		}, UI_RUNTIME.connectionHealthRefreshMs);
		return () => globalThis.clearInterval(healthRefreshTimer);
	});

	function setTheme(newTheme: 'light' | 'dark' | 'system') {
		theme = newTheme;

		// Update localStorage
		if (newTheme === 'system') {
			localStorage.removeItem('theme');
		} else {
			localStorage.setItem('theme', newTheme);
		}

		// Update DOM
		document.documentElement.classList.remove('light', 'dark');
		if (newTheme === 'dark') {
			document.documentElement.classList.add('dark');
		} else if (newTheme === 'light') {
			document.documentElement.classList.add('light');
		} else {
			// System preference
			if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
				document.documentElement.classList.add('dark');
			}
		}
	}

	async function handleSwitchConnection(id: string) {
		const switched = await connectionStore.switchToConnection(id);
		if (switched) {
			connOpen.set(false);
			window.dispatchEvent(new CustomEvent('connection-switched'));
		}
	}

	async function handleDisconnect(id: string, e: MouseEvent) {
		e.stopPropagation();
		await connectionStore.removeConnection(id);
	}

	function openConnectionManager() {
		connOpen.set(false);
		window.dispatchEvent(new CustomEvent('open-connection-manager'));
	}

	function openQueryTab() {
		const connectionId = connectionStore.activeConnection?.id;
		if (connectionId) tabsStore.newQueryTab(connectionId);
	}

	function openCommandPalette() {
		onOpenCommandPalette();
	}

	function openDiagnostics() {
		window.dispatchEvent(new CustomEvent('open-diagnostics'));
	}

	function openDatabaseTools() {
		window.dispatchEvent(new CustomEvent('open-database-tools'));
	}

	function connectionEndpoint(connection: NonNullable<typeof connectionStore.activeConnection>) {
		if (connection.driver === 'sqlite') return connection.database;
		return `${connection.database} · ${connection.host}`;
	}

	function writeConfirmationName(connection: NonNullable<typeof connectionStore.activeConnection>) {
		return connection.name || connection.database;
	}

	function healthColor(state?: string): string {
		switch (state) {
			case 'healthy':
				return 'bg-success';
			case 'degraded':
				return 'bg-danger';
			case 'reconnecting':
				return 'bg-warning';
			default:
				return 'bg-muted-foreground';
		}
	}

	function healthLabel(state?: string): string {
		switch (state) {
			case 'healthy':
				return 'Healthy';
			case 'degraded':
				return 'Needs attention';
			case 'reconnecting':
				return 'Reconnecting';
			default:
				return 'Not checked';
		}
	}

	async function checkActiveConnection() {
		const connection = connectionStore.activeConnection;
		if (!connection || healthBusy) return;
		healthBusy = true;
		try {
			const response = await CheckConnection(connection.id);
			await connectionStore.refreshConnections();
			if (response.errors?.length) {
				updateStatus(response.errors[0].detail, 'error');
			} else {
				updateStatus(`Connection healthy · ${response.data?.latencyMs || 0}ms`, 'success');
			}
		} finally {
			healthBusy = false;
		}
	}

	async function reconnectActiveConnection() {
		const connection = connectionStore.activeConnection;
		if (!connection || healthBusy) return;
		healthBusy = true;
		updateStatus(`Reconnecting ${connection.name || connection.database} safely…`, 'info');
		try {
			const response = await ReconnectConnection(connection.id, crypto.randomUUID());
			await connectionStore.refreshConnections();
			if (response.errors?.length) {
				updateStatus(`${response.errors[0].detail} The previous connection was kept.`, 'error');
			} else {
				window.dispatchEvent(new CustomEvent('connection-switched'));
				updateStatus('Replacement connection is healthy and active', 'success');
			}
		} finally {
			healthBusy = false;
		}
	}

	async function setActiveWriteAccess(enable: boolean) {
		const connection = connectionStore.activeConnection;
		if (!connection || writeGuardBusy) return;
		writeGuardBusy = true;
		writeGuardError = '';
		try {
			const response = await SetConnectionWriteAccess(
				new db.SetConnectionWriteAccessRequest({
					connectionId: connection.id,
					enable,
					confirmation: enable ? writeGuardConfirmation : ''
				})
			);
			if (response.errors?.length) {
				throw createServiceError(
					response.errors[0],
					enable ? 'Could not unlock writes' : 'Could not lock writes'
				);
			}
			await connectionStore.refreshConnections();
			writeGuardOpen = false;
			writeGuardConfirmation = '';
			updateStatus(
				enable
					? `Writes temporarily unlocked for ${connection.name || connection.database}`
					: `Writes locked for ${connection.name || connection.database}`,
				enable ? 'info' : 'success'
			);
		} catch (error: any) {
			writeGuardError = error?.message || 'Could not change write access';
		} finally {
			writeGuardBusy = false;
		}
	}

	function handleWriteGuard() {
		const connection = connectionStore.activeConnection;
		if (!connection?.readOnly) return;
		connOpen.set(false);
		if (connection.writeUnlocked) {
			void setActiveWriteAccess(false);
			return;
		}
		writeGuardConfirmation = '';
		writeGuardError = '';
		writeGuardOpen = true;
	}
</script>

<header
	class="rt-app-header z-20 flex h-[52px] shrink-0 items-center justify-between border-b px-3"
>
	<div class="flex min-w-0 items-center">
		<div class="flex shrink-0 items-center gap-2.5 pr-3">
			<img src="/logo.png" alt="" class="rt-brand-logo h-8 w-8 rounded-[9px]" />
			<span class="hidden leading-none sm:block">
				<span class="block text-[13px] font-bold tracking-[-0.02em]">{APPLICATION.name}</span>
				<span
					class="text-muted-foreground mt-1 block text-[9px] font-semibold tracking-[0.14em] uppercase"
					>Database studio</span
				>
			</span>
		</div>

		{#if connectionStore.connections.length > 0}
			<div class="relative ml-1 border-l pl-3">
				<button
					use:melt={$connTrigger}
					class="group hover:border-border flex h-9 max-w-[310px] min-w-[190px] items-center gap-2.5 rounded-lg border border-transparent px-2 text-left transition-colors hover:bg-[var(--surface-hover)]"
					title="Switch connection"
					aria-label="Switch active connection"
					aria-expanded={$connOpen}
				>
					{#if connectionStore.activeConnection}
						{@const activeEnvironment = connectionEnvironmentOption(
							connectionStore.activeConnection.environment
						)}
						<span
							class="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-[var(--surface-sunken)]"
						>
							<Database class="text-muted-foreground h-3.5 w-3.5" />
							<span
								class="ring-background absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 {healthColor(
									connectionStore.activeConnection.health?.state
								)}"
							></span>
						</span>
						<span class="min-w-0 flex-1">
							<span class="flex max-w-44 items-center gap-1.5">
								<span class="min-w-0 truncate text-[11px] font-bold">
									{connectionStore.activeConnection.name ||
										connectionStore.activeConnection.database}
								</span>
								<span
									class="shrink-0 rounded border px-1 py-0.5 text-[6px] font-bold tracking-wide uppercase {activeEnvironment.toneClass}"
									title={activeEnvironment.description}
								>
									{activeEnvironment.label}
								</span>
								{#if connectionStore.activeConnection.readOnly}
									<span
										class="text-muted-foreground flex shrink-0 items-center gap-0.5 rounded border px-1 py-0.5 text-[6px] font-bold tracking-wide uppercase"
										title={connectionStore.activeConnection.writeUnlocked
											? 'Writes temporarily unlocked'
											: 'Database writes are blocked'}
									>
										{#if connectionStore.activeConnection.writeUnlocked}
											<Unlock class="h-2.5 w-2.5" />
											Unlocked
										{:else}
											<Lock class="h-2.5 w-2.5" />
											Read only
										{/if}
									</span>
								{/if}
							</span>
							<span class="text-muted-foreground block max-w-44 truncate text-[9px]">
								{providerOption(connectionStore.activeConnection.driver).name} ·
								{connectionEndpoint(connectionStore.activeConnection)}
							</span>
						</span>
					{:else}
						<span
							class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-[var(--surface-sunken)]"
						>
							<Plug class="text-muted-foreground h-3.5 w-3.5" />
						</span>
						<span class="text-xs font-semibold">No active connection</span>
					{/if}
					<ChevronDown
						class="text-muted-foreground ml-auto h-3.5 w-3.5 shrink-0 transition-transform {$connOpen
							? 'rotate-180'
							: ''}"
					/>
				</button>

				{#if $connOpen}
					<div
						use:melt={$connMenu}
						class="rt-popover text-popover-foreground z-50 w-[320px] rounded-xl p-2"
						transition:fly={{ duration: 130, y: -6 }}
					>
						<div class="flex items-center justify-between px-2 pt-0.5 pb-2">
							<div>
								<p class="text-[11px] font-bold">Connections</p>
								<p class="text-muted-foreground mt-0.5 text-[9px]">Switch active workspace</p>
							</div>
							<span
								class="bg-muted text-muted-foreground rounded-full px-2 py-1 text-[9px] font-semibold"
							>
								{connectionStore.connections.length} open
							</span>
						</div>

						<div class="space-y-1">
							{#each connectionStore.connections as conn (conn.id)}
								<div
									class="group flex items-center rounded-lg border transition-colors {conn.isActive
										? 'border-border bg-[var(--surface-sunken)]'
										: 'border-transparent hover:bg-[var(--surface-hover)]'}"
								>
									<button
										type="button"
										use:melt={$connItem}
										class="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 px-2 py-2 text-left outline-none"
										onclick={() => handleSwitchConnection(conn.id)}
										aria-label="Switch to {conn.name || conn.database}"
									>
										<span
											class="relative flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-raised)]"
										>
											<Database class="text-muted-foreground h-3.5 w-3.5" />
											<span
												class="absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 ring-[var(--surface-raised)] {healthColor(
													conn.health?.state
												)}"
											></span>
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[11px] font-bold"
												>{conn.name || conn.database}</span
											>
											<span class="text-muted-foreground mt-0.5 block truncate text-[9px]"
												>{providerOption(conn.driver).name} · {connectionEndpoint(conn)}</span
											>
										</span>
										{#if conn.isActive}
											<span
												class="text-muted-foreground flex shrink-0 items-center gap-1 text-[9px] font-semibold"
											>
												<Check class="text-success h-3 w-3" />
												Active
											</span>
										{/if}
										<span class="sr-only">{healthLabel(conn.health?.state)}</span>
									</button>
									<button
										type="button"
										class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive mr-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-md opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
										onclick={(e) => handleDisconnect(conn.id, e)}
										title="Disconnect {conn.name || conn.database}"
										aria-label="Disconnect {conn.name || conn.database}"
									>
										<Unplug class="h-3.5 w-3.5" />
									</button>
								</div>
							{/each}
						</div>

						<div class="mt-2 border-t pt-2">
							<div class="mb-1 flex items-center gap-1 rounded-lg bg-[var(--surface-sunken)] p-1">
								<button
									type="button"
									class="hover:bg-accent flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-[9px] font-semibold"
									onclick={checkActiveConnection}
									disabled={healthBusy || !connectionStore.activeConnection}
									title={connectionStore.activeConnection?.health?.message || 'Run health check'}
								>
									{#if healthBusy}
										<RefreshCw class="h-3.5 w-3.5 animate-spin" />
									{:else}
										<HeartPulse class="h-3.5 w-3.5" />
									{/if}
									<span class="min-w-0 flex-1 truncate">
										{healthLabel(connectionStore.activeConnection?.health?.state)}
									</span>
									{#if connectionStore.activeConnection?.health?.state === 'healthy'}
										<span class="text-muted-foreground font-mono text-[8px]">
											{connectionStore.activeConnection.health.latencyMs}ms
										</span>
									{/if}
								</button>
								{#if connectionStore.activeConnection?.health?.state === 'degraded'}
									<button
										type="button"
										class="rt-primary-button h-8 cursor-pointer rounded-md px-2 text-[8px] font-bold"
										onclick={reconnectActiveConnection}
										disabled={healthBusy}
									>
										Reconnect
									</button>
								{/if}
							</div>
							{#if connectionStore.activeConnection?.readOnly}
								<button
									type="button"
									class="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-[var(--surface-hover)]"
									onclick={handleWriteGuard}
									disabled={writeGuardBusy}
								>
									<span
										class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)]"
									>
										{#if connectionStore.activeConnection.writeUnlocked}
											<Unlock class="text-muted-foreground h-3.5 w-3.5" />
										{:else}
											<Lock class="text-muted-foreground h-3.5 w-3.5" />
										{/if}
									</span>
									<span class="min-w-0 flex-1">
										<span class="block text-[11px] font-bold">
											{connectionStore.activeConnection.writeUnlocked
												? 'Lock writes now'
												: 'Temporarily unlock writes'}
										</span>
										<span class="text-muted-foreground mt-0.5 block text-[9px]">
											{connectionStore.activeConnection.writeUnlocked
												? 'End this session override'
												: 'Requires the exact connection name'}
										</span>
									</span>
								</button>
							{/if}
							<button
								type="button"
								class="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-[var(--surface-hover)]"
								onclick={openConnectionManager}
							>
								<span
									class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)]"
								>
									<Settings2 class="text-muted-foreground h-3.5 w-3.5" />
								</span>
								<span class="min-w-0">
									<span class="block text-[11px] font-bold">Manage connections</span>
									<span class="text-muted-foreground mt-0.5 block text-[9px]"
										>Add, edit, or reconnect profiles</span
									>
								</span>
								<ChevronDown
									class="text-muted-foreground ml-auto h-3 w-3 -rotate-90"
									aria-hidden="true"
								/>
							</button>
							<button
								type="button"
								class="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-[var(--surface-hover)]"
								onclick={() => {
									connOpen.set(false);
									openDatabaseTools();
								}}
							>
								<span
									class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)]"
								>
									<ArchiveRestore class="text-muted-foreground h-3.5 w-3.5" />
								</span>
								<span class="min-w-0">
									<span class="block text-[11px] font-bold">Database tools</span>
									<span class="text-muted-foreground mt-0.5 block text-[9px]"
										>Schema sync, backup, security, and activity</span
									>
								</span>
								<ChevronDown
									class="text-muted-foreground ml-auto h-3 w-3 -rotate-90"
									aria-hidden="true"
								/>
							</button>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<div class="flex shrink-0 items-center gap-1">
		<button
			type="button"
			class="rt-toolbar-button hidden h-8 cursor-pointer gap-2 px-2.5 text-[10px] font-semibold lg:inline-flex"
			onclick={openDatabaseTools}
			disabled={!connectionStore.activeConnection}
			title="Schema migration, backup, security, and activity"
		>
			<ArchiveRestore class="h-3.5 w-3.5" />
			<span>Database tools</span>
		</button>
		<button
			type="button"
			class="rt-toolbar-button hidden h-8 cursor-pointer gap-2 px-2.5 text-[10px] font-semibold md:inline-flex"
			onclick={openCommandPalette}
			title="Command palette"
			aria-label="Open command palette"
		>
			<Search class="h-3.5 w-3.5" />
			<span>Commands</span>
			<span class="text-muted-foreground rounded border px-1 py-0.5 font-mono text-[8px]">⌘K</span>
		</button>
		<button
			class="rt-primary-button inline-flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md px-3 text-xs font-semibold"
			onclick={openQueryTab}
			disabled={!connectionStore.activeConnection}
		>
			<TerminalSquare class="h-3.5 w-3.5" />
			<span class="hidden sm:inline">New query</span>
		</button>

		<div class="mx-1 h-5 border-l"></div>

		<button
			type="button"
			class="rt-toolbar-button h-8 w-8 cursor-pointer"
			onclick={openDiagnostics}
			title="Privacy and diagnostics"
			aria-label="Open privacy and diagnostics"
		>
			<ShieldCheck class="h-4 w-4" />
		</button>

		<button use:melt={$trigger} class="rt-toolbar-button h-8 w-8 cursor-pointer" title="Appearance">
			{#if theme === 'light'}
				<Sun class="h-4 w-4" />
			{:else if theme === 'dark'}
				<Moon class="h-4 w-4" />
			{:else}
				<Monitor class="h-4 w-4" />
			{/if}
			<span class="sr-only">Toggle theme</span>
		</button>

		{#if $open}
			<div
				use:melt={$menu}
				class="rt-popover text-popover-foreground z-50 min-w-36 rounded-lg p-1.5"
				transition:fly={{ duration: 130, y: -6 }}
			>
				<div
					class="text-muted-foreground px-2 py-1 text-[10px] font-bold tracking-[0.1em] uppercase"
				>
					Appearance
				</div>
				<button
					use:melt={$item}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => setTheme('light')}
				>
					<Sun class="h-3.5 w-3.5" />
					Light
				</button>
				<button
					use:melt={$item}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => setTheme('dark')}
				>
					<Moon class="h-3.5 w-3.5" />
					Dark
				</button>
				<button
					use:melt={$item}
					class="hover:bg-accent flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs outline-none"
					onclick={() => setTheme('system')}
				>
					<Monitor class="h-3.5 w-3.5" />
					System
				</button>
			</div>
		{/if}
	</div>
</header>

{#if writeGuardOpen && connectionStore.activeConnection}
	<div class="fixed inset-0 z-[120] flex items-center justify-center p-5">
		<button
			type="button"
			class="bg-overlay/45 absolute inset-0 cursor-default backdrop-blur-[2px]"
			aria-label="Cancel write unlock"
			onclick={() => {
				if (!writeGuardBusy) writeGuardOpen = false;
			}}
		></button>
		<div
			class="rt-popover relative w-full max-w-[430px] overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="write-guard-title"
		>
			<div class="flex items-start gap-3 border-b p-4">
				<span
					class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-[var(--surface-sunken)]"
				>
					<AlertTriangle class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="write-guard-title" class="text-[13px] font-bold">Temporarily unlock writes</h2>
					<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">
						This enables data, schema, import, restore, and administrative changes until the
						connection closes or you lock it again.
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-8 w-8"
					aria-label="Close"
					disabled={writeGuardBusy}
					onclick={() => (writeGuardOpen = false)}
				>
					<X class="h-4 w-4" />
				</button>
			</div>
			<form
				class="p-4"
				onsubmit={(event) => {
					event.preventDefault();
					void setActiveWriteAccess(true);
				}}
			>
				<label for="write-guard-confirmation" class="text-[9px] font-bold">
					Type <span class="font-mono"
						>{writeConfirmationName(connectionStore.activeConnection)}</span
					> to confirm
				</label>
				<input
					id="write-guard-confirmation"
					class="rt-input mt-2 h-9 w-full px-3 font-mono text-[10px]"
					bind:value={writeGuardConfirmation}
					autocomplete="off"
					disabled={writeGuardBusy}
				/>
				{#if writeGuardError}
					<p class="text-danger mt-2 text-[9px]">{writeGuardError}</p>
				{/if}
				<div class="mt-4 flex justify-end gap-2 border-t pt-3">
					<button
						type="button"
						class="rt-toolbar-button h-8 px-3 text-[9px] font-bold"
						disabled={writeGuardBusy}
						onclick={() => (writeGuardOpen = false)}
					>
						Cancel
					</button>
					<button
						type="submit"
						class="rt-primary-button h-8 rounded-md px-3 text-[9px] font-bold"
						disabled={writeGuardBusy ||
							writeGuardConfirmation !== writeConfirmationName(connectionStore.activeConnection)}
					>
						{writeGuardBusy ? 'Unlocking…' : 'Unlock for this session'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}
