<script lang="ts">
	import { Database, Settings, Moon, Sun, Monitor, TerminalSquare } from 'lucide-svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Button } from '$lib/components/ui/button';
	import { onMount } from 'svelte';
	import { newQueryTab } from '$lib/stores/tabs.svelte';

	let theme: 'light' | 'dark' | 'system' = $state('system');

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
		<Button variant="ghost" size="sm" class="gap-1.5" disabled={false} onclick={newQueryTab}>
			<TerminalSquare class="h-4 w-4" />
			New Query
		</Button>

		<!-- Theme Toggle -->
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button variant="ghost" size="icon" class="" disabled={false} {...props}>
						{#if theme === 'light'}
							<Sun class="h-4 w-4" />
						{:else if theme === 'dark'}
							<Moon class="h-4 w-4" />
						{:else}
							<Monitor class="h-4 w-4" />
						{/if}
						<span class="sr-only">Toggle theme</span>
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content class="w-32" align="end">
				<DropdownMenu.Item class="" inset={false} onclick={() => setTheme('light')}>
					<Sun class="mr-2 h-4 w-4" />
					Light
				</DropdownMenu.Item>
				<DropdownMenu.Item class="" inset={false} onclick={() => setTheme('dark')}>
					<Moon class="mr-2 h-4 w-4" />
					Dark
				</DropdownMenu.Item>
				<DropdownMenu.Item class="" inset={false} onclick={() => setTheme('system')}>
					<Monitor class="mr-2 h-4 w-4" />
					System
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>

		<!-- Settings -->
		<Button variant="ghost" size="icon" class="" disabled={false}>
			<Settings class="h-4 w-4" />
			<span class="sr-only">Settings</span>
		</Button>
	</div>
</header>
