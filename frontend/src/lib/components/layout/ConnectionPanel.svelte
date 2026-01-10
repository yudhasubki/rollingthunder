<script lang="ts">
	import { Database, Plus, X } from 'lucide-svelte';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { goto } from '$app/navigation';
	import { fly } from 'svelte/transition';

	// Refresh on mount
	$effect(() => {
		connectionStore.refreshConnections();
	});

	// Context menu state
	let showContextMenu = $state(false);
	let contextMenuPos = $state({ x: 0, y: 0 });
	let contextMenuConnId = $state<string | null>(null);

	async function handleSwitch(id: string) {
		const success = await connectionStore.switchToConnection(id);
		if (success) {
			// Dispatch event for other components to react to connection change
			window.dispatchEvent(new CustomEvent('connection-switched'));
		}
	}

	function handleNewConnection() {
		goto('/');
	}

	function handleContextMenu(e: MouseEvent, connId: string) {
		e.preventDefault();
		contextMenuPos = { x: e.clientX, y: e.clientY };
		contextMenuConnId = connId;
		showContextMenu = true;
	}

	function closeContextMenu() {
		showContextMenu = false;
		contextMenuConnId = null;
	}

	async function handleDisconnect() {
		if (contextMenuConnId) {
			await connectionStore.removeConnection(contextMenuConnId);
			closeContextMenu();
			// If no connections left, redirect to login page
			if (connectionStore.connections.length === 0) {
				goto('/');
			}
		}
	}
</script>

<aside class="bg-sidebar flex h-full w-16 flex-col items-center border-r py-2">
	<!-- Connections -->
	<div class="flex flex-1 flex-col items-center gap-2 overflow-auto">
		{#each connectionStore.connections as conn (conn.id)}
			<button
				class="group relative flex h-12 w-12 cursor-pointer items-center justify-center rounded-xl transition-all {conn.isActive
					? 'bg-accent'
					: 'hover:bg-accent/50'}"
				onclick={() => handleSwitch(conn.id)}
				oncontextmenu={(e) => handleContextMenu(e, conn.id)}
				title="{conn.name || conn.database} @ {conn.host}"
			>
				<div
					class="flex h-10 w-10 items-center justify-center rounded-lg"
					style="background-color: {conn.color || '#6366f1'}20"
				>
					<Database class="h-5 w-5" style="color: {conn.color || '#6366f1'}" />
				</div>
				<!-- Active indicator -->
				{#if conn.isActive}
					<div
						class="absolute -left-0.5 top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full"
						style="background-color: {conn.color || '#6366f1'}"
					></div>
				{/if}
				<!-- Tooltip -->
				<div
					class="bg-popover text-popover-foreground pointer-events-none absolute left-full z-50 ml-2 hidden whitespace-nowrap rounded-md border px-2 py-1 text-xs shadow-md group-hover:block"
				>
					{conn.name || conn.database}
				</div>
			</button>
		{/each}
	</div>

	<!-- Add New Connection -->
	<div class="border-t pt-2">
		<button
			class="hover:bg-accent flex h-10 w-10 cursor-pointer items-center justify-center rounded-xl transition-colors"
			onclick={handleNewConnection}
			title="New Connection"
		>
			<Plus class="text-muted-foreground h-5 w-5" />
		</button>
	</div>

	<!-- Right-click Context Menu -->
	{#if showContextMenu}
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			onclick={closeContextMenu}
			aria-label="Close menu"
		></button>
		<div
			class="bg-popover text-popover-foreground fixed z-50 min-w-36 rounded-md border p-1 shadow-lg"
			style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
			transition:fly={{ duration: 100, y: -5 }}
		>
			<button
				type="button"
				class="hover:bg-accent hover:text-accent-foreground flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-red-500 hover:bg-red-50"
				onclick={handleDisconnect}
			>
				<X class="h-4 w-4" />
				Disconnect
			</button>
		</div>
	{/if}
</aside>
