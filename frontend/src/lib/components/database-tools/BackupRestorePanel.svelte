<script lang="ts">
	import {
		Archive,
		Check,
		CircleAlert,
		DatabaseBackup,
		FileCheck2,
		FolderOpen,
		HardDrive,
		Loader2,
		RotateCcw,
		ShieldAlert,
		Square
	} from 'lucide-svelte';
	import {
		ApplyDatabaseRestore,
		BackupDatabase,
		CancelMaintenance,
		ChooseRestoreFile,
		GetBackupCapabilities,
		GetMaintenanceProgress,
		PreviewDatabaseRestore
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { createServiceError } from '$lib/errors/service';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';
	import { addConsoleLog, updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { UI_RUNTIME } from '$lib/config/application';

	let connectionId = $state('');
	let capabilities = $state<database.BackupCapabilities | null>(null);
	let scope = $state<'full' | 'schema' | 'data'>('full');
	let schema = $state('');
	let loading = $state(false);
	let backupRunning = $state(false);
	let backupJobId = $state('');
	let backupResult = $state<database.BackupResult | null>(null);
	let restoreSelection = $state<database.RestoreFileSelection | null>(null);
	let restorePreview = $state<database.RestorePreview | null>(null);
	let restoreRunning = $state(false);
	let restoreJobId = $state('');
	let restoreReviewed = $state(false);
	let restoreConfirmation = $state('');
	let maintenanceElapsedMs = $state(0);
	let maintenanceStatus = $state('');
	let error = $state('');
	let message = $state('');
	let initialized = false;

	const connections = $derived(connectionStore.connections);
	const connectionOptions = $derived(
		connections.map((item) => ({
			value: item.id,
			label: `${item.name || item.database} · ${item.driver} · ${item.database}`
		}))
	);
	const backupScopeOptions = [
		{ value: 'full', label: 'Structure + data' },
		{ value: 'schema', label: 'Structure only' },
		{ value: 'data', label: 'Data only' }
	];
	const connection = $derived(
		connections.find((candidate) => candidate.id === connectionId) ?? null
	);
	const canRestore = $derived(
		Boolean(
			restorePreview &&
				restoreReviewed &&
				restoreConfirmation === restorePreview.database &&
				!backupRunning &&
				!restoreRunning
		)
	);

	$effect(() => {
		if (initialized || connections.length === 0) return;
		initialized = true;
		connectionId = (connectionStore.activeConnection ?? connections[0]).id;
		void loadCapabilities();
	});

	$effect(() => {
		const jobId = backupJobId || restoreJobId;
		if (!jobId) {
			maintenanceElapsedMs = 0;
			maintenanceStatus = '';
			return;
		}
		async function refreshProgress(): Promise<void> {
			try {
				const response = await GetMaintenanceProgress(jobId);
				if (response.data) {
					maintenanceElapsedMs = response.data.elapsedMs;
					maintenanceStatus = response.data.status;
				}
			} catch {
				// The native file picker may still be open, or the job may
				// have completed between polls.
			}
		}
		void refreshProgress();
		const timer = globalThis.setInterval(refreshProgress, UI_RUNTIME.maintenanceProgressPollMs);
		return () => globalThis.clearInterval(timer);
	});

	function resetRestore(): void {
		restoreSelection = null;
		restorePreview = null;
		restoreReviewed = false;
		restoreConfirmation = '';
	}

	async function loadCapabilities(): Promise<void> {
		if (!connectionId) return;
		loading = true;
		error = '';
		message = '';
		backupResult = null;
		resetRestore();
		try {
			const response = await GetBackupCapabilities(connectionId);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not inspect backup tooling');
			}
			capabilities = response.data ?? null;
		} catch (loadError: any) {
			error = loadError?.message ?? 'Could not inspect backup tooling.';
			capabilities = null;
		} finally {
			loading = false;
		}
	}

	function formatBytes(bytes: number): string {
		if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
	}

	function formatElapsed(milliseconds: number): string {
		const seconds = Math.max(0, Math.floor(milliseconds / 1000));
		if (seconds < 60) return `${seconds}s`;
		return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
	}

	async function startBackup(): Promise<void> {
		if (!connectionId || backupRunning || restoreRunning || !capabilities?.available) return;
		if (!hasBackendMethod('BackupDatabase')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		backupRunning = true;
		backupJobId = crypto.randomUUID();
		backupResult = null;
		error = '';
		message = '';
		updateStatus('Preparing database backup…', 'info');
		try {
			const response = await BackupDatabase(
				new database.BackupRequest({
					connectionId,
					jobId: backupJobId,
					schema: schema.trim(),
					schemaOnly: scope === 'schema',
					dataOnly: scope === 'data'
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Database backup failed');
			}
			if (response.data?.cancelled) {
				message = 'Backup cancelled. The destination was not replaced.';
				updateStatus(message, 'info');
			} else if (response.data) {
				backupResult = response.data;
				message = `Backup saved · ${formatBytes(response.data.bytes)}`;
				updateStatus(message, 'success');
				addConsoleLog(
					`Database backup saved: ${response.data.path} (${formatBytes(response.data.bytes)})`,
					'success'
				);
			}
		} catch (backupError: any) {
			error = backupError?.message ?? 'Database backup failed.';
			updateStatus(error, 'error');
			addConsoleLog(error, 'error');
		} finally {
			backupRunning = false;
			backupJobId = '';
		}
	}

	async function cancelJob(jobId: string): Promise<void> {
		if (!jobId) return;
		try {
			await CancelMaintenance(jobId);
			updateStatus('Cancelling database maintenance safely…', 'info');
		} catch {
			// The operation may have completed between the click and cancellation.
		}
	}

	async function chooseRestore(): Promise<void> {
		if (!connectionId || restoreRunning || backupRunning) return;
		if (!hasBackendMethod('ChooseRestoreFile')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		error = '';
		message = '';
		resetRestore();
		try {
			const selected = await ChooseRestoreFile(connectionId);
			if (selected.errors?.length) {
				throw createServiceError(selected.errors[0], 'Could not choose a backup');
			}
			if (!selected.data?.token) return;
			restoreSelection = selected.data;
			const request = new database.RestorePreviewRequest({
				connectionId,
				token: selected.data.token,
				schema: schema.trim()
			});
			const response = await PreviewDatabaseRestore(request);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not prepare restore');
			}
			restorePreview = response.data ?? null;
			if (!restorePreview) throw new Error('Restore preview returned no data.');
			updateStatus('Restore preview ready. Confirm the target before applying.', 'warn');
		} catch (restoreError: any) {
			error = restoreError?.message ?? 'Could not prepare restore.';
			resetRestore();
			updateStatus(error, 'error');
		}
	}

	async function applyRestore(): Promise<void> {
		if (!restorePreview || !restoreSelection || !canRestore || backupRunning) return;
		if (!hasBackendMethod('ApplyDatabaseRestore')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		restoreRunning = true;
		restoreJobId = crypto.randomUUID();
		error = '';
		message = '';
		updateStatus(`Restoring ${restorePreview.database}…`, 'warn');
		try {
			const restore = new database.RestorePreviewRequest({
				connectionId,
				token: restoreSelection.token,
				schema: schema.trim()
			});
			const response = await ApplyDatabaseRestore(
				new database.ApplyRestoreRequest({
					restore,
					fingerprint: restorePreview.fingerprint,
					jobId: restoreJobId
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Database restore failed');
			}
			if (response.data?.cancelled) {
				message = 'Restore cancelled. Inspect the target before retrying.';
				updateStatus(message, 'warn');
			} else if (response.data?.restored) {
				message = `Restore completed for ${restorePreview.database}`;
				updateStatus(message, 'success');
				addConsoleLog(message, 'success');
				window.dispatchEvent(
					new CustomEvent('database-objects-changed', {
						detail: { connectionId, schema: schema.trim() || undefined }
					})
				);
				resetRestore();
			}
		} catch (restoreError: any) {
			error = restoreError?.message ?? 'Database restore failed.';
			updateStatus(error, 'error');
			addConsoleLog(error, 'error');
		} finally {
			restoreRunning = false;
			restoreJobId = '';
		}
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
	<div
		class="grid shrink-0 grid-cols-[minmax(220px,1fr)_minmax(180px,0.65fr)_minmax(190px,0.7fr)] gap-3 border-b bg-[var(--surface-sunken)] p-4"
	>
		<label>
			<span class="text-muted-foreground mb-1 block text-[8px]">Connection</span>
			<FilterCombobox
				id="backup-connection"
				options={connectionOptions}
				value={connectionId}
				disabled={backupRunning || restoreRunning}
				onChange={(value) => {
					connectionId = value;
					void loadCapabilities();
				}}
				searchable={connections.length > 8}
				searchPlaceholder="Find connection…"
				triggerClass="h-9 px-2 text-[9px]"
			/>
		</label>
		<label>
			<span class="text-muted-foreground mb-1 block text-[8px]">Backup scope</span>
			<FilterCombobox
				id="backup-scope"
				options={backupScopeOptions}
				value={scope}
				onChange={(value) => (scope = value as 'full' | 'schema' | 'data')}
				disabled={!capabilities?.supportsScope || backupRunning || restoreRunning}
				searchable={false}
				triggerClass="h-9 px-2 text-[9px]"
			/>
		</label>
		<label>
			<span class="text-muted-foreground mb-1 block text-[8px]">Schema filter (optional)</span>
			<input
				class="rt-input h-9 w-full px-2 text-[9px]"
				bind:value={schema}
				disabled={capabilities?.engine === 'sqlite' || backupRunning || restoreRunning}
				placeholder={capabilities?.engine === 'postgres' ? 'public' : 'All schemas'}
			/>
		</label>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto p-4">
		{#if error}
			<div
				class="border-danger-border bg-danger-soft text-danger mb-4 flex items-start gap-2 rounded-lg border px-3 py-2 text-[8px]"
				role="alert"
			>
				<CircleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				{error}
			</div>
		{/if}
		{#if message}
			<div
				class="border-success-border bg-success-soft text-success mb-4 flex items-center gap-2 rounded-lg border px-3 py-2 text-[8px]"
				role="status"
			>
				<Check class="h-3.5 w-3.5" />
				{message}
			</div>
		{/if}

		{#if loading}
			<div class="flex min-h-72 items-center justify-center">
				<Loader2 class="text-muted-foreground h-5 w-5 animate-spin" />
			</div>
		{:else}
			<div class="grid grid-cols-2 gap-4">
				<section class="flex min-h-[360px] flex-col rounded-xl border">
					<header class="flex items-center gap-3 border-b p-4">
						<span
							class="bg-primary/10 text-primary flex h-9 w-9 items-center justify-center rounded-lg"
						>
							<DatabaseBackup class="h-4 w-4" />
						</span>
						<div class="min-w-0 flex-1">
							<h3 class="text-[10px] font-bold">Create backup</h3>
							<p class="text-muted-foreground mt-0.5 truncate text-[8px]">
								{capabilities?.builtIn
									? 'Consistent online snapshot'
									: `Uses ${capabilities?.backupTool || 'database client tools'}`}
							</p>
						</div>
						<span class="bg-muted rounded-full px-2 py-1 text-[7px] font-semibold">
							{capabilities?.extension || '-'}
						</span>
					</header>

					<div class="flex flex-1 flex-col p-4">
						{#if capabilities?.available}
							<div class="space-y-2 text-[8px]">
								<div
									class="flex items-center justify-between rounded-lg bg-[var(--surface-sunken)] p-3"
								>
									<span class="text-muted-foreground">Engine</span>
									<strong class="uppercase">{capabilities.engine}</strong>
								</div>
								<div
									class="flex items-center justify-between rounded-lg bg-[var(--surface-sunken)] p-3"
								>
									<span class="text-muted-foreground">Target</span>
									<strong>{connection?.database}</strong>
								</div>
								<div
									class="flex items-center justify-between rounded-lg bg-[var(--surface-sunken)] p-3"
								>
									<span class="text-muted-foreground">Method</span>
									<strong>{capabilities.builtIn ? 'Built in' : capabilities.backupTool}</strong>
								</div>
							</div>

							{#if backupResult}
								<div class="border-success-border bg-success-soft mt-3 rounded-lg border p-3">
									<div class="text-success flex items-center gap-2 text-[8px] font-bold">
										<FileCheck2 class="h-3.5 w-3.5" />
										Backup complete
									</div>
									<p class="text-muted-foreground mt-1 text-[7px] break-all">
										{backupResult.path}
									</p>
								</div>
							{/if}

							<div class="mt-auto pt-4">
								{#if backupRunning}
									<button
										type="button"
										class="rt-toolbar-button flex h-9 w-full cursor-pointer items-center justify-center gap-2 text-[9px] font-bold"
										onclick={() => cancelJob(backupJobId)}
									>
										<Square class="h-3 w-3 fill-current" />
										Cancel backup · {formatElapsed(maintenanceElapsedMs)}
									</button>
								{:else}
									<button
										type="button"
										class="rt-primary-button flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-lg text-[9px] font-bold"
										onclick={startBackup}
										disabled={restoreRunning}
									>
										<Archive class="h-3.5 w-3.5" />
										Choose destination & back up
									</button>
								{/if}
							</div>
						{:else}
							<div class="flex flex-1 items-center justify-center text-center">
								<div class="max-w-xs">
									<HardDrive class="text-muted-foreground mx-auto h-6 w-6" />
									<p class="mt-3 text-[9px] font-bold">Backup tool unavailable</p>
									<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
										{capabilities?.message || 'No backup provider is available for this engine.'}
									</p>
								</div>
							</div>
						{/if}
					</div>
				</section>

				<section class="flex min-h-[360px] flex-col rounded-xl border">
					<header class="flex items-center gap-3 border-b p-4">
						<span
							class="bg-warning-soft text-warning flex h-9 w-9 items-center justify-center rounded-lg"
						>
							<RotateCcw class="h-4 w-4" />
						</span>
						<div class="min-w-0 flex-1">
							<h3 class="text-[10px] font-bold">Restore backup</h3>
							<p class="text-muted-foreground mt-0.5 truncate text-[8px]">
								Preview and explicit target confirmation required
							</p>
						</div>
					</header>

					<div class="flex flex-1 flex-col p-4">
						{#if !capabilities?.restoreReady}
							<div class="flex flex-1 items-center justify-center text-center">
								<div class="max-w-xs">
									<HardDrive class="text-muted-foreground mx-auto h-6 w-6" />
									<p class="mt-3 text-[9px] font-bold">Restore tool unavailable</p>
									<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
										{capabilities?.message || 'Install the matching restore client first.'}
									</p>
								</div>
							</div>
						{:else if !restorePreview}
							<div class="flex flex-1 items-center justify-center text-center">
								<div class="max-w-xs">
									<FolderOpen class="text-muted-foreground mx-auto h-6 w-6" />
									<p class="mt-3 text-[9px] font-bold">Choose a compatible backup</p>
									<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
										The file stays backend-only and is hashed before the restore can run.
									</p>
									<button
										type="button"
										class="rt-toolbar-button mt-4 h-9 cursor-pointer gap-2 px-3 text-[9px] font-bold"
										onclick={chooseRestore}
										disabled={backupRunning}
									>
										<FolderOpen class="h-3.5 w-3.5" />
										Choose backup file
									</button>
								</div>
							</div>
						{:else}
							<div class="border-warning-border bg-warning-soft rounded-lg border p-3">
								<div class="flex items-start gap-2">
									<ShieldAlert class="text-warning mt-0.5 h-4 w-4 shrink-0" />
									<div class="min-w-0">
										<p class="truncate text-[9px] font-bold">{restorePreview.file}</p>
										<p class="text-muted-foreground mt-0.5 text-[7px]">
											{formatBytes(restorePreview.size)} · {restorePreview.format} ·
											{restorePreview.transactional ? 'rollback protected' : 'external restore'}
										</p>
									</div>
								</div>
							</div>

							<ul class="mt-3 space-y-1.5">
								{#each restorePreview.warnings as warning}
									<li class="text-muted-foreground flex gap-2 text-[7px] leading-relaxed">
										<span class="bg-warning mt-1 h-1 w-1 shrink-0 rounded-full"></span>
										{warning}
									</li>
								{/each}
							</ul>

							<div class="mt-auto space-y-2 pt-4">
								<label class="flex cursor-pointer items-center gap-2 text-[8px]">
									<input type="checkbox" bind:checked={restoreReviewed} />
									I reviewed the target and backup warnings
								</label>
								<label class="block">
									<span class="text-muted-foreground mb-1 block text-[7px]">
										Type <strong>{restorePreview.database}</strong> to confirm
									</span>
									<input
										class="rt-input h-8 w-full px-2 text-[8px]"
										bind:value={restoreConfirmation}
										placeholder={restorePreview.database}
									/>
								</label>
								{#if restoreRunning}
									<button
										type="button"
										class="rt-toolbar-button flex h-9 w-full cursor-pointer items-center justify-center gap-2 text-[9px] font-bold"
										onclick={() => cancelJob(restoreJobId)}
									>
										<Square class="h-3 w-3 fill-current" />
										{maintenanceStatus === 'cancelling' ? 'Cancelling' : 'Cancel restore'} ·
										{formatElapsed(maintenanceElapsedMs)}
									</button>
								{:else}
									<button
										type="button"
										class="bg-danger text-on-solid flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-lg text-[9px] font-bold disabled:cursor-not-allowed disabled:opacity-35"
										onclick={applyRestore}
										disabled={!canRestore}
									>
										<RotateCcw class="h-3.5 w-3.5" />
										Restore reviewed backup
									</button>
								{/if}
							</div>
						{/if}
					</div>
				</section>
			</div>
		{/if}
	</div>
</div>
