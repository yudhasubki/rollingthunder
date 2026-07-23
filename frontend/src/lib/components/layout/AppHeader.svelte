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
		Unplug
	} from 'lucide-svelte';
	import { createDropdownMenu, melt } from '@melt-ui/svelte';
	import { onMount } from 'svelte';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { fly } from 'svelte/transition';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';

	let theme: 'light' | 'dark' | 'system' = $state('system');

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
		connectionStore.refreshConnections();
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
</script>

<header
	class="rt-app-header z-20 flex h-[52px] shrink-0 items-center justify-between border-b px-3"
>
	<div class="flex min-w-0 items-center">
		<div class="flex shrink-0 items-center gap-2.5 pr-3">
			<img src="/logo.png" alt="Rolling Thunder" class="rt-brand-logo h-8 w-8 rounded-[9px]" />
			<span class="hidden leading-none sm:block">
				<span class="block text-[13px] font-bold tracking-[-0.02em]">Rolling Thunder</span>
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
						<span
							class="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-[var(--surface-sunken)]"
						>
							<Database class="text-muted-foreground h-3.5 w-3.5" />
							<span
								class="ring-background absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full bg-emerald-500 ring-2"
							></span>
						</span>
						<span class="min-w-0 flex-1">
							<span class="block max-w-40 truncate text-[11px] font-bold">
								{connectionStore.activeConnection.name || connectionStore.activeConnection.database}
							</span>
							<span class="text-muted-foreground block max-w-44 truncate text-[9px]">
								{connectionStore.activeConnection.database}
								<span aria-hidden="true"> · </span>
								{connectionStore.activeConnection.host}
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
												class="absolute -right-0.5 -bottom-0.5 h-2 w-2 rounded-full ring-2 ring-[var(--surface-raised)]"
												style="background-color: {conn.color || '#ef5b50'}"
											></span>
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[11px] font-bold"
												>{conn.name || conn.database}</span
											>
											<span class="text-muted-foreground mt-0.5 block truncate text-[9px]"
												>{conn.database} · {conn.host}</span
											>
										</span>
										{#if conn.isActive}
											<span
												class="text-muted-foreground flex shrink-0 items-center gap-1 text-[9px] font-semibold"
											>
												<Check class="h-3 w-3 text-emerald-500" />
												Active
											</span>
										{/if}
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
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<div class="flex shrink-0 items-center gap-1">
		<button
			class="rt-primary-button inline-flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md px-3 text-xs font-semibold"
			onclick={openQueryTab}
			disabled={!connectionStore.activeConnection}
		>
			<TerminalSquare class="h-3.5 w-3.5" />
			<span class="hidden sm:inline">New query</span>
		</button>

		<div class="mx-1 h-5 border-l"></div>

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
