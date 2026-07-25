<script lang="ts">
	import {
		AlertCircle,
		ArrowUpRight,
		Boxes,
		Check,
		Columns3,
		Copy,
		FileCode2,
		GitBranch,
		Loader2,
		RefreshCw,
		Settings2,
		Pencil,
		Power,
		PowerOff,
		Trash2
	} from 'lucide-svelte';
	import type { Tab } from '$lib/models/Tab';
	import { database } from '$lib/wailsjs/go/models';
	import { GetCapabilities, GetDatabaseObject } from '$lib/wailsjs/go/db/Service';
	import { ClipboardSetText } from '$lib/wailsjs/runtime/runtime';
	import { databaseObjectKindLabel, databaseObjectQualifiedName } from '$lib/database/objects';
	import { createServiceError } from '$lib/errors/service';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import { updateStatus } from '$lib/stores/status.svelte';
	import { UI_RUNTIME } from '$lib/config/application';
	import ObjectChangeDialog from '$lib/components/database/ObjectChangeDialog.svelte';
	import type { StructuralChangeIntent } from '$lib/database/changeTemplates';

	interface Props {
		tab: Tab;
	}

	let { tab }: Props = $props();

	type InspectorSection = 'definition' | 'dependencies' | 'properties';

	let detail = $state<database.ObjectDetail | null>(null);
	let capabilities = $state<database.Capabilities | null>(null);
	let loading = $state(false);
	let error = $state('');
	let errorHint = $state('');
	let activeSection = $state<InspectorSection>('definition');
	let copied = $state('');
	let manageMenuOpen = $state(false);
	let changeIntent = $state<StructuralChangeIntent | null>(null);
	let loadGeneration = 0;

	const reference = $derived(
		new database.ObjectReference({
			id: tab.objectId || '',
			kind: tab.objectKind || 'unknown',
			schema: tab.schema || '',
			name: tab.objectName || tab.title,
			signature: tab.objectSignature || '',
			parentSchema: tab.parentSchema || '',
			parentName: tab.parentName || ''
		})
	);
	const qualifiedName = $derived(
		detail
			? databaseObjectQualifiedName(detail.object.reference)
			: databaseObjectQualifiedName(reference)
	);
	const objectKind = $derived(detail?.object.reference.kind || reference.kind);
	const definitionLines = $derived(detail?.definition ? detail.definition.split('\n').length : 0);
	const dependencies = $derived(detail?.dependencies || []);
	const dependents = $derived(detail?.dependents || []);
	const properties = $derived(detail?.properties || []);
	const columns = $derived(detail?.columns || []);
	const triggerEnabled = $derived(
		properties.find((property) => property.name === 'Enabled')?.value !== 'false'
	);
	const canEditDefinition = $derived(
		objectKind === 'view' ||
			objectKind === 'function' ||
			objectKind === 'procedure' ||
			objectKind === 'trigger'
	);
	const allowedActions = $derived(detail?.object.allowedActions || []);

	function canChange(action: string): boolean {
		return (allowedActions as string[]).includes(action);
	}

	$effect(() => {
		const key = [
			tab.connectionId,
			tab.objectId,
			tab.objectKind,
			tab.schema,
			tab.objectName,
			tab.objectSignature,
			tab.revision
		].join(':');
		if (!key) return;
		void loadObject();
	});

	async function loadObject() {
		const generation = ++loadGeneration;
		loading = true;
		error = '';
		errorHint = '';
		try {
			const [response, capabilityResponse] = await Promise.all([
				GetDatabaseObject(tab.connectionId, reference),
				GetCapabilities(tab.connectionId)
			]);
			if (generation !== loadGeneration) return;
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not inspect database object');
			}
			detail = response.data || null;
			if (!capabilityResponse.errors?.length) {
				capabilities = capabilityResponse.data || null;
			}
			if (!detail) {
				throw new Error('The database returned no object details.');
			}
			updateStatus('', 'info');
		} catch (loadError: any) {
			if (generation !== loadGeneration) return;
			detail = null;
			error = loadError?.message || 'Could not inspect database object';
			errorHint = loadError?.hint || '';
			updateStatus(error, 'error');
		} finally {
			if (generation === loadGeneration) loading = false;
		}
	}

	async function copyText(value: string, key: string) {
		if (!value) return;
		try {
			await ClipboardSetText(value);
			copied = key;
			updateStatus('Copied to clipboard', 'success');
			window.setTimeout(() => {
				if (copied === key) copied = '';
			}, UI_RUNTIME.copyFeedbackMs);
		} catch (copyError: any) {
			updateStatus(copyError?.message || 'Could not copy to clipboard', 'error');
		}
	}

	function openDefinitionInQuery() {
		if (!detail?.definition) return;
		tabsStore.newQueryTabWithContent(
			tab.connectionId,
			detail.definition,
			`${detail.object.reference.name} DDL`
		);
		updateStatus(`Opened ${qualifiedName} definition in a query tab`, 'info');
	}

	function openDependency(dependency: database.ObjectDependency) {
		const target = dependency.reference;
		if (!target || target.kind === 'unknown' || (!target.id && !target.name)) return;
		tabsStore.newDatabaseObjectTab(tab.connectionId, target);
	}

	function dependencyTitle(dependency: database.ObjectDependency): string {
		const target = dependency.reference;
		if (target?.name) return databaseObjectQualifiedName(target);
		return dependency.description || 'Database object';
	}

	function openChange(intent: StructuralChangeIntent) {
		manageMenuOpen = false;
		changeIntent = intent;
	}

	function handleStructuralChangeApplied(result: database.ObjectChangeResult) {
		window.dispatchEvent(
			new CustomEvent('database-objects-changed', {
				detail: { connectionId: tab.connectionId, schema: tab.schema }
			})
		);
		const appliedIntent = changeIntent;
		const refreshed = result.refresh?.[0];
		changeIntent = null;
		if (appliedIntent === 'drop') {
			tabsStore.closeTab(tab.id);
			return;
		}
		if (refreshed?.name) {
			const signature = refreshed.signature ? `(${refreshed.signature})` : '';
			tabsStore.updateTab(tab.id, {
				title: `${refreshed.name}${signature}`,
				schema: refreshed.schema || tab.schema,
				objectId: refreshed.id || tab.objectId,
				objectKind: refreshed.kind || tab.objectKind,
				objectName: refreshed.name,
				objectSignature: refreshed.signature || tab.objectSignature,
				parentSchema: refreshed.parentSchema || tab.parentSchema,
				parentName: refreshed.parentName || tab.parentName,
				revision: Date.now()
			});
		} else {
			tabsStore.updateTab(tab.id, { revision: Date.now() });
		}
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
	<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
		<span
			class="bg-primary/10 text-primary flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
		>
			<Boxes class="h-4 w-4" />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex min-w-0 items-center gap-2">
				<h1 class="truncate text-[13px] font-bold">{qualifiedName}</h1>
				<span
					class="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[8px] font-bold tracking-[0.08em] uppercase"
				>
					{databaseObjectKindLabel(objectKind)}
				</span>
			</div>
			<p class="text-muted-foreground mt-0.5 truncate text-[9px]">
				{detail?.comment || 'Definition, metadata, and object relationships'}
			</p>
		</div>
		<div class="flex shrink-0 items-center gap-1">
			<button
				type="button"
				class="rt-toolbar-button h-8 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold"
				onclick={() => copyText(detail?.object.reference.name || reference.name, 'name')}
				title="Copy object name"
			>
				{#if copied === 'name'}<Check class="text-success h-3 w-3" />{:else}<Copy
						class="h-3 w-3"
					/>{/if}
				Name
			</button>
			<button
				type="button"
				class="rt-toolbar-button h-8 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold"
				onclick={() => copyText(qualifiedName, 'qualified')}
				title="Copy qualified name"
			>
				{#if copied === 'qualified'}<Check class="text-success h-3 w-3" />{:else}<Copy
						class="h-3 w-3"
					/>{/if}
				Qualified
			</button>
			<button
				type="button"
				class="rt-toolbar-button h-8 w-8 cursor-pointer"
				onclick={() => void loadObject()}
				disabled={loading}
				title="Refresh object details"
				aria-label="Refresh object details"
			>
				<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
			</button>
			{#if detail?.object.canManage}
				<div class="relative">
					<button
						type="button"
						class="rt-primary-button inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-2.5 text-[9px] font-semibold"
						onclick={() => (manageMenuOpen = !manageMenuOpen)}
						aria-expanded={manageMenuOpen}
					>
						<Settings2 class="h-3.5 w-3.5" />
						Manage
					</button>
					{#if manageMenuOpen}
						<button
							type="button"
							class="fixed inset-0 z-40 cursor-default"
							onclick={() => (manageMenuOpen = false)}
							aria-label="Close object management menu"
						></button>
						<div class="rt-popover absolute top-9 right-0 z-50 w-56 rounded-lg p-1.5">
							<div
								class="text-muted-foreground px-2 py-1 text-[8px] font-bold tracking-[0.1em] uppercase"
							>
								Reviewed structural changes
							</div>
							{#if canEditDefinition && canChange('replace')}
								<button
									type="button"
									class="flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-2 text-left hover:bg-[var(--surface-hover)]"
									onclick={() => openChange('edit')}
								>
									<Pencil class="mt-0.5 h-3.5 w-3.5" />
									<span>
										<span class="block text-[9px] font-semibold">Edit definition</span>
										<span class="text-muted-foreground block text-[8px]"
											>Preview replacement DDL</span
										>
									</span>
								</button>
							{/if}
							{#if canChange('rename')}
								<button
									type="button"
									class="flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-2 text-left hover:bg-[var(--surface-hover)]"
									onclick={() => openChange('rename')}
								>
									<Pencil class="mt-0.5 h-3.5 w-3.5" />
									<span>
										<span class="block text-[9px] font-semibold">Rename</span>
										<span class="text-muted-foreground block text-[8px]"
											>Keep the object identity</span
										>
									</span>
								</button>
							{/if}
							{#if objectKind === 'trigger' && capabilities?.triggerToggle && canChange(triggerEnabled ? 'disable' : 'enable')}
								<button
									type="button"
									class="flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-2 text-left hover:bg-[var(--surface-hover)]"
									onclick={() => openChange(triggerEnabled ? 'disable' : 'enable')}
								>
									{#if triggerEnabled}
										<PowerOff class="mt-0.5 h-3.5 w-3.5" />
									{:else}
										<Power class="mt-0.5 h-3.5 w-3.5" />
									{/if}
									<span>
										<span class="block text-[9px] font-semibold">
											{triggerEnabled ? 'Disable trigger' : 'Enable trigger'}
										</span>
										<span class="text-muted-foreground block text-[8px]">
											{triggerEnabled ? 'Stop firing without dropping' : 'Resume trigger execution'}
										</span>
									</span>
								</button>
							{/if}
							{#if canChange('drop')}
								<div class="bg-border my-1 h-px"></div>
								<button
									type="button"
									class="text-danger hover:bg-danger-soft flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-2 text-left"
									onclick={() => openChange('drop')}
								>
									<Trash2 class="mt-0.5 h-3.5 w-3.5" />
									<span>
										<span class="block text-[9px] font-semibold">Drop object</span>
										<span class="block text-[8px] opacity-75">Permanent structural removal</span>
									</span>
								</button>
							{/if}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</header>

	<nav class="flex h-10 shrink-0 items-end gap-1 border-b px-3" aria-label="Object detail sections">
		<button
			type="button"
			class="relative flex h-9 cursor-pointer items-center gap-1.5 px-3 text-[10px] font-semibold {activeSection ===
			'definition'
				? 'text-foreground'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeSection = 'definition')}
		>
			<FileCode2 class="h-3.5 w-3.5" />
			Definition
			{#if activeSection === 'definition'}
				<span class="bg-primary absolute right-2 bottom-0 left-2 h-0.5 rounded-t"></span>
			{/if}
		</button>
		<button
			type="button"
			class="relative flex h-9 cursor-pointer items-center gap-1.5 px-3 text-[10px] font-semibold {activeSection ===
			'dependencies'
				? 'text-foreground'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeSection = 'dependencies')}
		>
			<GitBranch class="h-3.5 w-3.5" />
			Relationships
			<span class="bg-muted rounded px-1.5 py-0.5 text-[8px] tabular-nums">
				{dependencies.length + dependents.length}
			</span>
			{#if activeSection === 'dependencies'}
				<span class="bg-primary absolute right-2 bottom-0 left-2 h-0.5 rounded-t"></span>
			{/if}
		</button>
		<button
			type="button"
			class="relative flex h-9 cursor-pointer items-center gap-1.5 px-3 text-[10px] font-semibold {activeSection ===
			'properties'
				? 'text-foreground'
				: 'text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeSection = 'properties')}
		>
			<Columns3 class="h-3.5 w-3.5" />
			Properties
			{#if activeSection === 'properties'}
				<span class="bg-primary absolute right-2 bottom-0 left-2 h-0.5 rounded-t"></span>
			{/if}
		</button>
	</nav>

	<div class="min-h-0 flex-1 overflow-auto bg-[var(--surface-sunken)] p-4">
		{#if loading && !detail}
			<div class="text-muted-foreground flex h-full min-h-56 flex-col items-center justify-center">
				<Loader2 class="h-5 w-5 animate-spin" />
				<p class="mt-2 text-[10px] font-semibold">Inspecting {qualifiedName}…</p>
				<p class="mt-1 text-[9px]">Loading definition and relationships</p>
			</div>
		{:else if error}
			<div class="border-danger-border bg-danger-soft mx-auto mt-8 max-w-lg rounded-xl border p-4">
				<div class="flex items-start gap-3">
					<AlertCircle class="text-danger mt-0.5 h-4 w-4 shrink-0" />
					<div>
						<h2 class="text-danger text-[11px] font-bold">{error}</h2>
						{#if errorHint}
							<p class="text-muted-foreground mt-1 text-[9px] leading-relaxed">{errorHint}</p>
						{/if}
						<button
							type="button"
							class="rt-toolbar-button mt-3 h-7 cursor-pointer gap-1.5 px-2.5 text-[9px] font-semibold"
							onclick={() => void loadObject()}
						>
							<RefreshCw class="h-3 w-3" />
							Try again
						</button>
					</div>
				</div>
			</div>
		{:else if detail}
			{#if activeSection === 'definition'}
				<div class="mx-auto flex max-w-5xl flex-col gap-3">
					<div
						class="flex items-center justify-between rounded-lg border bg-[var(--surface-raised)] px-3 py-2"
					>
						<div>
							<div class="text-[10px] font-bold">Database definition</div>
							<div class="text-muted-foreground mt-0.5 text-[8px]">
								{definitionLines} lines · generated from live metadata
							</div>
						</div>
						<div class="flex items-center gap-1">
							<button
								type="button"
								class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold"
								onclick={() => copyText(detail?.definition || '', 'definition')}
								disabled={!detail.definition}
							>
								{#if copied === 'definition'}<Check class="text-success h-3 w-3" />{:else}<Copy
										class="h-3 w-3"
									/>{/if}
								Copy SQL
							</button>
							<button
								type="button"
								class="rt-primary-button inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-2 text-[9px] font-semibold disabled:opacity-40"
								onclick={openDefinitionInQuery}
								disabled={!detail.definition}
							>
								<ArrowUpRight class="h-3 w-3" />
								Open in query
							</button>
						</div>
					</div>
					{#if detail.definition}
						<pre
							class="rt-code-surface min-h-52 overflow-auto rounded-xl border p-4 font-mono text-[10px] leading-[1.65] whitespace-pre"><code
								>{detail.definition}</code
							></pre>
					{:else}
						<div
							class="text-muted-foreground flex min-h-52 items-center justify-center rounded-xl border border-dashed bg-[var(--surface-raised)] text-[10px]"
						>
							No definition is available for this object.
						</div>
					{/if}
				</div>
			{:else if activeSection === 'dependencies'}
				<div class="mx-auto grid max-w-5xl gap-4 lg:grid-cols-2">
					<section class="overflow-hidden rounded-xl border bg-[var(--surface-raised)]">
						<header class="flex h-10 items-center justify-between border-b px-3">
							<div class="flex items-center gap-2">
								<GitBranch class="text-muted-foreground h-3.5 w-3.5" />
								<h2 class="text-[10px] font-bold">Uses</h2>
							</div>
							<span class="text-muted-foreground text-[9px]">{dependencies.length}</span>
						</header>
						{#if dependencies.length > 0}
							<div class="divide-y">
								{#each dependencies as dependency}
									<button
										type="button"
										class="flex w-full cursor-pointer items-start gap-2.5 px-3 py-2.5 text-left hover:bg-[var(--surface-hover)] disabled:cursor-default"
										onclick={() => openDependency(dependency)}
										disabled={dependency.reference.kind === 'unknown'}
									>
										<span
											class="bg-muted mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded"
										>
											<Boxes class="h-3 w-3" />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[10px] font-semibold"
												>{dependencyTitle(dependency)}</span
											>
											<span class="text-muted-foreground mt-0.5 block truncate text-[8px]"
												>{dependency.description}</span
											>
										</span>
										{#if dependency.reference.kind !== 'unknown'}
											<ArrowUpRight class="text-muted-foreground mt-1 h-3 w-3 shrink-0" />
										{/if}
									</button>
								{/each}
							</div>
						{:else}
							<div class="text-muted-foreground flex h-28 items-center justify-center text-[9px]">
								No direct dependencies detected
							</div>
						{/if}
					</section>

					<section class="overflow-hidden rounded-xl border bg-[var(--surface-raised)]">
						<header class="flex h-10 items-center justify-between border-b px-3">
							<div class="flex items-center gap-2">
								<GitBranch class="text-muted-foreground h-3.5 w-3.5 rotate-180" />
								<h2 class="text-[10px] font-bold">Used by</h2>
							</div>
							<span class="text-muted-foreground text-[9px]">{dependents.length}</span>
						</header>
						{#if dependents.length > 0}
							<div class="divide-y">
								{#each dependents as dependent}
									<button
										type="button"
										class="flex w-full cursor-pointer items-start gap-2.5 px-3 py-2.5 text-left hover:bg-[var(--surface-hover)] disabled:cursor-default"
										onclick={() => openDependency(dependent)}
										disabled={dependent.reference.kind === 'unknown'}
									>
										<span
											class="bg-muted mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded"
										>
											<Boxes class="h-3 w-3" />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-[10px] font-semibold"
												>{dependencyTitle(dependent)}</span
											>
											<span class="text-muted-foreground mt-0.5 block truncate text-[8px]"
												>{dependent.description}</span
											>
										</span>
										{#if dependent.reference.kind !== 'unknown'}
											<ArrowUpRight class="text-muted-foreground mt-1 h-3 w-3 shrink-0" />
										{/if}
									</button>
								{/each}
							</div>
						{:else}
							<div class="text-muted-foreground flex h-28 items-center justify-center text-[9px]">
								No direct dependents detected
							</div>
						{/if}
					</section>
				</div>
			{:else}
				<div class="mx-auto max-w-5xl space-y-4">
					<section class="overflow-hidden rounded-xl border bg-[var(--surface-raised)]">
						<header class="flex h-10 items-center justify-between border-b px-3">
							<h2 class="text-[10px] font-bold">Object properties</h2>
							<span class="text-muted-foreground text-[8px]">
								{databaseObjectKindLabel(objectKind)}
							</span>
						</header>
						<div class="grid sm:grid-cols-2 xl:grid-cols-3">
							<div class="border-b px-3 py-2.5 sm:border-r">
								<div class="text-muted-foreground text-[8px] font-bold uppercase">Name</div>
								<div class="mt-1 truncate font-mono text-[9px]">{detail.object.reference.name}</div>
							</div>
							<div class="border-b px-3 py-2.5 xl:border-r">
								<div class="text-muted-foreground text-[8px] font-bold uppercase">Namespace</div>
								<div class="mt-1 truncate font-mono text-[9px]">
									{detail.object.reference.schema || 'database'}
								</div>
							</div>
							<div class="border-b px-3 py-2.5">
								<div class="text-muted-foreground text-[8px] font-bold uppercase">Type</div>
								<div class="mt-1 text-[9px]">{databaseObjectKindLabel(objectKind)}</div>
							</div>
							{#each properties as property}
								<div class="border-r border-b px-3 py-2.5 last:border-r-0">
									<div class="text-muted-foreground text-[8px] font-bold uppercase">
										{property.name}
									</div>
									<div class="mt-1 font-mono text-[9px] break-words">{property.value}</div>
								</div>
							{/each}
						</div>
					</section>

					{#if columns.length > 0}
						<section class="overflow-hidden rounded-xl border bg-[var(--surface-raised)]">
							<header class="flex h-10 items-center justify-between border-b px-3">
								<div class="flex items-center gap-2">
									<Columns3 class="text-muted-foreground h-3.5 w-3.5" />
									<h2 class="text-[10px] font-bold">Columns</h2>
								</div>
								<span class="text-muted-foreground text-[9px]">{columns.length}</span>
							</header>
							<div class="divide-y">
								{#each columns as column}
									<div
										class="grid grid-cols-[minmax(140px,1fr)_minmax(120px,0.8fr)_auto] items-center gap-3 px-3 py-2"
									>
										<span class="truncate font-mono text-[9px] font-semibold">{column.name}</span>
										<span class="text-muted-foreground truncate font-mono text-[9px]"
											>{column.data_type}</span
										>
										<div class="flex justify-end gap-1">
											{#if column.is_primary}
												<span
													class="bg-muted text-foreground rounded px-1.5 py-0.5 text-[7px] font-bold"
													>PK</span
												>
											{/if}
											{#if column.foreign_table}
												<span
													class="bg-info-soft text-info rounded px-1.5 py-0.5 text-[7px] font-bold"
													>FK</span
												>
											{/if}
											{#if !column.nullable}
												<span
													class="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[7px] font-bold"
													>NOT NULL</span
												>
											{/if}
										</div>
									</div>
								{/each}
							</div>
						</section>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>

<ObjectChangeDialog
	open={changeIntent !== null}
	connectionId={tab.connectionId}
	intent={changeIntent}
	{capabilities}
	reference={detail?.object.reference || reference}
	definition={detail?.definition || ''}
	table={detail?.object.reference.parentName
		? new database.Table({
				Schema: detail.object.reference.parentSchema || detail.object.reference.schema,
				Name: detail.object.reference.parentName
			})
		: null}
	{columns}
	onClose={() => (changeIntent = null)}
	onApplied={handleStructuralChangeApplied}
/>
