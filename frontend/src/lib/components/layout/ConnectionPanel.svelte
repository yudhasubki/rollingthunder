<script lang="ts">
	import { Database, Plus, Unplug, Cable } from 'lucide-svelte';
	import {
		connectionState,
		refreshConnections,
		switchToConnection,
		removeConnection
	} from '$lib/stores/connectionStore.svelte';
	import { goto } from '$app/navigation';
	import { getContextMenuPosition } from '$lib/utils/contextMenu';
	import { fly } from 'svelte/transition';
	import { providerOption } from '$lib/config/application';

	// Refresh on mount
	$effect(() => {
		refreshConnections();
	});

	// Context menu state
	let showContextMenu = $state(false);
	let contextMenuPos = $state({ x: 0, y: 0 });
	let contextMenuConnId = $state<string | null>(null);
	const contextMenuConnection = $derived(
		connectionState.connections.find((connection) => connection.id === contextMenuConnId) ?? null
	);

	async function handleSwitch(id: string) {
		const success = await switchToConnection(id);
		if (success) {
			window.dispatchEvent(new CustomEvent('connection-switched'));
		}
	}

	function handleNewConnection() {
		window.dispatchEvent(new CustomEvent('open-connection-manager'));
	}

	function handleContextMenu(e: MouseEvent, connId: string) {
		e.preventDefault();
		contextMenuPos = getContextMenuPosition(e, 236, 132);
		contextMenuConnId = connId;
		showContextMenu = true;
	}

	function closeContextMenu() {
		showContextMenu = false;
		contextMenuConnId = null;
	}

	async function handleDisconnect() {
		if (contextMenuConnId) {
			const success = await removeConnection(contextMenuConnId);
			closeContextMenu();

			if (success && connectionState.connections.length === 0) {
				goto('/');
			}
		}
	}

	function connectionEndpoint(connection: (typeof connectionState.connections)[number]): string {
		if (connection.driver === 'sqlite') return connection.database;
		return `${connection.database} · ${connection.host}`;
	}
</script>

<aside
	class="flex h-full w-[58px] shrink-0 flex-col items-center border-r bg-[var(--rail)] py-2.5"
	aria-label="Connections"
>
	<div
		class="text-muted-foreground mb-2 flex h-8 w-8 items-center justify-center rounded-lg border bg-[var(--surface-raised)]"
		title="Connections"
	>
		<Cable class="h-4 w-4" />
	</div>

	<div class="flex w-full flex-1 flex-col items-center gap-1.5 overflow-auto px-1.5">
		{#each connectionState.connections as conn (conn.id)}
			<button
				class="group relative flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center rounded-[10px] border transition-colors {conn.isActive
					? 'border-border bg-[var(--surface-raised)] shadow-sm'
					: 'hover:border-border border-transparent hover:bg-[var(--surface-hover)]'}"
				onclick={() => handleSwitch(conn.id)}
				oncontextmenu={(e) => handleContextMenu(e, conn.id)}
				title="{providerOption(conn.driver).name} · {connectionEndpoint(conn)}"
			>
				<div class="flex h-7 w-7 items-center justify-center rounded-md">
					<Database class="h-4 w-4 {conn.isActive ? 'text-foreground' : 'text-muted-foreground'}" />
				</div>
				{#if conn.isActive}
					<div
						class="bg-foreground absolute top-1/2 -left-[7px] h-5 w-[2px] -translate-y-1/2 rounded-r-full"
					></div>
				{/if}
				<div
					class="rt-popover text-popover-foreground pointer-events-none absolute left-full z-50 ml-2 hidden rounded-md px-2.5 py-1.5 text-[11px] font-medium whitespace-nowrap group-hover:block"
				>
					{conn.name || conn.database}
				</div>
			</button>
		{/each}
	</div>

	<div class="mt-2 flex w-10 flex-col items-center gap-1.5 border-t pt-2">
		<button
			class="rt-toolbar-button !border-border h-9 w-9 cursor-pointer border-dashed"
			onclick={handleNewConnection}
			title="New Connection"
		>
			<Plus class="h-4 w-4" />
		</button>
	</div>

	<!-- Right-click Context Menu -->
	{#if showContextMenu && contextMenuConnection}
		<button
			type="button"
			class="fixed inset-0 z-40 cursor-default"
			onclick={closeContextMenu}
			aria-label="Close menu"
		></button>
		<div
			class="rt-context-menu fixed z-50"
			style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
			transition:fly={{ duration: 100, y: -5 }}
			role="menu"
			data-context-menu="connection"
		>
			<div class="rt-context-header">
				<span class="rt-context-header-icon">
					<Database class="h-3.5 w-3.5" />
				</span>
				<span class="min-w-0">
					<span class="rt-context-title"
						>{contextMenuConnection.name || contextMenuConnection.database}</span
					>
					<span class="rt-context-meta"
						>{providerOption(contextMenuConnection.driver).name} · {connectionEndpoint(
							contextMenuConnection
						)}</span
					>
				</span>
			</div>
			<button
				type="button"
				class="rt-context-item rt-context-item--danger"
				onclick={handleDisconnect}
				role="menuitem"
			>
				<span class="rt-context-item-icon">
					<Unplug class="h-3.5 w-3.5" />
				</span>
				<span>
					<span class="rt-context-label">Disconnect</span>
					<span class="rt-context-meta">Close this database session</span>
				</span>
			</button>
		</div>
	{/if}
</aside>
