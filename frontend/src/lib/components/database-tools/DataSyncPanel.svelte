<script lang="ts">
	import {
		ArrowRight,
		Check,
		CircleAlert,
		DatabaseZap,
		Loader2,
		RefreshCw,
		Rows3,
		ShieldAlert
	} from 'lucide-svelte';
	import {
		ApplyDataSync,
		GetCollections,
		GetSchemas,
		PreviewDataSync
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { connectionStore, type ConnectionInfo } from '$lib/stores/connectionStore.svelte';
	import { createServiceError } from '$lib/errors/service';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';
	import { addConsoleLog, updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { providerOption } from '$lib/config/application';

	type Side = 'source' | 'target';

	let sourceConnectionId = $state('');
	let targetConnectionId = $state('');
	let sourceSchema = $state('');
	let targetSchema = $state('');
	let sourceTable = $state('');
	let targetTable = $state('');
	let sourceSchemas = $state<string[]>([]);
	let targetSchemas = $state<string[]>([]);
	let sourceTables = $state<string[]>([]);
	let targetTables = $state<string[]>([]);
	let maxRows = $state(5000);
	let loadingEndpoints = $state(false);
	let comparing = $state(false);
	let applying = $state(false);
	let preview = $state<database.DataSyncPreview | null>(null);
	let selectedChangeIDs = $state<string[]>([]);
	let reviewed = $state(false);
	let deleteConfirmation = $state('');
	let error = $state('');
	let initialized = false;

	const connections = $derived(connectionStore.connections);
	const connectionOptions = $derived(
		connections.map((connection) => ({
			value: connection.id,
			label: `${connection.name || connection.database} · ${providerOption(connection.driver).name}`
		}))
	);
	const sourceConnection = $derived(
		connections.find((connection) => connection.id === sourceConnectionId) ?? null
	);
	const targetConnection = $derived(
		connections.find((connection) => connection.id === targetConnectionId) ?? null
	);
	const sourceSchemaOptions = $derived(
		sourceSchemas.map((schema) => ({ value: schema, label: schema }))
	);
	const targetSchemaOptions = $derived(
		targetSchemas.map((schema) => ({ value: schema, label: schema }))
	);
	const sourceTableOptions = $derived(
		sourceTables.map((table) => ({ value: table, label: table }))
	);
	const targetTableOptions = $derived(
		targetTables.map((table) => ({ value: table, label: table }))
	);
	const selectedChanges = $derived(
		preview?.changes.filter((change) => selectedChangeIDs.includes(change.id)) ?? []
	);
	const selectedDeletes = $derived(
		selectedChanges.filter((change) => change.kind === 'delete').length
	);
	const allSelected = $derived(
		Boolean(preview?.changes.length && selectedChangeIDs.length === preview.changes.length)
	);
	const canCompare = $derived(
		Boolean(
			sourceConnectionId &&
				targetConnectionId &&
				sourceTable &&
				targetTable &&
				maxRows > 0 &&
				maxRows <= 10000 &&
				(sourceConnectionId !== targetConnectionId ||
					sourceSchema !== targetSchema ||
					sourceTable !== targetTable)
		)
	);
	const canApply = $derived(
		Boolean(
			preview?.safeToApply &&
				selectedChangeIDs.length > 0 &&
				reviewed &&
				(selectedDeletes === 0 || deleteConfirmation === targetTable) &&
				!(targetConnection?.readOnly && !targetConnection.writeUnlocked) &&
				!applying
		)
	);

	$effect(() => {
		if (initialized || connections.length === 0) return;
		initialized = true;
		const active = connectionStore.activeConnection ?? connections[0];
		sourceConnectionId = active.id;
		targetConnectionId =
			connections.find((connection) => connection.id !== active.id)?.id ?? active.id;
		void loadEndpoint('source');
		void loadEndpoint('target');
	});

	function fallbackSchemas(connection: ConnectionInfo | null): string[] {
		if (!connection) return [];
		if (connection.driver === 'sqlite') return ['main'];
		if (connection.driver === 'sqlserver') return ['dbo'];
		if (connection.driver === 'oracle') return [];
		return connection.database ? [connection.database] : ['public'];
	}

	async function fetchSchemas(connection: ConnectionInfo | null): Promise<string[]> {
		if (!connection) return [];
		const response = await GetSchemas(connection.id);
		if (response.errors?.length) {
			throw createServiceError(response.errors[0], 'Could not list schemas');
		}
		return response.data?.length ? response.data : fallbackSchemas(connection);
	}

	async function fetchTables(connectionID: string, schema: string): Promise<string[]> {
		if (!connectionID || !schema) return [];
		const response = await GetCollections(connectionID, [schema]);
		if (response.errors?.length) {
			throw createServiceError(response.errors[0], 'Could not list tables');
		}
		return response.data ?? [];
	}

	function resetPreview(): void {
		preview = null;
		selectedChangeIDs = [];
		reviewed = false;
		deleteConfirmation = '';
		error = '';
	}

	async function loadEndpoint(side: Side, schemasOnly = false): Promise<void> {
		loadingEndpoints = true;
		resetPreview();
		try {
			const connectionID = side === 'source' ? sourceConnectionId : targetConnectionId;
			const connection = connections.find((candidate) => candidate.id === connectionID) ?? null;
			let schema = side === 'source' ? sourceSchema : targetSchema;
			if (!schemasOnly || !schema) {
				const schemas = await fetchSchemas(connection);
				schema = schemas.includes(schema) ? schema : (schemas[0] ?? '');
				if (side === 'source') {
					sourceSchemas = schemas;
					sourceSchema = schema;
				} else {
					targetSchemas = schemas;
					targetSchema = schema;
				}
			}
			const tables = await fetchTables(connectionID, schema);
			if (side === 'source') {
				sourceTables = tables;
				sourceTable = tables.includes(sourceTable) ? sourceTable : (tables[0] ?? '');
			} else {
				targetTables = tables;
				targetTable = tables.includes(targetTable)
					? targetTable
					: tables.includes(sourceTable)
						? sourceTable
						: (tables[0] ?? '');
			}
		} catch (loadError: any) {
			error = loadError?.message || 'Could not load tables for this endpoint.';
		} finally {
			loadingEndpoints = false;
		}
	}

	function syncRequest(): database.DataSyncRequest {
		return new database.DataSyncRequest({
			sourceConnectionId,
			sourceSchema,
			sourceTable,
			targetConnectionId,
			targetSchema,
			targetTable,
			maxRows
		});
	}

	async function compareData(): Promise<void> {
		if (!canCompare || comparing) return;
		if (!hasBackendMethod('PreviewDataSync')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		comparing = true;
		resetPreview();
		updateStatus('Comparing table rows by stable keys…', 'info');
		try {
			const response = await PreviewDataSync(syncRequest());
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not compare table data');
			}
			preview = response.data ?? null;
			if (!preview) throw new Error('The data comparison returned no preview.');
			selectedChangeIDs = preview.changes.map((change) => change.id);
			const total = preview.changes.length;
			const message =
				total === 0
					? 'Table data is already aligned'
					: `${total} row changes found · ${preview.added} insert · ${preview.updated} update · ${preview.deleted} delete`;
			updateStatus(message, total === 0 ? 'success' : 'info');
			addConsoleLog(message, total === 0 ? 'success' : 'info');
		} catch (compareError: any) {
			error = compareError?.message || 'Could not compare table data.';
			updateStatus(error, 'error');
		} finally {
			comparing = false;
		}
	}

	async function applyData(): Promise<void> {
		if (!preview || !canApply) return;
		if (!hasBackendMethod('ApplyDataSync')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		applying = true;
		error = '';
		updateStatus(`Applying ${selectedChangeIDs.length} reviewed row changes…`, 'info');
		try {
			const response = await ApplyDataSync(
				new database.ApplyDataSyncRequest({
					sync: syncRequest(),
					fingerprint: preview.fingerprint,
					selectedChangeIds: selectedChangeIDs
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Data synchronization failed');
			}
			const result = response.data;
			const message = `Data synchronized · ${result?.inserted || 0} inserted · ${result?.updated || 0} updated · ${result?.deleted || 0} deleted`;
			updateStatus(message, 'success');
			addConsoleLog(message, 'success');
			window.dispatchEvent(
				new CustomEvent('database-objects-changed', {
					detail: { connectionId: targetConnectionId, schema: targetSchema }
				})
			);
			await compareData();
		} catch (applyError: any) {
			error = applyError?.message || 'Data synchronization failed.';
			updateStatus(error, 'error');
			addConsoleLog(error, 'error');
		} finally {
			applying = false;
		}
	}

	function toggleChange(id: string): void {
		selectedChangeIDs = selectedChangeIDs.includes(id)
			? selectedChangeIDs.filter((candidate) => candidate !== id)
			: [...selectedChangeIDs, id];
		reviewed = false;
		deleteConfirmation = '';
	}

	function toggleAll(): void {
		selectedChangeIDs = allSelected ? [] : (preview?.changes.map((change) => change.id) ?? []);
		reviewed = false;
		deleteConfirmation = '';
	}

	function formatKey(change: database.DataSyncChange): string {
		return Object.entries(change.key)
			.map(([column, value]) => `${column}=${String(value)}`)
			.join(' · ');
	}
</script>

<div class="grid min-h-0 flex-1 grid-cols-[330px_minmax(0,1fr)] overflow-hidden">
	<aside class="min-h-0 overflow-y-auto border-r bg-[var(--surface-sunken)] p-4">
		<div class="mb-4">
			<h3 class="text-[11px] font-bold">Compare table data</h3>
			<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">
				Build a reviewed source-to-target row plan. Nothing is changed during comparison.
			</p>
		</div>

		<div class="space-y-4">
			<section class="rounded-lg border bg-[var(--surface-raised)] p-3">
				<div class="mb-3 flex items-center gap-2">
					<span class="flex h-6 w-6 items-center justify-center rounded-md border">
						<span class="text-[8px] font-bold">1</span>
					</span>
					<div>
						<p class="text-[9px] font-bold">Source</p>
						<p class="text-muted-foreground text-[7px]">Rows to mirror</p>
					</div>
				</div>
				<div class="space-y-2.5">
					<div>
						<label for="data-sync-source-connection">Connection</label>
						<FilterCombobox
							id="data-sync-source-connection"
							options={connectionOptions}
							value={sourceConnectionId}
							onChange={(value) => {
								sourceConnectionId = value;
								void loadEndpoint('source');
							}}
							disabled={loadingEndpoints || comparing || applying}
							triggerClass="h-8 px-2.5 text-[9px]"
						/>
					</div>
					<div class="grid grid-cols-2 gap-2">
						<div>
							<label for="data-sync-source-schema">Schema</label>
							<FilterCombobox
								id="data-sync-source-schema"
								options={sourceSchemaOptions}
								value={sourceSchema}
								onChange={(value) => {
									sourceSchema = value;
									void loadEndpoint('source', true);
								}}
								disabled={loadingEndpoints || comparing || applying}
								triggerClass="h-8 px-2.5 text-[9px]"
							/>
						</div>
						<div>
							<label for="data-sync-source-table">Table</label>
							<FilterCombobox
								id="data-sync-source-table"
								options={sourceTableOptions}
								value={sourceTable}
								onChange={(value) => {
									sourceTable = value;
									resetPreview();
								}}
								disabled={loadingEndpoints || comparing || applying}
								triggerClass="h-8 px-2.5 text-[9px]"
							/>
						</div>
					</div>
				</div>
			</section>

			<div class="flex justify-center">
				<span
					class="flex h-7 w-7 items-center justify-center rounded-full border bg-[var(--surface-raised)]"
				>
					<ArrowRight class="h-3.5 w-3.5 rotate-90" />
				</span>
			</div>

			<section class="rounded-lg border bg-[var(--surface-raised)] p-3">
				<div class="mb-3 flex items-center gap-2">
					<span class="flex h-6 w-6 items-center justify-center rounded-md border">
						<span class="text-[8px] font-bold">2</span>
					</span>
					<div class="min-w-0">
						<p class="text-[9px] font-bold">Target</p>
						<p class="text-muted-foreground truncate text-[7px]">
							Reviewed changes are applied here
						</p>
					</div>
				</div>
				<div class="space-y-2.5">
					<div>
						<label for="data-sync-target-connection">Connection</label>
						<FilterCombobox
							id="data-sync-target-connection"
							options={connectionOptions}
							value={targetConnectionId}
							onChange={(value) => {
								targetConnectionId = value;
								void loadEndpoint('target');
							}}
							disabled={loadingEndpoints || comparing || applying}
							triggerClass="h-8 px-2.5 text-[9px]"
						/>
					</div>
					<div class="grid grid-cols-2 gap-2">
						<div>
							<label for="data-sync-target-schema">Schema</label>
							<FilterCombobox
								id="data-sync-target-schema"
								options={targetSchemaOptions}
								value={targetSchema}
								onChange={(value) => {
									targetSchema = value;
									void loadEndpoint('target', true);
								}}
								disabled={loadingEndpoints || comparing || applying}
								triggerClass="h-8 px-2.5 text-[9px]"
							/>
						</div>
						<div>
							<label for="data-sync-target-table">Table</label>
							<FilterCombobox
								id="data-sync-target-table"
								options={targetTableOptions}
								value={targetTable}
								onChange={(value) => {
									targetTable = value;
									resetPreview();
								}}
								disabled={loadingEndpoints || comparing || applying}
								triggerClass="h-8 px-2.5 text-[9px]"
							/>
						</div>
					</div>
				</div>
				{#if targetConnection?.readOnly && !targetConnection.writeUnlocked}
					<div class="text-muted-foreground mt-3 flex items-start gap-2 border-t pt-2 text-[8px]">
						<ShieldAlert class="mt-0.5 h-3 w-3 shrink-0" />
						<span>Unlock target writes from the connection menu before applying.</span>
					</div>
				{/if}
			</section>

			<div>
				<label for="data-sync-row-limit">Maximum rows per table</label>
				<input
					id="data-sync-row-limit"
					type="number"
					min="1"
					max="10000"
					step="100"
					bind:value={maxRows}
					disabled={comparing || applying}
				/>
				<p class="text-muted-foreground mt-1 text-[7px]">
					Apply is blocked when either side exceeds this reviewed limit.
				</p>
			</div>

			<button
				type="button"
				class="rt-primary-button flex h-9 w-full items-center justify-center gap-2 rounded-md text-[9px] font-bold"
				onclick={compareData}
				disabled={!canCompare || comparing || loadingEndpoints || applying}
			>
				{#if comparing || loadingEndpoints}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{loadingEndpoints ? 'Loading endpoints…' : 'Comparing rows…'}
				{:else}
					<RefreshCw class="h-3.5 w-3.5" />
					Compare data
				{/if}
			</button>
		</div>
	</aside>

	<section class="flex min-h-0 min-w-0 flex-col bg-[var(--background)]">
		{#if error}
			<div
				class="border-danger-border bg-danger-soft text-danger m-4 flex gap-2 rounded-lg border p-3 text-[9px]"
			>
				<CircleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
				<span>{error}</span>
			</div>
		{/if}

		{#if !preview}
			<div class="flex min-h-0 flex-1 items-center justify-center p-8">
				<div class="max-w-sm text-center">
					<span
						class="mx-auto flex h-11 w-11 items-center justify-center rounded-xl border bg-[var(--surface-raised)]"
					>
						<DatabaseZap class="text-muted-foreground h-5 w-5" />
					</span>
					<h3 class="mt-3 text-[12px] font-bold">Review row-level differences</h3>
					<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">
						Rolling Thunder discovers primary keys, compares matching writable columns, and builds
						an atomic source-to-target change set.
					</p>
				</div>
			</div>
		{:else}
			<header class="flex min-h-14 shrink-0 items-center justify-between gap-3 border-b px-4">
				<div>
					<h3 class="text-[11px] font-bold">
						{sourceTable}
						<ArrowRight class="mx-1 inline h-3 w-3" />
						{targetTable}
					</h3>
					<p class="text-muted-foreground mt-1 text-[8px]">
						Key: {preview.keyColumns.join(', ')} · {preview.sourceRows.toLocaleString()} source ·
						{preview.targetRows.toLocaleString()} target
					</p>
				</div>
				<div class="flex items-center gap-1.5">
					<span
						class="border-success-border bg-success-soft text-success rounded-md border px-2 py-1 text-[8px] font-bold"
					>
						+{preview.added}
					</span>
					<span
						class="border-warning-border bg-warning-soft text-warning rounded-md border px-2 py-1 text-[8px] font-bold"
					>
						~{preview.updated}
					</span>
					<span
						class="border-danger-border bg-danger-soft text-danger rounded-md border px-2 py-1 text-[8px] font-bold"
					>
						−{preview.deleted}
					</span>
				</div>
			</header>

			{#if preview.warnings?.length}
				<div class="border-warning-border bg-warning-soft mx-4 mt-3 rounded-lg border p-3">
					{#each preview.warnings as warning}
						<p class="text-warning flex items-start gap-2 text-[8px]">
							<ShieldAlert class="mt-0.5 h-3 w-3 shrink-0" />
							<span>{warning}</span>
						</p>
					{/each}
				</div>
			{/if}

			<div class="flex h-10 shrink-0 items-center justify-between border-b px-4">
				<label class="flex cursor-pointer items-center gap-2 text-[8px] font-bold">
					<input
						type="checkbox"
						class="accent-primary h-3.5 w-3.5"
						checked={allSelected}
						onchange={toggleAll}
						disabled={preview.changes.length === 0}
					/>
					{selectedChangeIDs.length} of {preview.changes.length} selected
				</label>
				<span class="text-muted-foreground text-[8px]">
					{preview.compareColumns.length} compared columns
				</span>
			</div>

			<div class="min-h-0 flex-1 overflow-auto p-3">
				{#if preview.changes.length === 0}
					<div class="flex h-full min-h-52 items-center justify-center">
						<div class="text-center">
							<Check class="text-success mx-auto h-6 w-6" />
							<p class="mt-2 text-[10px] font-bold">Tables are aligned</p>
							<p class="text-muted-foreground mt-1 text-[8px]">No row changes are required.</p>
						</div>
					</div>
				{:else}
					<div class="space-y-1.5">
						{#each preview.changes as change (change.id)}
							<label
								class="flex cursor-pointer items-start gap-3 rounded-lg border bg-[var(--surface-raised)] p-3 hover:bg-[var(--surface-hover)]"
							>
								<input
									type="checkbox"
									class="accent-primary mt-0.5 h-3.5 w-3.5"
									checked={selectedChangeIDs.includes(change.id)}
									onchange={() => toggleChange(change.id)}
								/>
								<span class="min-w-0 flex-1">
									<span class="flex items-center gap-2">
										<span class="font-mono text-[9px] font-bold">{formatKey(change)}</span>
										<span
											class="text-muted-foreground rounded border px-1.5 py-0.5 text-[7px] font-bold uppercase"
										>
											{change.kind}
										</span>
									</span>
									<span class="text-muted-foreground mt-1 block truncate text-[8px]">
										{change.kind === 'update'
											? `Changed: ${change.changedColumns?.join(', ')}`
											: change.kind === 'insert'
												? 'Present in source, missing from target'
												: 'Missing from source, present in target'}
									</span>
								</span>
							</label>
						{/each}
					</div>
				{/if}
			</div>

			{#if preview.changes.length > 0}
				<footer class="shrink-0 border-t bg-[var(--surface-raised)] p-4">
					<div class="flex items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
							<label class="flex cursor-pointer items-start gap-2 text-[9px] font-semibold">
								<input
									type="checkbox"
									class="accent-primary mt-0.5 h-3.5 w-3.5"
									bind:checked={reviewed}
								/>
								<span>I reviewed the selected source-to-target row changes.</span>
							</label>
							{#if selectedDeletes > 0}
								<div class="mt-2 max-w-sm">
									<label for="data-sync-delete-confirmation">
										Type <span class="font-mono">{targetTable}</span> to confirm {selectedDeletes}
										delete{selectedDeletes === 1 ? '' : 's'}
									</label>
									<input
										id="data-sync-delete-confirmation"
										class="mt-1 font-mono"
										bind:value={deleteConfirmation}
										disabled={applying}
									/>
								</div>
							{/if}
						</div>
						<button
							type="button"
							class="rt-primary-button flex h-9 shrink-0 items-center gap-2 rounded-md px-4 text-[9px] font-bold"
							onclick={applyData}
							disabled={!canApply}
							title={targetConnection?.readOnly && !targetConnection.writeUnlocked
								? 'Unlock target writes before applying'
								: !preview.safeToApply
									? 'A truncated comparison cannot be applied'
									: 'Apply selected changes atomically'}
						>
							{#if applying}
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
								Applying…
							{:else}
								<Rows3 class="h-3.5 w-3.5" />
								Apply {selectedChangeIDs.length} changes
							{/if}
						</button>
					</div>
				</footer>
			{/if}
		{/if}
	</section>
</div>
