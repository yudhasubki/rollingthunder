<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { GetCollections, GetCollectionStructures } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { getColumnTypeLabel } from '$lib/table/cells';
	import { getForeignRelation } from '$lib/table/relations';
	import {
		AlertCircle,
		Columns3,
		Database,
		KeyRound,
		Link2,
		Loader2,
		Maximize2,
		Minus,
		Plus,
		RefreshCw,
		Table2,
		Workflow
	} from 'lucide-svelte';

	interface Props {
		connectionId: string;
		schema: string;
	}

	interface DiagramTable {
		name: string;
		columns: database.Structure[];
	}

	interface PositionedTable extends DiagramTable {
		x: number;
		y: number;
		height: number;
		visibleColumns: database.Structure[];
		hiddenColumnCount: number;
	}

	interface Relation {
		id: string;
		sourceTable: string;
		sourceColumn: string;
		targetTable: string;
		targetColumn: string;
		startX: number;
		startY: number;
		endX: number;
		endY: number;
		path: string;
	}

	let { connectionId, schema }: Props = $props();

	const nodeWidth = 244;
	const columnHeight = 25;
	const canvasPadding = 48;
	const horizontalGap = 72;
	const verticalGap = 68;
	const columnsPerRow = 3;

	let tables = $state<DiagramTable[]>([]);
	let loading = $state(false);
	let errorMessage = $state('');
	let loadStage = $state('Preparing schema metadata');
	let loadedCount = $state(0);
	let totalCount = $state(0);
	let zoom = $state(0.9);
	let viewport: HTMLDivElement;

	const positionedTables = $derived.by<PositionedTable[]>(() => {
		const rowBottoms: number[] = [];

		return tables.map((table, index) => {
			const column = index % columnsPerRow;
			const row = Math.floor(index / columnsPerRow);
			const visibleColumns = table.columns.slice(0, 8);
			const hiddenColumnCount = Math.max(0, table.columns.length - visibleColumns.length);
			const height = 56 + visibleColumns.length * columnHeight + (hiddenColumnCount > 0 ? 27 : 0);
			const previousRowBottom = row === 0 ? canvasPadding : rowBottoms[row - 1] + verticalGap;
			const y = previousRowBottom;
			rowBottoms[row] = Math.max(rowBottoms[row] || 0, y + height);

			return {
				...table,
				x: canvasPadding + column * (nodeWidth + horizontalGap),
				y,
				height,
				visibleColumns,
				hiddenColumnCount
			};
		});
	});

	const canvasWidth = $derived(
		Math.max(
			920,
			canvasPadding * 2 +
				Math.min(columnsPerRow, Math.max(positionedTables.length, 1)) * nodeWidth +
				(Math.min(columnsPerRow, Math.max(positionedTables.length, 1)) - 1) * horizontalGap
		)
	);

	const canvasHeight = $derived(
		Math.max(
			560,
			positionedTables.length
				? Math.max(...positionedTables.map((table) => table.y + table.height)) + canvasPadding
				: 560
		)
	);

	const relations = $derived.by<Relation[]>(() => {
		const tableMap = new Map(positionedTables.map((table) => [table.name, table]));
		const output: Relation[] = [];

		for (const source of positionedTables) {
			source.columns.forEach((column, columnIndex) => {
				const relation = getForeignRelation(column, schema);
				if (!relation || relation.schema !== schema) return;

				const targetTableName = relation.table;
				const targetColumnName = relation.column;
				const target = tableMap.get(targetTableName);
				if (!target) return;

				const targetColumnIndex = Math.max(
					0,
					target.columns.findIndex((targetColumn) => targetColumn.name === targetColumnName)
				);
				const sourceRow = Math.min(columnIndex, 7);
				const targetRow = Math.min(targetColumnIndex, 7);
				const sourceIsLeft = source.x <= target.x;
				const startX = sourceIsLeft ? source.x + nodeWidth : source.x;
				const endX = sourceIsLeft ? target.x : target.x + nodeWidth;
				const startY = source.y + 56 + sourceRow * columnHeight + columnHeight / 2;
				const endY = target.y + 56 + targetRow * columnHeight + columnHeight / 2;
				const curve = Math.max(54, Math.abs(endX - startX) * 0.42);
				const direction = sourceIsLeft ? 1 : -1;
				const path = `M ${startX} ${startY} C ${startX + curve * direction} ${startY}, ${endX - curve * direction} ${endY}, ${endX} ${endY}`;

				output.push({
					id: `${source.name}.${column.name}-${targetTableName}.${targetColumnName}`,
					sourceTable: source.name,
					sourceColumn: column.name,
					targetTable: targetTableName,
					targetColumn: targetColumnName,
					startX,
					startY,
					endX,
					endY,
					path
				});
			});
		}

		return output;
	});

	onMount(() => {
		void loadDiagram();
	});

	async function loadDiagram() {
		loading = true;
		errorMessage = '';
		tables = [];
		loadedCount = 0;
		totalCount = 0;
		loadStage = `Discovering tables in ${schema}`;
		updateStatus(`Building schema diagram for ${schema}…`, 'info');
		const startedAt = performance.now();

		try {
			const tableResponse = await GetCollections(connectionId, [schema]);
			if (tableResponse.errors?.length) {
				throw new Error(tableResponse.errors[0].detail);
			}

			const tableNames = (tableResponse.data || []).sort((a, b) => a.localeCompare(b));
			totalCount = tableNames.length;

			for (let index = 0; index < tableNames.length; index += 4) {
				const batch = tableNames.slice(index, index + 4);
				loadStage = `Reading columns from ${batch.join(', ')}`;

				const loadedBatch = await Promise.all(
					batch.map(async (tableName) => {
						const request = new database.Table();
						request.Schema = schema;
						request.Name = tableName;
						const response = await GetCollectionStructures(connectionId, request);
						if (response.errors?.length) {
							throw new Error(`${tableName}: ${response.errors[0].detail}`);
						}
						return { name: tableName, columns: response.data || [] };
					})
				);

				tables = [...tables, ...loadedBatch];
				loadedCount += loadedBatch.length;
			}

			const duration = Math.round(performance.now() - startedAt);
			updateStatus(
				`Schema diagram ready: ${tables.length} tables, ${relations.length} relationships in ${duration}ms`,
				'success'
			);
			await tick();
			fitDiagram();
		} catch (error: any) {
			errorMessage = error?.message || 'Failed to load schema diagram';
			updateStatus(errorMessage, 'error');
		} finally {
			loading = false;
		}
	}

	function zoomIn() {
		zoom = Math.min(1.5, Number((zoom + 0.1).toFixed(2)));
	}

	function zoomOut() {
		zoom = Math.max(0.5, Number((zoom - 0.1).toFixed(2)));
	}

	function fitDiagram() {
		if (!viewport) return;
		const availableWidth = Math.max(320, viewport.clientWidth - 40);
		zoom = Math.max(0.5, Math.min(1, Number((availableWidth / canvasWidth).toFixed(2))));
		viewport.scrollTo({ top: 0, left: 0, behavior: 'smooth' });
	}

	function openTable(tableName: string) {
		const existing = tabsStore.findTableTab(connectionId, schema, tableName);
		if (existing) tabsStore.setActive(existing.id);
		else tabsStore.newTableTab(connectionId, schema, tableName);
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--background)]">
	<div
		class="flex h-11 shrink-0 items-center justify-between border-b bg-[var(--surface-raised)] px-3"
	>
		<div class="flex min-w-0 items-center gap-2.5">
			<span class="bg-primary/10 text-primary flex h-7 w-7 items-center justify-center rounded-md">
				<Workflow class="h-3.5 w-3.5" />
			</span>
			<div class="min-w-0">
				<div class="flex items-center gap-2">
					<h2 class="truncate text-[11px] font-bold">{schema} schema diagram</h2>
					<span class="text-muted-foreground rounded border px-1.5 py-0.5 text-[8px] font-semibold">
						{tables.length} tables
					</span>
					<span class="text-muted-foreground rounded border px-1.5 py-0.5 text-[8px] font-semibold">
						{relations.length} relations
					</span>
				</div>
				<p class="text-muted-foreground mt-0.5 text-[8px]">
					Foreign-key relationships for the selected schema
				</p>
			</div>
		</div>

		<div class="flex items-center gap-1">
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7"
				onclick={zoomOut}
				title="Zoom out"
				aria-label="Zoom out"
			>
				<Minus class="h-3.5 w-3.5" />
			</button>
			<span class="text-muted-foreground w-10 text-center text-[9px] font-semibold">
				{Math.round(zoom * 100)}%
			</span>
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7"
				onclick={zoomIn}
				title="Zoom in"
				aria-label="Zoom in"
			>
				<Plus class="h-3.5 w-3.5" />
			</button>
			<button
				type="button"
				class="rt-toolbar-button h-7 w-7"
				onclick={fitDiagram}
				title="Fit diagram"
				aria-label="Fit diagram"
			>
				<Maximize2 class="h-3.5 w-3.5" />
			</button>
			<div class="mx-1 h-4 border-l"></div>
			<button
				type="button"
				class="rt-toolbar-button h-7 gap-1.5 px-2 text-[9px] font-semibold"
				onclick={loadDiagram}
				disabled={loading}
			>
				<RefreshCw class="h-3 w-3 {loading ? 'animate-spin' : ''}" />
				Refresh
			</button>
		</div>
	</div>

	<div bind:this={viewport} class="rt-diagram-viewport relative min-h-0 flex-1 overflow-auto">
		{#if loading}
			<div
				class="bg-background/80 absolute inset-0 z-30 flex items-center justify-center backdrop-blur-sm"
			>
				<div class="rt-form-card w-[340px] rounded-xl p-5">
					<div class="flex items-center gap-3">
						<span
							class="bg-primary/10 text-primary flex h-10 w-10 items-center justify-center rounded-lg"
						>
							<Loader2 class="h-5 w-5 animate-spin" />
						</span>
						<div class="min-w-0 flex-1">
							<h3 class="text-[11px] font-bold">Building schema diagram</h3>
							<p class="text-muted-foreground mt-1 truncate text-[9px]">{loadStage}</p>
						</div>
					</div>
					<div class="mt-4">
						<div class="flex items-center justify-between text-[8px] font-semibold">
							<span class="text-muted-foreground">Metadata loaded</span>
							<span>{loadedCount} / {totalCount || '-'}</span>
						</div>
						<div class="bg-muted mt-2 h-1.5 overflow-hidden rounded-full">
							<div
								class="bg-primary h-full rounded-full transition-all duration-300"
								style="width: {totalCount ? Math.max(8, (loadedCount / totalCount) * 100) : 8}%"
							></div>
						</div>
					</div>
				</div>
			</div>
		{:else if errorMessage}
			<div class="absolute inset-0 flex items-center justify-center p-6">
				<div
					class="max-w-sm rounded-xl border bg-[var(--surface-raised)] p-5 text-center shadow-sm"
				>
					<AlertCircle class="text-destructive mx-auto h-6 w-6" />
					<h3 class="mt-3 text-xs font-bold">Diagram could not be loaded</h3>
					<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">{errorMessage}</p>
					<button
						type="button"
						class="rt-primary-button mt-4 h-8 rounded-md px-3 text-[10px] font-bold"
						onclick={loadDiagram}
					>
						Try again
					</button>
				</div>
			</div>
		{:else if tables.length === 0}
			<div class="absolute inset-0 flex items-center justify-center">
				<div class="text-muted-foreground text-center">
					<Database class="mx-auto h-7 w-7 opacity-50" />
					<p class="mt-2 text-[11px] font-bold">No tables in {schema}</p>
					<p class="mt-1 text-[9px]">Create a table to start mapping this schema.</p>
				</div>
			</div>
		{:else}
			<div class="relative" style="width: {canvasWidth * zoom}px; height: {canvasHeight * zoom}px;">
				<div
					class="absolute top-0 left-0 origin-top-left"
					style="width: {canvasWidth}px; height: {canvasHeight}px; transform: scale({zoom});"
				>
					<svg
						class="pointer-events-none absolute inset-0 z-0 overflow-visible"
						width={canvasWidth}
						height={canvasHeight}
						aria-hidden="true"
					>
						<defs>
							<marker
								id="relation-arrow"
								markerWidth="8"
								markerHeight="8"
								refX="7"
								refY="4"
								orient="auto"
								markerUnits="strokeWidth"
							>
								<path d="M 0 0 L 8 4 L 0 8 z" fill="var(--primary)" />
							</marker>
						</defs>
						{#each relations as relation (relation.id)}
							<path
								d={relation.path}
								fill="none"
								stroke="color-mix(in oklab, var(--primary) 72%, var(--border))"
								stroke-width="1.5"
								marker-end="url(#relation-arrow)"
							/>
							<circle cx={relation.startX} cy={relation.startY} r="3" fill="var(--primary)" />
						{/each}
					</svg>

					{#each positionedTables as table (table.name)}
						<article
							class="absolute z-10 overflow-hidden rounded-lg border bg-[var(--surface-raised)] shadow-md"
							style="left: {table.x}px; top: {table.y}px; width: {nodeWidth}px; height: {table.height}px;"
						>
							<button
								type="button"
								class="flex h-14 w-full items-center gap-2.5 border-b bg-[var(--surface-sunken)] px-3 text-left hover:bg-[var(--surface-hover)]"
								onclick={() => openTable(table.name)}
								title="Open {schema}.{table.name}"
							>
								<span
									class="bg-primary/10 text-primary flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
								>
									<Table2 class="h-3.5 w-3.5" />
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-[10px] font-bold">{table.name}</span>
									<span class="text-muted-foreground mt-0.5 block text-[8px]"
										>{table.columns.length} columns</span
									>
								</span>
								<Columns3 class="text-muted-foreground h-3.5 w-3.5" />
							</button>

							<div>
								{#each table.visibleColumns as column (column.name)}
									<div class="flex h-[25px] items-center gap-2 border-b px-3 last:border-b-0">
										<span class="flex h-4 w-4 shrink-0 items-center justify-center">
											{#if column.is_primary}
												<KeyRound class="text-foreground h-3 w-3" />
											{:else if getForeignRelation(column, schema)}
												<Link2 class="text-primary h-3 w-3" />
											{:else}
												<span class="bg-muted-foreground/35 h-1.5 w-1.5 rounded-full"></span>
											{/if}
										</span>
										<span class="min-w-0 flex-1 truncate font-mono text-[8px] font-semibold">
											{column.name}
										</span>
										<span class="text-muted-foreground max-w-20 truncate font-mono text-[7px]">
											{getColumnTypeLabel(column)}
										</span>
									</div>
								{/each}
								{#if table.hiddenColumnCount > 0}
									<div
										class="text-muted-foreground flex h-[27px] items-center justify-center bg-[var(--surface-sunken)] text-[8px] font-semibold"
									>
										+{table.hiddenColumnCount} more columns
									</div>
								{/if}
							</div>
						</article>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>
