<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { GetDiagnosticsSettings, RecordFrontendError } from '$lib/wailsjs/go/db/Service';
	import { diagnostics } from '$lib/wailsjs/go/models';

	let { children } = $props();

	onMount(() => {
		let diagnosticsEnabled = false;
		const refreshDiagnostics = async () => {
			try {
				const response = await GetDiagnosticsSettings();
				diagnosticsEnabled = Boolean(response.data?.enabled);
			} catch {
				diagnosticsEnabled = false;
			}
		};
		void refreshDiagnostics();

		// Check if user has a preference stored
		const stored = localStorage.getItem('theme');
		if (stored === 'dark' || stored === 'light') {
			document.documentElement.classList.add(stored);
		} else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
			document.documentElement.classList.add('dark');
		}

		// Listen for system theme changes
		const media = window.matchMedia('(prefers-color-scheme: dark)');
		const handleThemeChange = (e: MediaQueryListEvent) => {
			const stored = localStorage.getItem('theme');
			if (!stored) {
				if (e.matches) {
					document.documentElement.classList.add('dark');
				} else {
					document.documentElement.classList.remove('dark');
				}
			}
		};
		const report = (message: string, stack = '', source = 'window') => {
			if (!diagnosticsEnabled) return;
			void RecordFrontendError(
				new diagnostics.FrontendReport({
					message,
					stack,
					source
				})
			).catch(() => {});
		};
		const handleError = (event: ErrorEvent) => {
			report(event.message || 'Unknown frontend error', event.error?.stack || '', 'window.error');
		};
		const handleRejection = (event: PromiseRejectionEvent) => {
			const reason = event.reason;
			report(
				reason instanceof Error ? reason.message : String(reason || 'Unhandled promise rejection'),
				reason instanceof Error ? reason.stack || '' : '',
				'unhandledrejection'
			);
		};
		const handleDiagnosticsChanged = (event: Event) => {
			diagnosticsEnabled = Boolean((event as CustomEvent<{ enabled?: boolean }>).detail?.enabled);
		};
		media.addEventListener('change', handleThemeChange);
		window.addEventListener('error', handleError);
		window.addEventListener('unhandledrejection', handleRejection);
		window.addEventListener('diagnostics-settings-changed', handleDiagnosticsChanged);
		return () => {
			media.removeEventListener('change', handleThemeChange);
			window.removeEventListener('error', handleError);
			window.removeEventListener('unhandledrejection', handleRejection);
			window.removeEventListener('diagnostics-settings-changed', handleDiagnosticsChanged);
		};
	});
</script>

<a class="rt-skip-link" href="#main-content">Skip to main content</a>
{@render children()}
