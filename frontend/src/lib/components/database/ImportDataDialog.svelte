<script lang="ts">
	import { tick } from 'svelte';
	import {
		ArrowLeft,
		ArrowRight,
		Check,
		FileJson2,
		FileSpreadsheet,
		FolderOpen,
		Import,
		Loader2,
		RefreshCw,
		Table2,
		TriangleAlert,
		X
	} from 'lucide-svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import {
		ChooseImportFile,
		GetCollections,
		GetCollectionStructures,
		GetSchemas,
		ImportData,
		InspectImportFile
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { focusTrap } from '$lib/actions/focusTrap';

	interface Props {
		open: boolean;
		connectionId: string;
		initialSchema?: string;
		onClose: () => void;
		onImported: (schema: string, table: string) => void | Promise<void>;
	}

	let { open, connectionId, initialSchema = '', onClose, onImported }: Props = $props();
	let step = $state(1);
	let selection = $state<database.ImportFileSelection | null>(null);
	let preview = $state<database.ImportPreview | null>(null);
	let options = $state(
		new database.ImportOptions({
			format: 'csv',
			delimiter: ',',
			header: true,
			emptyAsNull: true
		})
	);
	let columns = $state<database.ImportColumn[]>([]);
	let schemas = $state<string[]>([]);
	let tables = $state<string[]>([]);
	let targetStructures = $state<database.Structure[]>([]);
	let schema = $state('');
	let table = $state('');
	let createTable = $state(false);
	let busy = $state(false);
	let loadingTarget = $state(false);
	let error = $state('');
	let success = $state<database.ImportResult | null>(null);
	let initialized = false;
	let titleElement = $state<HTMLHeadingElement | null>(null);
	const typeOptions = [
		{ value: 'text', label: 'Text' },
		{ value: 'integer', label: 'Integer' },
		{ value: 'number', label: 'Decimal number' },
		{ value: 'boolean', label: 'Boolean' },
		{ value: 'datetime', label: 'Date / time' }
	];
	const delimiterOptions = [
		{ value: ',', label: 'Comma (,)' },
		{ value: ';', label: 'Semicolon (;)' },
		{ value: '\\t', label: 'Tab' },
		{ value: '|', label: 'Pipe (|)' }
	];
	const targetTableOptions = $derived(tables.map((name) => ({ value: name, label: name })));
	const schemaOptions = $derived(schemas.map((name) => ({ value: name, label: name })));
	const targetColumnOptions = $derived(
		targetStructures.map((column) => ({
			value: column.name,
			label: `${column.name} · ${column.data_type}`
		}))
	);
	const includedColumnCount = $derived(columns.filter((column) => column.included).length);

	$effect(() => {
		if (open && !initialized) {
			initialized = true;
			reset();
			void loadTargets();
			void tick().then(() => titleElement?.focus());
		} else if (!open) {
			initialized = false;
		}
	});

	function reset(): void {
		step = 1;
		selection = null;
		preview = null;
		options = new database.ImportOptions({
			format: 'csv',
			delimiter: ',',
			header: true,
			emptyAsNull: true
		});
		columns = [];
		tables = [];
		targetStructures = [];
		schema = initialSchema;
		table = '';
		createTable = false;
		busy = false;
		loadingTarget = false;
		error = '';
		success = null;
	}

	function responseError(response: { errors?: Array<{ detail?: string; hint?: string }> }): string {
		const serviceError = response.errors?.[0];
		return serviceError
			? `${serviceError.detail || 'The operation failed'}${serviceError.hint ? ` ${serviceError.hint}` : ''}`
			: '';
	}

	async function loadTargets(): Promise<void> {
		if (!connectionId) return;
		loadingTarget = true;
		try {
			const response = await GetSchemas(connectionId);
			const message = responseError(response);
			if (message) throw new Error(message);
			schemas = response.data || [];
			if (!schema || !schemas.includes(schema)) {
				schema =
					schemas.find((candidate) => candidate === initialSchema) ||
					schemas.find((candidate) => candidate === 'public') ||
					schemas.find((candidate) => candidate === 'main') ||
					schemas[0] ||
					'';
			}
			await loadTables();
		} catch (loadError: any) {
			error = loadError?.message || 'Could not load import targets.';
		} finally {
			loadingTarget = false;
		}
	}

	async function loadTables(): Promise<void> {
		if (!schema) {
			tables = [];
			return;
		}
		loadingTarget = true;
		try {
			const response = await GetCollections(connectionId, [schema]);
			const message = responseError(response);
			if (message) throw new Error(message);
			tables = (response.data || []).sort((left, right) => left.localeCompare(right));
			if (!createTable && table && !tables.includes(table)) {
				table = '';
				targetStructures = [];
			}
		} catch (loadError: any) {
			error = loadError?.message || 'Could not load target tables.';
		} finally {
			loadingTarget = false;
		}
	}

	async function chooseFile(): Promise<void> {
		busy = true;
		error = '';
		success = null;
		try {
			const response = await ChooseImportFile();
			const message = responseError(response);
			if (message) throw new Error(message);
			if (!response.data?.token) return;
			selection = response.data;
			options = new database.ImportOptions({
				format: selection.format,
				delimiter: ',',
				header: true,
				emptyAsNull: true
			});
			await refreshPreview();
		} catch (chooseError: any) {
			error = chooseError?.message || 'Could not choose the import file.';
		} finally {
			busy = false;
		}
	}

	async function refreshPreview(): Promise<void> {
		if (!selection) return;
		busy = true;
		error = '';
		try {
			const response = await InspectImportFile(
				new database.ImportPreviewRequest({
					token: selection.token,
					options,
					limit: 50
				})
			);
			const message = responseError(response);
			if (message) throw new Error(message);
			preview = response.data || null;
			columns = (preview?.columns || []).map((column) => new database.ImportColumn({ ...column }));
			if (createTable && !table) {
				table = fileStem(selection.name);
			}
		} catch (previewError: any) {
			preview = null;
			columns = [];
			error = previewError?.message || 'Could not preview the import file.';
		} finally {
			busy = false;
		}
	}

	function fileStem(name: string): string {
		return (
			name
				.replace(/\.(csv|json|jsonl|ndjson)$/i, '')
				.trim()
				.replace(/[^a-zA-Z0-9_]+/g, '_')
				.replace(/^_+|_+$/g, '') || 'imported_data'
		);
	}

	function updateColumn(index: number, patch: Partial<database.ImportColumn>): void {
		columns = columns.map((column, columnIndex) =>
			columnIndex === index ? new database.ImportColumn({ ...column, ...patch }) : column
		);
	}

	async function setSchema(value: string): Promise<void> {
		schema = value;
		table = '';
		targetStructures = [];
		await loadTables();
	}

	async function setTargetMode(nextCreateTable: boolean): Promise<void> {
		createTable = nextCreateTable;
		targetStructures = [];
		if (createTable) {
			table = selection ? fileStem(selection.name) : '';
			columns = columns.map(
				(column) =>
					new database.ImportColumn({
						...column,
						targetName: column.sourceName
					})
			);
		} else {
			table = '';
		}
	}

	async function setTargetTable(value: string): Promise<void> {
		table = value;
		targetStructures = [];
		if (!value) return;
		loadingTarget = true;
		error = '';
		try {
			const response = await GetCollectionStructures(
				connectionId,
				new database.Table({ Schema: schema, Name: table })
			);
			const message = responseError(response);
			if (message) throw new Error(message);
			targetStructures = response.data || [];
			columns = columns.map((column) => {
				const matching = targetStructures.find(
					(target) => target.name.toLowerCase() === column.sourceName.toLowerCase()
				);
				return new database.ImportColumn({
					...column,
					targetName: matching?.name || ''
				});
			});
		} catch (targetError: any) {
			error = targetError?.message || 'Could not inspect the target table.';
		} finally {
			loadingTarget = false;
		}
	}

	function continueToTarget(): void {
		if (!selection || !preview || columns.length === 0) {
			error = 'Choose a file and generate a valid preview first.';
			return;
		}
		error = '';
		step = 2;
	}

	function continueToReview(): void {
		error = '';
		if (!schema || !table.trim()) {
			error = 'Choose a namespace and target table.';
			return;
		}
		if (includedColumnCount === 0) {
			error = 'Include at least one source column.';
			return;
		}
		const mapped = columns.filter((column) => column.included);
		if (mapped.some((column) => !column.targetName.trim())) {
			error = 'Map every included source column to a target column.';
			return;
		}
		const targetNames = mapped.map((column) => column.targetName.trim().toLowerCase());
		if (new Set(targetNames).size !== targetNames.length) {
			error = 'Each target column can only be mapped once.';
			return;
		}
		step = 3;
	}

	async function runImport(): Promise<void> {
		if (!selection || busy) return;
		busy = true;
		error = '';
		try {
			const response = await ImportData(
				new database.ImportRequest({
					connectionId,
					token: selection.token,
					options,
					schema,
					table: table.trim(),
					createTable,
					columns
				})
			);
			const message = responseError(response);
			if (message) throw new Error(message);
			success = response.data || null;
			if (!success) throw new Error('The import completed without a result.');
			await onImported(success.schema, success.table);
		} catch (importError: any) {
			error = importError?.message || 'Import failed. No rows were committed.';
		} finally {
			busy = false;
		}
	}

	function formatBytes(value: number): string {
		if (value < 1024) return `${value} B`;
		if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
		return `${(value / (1024 * 1024)).toFixed(1)} MB`;
	}

	function previewValue(value: unknown): string {
		if (value === null || value === undefined) return 'NULL';
		if (typeof value === 'object') return JSON.stringify(value);
		return String(value);
	}

	function handleWindowKeydown(event: KeyboardEvent): void {
		if (open && event.key === 'Escape' && !busy) {
			onClose();
		}
	}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if open}
	<div class="fixed inset-0 z-[130] flex items-center justify-center p-5">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[2px]"
			onclick={() => !busy && onClose()}
			aria-label="Close import data"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[88vh] w-full max-w-[920px] flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="import-data-title"
		>
			<header class="flex h-15 shrink-0 items-center gap-3 border-b px-5">
				<span
					class="bg-primary/10 text-primary flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
				>
					<Import class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2
						id="import-data-title"
						bind:this={titleElement}
						tabindex="-1"
						class="text-[13px] font-bold outline-none"
					>
						Import CSV or JSON
					</h2>
					<p class="text-muted-foreground mt-0.5 text-[9px]">
						Preview, map, and review before any rows are written.
					</p>
				</div>
				{#if !success}
					<div class="flex items-center gap-1.5">
						{#each ['Source', 'Target', 'Review'] as label, index}
							<span
								class="flex items-center gap-1.5 text-[8px] font-semibold {step === index + 1
									? 'text-primary'
									: index + 1 < step
										? 'text-foreground'
										: 'text-muted-foreground'}"
							>
								<span
									class="flex h-5 w-5 items-center justify-center rounded-full border {index + 1 <=
									step
										? 'border-primary/40 bg-primary/10'
										: ''}"
								>
									{index + 1 < step ? '✓' : index + 1}
								</span>
								<span class="hidden sm:inline">{label}</span>
							</span>
							{#if index < 2}<span class="bg-border h-px w-5"></span>{/if}
						{/each}
					</div>
				{/if}
				<button
					type="button"
					class="rt-toolbar-button h-8 w-8 cursor-pointer"
					onclick={onClose}
					disabled={busy}
					aria-label="Close import data"
				>
					<X class="h-4 w-4" />
				</button>
			</header>

			{#if success}
				<div class="flex min-h-[360px] flex-col items-center justify-center p-8 text-center">
					<span
						class="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-500"
					>
						<Check class="h-7 w-7" />
					</span>
					<h3 class="text-base font-bold">Import committed</h3>
					<p class="text-muted-foreground mt-1 text-[10px]">
						{success.rowsInserted.toLocaleString()} rows inserted into
						<span class="text-foreground font-mono">{success.schema}.{success.table}</span>.
					</p>
					{#if success.warnings?.length}
						<div
							class="mt-4 max-w-lg rounded-lg border border-amber-500/25 bg-amber-500/10 px-4 py-3 text-left text-[9px] text-amber-700 dark:text-amber-300"
						>
							{#each success.warnings as warning}
								<p>{warning}</p>
							{/each}
						</div>
					{/if}
					<button
						type="button"
						class="rt-primary-button mt-6 h-8 cursor-pointer rounded-md px-4 text-[10px] font-bold"
						onclick={onClose}
					>
						Done
					</button>
				</div>
			{:else}
				<div class="min-h-0 flex-1 overflow-y-auto p-5">
					{#if step === 1}
						<div class="space-y-4">
							<button
								type="button"
								class="hover:border-primary/40 flex min-h-24 w-full cursor-pointer items-center gap-4 rounded-xl border border-dashed p-5 text-left transition-colors"
								onclick={chooseFile}
								disabled={busy}
							>
								<span
									class="bg-muted text-muted-foreground flex h-11 w-11 shrink-0 items-center justify-center rounded-xl"
								>
									{#if selection?.format === 'json'}
										<FileJson2 class="h-5 w-5" />
									{:else}
										<FileSpreadsheet class="h-5 w-5" />
									{/if}
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-[11px] font-bold">
										{selection?.name || 'Choose a CSV or JSON file'}
									</span>
									<span class="text-muted-foreground mt-1 block text-[9px]">
										{selection
											? `${selection.format.toUpperCase()} · ${formatBytes(selection.size)} · selected with the native picker`
											: 'CSV, JSON array, JSONL, and NDJSON are supported'}
									</span>
								</span>
								<span
									class="rt-toolbar-button flex h-8 items-center gap-1.5 px-3 text-[9px] font-semibold"
								>
									{#if busy}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<FolderOpen
											class="h-3.5 w-3.5"
										/>{/if}
									{selection ? 'Replace' : 'Browse'}
								</span>
							</button>

							{#if selection?.format === 'csv'}
								<section class="grid grid-cols-3 gap-3 rounded-xl border p-4">
									<label>
										<span class="text-muted-foreground mb-1 block text-[8px] font-semibold"
											>Delimiter</span
										>
										<FilterCombobox
											id="import-delimiter"
											options={delimiterOptions}
											value={options.delimiter || ','}
											onChange={(value) =>
												(options = new database.ImportOptions({
													...options,
													delimiter: value
												}))}
											searchable={false}
											triggerClass="h-8 px-2 text-[9px]"
										/>
									</label>
									<label
										class="flex h-8 items-center gap-2 self-end rounded-md border px-2.5 text-[9px]"
									>
										<input
											type="checkbox"
											checked={options.header}
											onchange={(event) =>
												(options = new database.ImportOptions({
													...options,
													header: event.currentTarget.checked
												}))}
										/>
										First row is header
									</label>
									<label
										class="flex h-8 items-center gap-2 self-end rounded-md border px-2.5 text-[9px]"
									>
										<input
											type="checkbox"
											checked={options.emptyAsNull}
											onchange={(event) =>
												(options = new database.ImportOptions({
													...options,
													emptyAsNull: event.currentTarget.checked
												}))}
										/>
										Empty values become NULL
									</label>
									<button
										type="button"
										class="rt-toolbar-button col-span-3 h-8 cursor-pointer gap-1.5 px-3 text-[9px] font-semibold"
										onclick={refreshPreview}
										disabled={!selection || busy}
									>
										<RefreshCw class="h-3.5 w-3.5 {busy ? 'animate-spin' : ''}" />
										Refresh preview with these settings
									</button>
								</section>
							{/if}

							{#if preview}
								<section class="overflow-hidden rounded-xl border">
									<div class="flex h-10 items-center justify-between border-b px-3">
										<div>
											<h3 class="text-[10px] font-bold">Source preview</h3>
											<p class="text-muted-foreground text-[8px]">
												{preview.sampled} sampled rows · {columns.length} detected columns
											</p>
										</div>
										<span class="text-muted-foreground text-[8px]"
											>Preview only · file stays local</span
										>
									</div>
									<div class="max-h-56 overflow-auto">
										<table class="w-full min-w-max text-left">
											<thead class="bg-muted/60 sticky top-0 z-10">
												<tr>
													{#each columns as column}
														<th class="h-8 border-r px-3 text-[8px] font-bold last:border-r-0">
															{column.sourceName}
															<span class="text-muted-foreground ml-1 font-normal"
																>{column.inferredType}</span
															>
														</th>
													{/each}
												</tr>
											</thead>
											<tbody>
												{#each preview.rows.slice(0, 12) as row}
													<tr class="border-t">
														{#each columns as column}
															<td
																class="text-muted-foreground max-w-56 truncate border-r px-3 py-2 font-mono text-[8px] last:border-r-0"
															>
																{previewValue(row[column.sourceName])}
															</td>
														{/each}
													</tr>
												{/each}
											</tbody>
										</table>
									</div>
								</section>
							{/if}
						</div>
					{:else if step === 2}
						<div class="space-y-4">
							<section class="grid grid-cols-[180px_1fr] gap-3 rounded-xl border p-4">
								<label>
									<span class="text-muted-foreground mb-1 block text-[8px] font-semibold"
										>Namespace</span
									>
									<FilterCombobox
										id="import-schema"
										options={schemaOptions}
										value={schema}
										onChange={(value) => void setSchema(value)}
										searchable={schemas.length > 8}
										triggerClass="h-8 px-2 text-[9px]"
										disabled={loadingTarget}
									/>
								</label>
								<div>
									<span class="text-muted-foreground mb-1 block text-[8px] font-semibold"
										>Target mode</span
									>
									<div class="grid grid-cols-2 rounded-lg border bg-[var(--surface-sunken)] p-1">
										<button
											type="button"
											class="h-7 cursor-pointer rounded-md text-[9px] font-semibold {createTable
												? 'text-muted-foreground'
												: 'bg-[var(--surface-raised)] shadow-sm'}"
											onclick={() => void setTargetMode(false)}
										>
											Existing table
										</button>
										<button
											type="button"
											class="h-7 cursor-pointer rounded-md text-[9px] font-semibold {createTable
												? 'bg-[var(--surface-raised)] shadow-sm'
												: 'text-muted-foreground'}"
											onclick={() => void setTargetMode(true)}
										>
											Create new table
										</button>
									</div>
								</div>
								<div class="col-span-2">
									<label>
										<span class="text-muted-foreground mb-1 block text-[8px] font-semibold"
											>Table</span
										>
										{#if createTable}
											<input
												class="rt-input h-8 w-full px-2.5 font-mono text-[9px]"
												placeholder="new_table"
												bind:value={table}
												autocomplete="off"
											/>
										{:else}
											<FilterCombobox
												id="import-table"
												options={targetTableOptions}
												value={table}
												onChange={(value) => void setTargetTable(value)}
												placeholder="Choose an existing table"
												searchable={true}
												triggerClass="h-8 px-2 text-[9px]"
												disabled={loadingTarget}
											/>
										{/if}
									</label>
								</div>
							</section>

							<section class="overflow-hidden rounded-xl border">
								<div class="flex h-10 items-center justify-between border-b px-3">
									<div>
										<h3 class="text-[10px] font-bold">Column mapping</h3>
										<p class="text-muted-foreground text-[8px]">
											{includedColumnCount} of {columns.length} source columns included
										</p>
									</div>
									{#if loadingTarget}<Loader2
											class="text-muted-foreground h-3.5 w-3.5 animate-spin"
										/>{/if}
								</div>
								<div class="max-h-[330px] overflow-y-auto">
									{#each columns as column, index (column.sourceName)}
										<div
											class="grid min-h-12 grid-cols-[28px_minmax(120px,0.8fr)_24px_minmax(180px,1fr)_130px] items-center gap-2 border-b px-3 last:border-b-0"
										>
											<input
												type="checkbox"
												checked={column.included}
												onchange={(event) =>
													updateColumn(index, { included: event.currentTarget.checked })}
												aria-label="Include {column.sourceName}"
											/>
											<div class="min-w-0">
												<div class="truncate font-mono text-[9px]">{column.sourceName}</div>
												<div class="text-muted-foreground text-[7px]">
													detected {column.inferredType}
												</div>
											</div>
											<ArrowRight class="text-muted-foreground h-3 w-3" />
											{#if createTable}
												<input
													class="rt-input h-8 min-w-0 px-2 font-mono text-[9px]"
													value={column.targetName}
													oninput={(event) =>
														updateColumn(index, { targetName: event.currentTarget.value })}
													disabled={!column.included}
													aria-label="Target column for {column.sourceName}"
												/>
											{:else}
												<FilterCombobox
													id={`import-target-column-${index}`}
													options={targetColumnOptions}
													value={column.targetName}
													onChange={(value) => updateColumn(index, { targetName: value })}
													placeholder="Choose target"
													searchable={true}
													triggerClass="h-8 px-2 text-[9px]"
													disabled={!column.included || !table}
												/>
											{/if}
											<FilterCombobox
												id={`import-column-type-${index}`}
												options={typeOptions}
												value={column.inferredType}
												onChange={(value) => updateColumn(index, { inferredType: value })}
												searchable={false}
												triggerClass="h-8 px-2 text-[9px]"
												disabled={!column.included || !createTable}
											/>
										</div>
									{/each}
								</div>
							</section>
						</div>
					{:else}
						<div class="space-y-4">
							<section class="grid grid-cols-3 divide-x overflow-hidden rounded-xl border">
								<div class="p-4">
									<div class="text-muted-foreground text-[8px] font-semibold">Source</div>
									<div class="mt-1 truncate text-[10px] font-bold">{selection?.name}</div>
									<div class="text-muted-foreground mt-1 text-[8px]">
										{selection?.format.toUpperCase()} · {selection
											? formatBytes(selection.size)
											: ''}
									</div>
								</div>
								<div class="p-4">
									<div class="text-muted-foreground text-[8px] font-semibold">Target</div>
									<div class="mt-1 truncate font-mono text-[10px] font-bold">{schema}.{table}</div>
									<div class="text-muted-foreground mt-1 text-[8px]">
										{createTable ? 'Create table, then insert' : 'Insert into existing table'}
									</div>
								</div>
								<div class="p-4">
									<div class="text-muted-foreground text-[8px] font-semibold">Mapping</div>
									<div class="mt-1 text-[10px] font-bold">{includedColumnCount} columns</div>
									<div class="text-muted-foreground mt-1 text-[8px]">Transactional row import</div>
								</div>
							</section>
							<section class="overflow-hidden rounded-xl border">
								<div class="h-9 border-b px-3 py-2 text-[9px] font-bold">Review mappings</div>
								<div class="max-h-64 overflow-y-auto">
									{#each columns.filter((column) => column.included) as column}
										<div
											class="grid grid-cols-[1fr_32px_1fr_120px] items-center gap-2 border-b px-3 py-2 last:border-b-0"
										>
											<code class="truncate text-[9px]">{column.sourceName}</code>
											<ArrowRight class="text-muted-foreground mx-auto h-3 w-3" />
											<code class="truncate text-[9px]">{column.targetName}</code>
											<span class="text-muted-foreground text-right text-[8px]"
												>{column.inferredType}</span
											>
										</div>
									{/each}
								</div>
							</section>
							<div
								class="flex items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2.5 text-[9px] text-amber-700 dark:text-amber-300"
							>
								<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
								<span>
									Existing constraints, required columns, and unique keys are enforced by the
									database. Row inserts are rolled back together if any batch fails.
								</span>
							</div>
						</div>
					{/if}

					{#if error}
						<div
							class="mt-4 flex items-start gap-2 rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2.5 text-[9px] text-red-600 dark:text-red-400"
							role="alert"
						>
							<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span>{error}</span>
						</div>
					{/if}
				</div>

				<footer class="flex h-14 shrink-0 items-center justify-between border-t px-5">
					<div>
						{#if step > 1}
							<button
								type="button"
								class="rt-toolbar-button h-8 cursor-pointer gap-1.5 px-3 text-[9px] font-semibold"
								onclick={() => {
									error = '';
									step -= 1;
								}}
								disabled={busy}
							>
								<ArrowLeft class="h-3.5 w-3.5" />
								Back
							</button>
						{/if}
					</div>
					<div class="flex items-center gap-2">
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer px-3 text-[9px] font-semibold"
							onclick={onClose}
							disabled={busy}
						>
							Cancel
						</button>
						{#if step === 1}
							<button
								type="button"
								class="rt-primary-button flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold"
								onclick={continueToTarget}
								disabled={!preview || busy}
							>
								Choose target
								<ArrowRight class="h-3.5 w-3.5" />
							</button>
						{:else if step === 2}
							<button
								type="button"
								class="rt-primary-button flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold"
								onclick={continueToReview}
								disabled={busy || loadingTarget}
							>
								Review import
								<ArrowRight class="h-3.5 w-3.5" />
							</button>
						{:else}
							<button
								type="button"
								class="rt-primary-button flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold"
								onclick={runImport}
								disabled={busy}
							>
								{#if busy}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Import
										class="h-3.5 w-3.5"
									/>{/if}
								{busy ? 'Importing…' : 'Import and commit'}
							</button>
						{/if}
					</div>
				</footer>
			{/if}
		</div>
	</div>
{/if}
