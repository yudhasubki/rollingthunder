<script lang="ts">
	import { BarChart3, LineChart, ScatterChart, TriangleAlert } from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	interface Props {
		data: Record<string, any>[];
		columns: database.Structure[];
	}

	interface ChartPoint {
		label: string;
		value: number;
	}

	let { data, columns }: Props = $props();

	const chartTypeOptions = [
		{ value: 'bar', label: 'Bar' },
		{ value: 'line', label: 'Line' },
		{ value: 'scatter', label: 'Scatter' }
	];
	const aggregationOptions = [
		{ value: 'none', label: 'Raw rows' },
		{ value: 'sum', label: 'Sum by category' },
		{ value: 'average', label: 'Average by category' },
		{ value: 'count', label: 'Count by category' }
	];
	const columnNames = $derived(columns.map((column) => column.name));
	const columnOptions = $derived(columnNames.map((name) => ({ value: name, label: name })));
	const numericColumns = $derived(
		columnNames.filter((name) =>
			data.some(
				(row) => row[name] !== null && row[name] !== '' && Number.isFinite(Number(row[name]))
			)
		)
	);
	const metricOptions = $derived(numericColumns.map((name) => ({ value: name, label: name })));

	let chartType = $state('bar');
	let categoryColumn = $state('');
	let metricColumn = $state('');
	let aggregation = $state('none');

	$effect(() => {
		if (!columnNames.includes(categoryColumn)) {
			categoryColumn =
				columnNames.find((name) => !numericColumns.includes(name)) ?? columnNames[0] ?? '';
		}
		if (!numericColumns.includes(metricColumn)) {
			metricColumn = numericColumns[0] ?? '';
		}
	});

	const completeSeries = $derived.by<ChartPoint[]>(() => {
		if (!categoryColumn || (aggregation !== 'count' && !metricColumn)) return [];
		if (aggregation === 'none') {
			return data
				.map((row, index) => ({
					label: String(row[categoryColumn] ?? `Row ${index + 1}`),
					value: Number(row[metricColumn])
				}))
				.filter((point) => Number.isFinite(point.value));
		}
		const groups = new Map<string, number[]>();
		for (let index = 0; index < data.length; index++) {
			const row = data[index];
			const label = String(row[categoryColumn] ?? 'NULL');
			const values = groups.get(label) ?? [];
			if (aggregation === 'count') {
				values.push(1);
			} else {
				const value = Number(row[metricColumn]);
				if (Number.isFinite(value)) values.push(value);
			}
			groups.set(label, values);
		}
		return [...groups.entries()]
			.filter(([, values]) => values.length > 0)
			.map(([label, values]) => ({
				label,
				value:
					aggregation === 'average'
						? values.reduce((total, value) => total + value, 0) / values.length
						: values.reduce((total, value) => total + value, 0)
			}));
	});
	const series = $derived(completeSeries.slice(0, 60));
	const truncated = $derived(completeSeries.length > series.length);
	const values = $derived(series.map((point) => point.value));
	const minimum = $derived(values.length ? Math.min(...values) : 0);
	const maximum = $derived(values.length ? Math.max(...values) : 0);
	const sum = $derived(values.reduce((total, value) => total + value, 0));
	const average = $derived(values.length ? sum / values.length : 0);
	const chartMinimum = $derived(Math.min(0, minimum));
	const chartMaximum = $derived(Math.max(0, maximum));
	const chartRange = $derived(chartMaximum - chartMinimum || 1);
	const plot = { left: 64, top: 24, width: 696, height: 236 };
	const barGap = $derived(Math.max(2, Math.min(10, plot.width / Math.max(1, series.length) / 4)));
	const barWidth = $derived(Math.max(2, plot.width / Math.max(1, series.length) - barGap));
	const labelStep = $derived(Math.max(1, Math.ceil(series.length / 10)));
	const zeroY = $derived(plot.top + plot.height - ((0 - chartMinimum) / chartRange) * plot.height);
	const linePoints = $derived(
		series.map((point, index) => `${pointX(index)},${pointY(point.value)}`).join(' ')
	);

	function pointX(index: number): number {
		return plot.left + ((index + 0.5) / Math.max(1, series.length)) * plot.width;
	}

	function pointY(value: number): number {
		return plot.top + plot.height - ((value - chartMinimum) / chartRange) * plot.height;
	}

	function formatNumber(value: number): string {
		return new Intl.NumberFormat(undefined, {
			maximumFractionDigits: Math.abs(value) < 10 ? 2 : 0
		}).format(value);
	}
</script>

<div
	class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border bg-[var(--surface-raised)]"
>
	<div class="grid shrink-0 grid-cols-4 gap-3 border-b bg-[var(--surface-sunken)] p-3">
		<div>
			<label for="result-chart-type">Chart</label>
			<FilterCombobox
				id="result-chart-type"
				options={chartTypeOptions}
				value={chartType}
				onChange={(value) => (chartType = value)}
				searchable={false}
				triggerClass="h-8 px-2.5 text-[9px]"
			/>
		</div>
		<div>
			<label for="result-chart-category">Category / X</label>
			<FilterCombobox
				id="result-chart-category"
				options={columnOptions}
				value={categoryColumn}
				onChange={(value) => (categoryColumn = value)}
				triggerClass="h-8 px-2.5 text-[9px]"
			/>
		</div>
		<div>
			<label for="result-chart-metric">Metric / Y</label>
			<FilterCombobox
				id="result-chart-metric"
				options={metricOptions}
				value={metricColumn}
				onChange={(value) => (metricColumn = value)}
				disabled={aggregation === 'count'}
				triggerClass="h-8 px-2.5 text-[9px]"
				placeholder="Choose numeric column"
			/>
		</div>
		<div>
			<label for="result-chart-aggregation">Aggregation</label>
			<FilterCombobox
				id="result-chart-aggregation"
				options={aggregationOptions}
				value={aggregation}
				onChange={(value) => (aggregation = value)}
				searchable={false}
				triggerClass="h-8 px-2.5 text-[9px]"
			/>
		</div>
	</div>

	{#if numericColumns.length === 0}
		<div class="flex min-h-48 flex-1 items-center justify-center p-6">
			<div class="max-w-sm text-center">
				<TriangleAlert class="text-muted-foreground mx-auto h-5 w-5" />
				<p class="mt-2 text-[10px] font-bold">No numeric result column</p>
				<p class="text-muted-foreground mt-1 text-[8px]">
					Select or calculate a numeric metric in the SQL query to chart these results.
				</p>
			</div>
		</div>
	{:else if series.length === 0}
		<div class="text-muted-foreground flex min-h-48 flex-1 items-center justify-center text-[9px]">
			The selected columns contain no chartable values.
		</div>
	{:else}
		<div class="min-h-0 flex-1 overflow-auto p-3">
			<div class="min-w-[720px]">
				<div class="mb-3 grid grid-cols-4 gap-2">
					{#each [['Points', series.length], ['Minimum', formatNumber(minimum)], ['Maximum', formatNumber(maximum)], ['Average', formatNumber(average)]] as statistic}
						<div class="rounded-md border bg-[var(--surface-sunken)] px-2.5 py-2">
							<p class="text-muted-foreground text-[7px] font-bold tracking-wide uppercase">
								{statistic[0]}
							</p>
							<p class="mt-1 font-mono text-[10px] font-bold">{statistic[1]}</p>
						</div>
					{/each}
				</div>
				<svg
					viewBox="0 0 800 300"
					class="h-auto w-full rounded-lg border bg-[var(--background)]"
					role="img"
					aria-label={`${chartType} chart of ${metricColumn || 'row count'} by ${categoryColumn}`}
				>
					<line
						x1={plot.left}
						x2={plot.left + plot.width}
						y1={zeroY}
						y2={zeroY}
						stroke="var(--border)"
						stroke-width="1"
					/>
					<line
						x1={plot.left}
						x2={plot.left}
						y1={plot.top}
						y2={plot.top + plot.height}
						stroke="var(--border)"
						stroke-width="1"
					/>
					<text
						x={plot.left - 8}
						y={plot.top + 4}
						text-anchor="end"
						class="fill-[var(--muted-foreground)] text-[9px]">{formatNumber(chartMaximum)}</text
					>
					<text
						x={plot.left - 8}
						y={plot.top + plot.height}
						text-anchor="end"
						class="fill-[var(--muted-foreground)] text-[9px]">{formatNumber(chartMinimum)}</text
					>

					{#if chartType === 'bar'}
						{#each series as point, index}
							{@const y = pointY(point.value)}
							<rect
								x={pointX(index) - barWidth / 2}
								y={Math.min(y, zeroY)}
								width={barWidth}
								height={Math.max(1, Math.abs(zeroY - y))}
								rx="2"
								fill="color-mix(in oklab, var(--primary) 76%, transparent)"
							>
								<title>{point.label}: {formatNumber(point.value)}</title>
							</rect>
						{/each}
					{:else if chartType === 'line'}
						<polyline
							points={linePoints}
							fill="none"
							stroke="var(--primary)"
							stroke-width="2"
							stroke-linejoin="round"
						/>
						{#each series as point, index}
							<circle cx={pointX(index)} cy={pointY(point.value)} r="3" fill="var(--primary)">
								<title>{point.label}: {formatNumber(point.value)}</title>
							</circle>
						{/each}
					{:else}
						{#each series as point, index}
							<circle cx={pointX(index)} cy={pointY(point.value)} r="4" fill="var(--primary)">
								<title>{point.label}: {formatNumber(point.value)}</title>
							</circle>
						{/each}
					{/if}

					{#each series as point, index}
						{#if index % labelStep === 0}
							<text
								x={pointX(index)}
								y={plot.top + plot.height + 18}
								text-anchor="middle"
								class="fill-[var(--muted-foreground)] text-[8px]"
							>
								{point.label.length > 12 ? `${point.label.slice(0, 11)}…` : point.label}
							</text>
						{/if}
					{/each}
				</svg>
				{#if truncated}
					<p class="text-muted-foreground mt-2 flex items-center gap-1.5 text-[8px]">
						<TriangleAlert class="h-3 w-3" />
						Chart preview is limited to the first 60 points. Aggregate the result for a clearer view.
					</p>
				{/if}
			</div>
		</div>
	{/if}
</div>
