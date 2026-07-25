<script lang="ts">
	import { tick } from 'svelte';
	import { Activity, ArchiveRestore, Blocks, DatabaseBackup, ShieldCheck, X } from 'lucide-svelte';
	import { focusTrap } from '$lib/actions/focusTrap';
	import SchemaMigrationPanel from '$lib/components/database-tools/SchemaMigrationPanel.svelte';
	import BackupRestorePanel from '$lib/components/database-tools/BackupRestorePanel.svelte';
	import SecurityPanel from '$lib/components/database-tools/SecurityPanel.svelte';
	import ActivityPanel from '$lib/components/database-tools/ActivityPanel.svelte';

	interface Props {
		open: boolean;
		onClose: () => void;
	}

	let { open, onClose }: Props = $props();
	let activeTool = $state<'schema' | 'backup' | 'security' | 'activity'>('schema');
	let heading = $state<HTMLHeadingElement | null>(null);

	$effect(() => {
		if (open) void tick().then(() => heading?.focus());
	});

	function handleKeydown(event: KeyboardEvent): void {
		if (open && event.key === 'Escape') onClose();
	}

	const tools = [
		{ id: 'schema', label: 'Schema sync', icon: Blocks },
		{ id: 'backup', label: 'Backup', icon: DatabaseBackup },
		{ id: 'security', label: 'Security', icon: ShieldCheck },
		{ id: 'activity', label: 'Activity', icon: Activity }
	] as const;
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div class="fixed inset-0 z-[140] flex items-center justify-center p-5">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/50 backdrop-blur-[2px]"
			onclick={onClose}
			aria-label="Close database tools"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex h-[min(820px,90vh)] w-full max-w-[1120px] flex-col overflow-hidden rounded-2xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="database-tools-title"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg"
				>
					<ArchiveRestore class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2
						id="database-tools-title"
						bind:this={heading}
						tabindex="-1"
						class="text-[12px] font-bold outline-none"
					>
						Database tools
					</h2>
					<p class="text-muted-foreground mt-0.5 text-[8px]">
						Reviewed maintenance workflows for your active connections
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					aria-label="Close database tools"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<nav class="flex h-11 shrink-0 items-end gap-1 border-b bg-[var(--surface-sunken)] px-4">
				{#each tools as tool}
					<button
						type="button"
						class="relative flex h-10 cursor-pointer items-center gap-2 px-3 text-[9px] font-semibold {activeTool ===
						tool.id
							? 'text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
						onclick={() => (activeTool = tool.id)}
					>
						<tool.icon class="h-3.5 w-3.5" />
						{tool.label}
						{#if activeTool === tool.id}
							<span class="bg-primary absolute right-2 bottom-0 left-2 h-0.5 rounded-full"></span>
						{/if}
					</button>
				{/each}
			</nav>

			<div class="flex min-h-0 flex-1">
				{#if activeTool === 'schema'}
					<SchemaMigrationPanel />
				{:else if activeTool === 'backup'}
					<BackupRestorePanel />
				{:else if activeTool === 'security'}
					<SecurityPanel />
				{:else if activeTool === 'activity'}
					<ActivityPanel />
				{/if}
			</div>
		</div>
	</div>
{/if}
