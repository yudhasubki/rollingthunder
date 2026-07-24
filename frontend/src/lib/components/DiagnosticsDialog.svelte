<script lang="ts">
	import { tick } from 'svelte';
	import {
		Archive,
		Check,
		DatabaseZap,
		FileWarning,
		Loader2,
		ShieldCheck,
		Trash2,
		X
	} from 'lucide-svelte';
	import {
		ClearDiagnostics,
		ExportDiagnostics,
		GetDiagnosticsSettings,
		UpdateDiagnosticsSettings
	} from '$lib/wailsjs/go/db/Service';
	import { diagnostics } from '$lib/wailsjs/go/models';
	import { focusTrap } from '$lib/actions/focusTrap';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';

	interface Props {
		open: boolean;
		onClose: () => void;
	}

	let { open, onClose }: Props = $props();
	let settings = $state(
		new diagnostics.Settings({
			enabled: false,
			includeSystemInfo: false
		})
	);
	let loading = $state(false);
	let saving = $state(false);
	let exporting = $state(false);
	let clearing = $state(false);
	let clearConfirm = $state(false);
	let message = $state('');
	let error = $state('');
	let initialized = false;
	let heading = $state<HTMLHeadingElement | null>(null);

	$effect(() => {
		if (open && !initialized) {
			initialized = true;
			void load();
			void tick().then(() => heading?.focus());
		} else if (!open) {
			initialized = false;
			clearConfirm = false;
			message = '';
			error = '';
		}
	});

	async function load(): Promise<void> {
		loading = true;
		error = '';
		if (!hasBackendMethod('GetDiagnosticsSettings')) {
			error = BACKEND_RESTART_MESSAGE;
			loading = false;
			return;
		}
		try {
			const response = await GetDiagnosticsSettings();
			if (response.errors?.length) throw new Error(response.errors[0].detail);
			settings =
				response.data ||
				new diagnostics.Settings({
					enabled: false,
					includeSystemInfo: false
				});
		} catch (loadError: any) {
			error = loadError?.message || 'Could not load diagnostics settings.';
		} finally {
			loading = false;
		}
	}

	async function save(): Promise<void> {
		saving = true;
		error = '';
		message = '';
		try {
			const response = await UpdateDiagnosticsSettings(settings);
			if (response.errors?.length) throw new Error(response.errors[0].detail);
			settings = response.data || settings;
			message = settings.enabled ? 'Opt-in diagnostics enabled.' : 'Optional diagnostics disabled.';
			window.dispatchEvent(
				new CustomEvent('diagnostics-settings-changed', {
					detail: { enabled: settings.enabled }
				})
			);
		} catch (saveError: any) {
			error = saveError?.message || 'Could not save diagnostics settings.';
		} finally {
			saving = false;
		}
	}

	async function exportReports(): Promise<void> {
		exporting = true;
		error = '';
		message = '';
		try {
			const response = await ExportDiagnostics();
			if (response.errors?.length) throw new Error(response.errors[0].detail);
			if (response.data?.path) {
				message = `Exported ${response.data.files} local reports. Review the ZIP before sharing.`;
			}
		} catch (exportError: any) {
			error = exportError?.message || 'Could not export diagnostics.';
		} finally {
			exporting = false;
		}
	}

	async function clearReports(): Promise<void> {
		if (!clearConfirm) {
			clearConfirm = true;
			message = 'Press “Delete local reports” again to confirm.';
			return;
		}
		clearing = true;
		error = '';
		try {
			const response = await ClearDiagnostics();
			if (response.errors?.length || !response.data) {
				throw new Error(response.errors?.[0]?.detail || 'Could not clear diagnostics.');
			}
			clearConfirm = false;
			message = 'Local diagnostic and crash reports deleted.';
		} catch (clearError: any) {
			error = clearError?.message || 'Could not clear diagnostics.';
		} finally {
			clearing = false;
		}
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (open && event.key === 'Escape' && !saving && !exporting && !clearing) {
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<div class="fixed inset-0 z-[135] flex items-center justify-center p-5">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={onClose}
			aria-label="Close privacy and diagnostics"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[86vh] w-full max-w-[620px] flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="diagnostics-title"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg"
				>
					<ShieldCheck class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2
						id="diagnostics-title"
						bind:this={heading}
						tabindex="-1"
						class="text-[12px] font-bold outline-none"
					>
						Privacy & diagnostics
					</h2>
					<p class="text-muted-foreground mt-0.5 text-[8px]">Nothing is uploaded automatically.</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					aria-label="Close privacy and diagnostics"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
				{#if loading}
					<div class="flex min-h-52 items-center justify-center">
						<Loader2 class="text-muted-foreground h-5 w-5 animate-spin" />
					</div>
				{:else}
					<section class="overflow-hidden rounded-xl border">
						<label class="flex cursor-pointer items-start gap-3 p-4">
							<input
								type="checkbox"
								class="mt-0.5"
								checked={settings.enabled}
								onchange={(event) =>
									(settings = new diagnostics.Settings({
										...settings,
										enabled: event.currentTarget.checked,
										includeSystemInfo: event.currentTarget.checked
											? settings.includeSystemInfo
											: false
									}))}
							/>
							<span class="min-w-0 flex-1">
								<span class="block text-[10px] font-bold">Optional error diagnostics</span>
								<span class="text-muted-foreground mt-1 block text-[8px] leading-relaxed">
									When enabled, frontend error messages and redacted stack traces are retained
									locally. Reports rotate automatically and stay on this device until you export
									them.
								</span>
							</span>
						</label>
						<label
							class="flex cursor-pointer items-start gap-3 border-t p-4 {settings.enabled
								? ''
								: 'opacity-45'}"
						>
							<input
								type="checkbox"
								class="mt-0.5"
								checked={settings.includeSystemInfo}
								disabled={!settings.enabled}
								onchange={(event) =>
									(settings = new diagnostics.Settings({
										...settings,
										includeSystemInfo: event.currentTarget.checked
									}))}
							/>
							<span class="min-w-0 flex-1">
								<span class="block text-[10px] font-bold">Include basic system information</span>
								<span class="text-muted-foreground mt-1 block text-[8px]">
									Operating system, architecture, Go runtime, and CPU count only.
								</span>
							</span>
						</label>
					</section>

					<section class="grid grid-cols-2 gap-3">
						<div class="rounded-xl border p-4">
							<FileWarning class="text-muted-foreground h-4 w-4" />
							<h3 class="mt-2 text-[9px] font-bold">Local crash reports</h3>
							<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
								A minimal redacted crash report may be kept locally even while optional diagnostics
								are off. It contains no query text, row data, or saved profile.
							</p>
						</div>
						<div class="rounded-xl border p-4">
							<DatabaseZap class="text-muted-foreground h-4 w-4" />
							<h3 class="mt-2 text-[9px] font-bold">Automatic redaction</h3>
							<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
								Connection URLs, passwords, home paths, and quoted values are replaced before a
								report is written.
							</p>
						</div>
					</section>

					<div class="grid grid-cols-2 gap-2">
						<button
							type="button"
							class="rt-toolbar-button h-9 cursor-pointer gap-2 px-3 text-[9px] font-semibold"
							onclick={exportReports}
							disabled={exporting || clearing}
						>
							{#if exporting}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Archive
									class="h-3.5 w-3.5"
								/>{/if}
							Export local reports
						</button>
						<button
							type="button"
							class="rt-toolbar-button hover:text-destructive h-9 cursor-pointer gap-2 px-3 text-[9px] font-semibold"
							onclick={clearReports}
							disabled={exporting || clearing}
						>
							{#if clearing}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Trash2
									class="h-3.5 w-3.5"
								/>{/if}
							{clearConfirm ? 'Delete local reports' : 'Clear local reports'}
						</button>
					</div>

					{#if message}
						<div
							class="flex items-center gap-2 rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-[8px] text-emerald-700 dark:text-emerald-300"
							role="status"
						>
							<Check class="h-3.5 w-3.5" />
							{message}
						</div>
					{/if}
					{#if error}
						<div
							class="rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2 text-[8px] text-red-600 dark:text-red-400"
							role="alert"
						>
							{error}
						</div>
					{/if}
				{/if}
			</div>

			<footer class="flex h-13 shrink-0 items-center justify-between border-t px-4">
				<span class="text-muted-foreground text-[8px]"> Export is always user-initiated. </span>
				<div class="flex gap-2">
					<button
						type="button"
						class="rt-toolbar-button h-8 cursor-pointer px-3 text-[9px] font-semibold"
						onclick={onClose}
						disabled={saving}
					>
						Close
					</button>
					<button
						type="button"
						class="rt-primary-button flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold"
						onclick={save}
						disabled={loading || saving}
					>
						{#if saving}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<ShieldCheck
								class="h-3.5 w-3.5"
							/>{/if}
						Save privacy settings
					</button>
				</div>
			</footer>
		</div>
	</div>
{/if}
