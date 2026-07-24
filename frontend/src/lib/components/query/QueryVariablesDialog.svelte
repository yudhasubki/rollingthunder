<script lang="ts">
	import { Braces, Loader2, Play, X } from 'lucide-svelte';
	import {
		coerceQueryVariable,
		type QueryVariableInput,
		type QueryVariableType
	} from '$lib/query/variables';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	interface Props {
		open: boolean;
		names: string[];
		busy?: boolean;
		actionLabel?: string;
		onClose: () => void;
		onSubmit: (variables: QueryVariableInput[]) => void | Promise<void>;
	}

	let { open, names, busy = false, actionLabel = 'Run query', onClose, onSubmit }: Props = $props();

	let values = $state<Record<string, QueryVariableInput>>({});
	let error = $state('');
	let initializedKey = '';
	const typeOptions = [
		{ value: 'text', label: 'Text' },
		{ value: 'number', label: 'Number' },
		{ value: 'boolean', label: 'Boolean' },
		{ value: 'date', label: 'Date / time' },
		{ value: 'null', label: 'NULL' }
	];
	const booleanOptions = [
		{ value: 'true', label: 'True' },
		{ value: 'false', label: 'False' }
	];

	$effect(() => {
		if (!open) {
			initializedKey = '';
			return;
		}
		const key = names.join(':');
		if (key === initializedKey) return;
		initializedKey = key;
		values = Object.fromEntries(
			names.map((name) => [
				name,
				values[name] || { name, value: '', type: 'text' as QueryVariableType }
			])
		);
		error = '';
	});

	function updateType(name: string, type: QueryVariableType): void {
		const current = values[name];
		values[name] = {
			...current,
			type,
			value: type === 'boolean' ? false : type === 'null' ? null : ''
		};
	}

	function variableToken(name: string): string {
		return `{{${name}}}`;
	}

	async function submit(): Promise<void> {
		error = '';
		try {
			const variables = names.map((name) => coerceQueryVariable(values[name]));
			await onSubmit(variables);
		} catch (submitError: any) {
			error = submitError?.message || 'Check the query variable values.';
		}
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[120] flex items-center justify-center p-6">
		<button
			type="button"
			class="absolute inset-0 cursor-default bg-black/45 backdrop-blur-[1px]"
			onclick={() => !busy && onClose()}
			aria-label="Close query variables"
		></button>
		<div
			class="rt-popover relative flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="query-variables-title"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
				>
					<Braces class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="query-variables-title" class="text-[13px] font-bold">Query variables</h2>
					<p class="text-muted-foreground mt-0.5 text-[9px]">
						Values are bound as driver parameters and never interpolated into SQL.
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					disabled={busy}
					aria-label="Close query variables"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="min-h-0 space-y-3 overflow-y-auto p-4">
				{#each names as name (name)}
					<div class="grid grid-cols-[minmax(120px,0.8fr)_120px_minmax(150px,1fr)] gap-2">
						<div class="min-w-0">
							<span class="text-muted-foreground mb-1 block text-[8px] font-semibold">Variable</span
							>
							<div class="rt-input flex h-8 items-center px-2.5 font-mono text-[9px]">
								{variableToken(name)}
							</div>
						</div>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px] font-semibold">Type</span>
							<FilterCombobox
								id={`query-variable-type-${name}`}
								options={typeOptions}
								value={values[name]?.type || 'text'}
								onChange={(value) => updateType(name, value as QueryVariableType)}
								searchable={false}
								triggerClass="h-8 px-2 text-[9px]"
								disabled={busy}
							/>
						</label>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px] font-semibold">Value</span>
							{#if values[name]?.type === 'boolean'}
								<FilterCombobox
									id={`query-variable-value-${name}`}
									options={booleanOptions}
									value={String(values[name]?.value ?? false)}
									onChange={(value) =>
										(values[name] = { ...values[name], value: value === 'true' })}
									searchable={false}
									triggerClass="h-8 px-2 text-[9px]"
									disabled={busy}
								/>
							{:else if values[name]?.type === 'null'}
								<div
									class="rt-input text-muted-foreground flex h-8 items-center px-2.5 font-mono text-[9px]"
								>
									NULL
								</div>
							{:else}
								<input
									class="rt-input h-8 w-full px-2.5 font-mono text-[9px]"
									type={values[name]?.type === 'number'
										? 'number'
										: values[name]?.type === 'date'
											? 'datetime-local'
											: 'text'}
									value={String(values[name]?.value ?? '')}
									oninput={(event) =>
										(values[name] = { ...values[name], value: event.currentTarget.value })}
									disabled={busy}
									autocomplete="off"
								/>
							{/if}
						</label>
					</div>
				{/each}
				{#if error}
					<div
						class="rounded-lg border border-red-500/25 bg-red-500/10 px-3 py-2 text-[9px] text-red-600 dark:text-red-400"
					>
						{error}
					</div>
				{/if}
			</div>

			<footer
				class="flex items-center justify-between border-t bg-[var(--surface-sunken)] px-4 py-3"
			>
				<span class="text-muted-foreground text-[8px]">
					Values are kept only for this open query tab.
				</span>
				<div class="flex gap-2">
					<button
						type="button"
						class="rt-toolbar-button h-8 cursor-pointer px-3 text-[10px] font-semibold"
						onclick={onClose}
						disabled={busy}
					>
						Cancel
					</button>
					<button
						type="button"
						class="rt-primary-button inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[10px] font-bold"
						onclick={submit}
						disabled={busy}
					>
						{#if busy}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Play
								class="h-3.5 w-3.5"
							/>{/if}
						{actionLabel}
					</button>
				</div>
			</footer>
		</div>
	</div>
{/if}
