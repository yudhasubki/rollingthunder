<script lang="ts">
	import {
		ArrowRight,
		Check,
		CircleAlert,
		CircleDashed,
		Code2,
		Loader2,
		RefreshCw,
		ShieldAlert,
		Sparkles
	} from 'lucide-svelte';
	import {
		ApplySchemaMigration,
		GetSchemas,
		PreviewSchemaMigration
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { connectionStore, type ConnectionInfo } from '$lib/stores/connectionStore.svelte';
	import { createServiceError } from '$lib/errors/service';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';
	import { addConsoleLog, updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { APPLICATION } from '$lib/config/application';

	let sourceConnectionId = $state('');
	let targetConnectionId = $state('');
	let sourceSchema = $state('');
	let targetSchema = $state('');
	let sourceSchemas = $state<string[]>([]);
	let targetSchemas = $state<string[]>([]);
	let includeDestructive = $state(false);
	let loadingSchemas = $state(false);
	let comparing = $state(false);
	let applying = $state(false);
	let preview = $state<database.SchemaMigrationPreview | null>(null);
	let error = $state('');
	let reviewed = $state(false);
	let acknowledgeManual = $state(false);
	let destructiveConfirmation = $state('');
	let initialized = false;

	const connections = $derived(connectionStore.connections);
	const connectionOptions = $derived(
		connections.map((connection) => ({
			value: connection.id,
			label: `${connection.name || connection.database} · ${connection.driver}`
		}))
	);
	const sourceConnection = $derived(
		connections.find((connection) => connection.id === sourceConnectionId) ?? null
	);
	const compatibleTargets = $derived(
		connections.filter(
			(connection) => !sourceConnection || connection.driver === sourceConnection.driver
		)
	);
	const targetConnectionOptions = $derived(
		compatibleTargets.map((connection) => ({
			value: connection.id,
			label: `${connection.name || connection.database} · ${connection.driver}`
		}))
	);
	const sourceSchemaOptions = $derived(
		sourceSchemas.map((schema) => ({ value: schema, label: schema }))
	);
	const targetSchemaOptions = $derived(
		targetSchemas.map((schema) => ({ value: schema, label: schema }))
	);
	const targetConnection = $derived(
		connections.find((connection) => connection.id === targetConnectionId) ?? null
	);
	const canCompare = $derived(
		Boolean(
			sourceConnectionId &&
				targetConnectionId &&
				sourceSchema &&
				targetSchema &&
				(sourceConnectionId !== targetConnectionId || sourceSchema !== targetSchema)
		)
	);
	const canApply = $derived(
		Boolean(
			preview &&
				preview.statementCount > 0 &&
				reviewed &&
				(preview.manualChanges === 0 || acknowledgeManual) &&
				(!preview.destructive || destructiveConfirmation === targetSchema) &&
				!applying
		)
	);

	$effect(() => {
		if (initialized || connections.length === 0) return;
		initialized = true;
		const active = connectionStore.activeConnection ?? connections[0];
		sourceConnectionId = active.id;
		targetConnectionId =
			connections.find(
				(connection) => connection.id !== active.id && connection.driver === active.driver
			)?.id ?? active.id;
		void loadEndpointSchemas('source');
		void loadEndpointSchemas('target');
	});

	function fallbackSchemas(connection: ConnectionInfo | null): string[] {
		if (!connection) return [];
		if (connection.driver === 'sqlite') return ['main'];
		if (connection.database) return [connection.database];
		return ['public'];
	}

	async function fetchSchemas(connection: ConnectionInfo | null): Promise<string[]> {
		if (!connection) return [];
		const response = await GetSchemas(connection.id);
		if (response.errors?.length) {
			throw createServiceError(response.errors[0], 'Could not list schemas');
		}
		return response.data?.length ? response.data : fallbackSchemas(connection);
	}

	async function loadEndpointSchemas(side: 'source' | 'target'): Promise<void> {
		loadingSchemas = true;
		error = '';
		preview = null;
		reviewed = false;
		try {
			const connection =
				side === 'source'
					? (connections.find((item) => item.id === sourceConnectionId) ?? null)
					: (connections.find((item) => item.id === targetConnectionId) ?? null);
			const schemas = await fetchSchemas(connection);
			if (side === 'source') {
				sourceSchemas = schemas;
				sourceSchema = schemas.includes(sourceSchema) ? sourceSchema : (schemas[0] ?? '');
				if (!compatibleTargets.some((item) => item.id === targetConnectionId)) {
					targetConnectionId = compatibleTargets[0]?.id ?? sourceConnectionId;
					await loadEndpointSchemas('target');
				}
			} else {
				targetSchemas = schemas;
				targetSchema = schemas.includes(targetSchema) ? targetSchema : (schemas[0] ?? '');
				if (
					sourceConnectionId === targetConnectionId &&
					sourceSchema === targetSchema &&
					schemas.length > 1
				) {
					targetSchema = schemas.find((schema) => schema !== sourceSchema) ?? targetSchema;
				}
			}
		} catch (loadError: any) {
			error = loadError?.message ?? 'Could not load connection schemas.';
		} finally {
			loadingSchemas = false;
		}
	}

	function migrationRequest(): database.SchemaMigrationRequest {
		return new database.SchemaMigrationRequest({
			sourceConnectionId,
			sourceSchema,
			targetConnectionId,
			targetSchema,
			includeDestructive
		});
	}

	function resetReview(): void {
		preview = null;
		reviewed = false;
		acknowledgeManual = false;
		destructiveConfirmation = '';
		error = '';
	}

	async function compareSchemas(): Promise<void> {
		if (!canCompare || comparing) return;
		if (!hasBackendMethod('PreviewSchemaMigration')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		comparing = true;
		error = '';
		reviewed = false;
		acknowledgeManual = false;
		destructiveConfirmation = '';
		updateStatus('Comparing schema snapshots…', 'info');
		try {
			const response = await PreviewSchemaMigration(migrationRequest());
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not compare schemas');
			}
			preview = response.data ?? null;
			if (!preview) throw new Error('The schema comparison returned no preview.');
			const message =
				preview.changes.length === 0
					? 'Schemas are already aligned'
					: `${preview.changes.length} schema differences found · ${preview.statementCount} SQL statements`;
			updateStatus(message, preview.changes.length === 0 ? 'success' : 'info');
			addConsoleLog(message, preview.changes.length === 0 ? 'success' : 'info');
		} catch (compareError: any) {
			error = compareError?.message ?? 'Could not compare schemas.';
			updateStatus(error, 'error');
		} finally {
			comparing = false;
		}
	}

	async function applyMigration(): Promise<void> {
		if (!preview || !canApply) return;
		if (!hasBackendMethod('ApplySchemaMigration')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		applying = true;
		error = '';
		updateStatus(`Applying ${preview.statementCount} reviewed schema statements…`, 'info');
		try {
			const response = await ApplySchemaMigration(
				new database.ApplySchemaMigrationRequest({
					migration: migrationRequest(),
					fingerprint: preview.fingerprint
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Schema migration failed');
			}
			const count = response.data?.statementCount ?? 0;
			updateStatus(`Schema migration applied · ${count} statements`, 'success');
			addConsoleLog(`Schema migration applied to ${targetSchema}: ${count} statements`, 'success');
			window.dispatchEvent(
				new CustomEvent('database-objects-changed', {
					detail: { connectionId: targetConnectionId, schema: targetSchema }
				})
			);
			await compareSchemas();
		} catch (applyError: any) {
			error = applyError?.message ?? 'Schema migration failed.';
			updateStatus(error, 'error');
			addConsoleLog(error, 'error');
		} finally {
			applying = false;
		}
	}
</script>

<div class="grid min-h-0 flex-1 grid-cols-[310px_minmax(0,1fr)] overflow-hidden">
	<aside class="min-h-0 overflow-y-auto border-r bg-[var(--surface-sunken)] p-4">
		<div class="mb-4">
			<h3 class="text-[11px] font-bold">Compare endpoints</h3>
			<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
				Source is the desired structure. Only reviewed statements run against the target.
			</p>
		</div>

		<div class="space-y-3">
			<section class="rounded-xl border bg-[var(--surface-raised)] p-3">
				<div class="mb-2 flex items-center justify-between">
					<span class="text-[8px] font-bold tracking-[0.08em] uppercase">Source</span>
					<span class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[7px]"
						>Desired</span
					>
				</div>
				<label class="block">
					<span class="text-muted-foreground mb-1 block text-[8px]">Connection</span>
					<FilterCombobox
						id="schema-sync-source-connection"
						options={connectionOptions}
						value={sourceConnectionId}
						onChange={(value) => {
							sourceConnectionId = value;
							resetReview();
							void loadEndpointSchemas('source');
						}}
						searchable={connections.length > 8}
						searchPlaceholder="Find connection…"
						disabled={loadingSchemas || comparing || applying}
						triggerClass="h-9 px-2 text-[9px]"
					/>
				</label>
				<label class="mt-2 block">
					<span class="text-muted-foreground mb-1 block text-[8px]">Schema / database</span>
					<FilterCombobox
						id="schema-sync-source-schema"
						options={sourceSchemaOptions}
						value={sourceSchema}
						onChange={(value) => {
							sourceSchema = value;
							resetReview();
						}}
						searchable={sourceSchemas.length > 8}
						searchPlaceholder="Find schema…"
						disabled={loadingSchemas || comparing || applying}
						triggerClass="h-9 px-2 text-[9px]"
					/>
				</label>
			</section>

			<div class="flex justify-center">
				<span
					class="bg-muted text-muted-foreground flex h-7 w-7 items-center justify-center rounded-full"
				>
					<ArrowRight class="h-3.5 w-3.5 rotate-90" />
				</span>
			</div>

			<section class="rounded-xl border bg-[var(--surface-raised)] p-3">
				<div class="mb-2 flex items-center justify-between">
					<span class="text-[8px] font-bold tracking-[0.08em] uppercase">Target</span>
					<span class="bg-warning-soft text-warning rounded px-1.5 py-0.5 text-[7px]"
						>Will change</span
					>
				</div>
				<label class="block">
					<span class="text-muted-foreground mb-1 block text-[8px]">Connection</span>
					<FilterCombobox
						id="schema-sync-target-connection"
						options={targetConnectionOptions}
						value={targetConnectionId}
						onChange={(value) => {
							targetConnectionId = value;
							resetReview();
							void loadEndpointSchemas('target');
						}}
						searchable={compatibleTargets.length > 8}
						searchPlaceholder="Find connection…"
						disabled={loadingSchemas || comparing || applying}
						triggerClass="h-9 px-2 text-[9px]"
					/>
				</label>
				<label class="mt-2 block">
					<span class="text-muted-foreground mb-1 block text-[8px]">Schema / database</span>
					<FilterCombobox
						id="schema-sync-target-schema"
						options={targetSchemaOptions}
						value={targetSchema}
						onChange={(value) => {
							targetSchema = value;
							resetReview();
						}}
						searchable={targetSchemas.length > 8}
						searchPlaceholder="Find schema…"
						disabled={loadingSchemas || comparing || applying}
						triggerClass="h-9 px-2 text-[9px]"
					/>
				</label>
			</section>

			<label class="flex cursor-pointer items-start gap-2.5 rounded-xl border p-3">
				<input
					type="checkbox"
					class="mt-0.5"
					checked={includeDestructive}
					onchange={(event) => {
						includeDestructive = event.currentTarget.checked;
						resetReview();
					}}
				/>
				<span>
					<span class="block text-[9px] font-bold">Include removals</span>
					<span class="text-muted-foreground mt-0.5 block text-[8px] leading-relaxed">
						Generate DROP statements for extra target tables, columns, and indexes.
					</span>
				</span>
			</label>

			<button
				type="button"
				class="rt-primary-button flex h-9 w-full cursor-pointer items-center justify-center gap-2 rounded-lg text-[9px] font-bold"
				onclick={compareSchemas}
				disabled={!canCompare || comparing || loadingSchemas}
			>
				{#if comparing || loadingSchemas}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else}
					<RefreshCw class="h-3.5 w-3.5" />
				{/if}
				Compare schemas
			</button>
		</div>
	</aside>

	<section class="flex min-h-0 min-w-0 flex-col">
		{#if error}
			<div
				class="border-danger-border bg-danger-soft text-danger m-4 mb-0 flex items-start gap-2 rounded-lg border px-3 py-2 text-[8px]"
				role="alert"
			>
				<CircleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				<span>{error}</span>
			</div>
		{/if}

		{#if !preview}
			<div class="flex flex-1 items-center justify-center p-8 text-center">
				<div class="max-w-xs">
					<span
						class="bg-primary/10 text-primary mx-auto flex h-11 w-11 items-center justify-center rounded-xl"
					>
						<Sparkles class="h-5 w-5" />
					</span>
					<h3 class="mt-4 text-[12px] font-bold">Generate a reviewed migration</h3>
					<p class="text-muted-foreground mt-1.5 text-[9px] leading-relaxed">
						{APPLICATION.name} compares tables, columns, constraints, and indexes without changing either
						connection.
					</p>
				</div>
			</div>
		{:else}
			<div class="flex shrink-0 items-center gap-3 border-b px-4 py-3">
				<div class="min-w-0 flex-1">
					<h3 class="text-[10px] font-bold">
						{preview.changes.length === 0
							? 'Schemas are aligned'
							: `${preview.changes.length} differences`}
					</h3>
					<p class="text-muted-foreground mt-0.5 text-[8px]">
						{preview.statementCount} statements · {preview.manualChanges} manual ·
						{preview.transactional ? 'transactional apply' : 'engine auto-commit'}
					</p>
				</div>
				{#if preview.destructive}
					<span
						class="bg-warning-soft text-warning flex items-center gap-1 rounded-full px-2 py-1 text-[8px] font-bold"
					>
						<ShieldAlert class="h-3 w-3" />
						Destructive
					</span>
				{/if}
			</div>

			<div class="grid min-h-0 flex-1 grid-rows-[minmax(150px,0.9fr)_minmax(180px,1.1fr)]">
				<div class="min-h-0 overflow-y-auto border-b p-3">
					{#if preview.changes.length === 0}
						<div
							class="flex h-full min-h-32 items-center justify-center rounded-xl border border-dashed text-center"
						>
							<div>
								<Check class="text-success mx-auto h-5 w-5" />
								<p class="mt-2 text-[9px] font-bold">No structural drift found</p>
							</div>
						</div>
					{:else}
						<div class="space-y-1.5">
							{#each preview.changes as change (change.id)}
								<div
									class="flex items-start gap-2.5 rounded-lg border px-3 py-2.5 {change.supported &&
									change.selected
										? 'bg-[var(--surface-sunken)]'
										: 'border-dashed opacity-75'}"
								>
									<span
										class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md {change.supported &&
										change.selected
											? change.destructive
												? 'bg-danger-soft text-danger'
												: 'bg-info-soft text-info'
											: 'bg-muted text-muted-foreground'}"
									>
										{#if change.supported && change.selected}
											<Check class="h-3 w-3" />
										{:else}
											<CircleDashed class="h-3 w-3" />
										{/if}
									</span>
									<span class="min-w-0 flex-1">
										<span class="block truncate text-[9px] font-bold">{change.summary}</span>
										<span class="text-muted-foreground mt-0.5 block text-[7px]">
											{change.action.replaceAll('_', ' ')} · {change.object}
										</span>
										{#if change.reason}
											<span class="text-warning mt-1 block text-[7px]">{change.reason}</span>
										{/if}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<div class="rt-code-surface flex min-h-0 flex-col">
					<div class="border-on-solid/10 flex h-9 shrink-0 items-center gap-2 border-b px-3">
						<Code2 class="text-muted-foreground h-3.5 w-3.5" />
						<span class="text-[8px] font-bold tracking-[0.08em] uppercase">Migration SQL</span>
						<span class="text-muted-foreground ml-auto font-mono text-[7px]"
							>{preview.fingerprint.slice(0, 12)}</span
						>
					</div>
					<pre
						class="min-h-0 flex-1 overflow-auto p-4 font-mono text-[9px] leading-relaxed whitespace-pre-wrap">{preview.sql ||
							'-- No automatic statements. Manual differences are listed above.'}</pre>
				</div>
			</div>

			{#if preview.statementCount > 0}
				<footer class="shrink-0 space-y-2 border-t bg-[var(--surface-raised)] px-4 py-3">
					<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
						<label class="flex cursor-pointer items-center gap-2 text-[8px]">
							<input type="checkbox" bind:checked={reviewed} />
							I reviewed the generated SQL
						</label>
						{#if preview.manualChanges > 0}
							<label class="flex cursor-pointer items-center gap-2 text-[8px]">
								<input type="checkbox" bind:checked={acknowledgeManual} />
								I understand {preview.manualChanges} differences remain manual
							</label>
						{/if}
						{#if preview.destructive}
							<label class="ml-auto flex items-center gap-2 text-[8px]">
								Type <strong>{targetSchema}</strong>
								<input
									class="rt-input h-7 w-28 px-2 text-[8px]"
									bind:value={destructiveConfirmation}
									placeholder={targetSchema}
								/>
							</label>
						{/if}
						<button
							type="button"
							class="rt-primary-button ml-auto flex h-8 cursor-pointer items-center gap-2 rounded-md px-3 text-[8px] font-bold"
							onclick={applyMigration}
							disabled={!canApply}
						>
							{#if applying}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Check
									class="h-3.5 w-3.5"
								/>{/if}
							Apply reviewed migration
						</button>
					</div>
				</footer>
			{/if}
		{/if}
	</section>
</div>
