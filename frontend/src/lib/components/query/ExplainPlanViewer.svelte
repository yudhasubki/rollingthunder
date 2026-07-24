<script lang="ts">
	import { Braces, ChevronDown, Database, Gauge, Rows3 } from 'lucide-svelte';
	import { database } from '$lib/wailsjs/go/models';

	interface Props {
		plan: database.ExplainPlan;
	}

	let { plan }: Props = $props();
	let view = $state<'tree' | 'raw'>('tree');
	let expanded = $state<Record<string, boolean>>({});

	const flatNodes = $derived.by(() => {
		const result: database.ExplainPlanNode[] = [];
		const visit = (node: database.ExplainPlanNode) => {
			result.push(node);
			for (const child of node.children || []) visit(child);
		};
		for (const root of plan.roots || []) visit(root);
		return result;
	});
	const maxCost = $derived(Math.max(0, ...flatNodes.map((node) => node.totalCost || 0)));

	function isExpanded(id: string): boolean {
		return expanded[id] !== false;
	}

	function toggle(id: string): void {
		expanded[id] = !isExpanded(id);
	}

	function formatMetric(value: number): string {
		if (!Number.isFinite(value)) return '0';
		if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`;
		if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
		return value < 10 && value % 1 ? value.toFixed(2) : value.toLocaleString();
	}
</script>

{#snippet renderNode(node: database.ExplainPlanNode, depth: number)}
	<div
		class="relative"
		role="treeitem"
		aria-selected="false"
		aria-expanded={node.children?.length ? isExpanded(node.id) : undefined}
	>
		<div
			class="group relative grid min-h-14 grid-cols-[minmax(180px,1fr)_88px_88px] items-center gap-3 border-b px-3 py-2 hover:bg-[var(--surface-hover)]"
			style={`padding-left: ${12 + depth * 24}px`}
		>
			{#if depth > 0}
				<span
					class="border-border absolute top-0 bottom-0 border-l"
					style={`left: ${14 + (depth - 1) * 24}px`}
				></span>
			{/if}
			<div class="flex min-w-0 items-start gap-2">
				{#if node.children?.length}
					<button
						type="button"
						class="rt-toolbar-button mt-0.5 h-5 w-5 shrink-0 cursor-pointer"
						onclick={() => toggle(node.id)}
						aria-label="{isExpanded(node.id) ? 'Collapse' : 'Expand'} {node.summary}"
					>
						<ChevronDown
							class="h-3 w-3 transition-transform {isExpanded(node.id) ? '' : '-rotate-90'}"
						/>
					</button>
				{:else}
					<span class="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--border-strong)]"></span>
				{/if}
				<div class="min-w-0">
					<div class="flex min-w-0 items-center gap-2">
						<span class="truncate text-[10px] font-bold">{node.summary}</span>
						<span
							class="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[8px] font-semibold uppercase"
						>
							{node.nodeType}
						</span>
					</div>
					{#if node.details && Object.keys(node.details).length > 0}
						<div class="text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[8px]">
							{#each Object.entries(node.details).slice(0, 3) as [name, value]}
								<span class="max-w-64 truncate" title={`${name}: ${value}`}>
									<span class="font-semibold">{name}</span> · {value}
								</span>
							{/each}
						</div>
					{/if}
					{#if maxCost > 0 && node.totalCost > 0}
						<div class="bg-muted mt-1.5 h-1 max-w-64 overflow-hidden rounded-full">
							<div
								class="bg-primary h-full min-w-px rounded-full"
								style={`width: ${Math.max(1, (node.totalCost / maxCost) * 100)}%`}
							></div>
						</div>
					{/if}
				</div>
			</div>
			<div class="text-right">
				<div class="flex items-center justify-end gap-1 text-[9px] font-semibold tabular-nums">
					<Gauge class="text-muted-foreground h-3 w-3" />
					{formatMetric(node.totalCost)}
				</div>
				<div class="text-muted-foreground mt-0.5 text-[8px]">estimated cost</div>
			</div>
			<div class="text-right">
				<div class="flex items-center justify-end gap-1 text-[9px] font-semibold tabular-nums">
					<Rows3 class="text-muted-foreground h-3 w-3" />
					{formatMetric(node.estimatedRows)}
				</div>
				<div class="text-muted-foreground mt-0.5 text-[8px]">estimated rows</div>
			</div>
		</div>
		{#if isExpanded(node.id)}
			{#each node.children || [] as child (child.id)}
				{@render renderNode(child, depth + 1)}
			{/each}
		{/if}
	</div>
{/snippet}

<section
	class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border bg-[var(--surface-raised)]"
>
	<header class="flex h-10 shrink-0 items-center justify-between border-b px-3">
		<div class="flex min-w-0 items-center gap-2">
			<span class="bg-primary/10 text-primary flex h-6 w-6 items-center justify-center rounded-md">
				<Database class="h-3.5 w-3.5" />
			</span>
			<div class="min-w-0">
				<div class="truncate text-[10px] font-bold">{plan.summary}</div>
				<div class="text-muted-foreground text-[8px]">
					{plan.engine} · {flatNodes.length} plan {flatNodes.length === 1 ? 'node' : 'nodes'} · estimates
					only
				</div>
			</div>
		</div>
		<div class="flex rounded-md border bg-[var(--surface-sunken)] p-0.5">
			<button
				type="button"
				class="h-6 cursor-pointer rounded px-2 text-[8px] font-semibold {view === 'tree'
					? 'bg-[var(--surface-raised)] shadow-sm'
					: 'text-muted-foreground'}"
				onclick={() => (view = 'tree')}
			>
				Tree
			</button>
			<button
				type="button"
				class="flex h-6 cursor-pointer items-center gap-1 rounded px-2 text-[8px] font-semibold {view ===
				'raw'
					? 'bg-[var(--surface-raised)] shadow-sm'
					: 'text-muted-foreground'}"
				onclick={() => (view = 'raw')}
			>
				<Braces class="h-3 w-3" />
				Raw
			</button>
		</div>
	</header>

	{#if view === 'raw'}
		<pre
			class="rt-code-surface min-h-0 flex-1 overflow-auto p-4 font-mono text-[9px] leading-relaxed"><code
				>{plan.raw}</code
			></pre>
	{:else}
		<div class="min-h-0 flex-1 overflow-auto" role="tree" aria-label="Query explain plan">
			<div
				class="text-muted-foreground sticky top-0 z-10 grid h-7 grid-cols-[minmax(180px,1fr)_88px_88px] items-center gap-3 border-b bg-[var(--surface-sunken)] px-3 text-[8px] font-bold tracking-[0.08em] uppercase"
			>
				<span>Operation</span>
				<span class="text-right">Cost</span>
				<span class="text-right">Rows</span>
			</div>
			{#each plan.roots || [] as root (root.id)}
				{@render renderNode(root, 0)}
			{/each}
		</div>
	{/if}
</section>
