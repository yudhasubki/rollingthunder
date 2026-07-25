<script lang="ts">
	import {
		AlertTriangle,
		ArrowLeft,
		Check,
		Code2,
		Eye,
		Loader2,
		ShieldCheck,
		Trash2,
		X
	} from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import {
		ApplyDatabaseObjectChange,
		PreviewDatabaseObjectChange
	} from '$lib/wailsjs/go/db/Service';
	import {
		createObjectDefinitionTemplate,
		defaultObjectName,
		structuralIntentLabel,
		type StructuralChangeIntent
	} from '$lib/database/changeTemplates';
	import { createServiceError } from '$lib/errors/service';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { focusTrap } from '$lib/actions/focusTrap';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	interface Props {
		open: boolean;
		connectionId: string;
		intent: StructuralChangeIntent | null;
		capabilities?: database.Capabilities | null;
		reference?: database.ObjectReference | null;
		definition?: string;
		table?: database.Table | null;
		columns?: database.Structure[];
		onClose: () => void;
		onApplied?: (result: database.ObjectChangeResult) => void | Promise<void>;
	}

	let {
		open,
		connectionId,
		intent,
		capabilities = null,
		reference = null,
		definition: initialDefinition = '',
		table = null,
		columns = [],
		onClose,
		onApplied = () => {}
	}: Props = $props();

	let preview = $state<database.ObjectChangePreview | null>(null);
	let previewing = $state(false);
	let applying = $state(false);
	let error = $state('');
	let errorHint = $state('');
	let destructiveConfirmed = $state(false);
	let initializedKey = '';

	let objectName = $state('');
	let newName = $state('');
	let definition = $state('');
	let parentTableName = $state('');
	let cascade = $state(false);
	let indexColumns = $state('');
	let indexUnique = $state(false);
	let indexMethod = $state('btree');
	let indexWhere = $state('');
	let addColumnName = $state('');
	let addColumnType = $state('text');
	let addColumnNullable = $state(true);
	let addColumnDefault = $state('');
	let addColumnUnique = $state(false);
	let addColumnPrimary = $state(false);
	let addColumnPosition = $state<'end' | 'first' | 'after'>('end');
	let addColumnAfter = $state('');
	let selectedColumn = $state('');
	let columnNewName = $state('');
	let columnDataType = $state('');
	let columnUsing = $state('');
	let nullableMode = $state<'keep' | 'nullable' | 'required'>('keep');
	let defaultMode = $state<'keep' | 'set' | 'drop'>('keep');
	let columnDefault = $state('');
	let constraintName = $state('');
	let constraintDefinition = $state('');

	const schema = $derived(table?.Schema || reference?.schema || '');
	const tableName = $derived(table?.Name || reference?.parentName || '');
	const title = $derived(intent ? structuralIntentLabel(intent) : 'Structural change');
	const isDefinitionIntent = $derived(
		intent === 'create-view' ||
			intent === 'create-materialized-view' ||
			intent === 'create-function' ||
			intent === 'create-procedure' ||
			intent === 'create-trigger' ||
			intent === 'edit'
	);
	const isDirectIntent = $derived(intent === 'enable' || intent === 'disable');
	const selectedColumnMetadata = $derived(
		columns.find((column) => column.name === selectedColumn) || null
	);
	const columnOptions = $derived(
		columns.map((column) => ({
			value: column.name,
			label: `${column.name} · ${column.data_type}`
		}))
	);
	const nullableOptions = [
		{ value: 'keep', label: 'Keep current' },
		{ value: 'nullable', label: 'Allow NULL' },
		{ value: 'required', label: 'Set NOT NULL' }
	];
	const defaultOptions = [
		{ value: 'keep', label: 'Keep current' },
		{ value: 'set', label: 'Set default' },
		{ value: 'drop', label: 'Drop default' }
	];
	const columnPositionOptions = [
		{ value: 'end', label: 'At the end' },
		{ value: 'first', label: 'First column' },
		{ value: 'after', label: 'After a column' }
	];

	$effect(() => {
		if (!open || !intent) return;
		const key = [
			intent,
			connectionId,
			reference?.id,
			reference?.kind,
			reference?.schema,
			reference?.name,
			table?.Schema,
			table?.Name,
			initialDefinition
		].join(':');
		if (initializedKey === key) return;
		initializedKey = key;
		initializeForm();
	});

	function initializeForm() {
		preview = null;
		previewing = false;
		applying = false;
		error = '';
		errorHint = '';
		destructiveConfirmed = false;
		cascade = false;
		objectName = reference?.name || (intent ? defaultObjectName(intent) : '');
		newName = reference?.name ? `${reference.name}_renamed` : '';
		parentTableName = tableName;
		indexColumns = '';
		indexUnique = false;
		indexMethod = capabilities?.engine === 'sqlite' ? '' : 'btree';
		indexWhere = '';
		addColumnName = 'new_column';
		addColumnType = capabilities?.engine === 'postgres' ? 'text' : 'TEXT';
		addColumnNullable = true;
		addColumnDefault = '';
		addColumnUnique = false;
		addColumnPrimary = false;
		addColumnPosition = 'end';
		addColumnAfter = columns.at(-1)?.name || '';
		selectedColumn = columns[0]?.name || '';
		columnNewName = '';
		columnDataType = '';
		columnUsing = '';
		nullableMode = 'keep';
		defaultMode = 'keep';
		columnDefault = '';
		constraintName =
			reference?.kind === 'constraint' ? reference.name : intent ? defaultObjectName(intent) : '';
		constraintDefinition = 'CHECK (column_name IS NOT NULL)';

		if (intent === 'edit') {
			definition = initialDefinition;
		} else if (isDefinitionIntent) {
			definition = createObjectDefinitionTemplate(
				intent,
				capabilities,
				schema,
				objectName,
				parentTableName
			);
		} else {
			definition = '';
		}
	}

	function closeDialog() {
		if (previewing || applying) return;
		initializedKey = '';
		onClose();
	}

	function objectKindForIntent(): string {
		switch (intent) {
			case 'create-view':
				return 'view';
			case 'create-materialized-view':
				return 'materialized_view';
			case 'create-function':
				return 'function';
			case 'create-procedure':
				return 'procedure';
			case 'create-trigger':
				return 'trigger';
			default:
				return reference?.kind || 'table';
		}
	}

	function currentTable(): database.Table {
		return new database.Table({
			Schema: table?.Schema || reference?.parentSchema || reference?.schema || schema,
			Name: table?.Name || reference?.parentName || parentTableName || tableName
		});
	}

	function currentReference(): database.ObjectReference {
		if (
			intent === 'create-view' ||
			intent === 'create-materialized-view' ||
			intent === 'create-function' ||
			intent === 'create-procedure' ||
			intent === 'create-trigger'
		) {
			return new database.ObjectReference({
				kind: objectKindForIntent(),
				schema,
				name: objectName.trim(),
				parentSchema: intent === 'create-trigger' ? schema : '',
				parentName: intent === 'create-trigger' ? parentTableName.trim() : ''
			});
		}
		return new database.ObjectReference({
			id: reference?.id || '',
			kind: reference?.kind || objectKindForIntent(),
			schema: reference?.schema || schema,
			name: reference?.name || objectName.trim() || tableName,
			signature: reference?.signature || '',
			parentSchema: reference?.parentSchema || '',
			parentName: reference?.parentName || ''
		});
	}

	function buildRequest(): database.ObjectChangeRequest {
		const objectReference = currentReference();
		switch (intent) {
			case 'create-view':
			case 'create-materialized-view':
			case 'create-function':
			case 'create-procedure':
			case 'create-trigger':
				return new database.ObjectChangeRequest({
					action: 'create',
					reference: objectReference,
					definition
				});
			case 'edit':
				return new database.ObjectChangeRequest({
					action: 'replace',
					reference: objectReference,
					definition
				});
			case 'rename':
				return new database.ObjectChangeRequest({
					action: 'rename',
					reference: objectReference,
					newName: newName.trim()
				});
			case 'drop':
				return new database.ObjectChangeRequest({
					action: 'drop',
					reference: objectReference,
					cascade
				});
			case 'enable':
			case 'disable':
				return new database.ObjectChangeRequest({
					action: intent,
					reference: objectReference
				});
			case 'create-index':
				return new database.ObjectChangeRequest({
					action: 'create_index',
					reference: objectReference,
					index: new database.IndexChange({
						table: currentTable(),
						name: objectName.trim(),
						columns: indexColumns
							.split(',')
							.map((column) => column.trim())
							.filter(Boolean),
						unique: indexUnique,
						method: indexMethod.trim(),
						where: indexWhere.trim()
					})
				});
			case 'add-column':
				return new database.ObjectChangeRequest({
					action: 'add_column',
					reference: objectReference,
					addColumn: new database.AddColumnChange({
						table: currentTable(),
						column: new database.ColumnDefinition({
							name: addColumnName.trim(),
							type: addColumnType.trim(),
							nullable: addColumnNullable,
							default: addColumnDefault.trim(),
							primaryKey: addColumnPrimary,
							unique: addColumnUnique
						}),
						first: capabilities?.engine === 'mysql' && addColumnPosition === 'first',
						after:
							capabilities?.engine === 'mysql' && addColumnPosition === 'after'
								? addColumnAfter
								: ''
					})
				});
			case 'alter-column': {
				const nullable = nullableMode === 'keep' ? undefined : nullableMode === 'nullable';
				const columnDefaultValue = defaultMode === 'set' ? columnDefault.trim() : undefined;
				return new database.ObjectChangeRequest({
					action: 'alter_column',
					reference: objectReference,
					column: new database.ColumnChange({
						table: currentTable(),
						name: selectedColumn,
						newName: columnNewName.trim(),
						dataType: columnDataType.trim(),
						using: columnUsing.trim(),
						nullable,
						default: columnDefaultValue,
						dropDefault: defaultMode === 'drop'
					})
				});
			}
			case 'drop-column':
				return new database.ObjectChangeRequest({
					action: 'drop_column',
					reference: objectReference,
					cascade,
					dropColumn: new database.DropColumnChange({
						table: currentTable(),
						name: selectedColumn
					})
				});
			case 'add-constraint':
				return new database.ObjectChangeRequest({
					action: 'add_constraint',
					reference: objectReference,
					constraint: new database.ConstraintChange({
						table: currentTable(),
						name: constraintName.trim(),
						definition: constraintDefinition.trim()
					})
				});
			case 'drop-constraint':
				return new database.ObjectChangeRequest({
					action: 'drop_constraint',
					reference: objectReference,
					cascade,
					constraint: new database.ConstraintChange({
						table: currentTable(),
						name: constraintName.trim(),
						definition: ''
					})
				});
			default:
				throw new Error('Choose a structural change first.');
		}
	}

	async function generatePreview() {
		if (!intent || previewing) return;
		previewing = true;
		error = '';
		errorHint = '';
		destructiveConfirmed = false;
		try {
			const response = await PreviewDatabaseObjectChange(connectionId, buildRequest());
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not generate SQL preview');
			}
			preview = response.data || null;
			if (!preview) throw new Error('The driver returned no SQL preview.');
		} catch (previewError: any) {
			error = previewError?.message || 'Could not generate SQL preview';
			errorHint = previewError?.hint || '';
		} finally {
			previewing = false;
		}
	}

	async function applyChange() {
		if (!preview || applying || (preview.destructive && !destructiveConfirmed)) return;
		applying = true;
		error = '';
		errorHint = '';
		try {
			const response = await ApplyDatabaseObjectChange(
				connectionId,
				new database.ApplyObjectChangeRequest({
					change: buildRequest(),
					fingerprint: preview.fingerprint
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not apply structural change');
			}
			if (!response.data?.applied) {
				throw new Error('The database did not confirm the structural change.');
			}
			updateStatus(
				`${preview.summary} · ${response.data.statementCount} statement${response.data.statementCount === 1 ? '' : 's'} applied`,
				'success'
			);
			await onApplied(response.data);
			initializedKey = '';
			onClose();
		} catch (applyError: any) {
			error = applyError?.message || 'Could not apply structural change';
			errorHint = applyError?.hint || '';
			updateStatus(error, 'error');
		} finally {
			applying = false;
		}
	}

	function backToEditor() {
		if (applying) return;
		preview = null;
		error = '';
		errorHint = '';
		destructiveConfirmed = false;
	}

	function resetColumnEdits() {
		columnNewName = '';
		columnDataType = '';
		columnUsing = '';
		nullableMode = 'keep';
		defaultMode = 'keep';
		columnDefault = '';
	}

	function selectColumn(value: string) {
		selectedColumn = value;
		resetColumnEdits();
	}

	function selectNullableMode(value: string) {
		if (value === 'keep' || value === 'nullable' || value === 'required') {
			nullableMode = value;
		}
	}

	function selectDefaultMode(value: string) {
		if (value === 'keep' || value === 'set' || value === 'drop') {
			defaultMode = value;
		}
	}

	function selectAddColumnPosition(value: string) {
		if (value === 'end' || value === 'first' || value === 'after') {
			addColumnPosition = value;
		}
	}
</script>

{#if open && intent}
	<div class="fixed inset-0 z-[130] flex items-center justify-center p-6">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/50 backdrop-blur-[2px]"
			onclick={closeDialog}
			aria-label="Close structural change dialog"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[88vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="object-change-title"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg {preview?.destructive
						? 'bg-red-500/10 text-red-500'
						: 'bg-primary/10 text-primary'}"
				>
					{#if preview?.destructive}
						<Trash2 class="h-4 w-4" />
					{:else}
						<ShieldCheck class="h-4 w-4" />
					{/if}
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="object-change-title" class="truncate text-[13px] font-bold">{title}</h2>
					<p class="text-muted-foreground mt-0.5 text-[9px]">
						{preview
							? 'Review the exact SQL before it reaches the database.'
							: 'Configure the change, then generate a driver-owned SQL preview.'}
					</p>
				</div>
				<div class="text-muted-foreground flex items-center gap-1.5 text-[8px] font-bold uppercase">
					<span
						class="flex h-5 w-5 items-center justify-center rounded-full {preview
							? 'bg-emerald-500/15 text-emerald-500'
							: 'bg-primary text-primary-foreground'}">1</span
					>
					<span>Configure</span>
					<span class="bg-border h-px w-5"></span>
					<span
						class="flex h-5 w-5 items-center justify-center rounded-full {preview
							? 'bg-primary text-primary-foreground'
							: 'bg-muted'}">2</span
					>
					<span>Review</span>
				</div>
				<button
					type="button"
					class="rt-toolbar-button ml-2 h-8 w-8 cursor-pointer"
					onclick={closeDialog}
					disabled={previewing || applying}
					aria-label="Close"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="min-h-0 flex-1 overflow-auto bg-[var(--surface-sunken)] p-4">
				{#if error}
					<div
						class="mb-3 flex items-start gap-2.5 rounded-lg border border-red-500/25 bg-red-500/5 p-3"
					>
						<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
						<div>
							<p class="text-[10px] font-semibold text-red-600 dark:text-red-400">{error}</p>
							{#if errorHint}
								<p class="text-muted-foreground mt-1 text-[9px]">{errorHint}</p>
							{/if}
						</div>
					</div>
				{/if}

				{#if preview}
					<div class="space-y-3">
						<div class="grid gap-3 sm:grid-cols-3">
							<div class="rounded-lg border bg-[var(--surface-raised)] p-3 sm:col-span-2">
								<div class="text-muted-foreground text-[8px] font-bold tracking-[0.1em] uppercase">
									Change summary
								</div>
								<div class="mt-1 text-[11px] font-bold">{preview.summary}</div>
							</div>
							<div class="rounded-lg border bg-[var(--surface-raised)] p-3">
								<div class="text-muted-foreground text-[8px] font-bold tracking-[0.1em] uppercase">
									Execution
								</div>
								<div class="mt-1 text-[10px] font-semibold">
									{preview.statementCount} statement{preview.statementCount === 1 ? '' : 's'}
								</div>
								<div class="text-muted-foreground mt-0.5 text-[8px]">
									{preview.transactional ? 'Atomic transaction' : 'Engine auto-commit'}
								</div>
							</div>
						</div>

						{#if (preview.warnings || []).length > 0}
							<div class="rounded-lg border border-amber-500/25 bg-amber-500/5 p-3">
								<div
									class="flex items-center gap-2 text-[9px] font-bold text-amber-700 dark:text-amber-300"
								>
									<AlertTriangle class="h-3.5 w-3.5" />
									Review impact
								</div>
								<ul class="text-muted-foreground mt-1.5 space-y-1 pl-5 text-[9px]">
									{#each preview.warnings || [] as warning}
										<li class="list-disc">{warning}</li>
									{/each}
								</ul>
							</div>
						{/if}

						<div class="overflow-hidden rounded-xl border bg-[var(--surface-raised)]">
							<div class="flex h-9 items-center justify-between border-b px-3">
								<div class="flex items-center gap-2">
									<Code2 class="h-3.5 w-3.5" />
									<span class="text-[9px] font-bold">SQL preview</span>
								</div>
								<span class="text-muted-foreground font-mono text-[8px]">
									Reviewed fingerprint {preview.fingerprint.slice(0, 10)}
								</span>
							</div>
							<pre
								class="rt-code-surface max-h-[42vh] min-h-44 overflow-auto p-4 font-mono text-[10px] leading-[1.65] whitespace-pre"><code
									>{preview.sql}</code
								></pre>
						</div>

						{#if preview.destructive}
							<label
								class="flex cursor-pointer items-start gap-2.5 rounded-lg border border-red-500/30 bg-red-500/5 p-3"
							>
								<input
									type="checkbox"
									class="mt-0.5 h-3.5 w-3.5 accent-red-600"
									bind:checked={destructiveConfirmed}
								/>
								<span>
									<span class="block text-[10px] font-bold text-red-600 dark:text-red-400">
										I understand this structural change is destructive.
									</span>
									<span class="text-muted-foreground mt-0.5 block text-[8px]">
										The operation can remove an object, constraint, or dependent metadata
										permanently.
									</span>
								</span>
							</label>
						{/if}
					</div>
				{:else}
					<div class="mx-auto max-w-2xl space-y-4">
						{#if intent === 'rename'}
							<section class="rounded-xl border bg-[var(--surface-raised)] p-4">
								<label class="text-[9px] font-bold" for="object-new-name">New name</label>
								<input
									id="object-new-name"
									class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
									bind:value={newName}
									placeholder="new_object_name"
								/>
							</section>
						{:else if intent === 'drop'}
							<section class="rounded-xl border border-red-500/20 bg-[var(--surface-raised)] p-4">
								<div class="flex items-start gap-3">
									<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
									<div>
										<h3 class="text-[10px] font-bold">Permanent object removal</h3>
										<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">
											The preview will show the exact DROP statement for
											<span class="text-foreground font-mono font-semibold"
												>{reference?.schema}.{reference?.name}</span
											>.
										</p>
										<label class="mt-3 flex cursor-pointer items-center gap-2 text-[9px]">
											<input type="checkbox" class="h-3.5 w-3.5" bind:checked={cascade} />
											Use CASCADE for dependent objects
										</label>
									</div>
								</div>
							</section>
						{:else if isDirectIntent}
							<section class="rounded-xl border bg-[var(--surface-raised)] p-4">
								<div class="flex items-start gap-3">
									<ShieldCheck class="text-primary mt-0.5 h-4 w-4 shrink-0" />
									<div>
										<h3 class="text-[10px] font-bold">{title}</h3>
										<p class="text-muted-foreground mt-1 text-[9px]">
											Generate the SQL preview to verify the target table and trigger.
										</p>
									</div>
								</div>
							</section>
						{:else if intent === 'create-index'}
							<section
								class="grid gap-3 rounded-xl border bg-[var(--surface-raised)] p-4 sm:grid-cols-2"
							>
								<label class="sm:col-span-2">
									<span class="text-[9px] font-bold">Index name</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={objectName}
										placeholder="orders_customer_idx"
									/>
								</label>
								<label class="sm:col-span-2">
									<span class="text-[9px] font-bold">Columns</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={indexColumns}
										placeholder={columns.length
											? columns
													.slice(0, 3)
													.map((column) => column.name)
													.join(', ')
											: 'column_a, column_b'}
									/>
									<span class="text-muted-foreground mt-1 block text-[8px]">
										Comma-separated, in index priority order.
									</span>
								</label>
								<label>
									<span class="text-[9px] font-bold">Method</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={indexMethod}
										placeholder="btree"
									/>
								</label>
								<label class="flex items-end pb-2">
									<span class="flex cursor-pointer items-center gap-2 text-[9px] font-semibold">
										<input type="checkbox" class="h-3.5 w-3.5" bind:checked={indexUnique} />
										Unique index
									</span>
								</label>
								<label class="sm:col-span-2">
									<span class="text-[9px] font-bold"
										>Predicate <span class="text-muted-foreground">(optional)</span></span
									>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={indexWhere}
										placeholder="deleted_at IS NULL"
									/>
								</label>
							</section>
						{:else if intent === 'add-column'}
							<section
								class="grid gap-3 rounded-xl border bg-[var(--surface-raised)] p-4 sm:grid-cols-2"
							>
								<label>
									<span class="text-[9px] font-bold">Column name</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={addColumnName}
										placeholder="new_column"
									/>
								</label>
								<label>
									<span class="text-[9px] font-bold">Data type</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={addColumnType}
										placeholder="text"
									/>
								</label>
								<label class="sm:col-span-2">
									<span class="text-[9px] font-bold"
										>Default expression
										<span class="text-muted-foreground">(optional)</span></span
									>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={addColumnDefault}
										placeholder={addColumnNullable ? 'NULL' : "'pending'"}
									/>
								</label>
								{#if capabilities?.engine === 'mysql'}
									<div>
										<label class="text-[9px] font-bold" for="add-column-position">Position</label>
										<FilterCombobox
											id="add-column-position"
											class="mt-1.5"
											options={columnPositionOptions}
											value={addColumnPosition}
											onChange={selectAddColumnPosition}
											searchable={false}
											triggerClass="h-9 px-3 text-[10px]"
										/>
									</div>
									{#if addColumnPosition === 'after'}
										<div>
											<label class="text-[9px] font-bold" for="add-column-after">After column</label
											>
											<FilterCombobox
												id="add-column-after"
												class="mt-1.5"
												options={columnOptions}
												value={addColumnAfter}
												onChange={(value) => (addColumnAfter = value)}
												placeholder="Select a column"
												searchPlaceholder="Search columns…"
												triggerClass="h-9 px-3 text-[10px] font-mono"
											/>
										</div>
									{/if}
								{/if}
								<div class="flex flex-wrap gap-4 border-t pt-3 sm:col-span-2">
									<label class="flex cursor-pointer items-center gap-2 text-[9px] font-semibold">
										<input type="checkbox" class="h-3.5 w-3.5" bind:checked={addColumnNullable} />
										Nullable
									</label>
									<label class="flex cursor-pointer items-center gap-2 text-[9px] font-semibold">
										<input
											type="checkbox"
											class="h-3.5 w-3.5"
											bind:checked={addColumnUnique}
											disabled={capabilities?.engine === 'sqlite'}
										/>
										Unique
									</label>
									<label class="flex cursor-pointer items-center gap-2 text-[9px] font-semibold">
										<input
											type="checkbox"
											class="h-3.5 w-3.5"
											bind:checked={addColumnPrimary}
											disabled={capabilities?.engine === 'sqlite'}
										/>
										Primary key
									</label>
								</div>
								{#if capabilities?.engine === 'sqlite'}
									<p class="text-muted-foreground text-[8px] leading-relaxed sm:col-span-2">
										SQLite appends columns. A required column needs a non-NULL default; primary and
										unique constraints must be created separately.
									</p>
								{/if}
							</section>
						{:else if intent === 'alter-column'}
							<section
								class="grid gap-3 rounded-xl border bg-[var(--surface-raised)] p-4 sm:grid-cols-2"
							>
								<div class="sm:col-span-2">
									<label class="text-[9px] font-bold" for="alter-column-source">Column</label>
									<FilterCombobox
										id="alter-column-source"
										class="mt-1.5"
										options={columnOptions}
										value={selectedColumn}
										onChange={selectColumn}
										placeholder="Select a column"
										searchPlaceholder="Search columns…"
										emptyText="No matching columns"
										triggerClass="h-9 px-3 text-[10px] font-mono"
									/>
								</div>
								<label>
									<span class="text-[9px] font-bold"
										>Rename to <span class="text-muted-foreground">(optional)</span></span
									>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={columnNewName}
										placeholder={selectedColumn}
									/>
								</label>
								<label>
									<span class="text-[9px] font-bold"
										>New data type <span class="text-muted-foreground">(optional)</span></span
									>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={columnDataType}
										placeholder={selectedColumnMetadata?.data_type || 'text'}
									/>
								</label>
								<label class="sm:col-span-2">
									<span class="text-[9px] font-bold"
										>USING expression <span class="text-muted-foreground"
											>(for type conversion)</span
										></span
									>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={columnUsing}
										placeholder={`${selectedColumn}::text`}
									/>
								</label>
								<div>
									<label class="text-[9px] font-bold" for="alter-column-nullability"
										>Nullability</label
									>
									<FilterCombobox
										id="alter-column-nullability"
										class="mt-1.5"
										options={nullableOptions}
										value={nullableMode}
										onChange={selectNullableMode}
										searchable={false}
										triggerClass="h-9 px-3 text-[10px]"
									/>
								</div>
								<div>
									<label class="text-[9px] font-bold" for="alter-column-default">Default</label>
									<FilterCombobox
										id="alter-column-default"
										class="mt-1.5"
										options={defaultOptions}
										value={defaultMode}
										onChange={selectDefaultMode}
										searchable={false}
										triggerClass="h-9 px-3 text-[10px]"
									/>
								</div>
								{#if defaultMode === 'set'}
									<label class="sm:col-span-2">
										<span class="text-[9px] font-bold">Default expression</span>
										<input
											class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
											bind:value={columnDefault}
											placeholder="now()"
										/>
									</label>
								{/if}
							</section>
						{:else if intent === 'drop-column'}
							<section class="rounded-xl border border-red-500/20 bg-[var(--surface-raised)] p-4">
								<div class="flex items-start gap-3">
									<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
									<div class="min-w-0 flex-1">
										<h3 class="text-[10px] font-bold">Permanent column removal</h3>
										<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">
											All values stored in this column will be permanently removed.
										</p>
										<div class="mt-3">
											<label class="text-[9px] font-bold" for="drop-column-source">Column</label>
											<FilterCombobox
												id="drop-column-source"
												class="mt-1.5"
												options={columnOptions}
												value={selectedColumn}
												onChange={selectColumn}
												placeholder="Select a column"
												searchPlaceholder="Search columns…"
												triggerClass="h-9 px-3 text-[10px] font-mono"
											/>
										</div>
										{#if capabilities?.engine === 'postgres'}
											<label class="mt-3 flex cursor-pointer items-center gap-2 text-[9px]">
												<input type="checkbox" class="h-3.5 w-3.5" bind:checked={cascade} />
												Also drop dependent objects with CASCADE
											</label>
										{/if}
									</div>
								</div>
							</section>
						{:else if intent === 'add-constraint' || intent === 'drop-constraint'}
							<section class="space-y-3 rounded-xl border bg-[var(--surface-raised)] p-4">
								<label>
									<span class="text-[9px] font-bold">Constraint name</span>
									<input
										class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
										bind:value={constraintName}
										readonly={intent === 'drop-constraint' && Boolean(reference?.name)}
									/>
								</label>
								{#if intent === 'add-constraint'}
									<label>
										<span class="text-[9px] font-bold">Constraint definition</span>
										<textarea
											class="rt-input mt-1.5 min-h-28 w-full resize-y p-3 font-mono text-[10px] leading-relaxed"
											bind:value={constraintDefinition}
											placeholder="FOREIGN KEY (customer_id) REFERENCES customers(id)"
										></textarea>
									</label>
								{:else}
									<label class="flex cursor-pointer items-center gap-2 text-[9px]">
										<input type="checkbox" class="h-3.5 w-3.5" bind:checked={cascade} />
										Drop dependent objects with CASCADE
									</label>
								{/if}
							</section>
						{:else if isDefinitionIntent}
							<section class="space-y-3 rounded-xl border bg-[var(--surface-raised)] p-4">
								{#if intent !== 'edit'}
									<div class="grid gap-3 sm:grid-cols-2">
										<label>
											<span class="text-[9px] font-bold">Object name</span>
											<input
												class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
												bind:value={objectName}
											/>
										</label>
										{#if intent === 'create-trigger'}
											<label>
												<span class="text-[9px] font-bold">Parent table</span>
												<input
													class="rt-input mt-1.5 h-9 w-full px-3 font-mono text-[10px]"
													bind:value={parentTableName}
													placeholder="table_name"
												/>
											</label>
										{:else}
											<div>
												<span class="text-[9px] font-bold">Namespace</span>
												<div
													class="text-muted-foreground mt-1.5 flex h-9 items-center rounded-md border bg-[var(--surface-sunken)] px-3 font-mono text-[10px]"
												>
													{schema || 'database'}
												</div>
											</div>
										{/if}
									</div>
								{/if}
								<label>
									<span class="text-[9px] font-bold">
										{intent === 'create-view' || intent === 'create-materialized-view'
											? 'View query'
											: 'DDL definition'}
									</span>
									<textarea
										class="rt-code-surface mt-1.5 min-h-64 w-full resize-y rounded-lg border p-3 font-mono text-[10px] leading-[1.6] outline-none"
										bind:value={definition}
										spellcheck="false"
									></textarea>
								</label>
								{#if intent !== 'edit'}
									<button
										type="button"
										class="rt-toolbar-button h-7 cursor-pointer px-2.5 text-[9px] font-semibold"
										onclick={() =>
											(definition = createObjectDefinitionTemplate(
												intent,
												capabilities,
												schema,
												objectName,
												parentTableName
											))}
									>
										Reset dialect template
									</button>
								{/if}
							</section>
						{/if}
					</div>
				{/if}
			</div>

			<footer class="flex shrink-0 items-center justify-between gap-3 border-t p-4">
				<div class="text-muted-foreground flex min-w-0 items-start gap-2 text-[8px]">
					<ShieldCheck class="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
					<span>
						The backend regenerates and fingerprints this SQL. A changed payload must be reviewed
						again.
					</span>
				</div>
				<div class="flex shrink-0 items-center gap-2">
					{#if preview}
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer gap-1.5 px-3 text-[9px] font-semibold"
							onclick={backToEditor}
							disabled={applying}
						>
							<ArrowLeft class="h-3 w-3" />
							Back
						</button>
						<button
							type="button"
							class="{preview.destructive
								? 'bg-red-600 text-white hover:bg-red-700'
								: 'rt-primary-button'} inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold disabled:pointer-events-none disabled:opacity-45"
							onclick={applyChange}
							disabled={applying || (preview.destructive && !destructiveConfirmed)}
						>
							{#if applying}
								<Loader2 class="h-3 w-3 animate-spin" />
								Applying…
							{:else}
								<Check class="h-3 w-3" />
								Apply reviewed SQL
							{/if}
						</button>
					{:else}
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer px-3 text-[9px] font-semibold"
							onclick={closeDialog}
							disabled={previewing}
						>
							Cancel
						</button>
						<button
							type="button"
							class="rt-primary-button inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[9px] font-bold disabled:opacity-45"
							onclick={generatePreview}
							disabled={previewing}
						>
							{#if previewing}
								<Loader2 class="h-3 w-3 animate-spin" />
								Generating…
							{:else}
								<Eye class="h-3 w-3" />
								Generate SQL preview
							{/if}
						</button>
					{/if}
				</div>
			</footer>
		</div>
	</div>
{/if}
