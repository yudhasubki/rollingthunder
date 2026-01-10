<script lang="ts">
	import {
		Database,
		Settings,
		Moon,
		Sun,
		Monitor,
		TerminalSquare,
		ChevronDown,
		X,
		Plug
	} from 'lucide-svelte';
	import { createDropdownMenu, melt } from '@melt-ui/svelte';
	import { onMount } from 'svelte';
	import { newQueryTab } from '$lib/stores/tabs.svelte';
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
		await connectionStore.switchToConnection(id);
		// Trigger UI refresh here if needed
		window.location.reload();
	}

	async function handleDisconnect(id: string, e: MouseEvent) {
		e.stopPropagation();
		await connectionStore.removeConnection(id);
	}
</script>

<header class="bg-sidebar flex h-12 items-center justify-between border-b px-4">
	<!-- Logo & Title -->
	<div class="flex items-center gap-3">
		<div class="flex items-center gap-2">
			<Database class="text-primary h-5 w-5" />
			<h1 class="text-lg font-semibold">RollingThunder</h1>
			<span class="bg-muted text-muted-foreground rounded-md px-1.5 py-0.5 text-xs">Beta</span>
		</div>

		<!-- Connection Switcher -->
		{#if connectionStore.connections.length > 0}
			<div class="ml-1 border-l pl-3">
				<button
					use:melt={$connTrigger}
					class="hover:bg-accent flex items-center gap-2 rounded-md px-2 py-1 text-sm transition-colors"
				>
					{#if connectionStore.activeConnection}
						<div
							class="h-2 w-2 rounded-full"
							style="background-color: {connectionStore.activeConnection.color || '#6366f1'}"
						></div>
						<span class="max-w-32 truncate"
							>{connectionStore.activeConnection.name ||
								connectionStore.activeConnection.database}</span
						>
					{:else}
						<Plug class="h-3.5 w-3.5" />
						<span>No Connection</span>
					{/if}
					<ChevronDown class="h-3.5 w-3.5 opacity-50" />
				</button>

				{#if $connOpen}
					<div
						use:melt={$connMenu}
						class="bg-popover text-popover-foreground z-50 min-w-48 rounded-md border p-1 shadow-md"
						transition:fly={{ duration: 150, y: -10 }}
					>
						<div class="text-muted-foreground px-2 py-1 text-xs font-medium">Connections</div>
						{#each connectionStore.connections as conn (conn.id)}
							<div
								role="button"
								tabindex="0"
								use:melt={$connItem}
								class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm outline-none {conn.isActive
									? 'bg-accent/50'
									: ''}"
								onclick={() => handleSwitchConnection(conn.id)}
								onkeydown={(e) => e.key === 'Enter' && handleSwitchConnection(conn.id)}
							>
								<div class="flex min-w-0 items-center gap-2">
									<div
										class="h-2 w-2 shrink-0 rounded-full"
										style="background-color: {conn.color || '#6366f1'}"
									></div>
									<span class="truncate">{conn.name || conn.database}</span>
									<span class="text-muted-foreground text-xs">@{conn.host}</span>
								</div>
								<button
									class="shrink-0 rounded p-0.5 hover:bg-red-100 hover:text-red-600"
									onclick={(e) => handleDisconnect(conn.id, e)}
									title="Disconnect"
								>
									<X class="h-3 w-3" />
								</button>
							</div>
						{/each}
						<div class="mt-1 border-t pt-1">
							<a
								href="/"
								class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
							>
								<span class="text-primary">+</span>
								New Connection
							</a>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Actions -->
	<div class="flex items-center gap-1">
		<!-- New Query Button -->
		<button
			class="hover:bg-accent hover:text-accent-foreground inline-flex h-9 cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 text-sm font-medium transition-colors"
			onclick={newQueryTab}
		>
			<TerminalSquare class="h-4 w-4" />
			New Query
		</button>

		<!-- Theme Toggle -->
		<button
			use:melt={$trigger}
			class="hover:bg-accent hover:text-accent-foreground inline-flex h-9 w-9 cursor-pointer items-center justify-center rounded-md transition-colors"
		>
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
				class="bg-popover text-popover-foreground z-50 min-w-32 rounded-md border p-1 shadow-md"
				transition:fly={{ duration: 150, y: -10 }}
			>
				<button
					use:melt={$item}
					class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
					onclick={() => setTheme('light')}
				>
					<Sun class="h-4 w-4" />
					Light
				</button>
				<button
					use:melt={$item}
					class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
					onclick={() => setTheme('dark')}
				>
					<Moon class="h-4 w-4" />
					Dark
				</button>
				<button
					use:melt={$item}
					class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none"
					onclick={() => setTheme('system')}
				>
					<Monitor class="h-4 w-4" />
					System
				</button>
			</div>
		{/if}

		<!-- Settings -->
		<button
			class="hover:bg-accent hover:text-accent-foreground inline-flex h-9 w-9 cursor-pointer items-center justify-center rounded-md transition-colors"
		>
			<Settings class="h-4 w-4" />
			<span class="sr-only">Settings</span>
		</button>
	</div>
</header>
