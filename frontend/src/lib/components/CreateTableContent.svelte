<script lang="ts">
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { setCreateTableSubmit } from '$lib/stores/staged.svelte';
	import { addTableToSidebar } from '$lib/stores/sidebar.svelte';
	import { Plus, Trash2, Table2 } from 'lucide-svelte';
	import { CreateTable, GetDataTypes } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { onMount } from 'svelte';
	import DataTypeSelect from '$lib/components/ui/DataTypeSelect.svelte';

	// Get current tab's schema
	const schema = $derived(tabsStore.activeTab?.schema ?? 'public');

	// Form state
	let tableName = $state('');
	let columns = $state<ColumnDef[]>([createEmptyColumn()]);
	let dataTypes = $state<{ name: string; category: string; description: string }[]>([]);
	let loading = $state(false);

	interface ColumnDef {
		id: string;
		name: string;
		type: string;
		size: string;
		nullable: boolean;
		defaultValue: string;
		primaryKey: boolean;
		unique: boolean;
	}

	// Types that require a size/length parameter
	const typesWithSize = ['varchar', 'char', 'numeric', 'decimal'];

	function typeNeedsSize(type: string): boolean {
		return typesWithSize.some((t) => type.toLowerCase().startsWith(t));
	}

	function createEmptyColumn(): ColumnDef {
		return {
			id: crypto.randomUUID(),
			name: '',
			type: 'integer',
			size: '',
			nullable: true,
			defaultValue: '',
			primaryKey: false,
			unique: false
		};
	}

	// Load data types and register submit callback on mount
	onMount(() => {
		// Register submit callback - reads state values at call time, not capture time
		setCreateTableSubmit(doSubmit);

		// Async data loading
		(async () => {
			try {
				const response = await GetDataTypes();
				if (response.data) {
					dataTypes = response.data;
				}
			} catch (e) {
				console.error('Failed to load data types', e);
				dataTypes = [
					{ name: 'integer', category: 'Numeric', description: '4 bytes integer' },
					{ name: 'bigint', category: 'Numeric', description: '8 bytes integer' },
					{ name: 'serial', category: 'Numeric', description: 'Auto-increment' },
					{ name: 'varchar', category: 'Character', description: 'Variable length' },
					{ name: 'text', category: 'Character', description: 'Unlimited text' },
					{ name: 'boolean', category: 'Boolean', description: 'true/false' },
					{ name: 'timestamp', category: 'Date/Time', description: 'Date and time' },
					{ name: 'uuid', category: 'UUID', description: 'Unique identifier' },
					{ name: 'jsonb', category: 'JSON', description: 'Binary JSON' }
				];
			}
		})();

		// Cleanup
		return () => {
			setCreateTableSubmit(null);
		};
	});

	// Submit function that reads current state values at call time
	async function doSubmit(): Promise<boolean> {
		// Read current values from $state at call time
		const currentSchema = schema;
		const currentTableName = tableName;
		const currentColumns = [...columns];

		if (!currentTableName.trim()) {
			updateStatus('Table name is required', 'error');
			return false;
		}

		const validColumns = currentColumns.filter((c) => c.name.trim() && c.type);

		if (validColumns.length === 0) {
			updateStatus('At least one column with name and type is required', 'error');
			return false;
		}

		loading = true;
		updateStatus('Creating table...', 'info');

		try {
			const table = new database.Table({ Schema: currentSchema, Name: currentTableName.trim() });
			const columnDefs = validColumns.map((c) => {
				let finalType = c.type;
				if (typeNeedsSize(c.type) && c.size) {
					finalType = `${c.type}(${c.size})`;
				}
				return {
					name: c.name.trim(),
					type: finalType,
					nullable: c.nullable,
					default: c.defaultValue,
					primaryKey: c.primaryKey,
					unique: c.unique
				};
			});

			const response = await CreateTable(table, columnDefs);
			if (response.errors?.length) {
				updateStatus(response.errors[0].detail, 'error');
				return false;
			} else {
				updateStatus(`CREATE TABLE ${currentSchema}.${currentTableName}`, 'info');
				// Add to sidebar immediately (optimistic update)
				addTableToSidebar(currentTableName.trim());
				const currentTabId = tabsStore.activeTabId;
				if (currentTabId) {
					tabsStore.closeTab(currentTabId);
				}
				tabsStore.newTableTab(currentSchema, currentTableName.trim());
				return true;
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to create table', 'error');
			return false;
		} finally {
			loading = false;
		}
	}

	// Check if any column is already primary key
	const hasPrimaryKey = $derived(columns.some((c) => c.primaryKey));

	// Check if form is valid for Apply button
	const isValid = $derived(tableName.trim() !== '' && columns.some((c) => c.name.trim() !== ''));

	function addColumn() {
		columns = [...columns, createEmptyColumn()];
	}

	function removeColumn(id: string) {
		if (columns.length > 1) {
			columns = columns.filter((c) => c.id !== id);
		}
	}

	function setPrimaryKey(colId: string, checked: boolean) {
		columns = columns.map((c) => {
			if (c.id === colId) {
				return { ...c, primaryKey: checked, nullable: checked ? false : c.nullable };
			}
			return { ...c, primaryKey: false };
		});
	}

	async function handleSubmit(): Promise<boolean> {
		if (!tableName.trim()) {
			updateStatus('Table name is required', 'error');
			return false;
		}

		const validColumns = columns.filter((c) => c.name.trim() && c.type);
		if (validColumns.length === 0) {
			updateStatus('At least one column with name and type is required', 'error');
			return false;
		}

		loading = true;
		updateStatus('Creating table...', 'info');

		try {
			const table = new database.Table({ schema, name: tableName.trim() });
			const columnDefs = validColumns.map((c) => {
				let finalType = c.type;
				if (typeNeedsSize(c.type) && c.size) {
					finalType = `${c.type}(${c.size})`;
				}
				return {
					name: c.name.trim(),
					type: finalType,
					nullable: c.nullable,
					default: c.defaultValue,
					primaryKey: c.primaryKey,
					unique: c.unique
				};
			});

			const response = await CreateTable(table, columnDefs);
			if (response.errors?.length) {
				updateStatus(response.errors[0].detail, 'error');
				return false;
			} else {
				updateStatus(`Table "${tableName}" created successfully`, 'success');
				const currentTabId = tabsStore.activeTabId;
				if (currentTabId) {
					tabsStore.closeTab(currentTabId);
				}
				tabsStore.newTableTab(schema, tableName.trim());
				return true;
			}
		} catch (e: any) {
			updateStatus(e?.message ?? 'Failed to create table', 'error');
			return false;
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex h-full flex-col overflow-hidden">
	<!-- Header -->
	<div class="flex h-12 shrink-0 items-center gap-3 border-b bg-[var(--surface-raised)] px-4">
		<span class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg">
			<Table2 class="h-4 w-4" />
		</span>
		<div>
			<h1 class="text-xs font-bold">Create new table</h1>
			<p class="text-muted-foreground mt-0.5 font-mono text-[9px]">{schema}</p>
		</div>
		{#if !isValid}
			<span class="text-muted-foreground ml-auto text-[9px]"
				>Add a table name and at least one valid column</span
			>
		{/if}
	</div>

	<!-- Content -->
	<div class="flex-1 overflow-auto bg-[var(--background)] p-4">
		<div class="mx-auto max-w-4xl space-y-3">
			<!-- Table Name -->
			<div class="rounded-lg border bg-[var(--surface-raised)] p-4 shadow-sm">
				<div class="mb-3">
					<label for="new-table-name" class="block text-xs font-bold">Table name</label>
					<p class="text-muted-foreground mt-0.5 text-[9px]">
						Choose a clear, schema-friendly name.
					</p>
				</div>
				<div class="flex items-center gap-2">
					<span
						class="text-muted-foreground bg-muted flex h-9 items-center rounded-md border px-2.5 font-mono text-xs"
						>{schema}.</span
					>
					<input
						id="new-table-name"
						type="text"
						class="rt-input h-9 flex-1 px-3 font-mono text-xs"
						placeholder="my_table_name"
						bind:value={tableName}
						autocapitalize="off"
						autocorrect="off"
						spellcheck="false"
					/>
				</div>
			</div>

			<!-- Columns -->
			<div class="overflow-hidden rounded-lg border bg-[var(--surface-raised)] shadow-sm">
				<div class="flex h-11 items-center justify-between border-b px-4">
					<div>
						<span class="text-xs font-bold">Columns</span>
						<span class="text-muted-foreground ml-1.5 text-[9px]">{columns.length} defined</span>
					</div>
					<button
						type="button"
						class="bg-primary/10 text-primary hover:bg-primary/20 inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border border-[var(--brand-border)] px-2.5 text-[10px] font-bold transition-colors"
						onclick={addColumn}
					>
						<Plus class="h-3 w-3" />
						Add column
					</button>
				</div>

				<!-- Column Table -->
				<div class="overflow-x-auto">
					<table class="w-full text-xs">
						<thead>
							<tr class="border-b">
								<th class="h-9 px-3 text-left font-medium">Name</th>
								<th class="h-9 px-3 text-left font-medium">Type</th>
								<th class="h-9 px-3 text-left font-medium">Default</th>
								<th class="h-9 px-3 text-center font-medium" title="Allow NULL values">Nullable</th>
								<th class="h-9 px-3 text-center font-medium" title="Primary Key">PK</th>
								<th class="h-9 px-3 text-center font-medium">Unique</th>
								<th class="w-10"></th>
							</tr>
						</thead>
						<tbody>
							{#each columns as col (col.id)}
								<tr class="border-b last:border-0">
									<td class="px-3 py-2">
										<input
											type="text"
											class="rt-input h-8 w-full min-w-32 px-2 font-mono text-[11px]"
											placeholder="column_name"
											bind:value={col.name}
											autocapitalize="off"
											autocorrect="off"
											spellcheck="false"
										/>
									</td>
									<td class="px-3 py-2">
										<div class="flex items-center gap-1">
											<div class="min-w-32">
												<DataTypeSelect
													{dataTypes}
													value={col.type}
													onChange={(v) => (col.type = v)}
												/>
											</div>
											{#if typeNeedsSize(col.type)}
												<span class="text-muted-foreground">(</span>
												<input
													type="text"
													class="rt-input h-8 w-16 px-2 text-center font-mono text-[11px]"
													placeholder="255"
													bind:value={col.size}
													autocapitalize="off"
													autocorrect="off"
													spellcheck="false"
												/>
												<span class="text-muted-foreground">)</span>
											{/if}
										</div>
									</td>
									<td class="px-3 py-2">
										<input
											type="text"
											class="rt-input h-8 w-full min-w-24 px-2 font-mono text-[11px]"
											placeholder="NULL"
											bind:value={col.defaultValue}
											autocapitalize="off"
											autocorrect="off"
											spellcheck="false"
										/>
									</td>
									<td class="px-3 py-2 text-center">
										<input
											type="checkbox"
											class="accent-primary h-4 w-4 rounded disabled:opacity-50"
											bind:checked={col.nullable}
											disabled={col.primaryKey}
										/>
									</td>
									<td class="px-3 py-2 text-center">
										<input
											type="checkbox"
											class="accent-primary h-4 w-4 rounded"
											checked={col.primaryKey}
											onchange={(e) => setPrimaryKey(col.id, e.currentTarget.checked)}
										/>
									</td>
									<td class="px-3 py-2 text-center">
										<input
											type="checkbox"
											class="accent-primary h-4 w-4 rounded"
											bind:checked={col.unique}
										/>
									</td>
									<td class="px-3 py-2">
										<button
											type="button"
											class="hover:bg-destructive/10 hover:text-destructive inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-md transition-colors disabled:opacity-30"
											onclick={() => removeColumn(col.id)}
											disabled={columns.length === 1}
											title="Remove column"
										>
											<Trash2 class="h-4 w-4" />
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Help text -->
				<div
					class="text-muted-foreground border-t bg-[var(--surface-sunken)] px-4 py-3 text-[10px]"
				>
					<p class="font-bold">Quick tips</p>
					<ul class="mt-1 list-inside list-disc space-y-0.5 leading-relaxed">
						<li>
							Use <code class="bg-muted rounded px-1">serial</code> or
							<code class="bg-muted rounded px-1">bigserial</code> for auto-increment IDs
						</li>
						<li>Primary Key columns are automatically NOT NULL</li>
						<li>
							Default values: <code class="bg-muted rounded px-1">NOW()</code>,
							<code class="bg-muted rounded px-1">'text'</code>,
							<code class="bg-muted rounded px-1">0</code>,
							<code class="bg-muted rounded px-1">gen_random_uuid()</code>
						</li>
						<li>Click <strong>Apply</strong> in the toolbar to create table</li>
					</ul>
				</div>
			</div>
		</div>
	</div>
</div>
