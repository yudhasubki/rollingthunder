<script lang="ts">
	import { Settings2, X } from 'lucide-svelte';
	import { queryToolingStore } from '$lib/stores/queryTooling.svelte';
	import { focusTrap } from '$lib/actions/focusTrap';

	interface Props {
		open: boolean;
		onClose: () => void;
		onChanged?: () => void;
	}

	let { open, onClose, onChanged = () => {} }: Props = $props();

	function updateLint(patch: Parameters<typeof queryToolingStore.updateLint>[0]): void {
		queryToolingStore.updateLint(patch);
		onChanged();
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
			class="rt-popover relative w-full max-w-md overflow-hidden rounded-xl"
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
					<h2 id="query-tooling-title" class="text-[12px] font-bold">SQL tooling</h2>
					<p class="text-muted-foreground mt-0.5 text-[8px]">Formatting and live lint rules</p>
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

			<div class="space-y-4 p-4">
				<section>
					<h3 class="text-[9px] font-bold">Formatter</h3>
					<div class="mt-2 grid grid-cols-2 gap-3">
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Keyword case</span>
							<select
								class="rt-input h-8 w-full px-2 text-[9px]"
								value={queryToolingStore.format.keywordCase}
								onchange={(event) => {
									queryToolingStore.updateFormat({
										keywordCase: event.currentTarget.value as 'upper' | 'lower' | 'preserve'
									});
									onChanged();
								}}
							>
								<option value="upper">UPPERCASE</option>
								<option value="lower">lowercase</option>
								<option value="preserve">Preserve</option>
							</select>
						</label>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Indent</span>
							<select
								class="rt-input h-8 w-full px-2 text-[9px]"
								value={queryToolingStore.format.indentSize}
								onchange={(event) => {
									queryToolingStore.updateFormat({
										indentSize: Number(event.currentTarget.value) as 2 | 4
									});
									onChanged();
								}}
							>
								<option value="2">2 spaces</option>
								<option value="4">4 spaces</option>
							</select>
						</label>
					</div>
				</section>

				<section class="border-t pt-4">
					<h3 class="text-[9px] font-bold">Live lint rules</h3>
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
