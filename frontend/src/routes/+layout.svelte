<script>
	import '../app.css';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		// Check if user has a preference stored
		const stored = localStorage.getItem('theme');
		if (stored === 'dark' || stored === 'light') {
			document.documentElement.classList.add(stored);
		} else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
			document.documentElement.classList.add('dark');
		}

		// Listen for system theme changes
		window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
			const stored = localStorage.getItem('theme');
			if (!stored) {
				if (e.matches) {
					document.documentElement.classList.add('dark');
				} else {
					document.documentElement.classList.remove('dark');
				}
			}
		});
	});
</script>

{@render children()}
