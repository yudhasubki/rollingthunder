<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { ArrowRight, Download, ExternalLink, Sparkles, X } from 'lucide-svelte';
	import { CheckForUpdates } from '$lib/wailsjs/go/db/Service';
	import { BrowserOpenURL } from '$lib/wailsjs/runtime/runtime';
	import { focusTrap } from '$lib/actions/focusTrap';
	import { hasBackendMethod } from '$lib/wails/backendCompatibility';
	import {
		UPDATE_DOWNLOAD_DELAY_MS,
		UPDATE_REMINDER_DELAY_MS,
		UPDATE_SNOOZE_STORAGE_KEY,
		createUpdateSnooze,
		displayVersion,
		isUpdateSnoozed
	} from '$lib/update/notification';

	interface UpdateInfo {
		currentVersion: string;
		latestVersion: string;
		name: string;
		releaseNotes: string;
		releaseUrl: string;
		publishedAt: string;
	}

	let open = $state(false);
	let update = $state<UpdateInfo | null>(null);
	let heading = $state<HTMLHeadingElement | null>(null);

	onMount(() => {
		if (hasBackendMethod('CheckForUpdates')) {
			void checkForUpdates();
		}
	});

	async function checkForUpdates(): Promise<void> {
		try {
			const response = await CheckForUpdates();
			const result = response.data;
			if (
				response.errors?.length ||
				!result?.available ||
				!result.latestVersion ||
				!result.releaseUrl
			) {
				return;
			}
			if (isUpdateSnoozed(storedSnooze(), result.latestVersion)) {
				return;
			}

			update = {
				currentVersion: result.currentVersion || '',
				latestVersion: result.latestVersion,
				name: result.name || `Rolling Thunder ${displayVersion(result.latestVersion)}`,
				releaseNotes: result.releaseNotes || '',
				releaseUrl: result.releaseUrl,
				publishedAt: result.publishedAt || ''
			};
			open = true;
			await tick();
			heading?.focus();
		} catch {
			// Update checks are best-effort and must never interrupt the database workflow.
		}
	}

	function storedSnooze(): string | null {
		try {
			return localStorage.getItem(UPDATE_SNOOZE_STORAGE_KEY);
		} catch {
			return null;
		}
	}

	function saveSnooze(version: string, delay: number): void {
		try {
			localStorage.setItem(UPDATE_SNOOZE_STORAGE_KEY, createUpdateSnooze(version, delay));
		} catch {
			// Storage availability must not prevent closing or downloading an update.
		}
	}

	function snooze(delay = UPDATE_REMINDER_DELAY_MS): void {
		if (update) {
			saveSnooze(update.latestVersion, delay);
		}
		open = false;
	}

	function openRelease(): void {
		if (!update) return;
		saveSnooze(update.latestVersion, UPDATE_DOWNLOAD_DELAY_MS);
		BrowserOpenURL(update.releaseUrl);
		open = false;
	}

	function publishedLabel(value: string): string {
		if (!value) return '';
		const publishedAt = new Date(value);
		if (Number.isNaN(publishedAt.getTime())) return '';
		return new Intl.DateTimeFormat(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		}).format(publishedAt);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (open && event.key === 'Escape') {
			snooze();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open && update}
	<div class="fixed inset-0 z-[170] flex items-center justify-center p-5">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={() => snooze()}
			aria-label="Remind me about this update tomorrow"
		></button>

		<div
			use:focusTrap
			class="rt-popover relative w-full max-w-[560px] overflow-hidden rounded-2xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="update-dialog-title"
			aria-describedby="update-dialog-description"
		>
			<header class="relative overflow-hidden border-b px-5 py-5">
				<div
					class="bg-primary/8 pointer-events-none absolute -top-24 -right-16 h-52 w-52 rounded-full blur-3xl"
				></div>
				<div class="relative flex items-start gap-4">
					<span
						class="bg-primary/10 text-primary flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-current/10"
					>
						<Sparkles class="h-4.5 w-4.5" />
					</span>
					<div class="min-w-0 flex-1">
						<div class="text-primary text-[8px] font-bold tracking-[0.14em] uppercase">
							Update available
						</div>
						<h2
							id="update-dialog-title"
							bind:this={heading}
							tabindex="-1"
							class="mt-1 text-[15px] font-bold tracking-[-0.02em] outline-none"
						>
							{update.name}
						</h2>
						<p
							id="update-dialog-description"
							class="text-muted-foreground mt-1 text-[9px] leading-relaxed"
						>
							A newer Rolling Thunder build is ready on GitHub Releases.
						</p>
					</div>
					<button
						type="button"
						class="rt-toolbar-button h-7 w-7 cursor-pointer"
						onclick={() => snooze()}
						aria-label="Remind me tomorrow"
					>
						<X class="h-3.5 w-3.5" />
					</button>
				</div>
			</header>

			<div class="space-y-4 p-5">
				<div class="flex items-center gap-3 rounded-xl border bg-[var(--surface-sunken)] px-4 py-3">
					<div class="min-w-0 flex-1">
						<div class="text-muted-foreground text-[8px] font-semibold uppercase">Installed</div>
						<div class="mt-1 font-mono text-[11px] font-bold">
							{displayVersion(update.currentVersion)}
						</div>
					</div>
					<span
						class="bg-background text-muted-foreground flex h-7 w-7 items-center justify-center rounded-full border"
					>
						<ArrowRight class="h-3.5 w-3.5" />
					</span>
					<div class="min-w-0 flex-1 text-right">
						<div class="text-primary text-[8px] font-semibold uppercase">Available</div>
						<div class="text-primary mt-1 font-mono text-[11px] font-bold">
							{displayVersion(update.latestVersion)}
						</div>
					</div>
				</div>

				{#if update.releaseNotes}
					<section class="overflow-hidden rounded-xl border">
						<div class="flex items-center justify-between border-b px-3.5 py-2.5">
							<span class="text-[9px] font-bold">What’s new</span>
							{#if publishedLabel(update.publishedAt)}
								<span class="text-muted-foreground text-[8px]">
									{publishedLabel(update.publishedAt)}
								</span>
							{/if}
						</div>
						<div
							class="text-muted-foreground max-h-36 overflow-y-auto px-3.5 py-3 text-[9px] leading-relaxed whitespace-pre-wrap"
						>
							{update.releaseNotes}
						</div>
					</section>
				{/if}

				<div class="flex items-center justify-between gap-3">
					<p class="text-muted-foreground max-w-64 text-[8px] leading-relaxed">
						The download opens in your browser. Installation stays under your control.
					</p>
					<div class="flex shrink-0 items-center gap-2">
						<button
							type="button"
							class="rt-toolbar-button h-9 cursor-pointer px-3 text-[9px] font-semibold"
							onclick={() => snooze()}
						>
							Remind me tomorrow
						</button>
						<button
							type="button"
							class="rt-primary-button inline-flex h-9 cursor-pointer items-center gap-2 rounded-lg px-3.5 text-[9px] font-bold"
							onclick={openRelease}
						>
							<Download class="h-3.5 w-3.5" />
							Download update
							<ExternalLink class="h-3 w-3 opacity-70" />
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
{/if}
