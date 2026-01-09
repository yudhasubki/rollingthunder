<script lang="ts">
	import { createDialog, melt } from '@melt-ui/svelte';
	import { writable } from 'svelte/store';
	import { X, Plus, Trash2, GripVertical } from 'lucide-svelte';
	import { fly, fade } from 'svelte/transition';
	import { CreateTable, GetDataTypes } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { onMount } from 'svelte';

	interface Props {
		schema: string;
		onSuccess: () => void;
	}

	let { schema, onSuccess }: Props = $props();

	// Dialog state
	const openStore = writable(false);
	const {
		elements: { trigger, overlay, content, title, close, portalled },
		states: { open }
	} = createDialog({
		open: openStore,
		forceVisible: true
	});

	// Form state
	let tableName = $state('');
	let columns = $state<ColumnDef[]>([createEmptyColumn()]);
	let dataTypes = $state<{ name: string; category: string; description: string }[]>([]);
	let loading = $state(false);

	interface ColumnDef {
		id: string;
		name: string;
		type: string;
		nullable: boolean;
		defaultValue: string;
		primaryKey: boolean;
		unique: boolean;
	}

	function createEmptyColumn(): ColumnDef {
		return {
			id: crypto.randomUUID(),
			name: '',
			type: 'integer',
			nullable: true,
			defaultValue: '',
			primaryKey: false,
			unique: false
		};
	}

	// Load data types on mount
	onMount(async () => {
		try {
			const response = await GetDataTypes();
			if (response.data) {
				dataTypes = response.data;
			}
		} catch (e) {
			console.error('Failed to load data types', e);
		}
	});

	// Data type options for combobox
	const dataTypeOptions = $derived(
		dataTypes.map((dt) => ({
			value: dt.name,
			label: `${dt.name} - ${dt.description}`
		}))
	);

	function addColumn() {
		columns = [...columns, createEmptyColumn()];
	}

	function removeColumn(id: string) {
		if (columns.length > 1) {
			columns = columns.filter((c) => c.id !== id);
		}
	}

	function resetForm() {
		tableName = '';
		columns = [createEmptyColumn()];
	}

	async function handleSubmit() {
		if (!tableName.trim()) {
			updateStatus('Table name is required', 'error');
			return;
		}

		const validColumns = columns.filter((c) => c.name.trim() && c.type);
		if (validColumns.length === 0) {
			updateStatus('At least one column is required', 'error');
			return;
		}

		loading = true;
		try {
			const table = new database.Table({ schema, name: tableName.trim() });
			const columnDefs = validColumns.map((c) => ({
				name: c.name.trim(),
				type: c.type,
				nullable: c.nullable,
				default: c.defaultValue,
				primaryKey: c.primaryKey,
				unique: c.unique
			}));

			const response = await CreateTable(table, columnDefs);
			if (response.errors?.length) {
				updateStatus(response.errors[0].detail, 'error');
			} else {
				updateStatus(`Table "${tableName}" created successfully`, 'success');
				openStore.set(false);
				resetForm();
				onSuccess();
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to create table', 'error');
		} finally {
			loading = false;
		}
	}

	export function openDialog() {
		openStore.set(true);
	}
</script>

<!-- Trigger is external, controlled via openDialog() -->

{#if $open}
	<div use:melt={$portalled}>
		<!-- Overlay -->
		<div
			use:melt={$overlay}
			class="fixed inset-0 z-50 bg-black/50"
			transition:fade={{ duration: 150 }}
		></div>

		<!-- Content -->
		<div
			use:melt={$content}
			class="bg-background fixed left-1/2 top-1/2 z-50 max-h-[85vh] w-[700px] -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-lg border shadow-lg"
			transition:fly={{ duration: 200, y: 10 }}
		>
			<!-- Header -->
			<div class="flex items-center justify-between border-b px-4 py-3">
				<h2 use:melt={$title} class="text-lg font-semibold">Create New Table</h2>
				<button use:melt={$close} class="hover:bg-muted rounded p-1 transition-colors">
					<X class="h-5 w-5" />
				</button>
			</div>

			<!-- Body -->
			<div class="max-h-[60vh] overflow-y-auto p-4">
				<!-- Table Name -->
				<div class="mb-4">
					<label class="text-sm font-medium">Table Name</label>
					<div class="mt-1.5 flex items-center gap-2">
						<span class="text-muted-foreground text-sm">{schema}.</span>
						<input
							type="text"
							class="border-input bg-background focus:ring-primary h-9 flex-1 rounded-md border px-3 text-sm focus:outline-none focus:ring-2"
							placeholder="table_name"
							bind:value={tableName}
						/>
					</div>
				</div>

				<!-- Columns -->
				<div class="mb-4">
					<div class="mb-2 flex items-center justify-between">
						<label class="text-sm font-medium">Columns</label>
						<button
							type="button"
							class="hover:bg-accent inline-flex h-7 cursor-pointer items-center gap-1 rounded-md px-2 text-xs transition-colors"
							onclick={addColumn}
						>
							<Plus class="h-3.5 w-3.5" />
							Add Column
						</button>
					</div>

					<!-- Column Headers -->
					<div
						class="text-muted-foreground mb-1 grid grid-cols-[1fr_1fr_80px_80px_60px_60px_32px] gap-2 px-1 text-xs font-medium"
					>
						<span>Name</span>
						<span>Type</span>
						<span>Default</span>
						<span class="text-center">Nullable</span>
						<span class="text-center">PK</span>
						<span class="text-center">Unique</span>
						<span></span>
					</div>

					<!-- Column Rows -->
					<div class="space-y-2">
						{#each columns as col (col.id)}
							<div class="grid grid-cols-[1fr_1fr_80px_80px_60px_60px_32px] items-center gap-2">
								<!-- Name -->
								<input
									type="text"
									class="border-input bg-background focus:ring-primary h-8 rounded-md border px-2 text-sm focus:outline-none focus:ring-1"
									placeholder="column_name"
									bind:value={col.name}
								/>

								<!-- Type -->
								<FilterCombobox
									options={dataTypeOptions.length > 0
										? dataTypeOptions
										: [{ value: 'integer', label: 'integer' }]}
									value={col.type}
									onChange={(v) => (col.type = v)}
									placeholder="Type..."
									class="w-full"
								/>

								<!-- Default -->
								<input
									type="text"
									class="border-input bg-background focus:ring-primary h-8 rounded-md border px-2 text-xs focus:outline-none focus:ring-1"
									placeholder="NULL"
									bind:value={col.defaultValue}
								/>

								<!-- Nullable -->
								<div class="flex justify-center">
									<input
										type="checkbox"
										class="accent-primary h-4 w-4 rounded"
										bind:checked={col.nullable}
									/>
								</div>

								<!-- Primary Key -->
								<div class="flex justify-center">
									<input
										type="checkbox"
										class="accent-primary h-4 w-4 rounded"
										bind:checked={col.primaryKey}
										onchange={() => {
											if (col.primaryKey) {
												col.nullable = false;
											}
										}}
									/>
								</div>

								<!-- Unique -->
								<div class="flex justify-center">
									<input
										type="checkbox"
										class="accent-primary h-4 w-4 rounded"
										bind:checked={col.unique}
									/>
								</div>

								<!-- Remove -->
								<button
									type="button"
									class="hover:bg-destructive/10 hover:text-destructive flex h-8 w-8 items-center justify-center rounded transition-colors disabled:opacity-30"
									onclick={() => removeColumn(col.id)}
									disabled={columns.length === 1}
								>
									<Trash2 class="h-4 w-4" />
								</button>
							</div>
						{/each}
					</div>
				</div>
			</div>

			<!-- Footer -->
			<div class="flex items-center justify-end gap-2 border-t px-4 py-3">
				<button
					type="button"
					class="hover:bg-accent h-9 rounded-md px-4 text-sm font-medium transition-colors"
					onclick={() => openStore.set(false)}
				>
					Cancel
				</button>
				<button
					type="button"
					class="bg-primary text-primary-foreground hover:bg-primary/90 h-9 rounded-md px-4 text-sm font-medium transition-colors disabled:opacity-50"
					onclick={handleSubmit}
					disabled={loading}
				>
					{loading ? 'Creating...' : 'Create Table'}
				</button>
			</div>
		</div>
	</div>
{/if}
