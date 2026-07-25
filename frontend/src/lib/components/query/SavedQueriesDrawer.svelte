<script lang="ts">
	import { Bookmark, Check, Search, Trash2, X } from 'lucide-svelte';
	import {
		deleteSavedQuery,
		getSavedQueries,
		saveNamedQuery
	} from '$lib/stores/savedQueries.svelte';
	import type { SavedQuery } from '$lib/query/snippets';
	import { focusTrap } from '$lib/actions/focusTrap';

	interface Props {
		open: boolean;
		currentSql: string;
		engine: string;
		savedQueryId?: string;
		onClose: () => void;
		onLoad: (query: SavedQuery) => void;
		onSaved: (query: SavedQuery) => void;
	}

	let { open, currentSql, engine, savedQueryId, onClose, onLoad, onSaved }: Props = $props();

	let queries = $state<SavedQuery[]>([]);
	let search = $state('');
	let name = $state('');
	let tags = $state('');
	let message = $state('');
	let initialized = false;
	const filtered = $derived(
		search.trim()
			? queries.filter((query) =>
					`${query.name} ${query.tags.join(' ')} ${query.query}`
						.toLowerCase()
						.includes(search.trim().toLowerCase())
				)
			: queries
	);

	$effect(() => {
		if (open && !initialized) {
			initialized = true;
			queries = [...getSavedQueries()];
			const current = queries.find((query) => query.id === savedQueryId);
			name = current?.name || '';
			tags = current?.tags.join(', ') || '';
			message = '';
		} else if (!open) {
			initialized = false;
		}
	});

	function save(): void {
		if (!currentSql.trim()) {
			message = 'Enter SQL before saving this query.';
			return;
		}
		const saved = saveNamedQuery({
			id: savedQueryId,
			name: name.trim() || 'Untitled query',
			query: currentSql,
			engine,
			tags: tags
				.split(',')
				.map((tag) => tag.trim())
				.filter(Boolean)
		});
		queries = [...getSavedQueries()];
		name = saved.name;
		tags = saved.tags.join(', ');
		message = 'Saved locally on this device.';
		onSaved(saved);
	}

	function remove(id: string): void {
		deleteSavedQuery(id);
		queries = [...getSavedQueries()];
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[110] flex justify-end overflow-hidden">
		<button
			type="button"
			class="bg-overlay/20 absolute inset-0 cursor-default"
			onclick={onClose}
			aria-label="Close saved queries"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative flex h-full w-[360px] max-w-[90%] flex-col border-l"
			role="dialog"
			aria-modal="true"
			aria-labelledby="saved-queries-title"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg"
				>
					<Bookmark class="h-4 w-4" />
				</span>
				<div class="min-w-0 flex-1">
					<h2 id="saved-queries-title" class="text-[12px] font-bold">Saved queries</h2>
					<p class="text-muted-foreground mt-0.5 text-[8px]">
						Named SQL snippets and reusable work
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={onClose}
					aria-label="Close saved queries"
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="space-y-2 border-b p-3">
				<label>
					<span class="text-muted-foreground mb-1 block text-[8px] font-semibold">Name</span>
					<input
						class="rt-input h-8 w-full px-2.5 text-[9px]"
						placeholder="Monthly active customers"
						bind:value={name}
					/>
				</label>
				<label>
					<span class="text-muted-foreground mb-1 block text-[8px] font-semibold">
						Tags <span class="font-normal">· comma separated</span>
					</span>
					<input
						class="rt-input h-8 w-full px-2.5 text-[9px]"
						placeholder="reporting, monthly"
						bind:value={tags}
					/>
				</label>
				<button
					type="button"
					class="rt-primary-button flex h-8 w-full cursor-pointer items-center justify-center gap-1.5 rounded-md text-[9px] font-bold"
					onclick={save}
				>
					<Check class="h-3.5 w-3.5" />
					{savedQueryId ? 'Update named query' : 'Save as named query'}
				</button>
				{#if message}
					<p class="text-muted-foreground text-[8px]">{message}</p>
				{/if}
			</div>

			<div class="relative border-b p-2">
				<Search
					class="text-muted-foreground pointer-events-none absolute top-1/2 left-4 h-3.5 w-3.5 -translate-y-1/2"
				/>
				<input
					type="search"
					class="rt-input h-8 w-full pr-2 pl-8 text-[9px]"
					placeholder="Filter named queries"
					bind:value={search}
				/>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto p-2">
				{#if filtered.length === 0}
					<div class="text-muted-foreground py-10 text-center text-[9px]">
						{search ? 'No matching named queries' : 'No named queries yet'}
					</div>
				{:else}
					<div class="space-y-1">
						{#each filtered as query (query.id)}
							<div
								class="group rounded-lg border p-2.5 transition-colors hover:bg-[var(--surface-hover)]"
							>
								<div class="flex items-start gap-2">
									<button
										type="button"
										class="min-w-0 flex-1 cursor-pointer text-left"
										onclick={() => onLoad(query)}
									>
										<div class="truncate text-[9px] font-bold">{query.name}</div>
										<code
											class="text-muted-foreground mt-1 line-clamp-2 block text-[8px] leading-relaxed"
										>
											{query.query}
										</code>
									</button>
									<button
										type="button"
										class="text-muted-foreground hover:text-destructive h-6 w-6 shrink-0 cursor-pointer rounded opacity-0 group-hover:opacity-100"
										onclick={() => remove(query.id)}
										aria-label="Delete {query.name}"
									>
										<Trash2 class="mx-auto h-3 w-3" />
									</button>
								</div>
								<div class="mt-2 flex flex-wrap items-center gap-1">
									<span class="bg-muted rounded px-1.5 py-0.5 text-[7px] font-semibold uppercase">
										{query.engine}
									</span>
									{#each query.tags as tag}
										<span class="bg-muted rounded px-1.5 py-0.5 text-[7px]">{tag}</span>
									{/each}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
