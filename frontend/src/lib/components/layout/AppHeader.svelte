<script lang="ts">
	import { Database, Settings, Moon, Sun, Monitor, TerminalSquare } from 'lucide-svelte';
	import { createDropdownMenu, melt } from '@melt-ui/svelte';
	import { onMount } from 'svelte';
	import { newQueryTab } from '$lib/stores/tabs.svelte';
	import { fly } from 'svelte/transition';

	let theme: 'light' | 'dark' | 'system' = $state('system');

	// Create melt-ui dropdown menu
	const {
		elements: { trigger, menu, item },
		states: { open }
	} = createDropdownMenu({
		positioning: {
			placement: 'bottom-end'
		}
	});

	onMount(() => {
		const stored = localStorage.getItem('theme');
		if (stored === 'dark' || stored === 'light') {
			theme = stored;
		}
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
</script>

<header class="bg-sidebar flex h-12 items-center justify-between border-b px-4">
	<!-- Logo & Title -->
	<div class="flex items-center gap-2">
		<Database class="text-primary h-5 w-5" />
		<h1 class="text-lg font-semibold">RollingThunder</h1>
		<span class="bg-muted text-muted-foreground rounded-md px-1.5 py-0.5 text-xs">Beta</span>
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
