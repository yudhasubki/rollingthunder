<script lang="ts">
	import { tick } from 'svelte';
	import {
		ArrowLeft,
		Command,
		CornerDownLeft,
		Keyboard,
		RotateCcw,
		Search,
		Settings2,
		X
	} from 'lucide-svelte';
	import {
		commandDefinitions,
		shortcutFromKeyboardEvent,
		type CommandDefinition,
		type CommandID
	} from '$lib/commands/shortcuts';
	import { shortcutStore } from '$lib/stores/shortcuts.svelte';
	import { focusTrap } from '$lib/actions/focusTrap';

	interface Props {
		open: boolean;
		hasConnection: boolean;
		activeTabKind?: string;
		onClose: () => void;
		onExecute: (command: CommandID) => void;
	}

	let { open, hasConnection, activeTabKind, onClose, onExecute }: Props = $props();
	let searchInput = $state<HTMLInputElement | null>(null);
	let query = $state('');
	let selectedIndex = $state(0);
	let shortcutsOpen = $state(false);
	let captureCommand = $state<CommandID | null>(null);
	let shortcutMessage = $state('');
	let wasOpen = false;

	const availableCommands = $derived(
		commandDefinitions.filter((command) => {
			if (!hasConnection && command.id !== 'manageConnections' && command.id !== 'commandPalette') {
				return false;
			}
			if (command.queryOnly && activeTabKind !== 'query') return false;
			const needle = query.trim().toLowerCase();
			return (
				!needle ||
				`${command.label} ${command.description} ${command.group}`.toLowerCase().includes(needle)
			);
		})
	);

	$effect(() => {
		if (open && !wasOpen) {
			wasOpen = true;
			query = '';
			selectedIndex = 0;
			shortcutsOpen = false;
			captureCommand = null;
			shortcutMessage = '';
			void tick().then(() => searchInput?.focus());
		} else if (!open) {
			wasOpen = false;
		}
	});

	function execute(command: CommandDefinition): void {
		onExecute(command.id);
		onClose();
	}

	function handleDialogKeydown(event: KeyboardEvent): void {
		if (!open) return;
		if (captureCommand) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			if (shortcutsOpen) shortcutsOpen = false;
			else onClose();
			return;
		}
		if (shortcutsOpen) return;
		if (event.key === 'ArrowDown') {
			event.preventDefault();
			selectedIndex = Math.min(selectedIndex + 1, Math.max(availableCommands.length - 1, 0));
		} else if (event.key === 'ArrowUp') {
			event.preventDefault();
			selectedIndex = Math.max(selectedIndex - 1, 0);
		} else if (event.key === 'Enter' && availableCommands[selectedIndex]) {
			event.preventDefault();
			execute(availableCommands[selectedIndex]);
		}
	}

	function captureShortcut(event: KeyboardEvent, command: CommandID): void {
		event.preventDefault();
		event.stopPropagation();
		if (event.key === 'Escape') {
			captureCommand = null;
			shortcutMessage = '';
			return;
		}
		const shortcut = shortcutFromKeyboardEvent(event);
		if (!shortcut || ['Control', 'Meta', 'Alt', 'Shift'].includes(event.key)) return;
		const collision = commandDefinitions.find(
			(candidate) => candidate.id !== command && shortcutStore.get(candidate.id) === shortcut
		);
		if (collision) {
			shortcutMessage = `${shortcut} is already assigned to ${collision.label}.`;
			return;
		}
		shortcutStore.set(command, shortcut);
		captureCommand = null;
		shortcutMessage = `${shortcut} saved.`;
	}

	function openShortcutSettings(): void {
		shortcutsOpen = true;
		captureCommand = null;
		shortcutMessage = '';
	}

	function resetShortcuts(): void {
		shortcutStore.reset();
		captureCommand = null;
		shortcutMessage = 'Default shortcuts restored.';
	}
</script>

<svelte:window onkeydown={handleDialogKeydown} />

{#if open}
	<div class="fixed inset-0 z-[140] flex items-start justify-center p-4 pt-[12vh]">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={onClose}
			aria-label="Close command palette"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[72vh] w-full max-w-[620px] flex-col overflow-hidden rounded-xl shadow-2xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="command-palette-title"
		>
			<div class="flex h-13 shrink-0 items-center gap-3 border-b px-4">
				{#if shortcutsOpen}
					<button
						type="button"
						class="rt-toolbar-button h-7 w-7 cursor-pointer"
						onclick={() => (shortcutsOpen = false)}
						aria-label="Back to commands"
					>
						<ArrowLeft class="h-3.5 w-3.5" />
					</button>
					<div class="min-w-0 flex-1">
						<h2 id="command-palette-title" class="text-[12px] font-bold">Keyboard shortcuts</h2>
						<p class="text-muted-foreground text-[8px]">
							Select a binding, then press the new key combination.
						</p>
					</div>
					<button
						type="button"
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
						onclick={resetShortcuts}
					>
						<RotateCcw class="h-3 w-3" />
						Reset
					</button>
				{:else}
					<Command class="text-primary h-4 w-4 shrink-0" />
					<label class="min-w-0 flex-1">
						<span id="command-palette-title" class="sr-only">Command palette</span>
						<input
							bind:this={searchInput}
							type="search"
							class="h-12 w-full border-0 bg-transparent text-[13px] outline-none"
							placeholder="Search commands…"
							bind:value={query}
							oninput={() => (selectedIndex = 0)}
							autocomplete="off"
						/>
					</label>
					<button
						type="button"
						class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[8px] font-semibold"
						onclick={openShortcutSettings}
					>
						<Settings2 class="h-3 w-3" />
						Shortcuts
					</button>
				{/if}
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					aria-label="Close command palette"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</div>

			{#if shortcutsOpen}
				<div class="min-h-0 flex-1 overflow-y-auto p-2">
					{#each commandDefinitions as command (command.id)}
						<div
							class="grid min-h-12 grid-cols-[minmax(0,1fr)_180px] items-center gap-3 rounded-lg px-3 hover:bg-[var(--surface-hover)]"
						>
							<div class="min-w-0">
								<div class="truncate text-[10px] font-semibold">{command.label}</div>
								<div class="text-muted-foreground truncate text-[8px]">{command.description}</div>
							</div>
							<button
								type="button"
								class="rt-input flex h-8 cursor-pointer items-center justify-center font-mono text-[9px] {captureCommand ===
								command.id
									? 'border-primary text-primary ring-primary/20 ring-2'
									: ''}"
								onclick={() => {
									captureCommand = command.id;
									shortcutMessage = 'Press a key combination, or Escape to cancel.';
								}}
								onkeydown={(event) => captureShortcut(event, command.id)}
							>
								{captureCommand === command.id ? 'Press shortcut…' : shortcutStore.get(command.id)}
							</button>
						</div>
					{/each}
				</div>
				<footer
					class="text-muted-foreground flex h-10 shrink-0 items-center border-t px-4 text-[8px]"
				>
					<Keyboard class="mr-1.5 h-3 w-3" />
					{shortcutMessage || 'Custom bindings are stored locally on this device.'}
				</footer>
			{:else}
				<div class="min-h-0 flex-1 overflow-y-auto p-2" role="listbox" aria-label="Commands">
					{#if availableCommands.length === 0}
						<div class="text-muted-foreground flex flex-col items-center py-12 text-[10px]">
							<Search class="mb-2 h-5 w-5 opacity-50" />
							No matching command
						</div>
					{:else}
						{#each availableCommands as command, index (command.id)}
							<button
								type="button"
								class="flex min-h-12 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left {selectedIndex ===
								index
									? 'bg-[var(--surface-hover)]'
									: ''}"
								role="option"
								aria-selected={selectedIndex === index}
								onmouseenter={() => (selectedIndex = index)}
								onclick={() => execute(command)}
							>
								<span
									class="bg-muted text-muted-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
								>
									<Command class="h-3.5 w-3.5" />
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-[10px] font-semibold">{command.label}</span>
									<span class="text-muted-foreground block truncate text-[8px]">
										{command.description}
									</span>
								</span>
								<span
									class="text-muted-foreground rounded-md border px-2 py-1 font-mono text-[8px]"
								>
									{shortcutStore.get(command.id)}
								</span>
							</button>
						{/each}
					{/if}
				</div>
				<footer
					class="text-muted-foreground flex h-9 shrink-0 items-center justify-between border-t px-4 text-[8px]"
				>
					<span>{availableCommands.length} available commands</span>
					<span class="flex items-center gap-1">
						<CornerDownLeft class="h-3 w-3" />
						run
					</span>
				</footer>
			{/if}
		</div>
	</div>
{/if}
