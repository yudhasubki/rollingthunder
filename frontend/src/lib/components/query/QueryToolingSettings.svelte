<script lang="ts">
	import { CheckCircle2, Settings2, TriangleAlert, WandSparkles, X } from 'lucide-svelte';
	import { queryToolingStore } from '$lib/stores/queryTooling.svelte';
	import type { SqlLintIssue } from '$lib/sql/tooling';
	import { focusTrap } from '$lib/actions/focusTrap';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	interface Props {
		open: boolean;
		issues?: SqlLintIssue[];
		onClose: () => void;
		onChanged?: () => void;
		onFormat?: () => void;
		onSelectIssue?: (issue: SqlLintIssue) => void;
	}

	let {
		open,
		issues = [],
		onClose,
		onChanged = () => {},
		onFormat = () => {},
		onSelectIssue = () => {}
	}: Props = $props();

	const keywordCaseOptions = [
		{ value: 'upper', label: 'UPPERCASE' },
		{ value: 'lower', label: 'lowercase' },
		{ value: 'preserve', label: 'Preserve' }
	];
	const indentSizeOptions = [
		{ value: '2', label: '2 spaces' },
		{ value: '4', label: '4 spaces' }
	];

	function updateLint(patch: Parameters<typeof queryToolingStore.updateLint>[0]): void {
		queryToolingStore.updateLint(patch);
		onChanged();
	}

	function formatCurrentSql(): void {
		onFormat();
		onClose();
	}

	function openIssue(issue: SqlLintIssue): void {
		onSelectIssue(issue);
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[115] flex items-center justify-center p-6">
		<button
			type="button"
			class="bg-overlay/35 absolute inset-0 cursor-default"
			onclick={onClose}
			aria-label="Close SQL tooling settings"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex max-h-[min(720px,calc(100vh-48px))] w-full max-w-lg flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="query-tooling-title"
		>
			<header class="flex h-14 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg"
				>
					<Settings2 class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="query-tooling-title" class="text-[12px] font-bold">Format & lint</h2>
					<p class="text-muted-foreground mt-0.5 text-[8px]">
						Manual formatting preferences and automatic editor checks
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					aria-label="Close SQL tooling settings"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
				<section>
					<div class="flex items-start justify-between gap-3">
						<div>
							<h3 class="text-[9px] font-bold">Formatter</h3>
							<p class="text-muted-foreground mt-0.5 text-[8px]">
								Runs only when you choose Format or press Shift+Alt+F.
							</p>
						</div>
						<span
							class="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[7px] font-semibold"
							>On demand</span
						>
					</div>
					<div class="mt-2 grid grid-cols-2 gap-3">
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Keyword case</span>
							<FilterCombobox
								options={keywordCaseOptions}
								value={queryToolingStore.format.keywordCase}
								onChange={(value) => {
									queryToolingStore.updateFormat({
										keywordCase: value as 'upper' | 'lower' | 'preserve'
									});
								}}
								searchable={false}
								triggerClass="h-8 px-2 text-[9px]"
								id="query-tooling-keyword-case"
							/>
						</label>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Indent</span>
							<FilterCombobox
								options={indentSizeOptions}
								value={String(queryToolingStore.format.indentSize)}
								onChange={(value) => {
									queryToolingStore.updateFormat({
										indentSize: Number(value) as 2 | 4
									});
								}}
								searchable={false}
								triggerClass="h-8 px-2 text-[9px]"
								id="query-tooling-indent-size"
							/>
						</label>
					</div>
					<button
						type="button"
						class="rt-toolbar-button mt-3 h-8 cursor-pointer gap-1.5 px-3 text-[9px] font-semibold"
						onclick={formatCurrentSql}
					>
						<WandSparkles class="h-3 w-3" />
						Format current SQL
					</button>
				</section>

				<section class="border-t pt-4">
					<div class="flex items-start justify-between gap-3">
						<div>
							<h3 class="text-[9px] font-bold">Live lint</h3>
							<p class="text-muted-foreground mt-0.5 text-[8px]">
								Runs automatically while you type and underlines issues in the editor.
							</p>
						</div>
						<span
							class="{issues.length
								? 'border-warning-border bg-warning-soft text-warning'
								: 'bg-muted text-muted-foreground'} shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[7px] font-semibold tabular-nums"
						>
							{issues.length ? `${issues.length} active` : 'No issues'}
						</span>
					</div>
					<div class="mt-2 space-y-2">
						<label class="flex cursor-pointer items-start gap-2 rounded-lg border p-2.5">
							<input
								type="checkbox"
								class="accent-primary mt-0.5 h-3.5 w-3.5"
								checked={queryToolingStore.lint.requireWhereForMutations}
								onchange={(event) =>
									updateLint({ requireWhereForMutations: event.currentTarget.checked })}
							/>
							<span>
								<span class="block text-[9px] font-semibold">Require WHERE for mutations</span>
								<span class="text-muted-foreground mt-0.5 block text-[8px]">
									Mark unfiltered UPDATE and DELETE as errors.
								</span>
							</span>
						</label>
						<label class="flex cursor-pointer items-start gap-2 rounded-lg border p-2.5">
							<input
								type="checkbox"
								class="accent-primary mt-0.5 h-3.5 w-3.5"
								checked={queryToolingStore.lint.disallowSelectStar}
								onchange={(event) =>
									updateLint({ disallowSelectStar: event.currentTarget.checked })}
							/>
							<span>
								<span class="block text-[9px] font-semibold">Warn on SELECT *</span>
								<span class="text-muted-foreground mt-0.5 block text-[8px]">
									Prefer an explicit projection in reviewed queries.
								</span>
							</span>
						</label>
						<label class="flex cursor-pointer items-start gap-2 rounded-lg border p-2.5">
							<input
								type="checkbox"
								class="accent-primary mt-0.5 h-3.5 w-3.5"
								checked={queryToolingStore.lint.requireSemicolon}
								onchange={(event) => updateLint({ requireSemicolon: event.currentTarget.checked })}
							/>
							<span class="text-[9px] font-semibold">Require a terminal semicolon</span>
						</label>
					</div>

					<div class="mt-3 rounded-lg border bg-[var(--surface-sunken)]">
						<div class="flex h-8 items-center justify-between border-b px-2.5">
							<span class="text-[8px] font-bold">Current editor</span>
							<span class="text-muted-foreground text-[7px]">Click an issue to reveal it</span>
						</div>
						{#if issues.length}
							<div class="divide-y">
								{#each issues.slice(0, 5) as issue}
									<button
										type="button"
										class="flex w-full cursor-pointer items-start gap-2 px-2.5 py-2 text-left hover:bg-[var(--surface-hover)]"
										onclick={() => openIssue(issue)}
									>
										<TriangleAlert
											class="{issue.severity === 'error'
												? 'text-danger'
												: 'text-warning'} mt-0.5 h-3 w-3 shrink-0"
										/>
										<span class="min-w-0 flex-1">
											<span class="block text-[8px] font-semibold">{issue.message}</span>
											<span class="text-muted-foreground mt-0.5 block text-[7px]">{issue.rule}</span
											>
										</span>
									</button>
								{/each}
								{#if issues.length > 5}
									<div class="text-muted-foreground px-2.5 py-2 text-[7px]">
										+{issues.length - 5} more issues are underlined in the editor
									</div>
								{/if}
							</div>
						{:else}
							<div class="text-muted-foreground flex items-center gap-2 px-2.5 py-3 text-[8px]">
								<CheckCircle2 class="h-3 w-3 shrink-0" />
								No active lint issues for the enabled rules.
							</div>
						{/if}
					</div>
				</section>
			</div>

			<footer class="flex justify-between border-t bg-[var(--surface-sunken)] px-4 py-3">
				<button
					type="button"
					class="rt-toolbar-button h-8 cursor-pointer px-3 text-[9px] font-semibold"
					onclick={() => {
						queryToolingStore.reset();
						onChanged();
					}}
				>
					Reset defaults
				</button>
				<button
					type="button"
					class="rt-primary-button h-8 cursor-pointer rounded-md px-3 text-[9px] font-bold"
					onclick={onClose}
				>
					Done
				</button>
			</footer>
		</div>
	</div>
{/if}
