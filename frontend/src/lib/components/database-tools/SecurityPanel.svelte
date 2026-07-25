<script lang="ts">
	import {
		Check,
		ChevronRight,
		CircleAlert,
		KeyRound,
		Loader2,
		LockKeyhole,
		Plus,
		RefreshCw,
		Search,
		Shield,
		ShieldAlert,
		Trash2,
		UserRound,
		UsersRound,
		X
	} from 'lucide-svelte';
	import {
		ApplySecurityChange,
		GetSecurityOverview,
		PreviewSecurityChange
	} from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { createServiceError } from '$lib/errors/service';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';
	import { addConsoleLog, updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';

	type EditorKind = 'account' | 'privilege' | 'membership' | 'drop';

	let connectionId = $state('');
	let overview = $state<database.SecurityOverview | null>(null);
	let selectedKey = $state('');
	let search = $state('');
	let loading = $state(false);
	let error = $state('');
	let initialized = false;

	let editorOpen = $state(false);
	let editorKind = $state<EditorKind>('account');
	let editorAction = $state('create_principal');
	let principalName = $state('');
	let principalHost = $state('%');
	let principalKind = $state('user');
	let password = $state('');
	let canLogin = $state(true);
	let superuser = $state(false);
	let createDb = $state(false);
	let createRole = $state(false);
	let inherit = $state(true);
	let replication = $state(false);
	let bypassRls = $state(false);
	let locked = $state(false);

	let grantee = $state('');
	let granteeHost = $state('');
	let role = $state('');
	let roleHost = $state('');
	let objectType = $state('table');
	let grantSchema = $state('');
	let grantObject = $state('');
	let privilege = $state('SELECT');
	let grantable = $state(false);

	let changePreview = $state<database.SecurityChangePreview | null>(null);
	let previewRequest = $state<database.SecurityChangeRequest | null>(null);
	let previewing = $state(false);
	let applying = $state(false);
	let reviewed = $state(false);
	let destructiveConfirmation = $state('');
	let editorError = $state('');

	const connections = $derived(connectionStore.connections);
	const connectionOptions = $derived(
		connections.map((item) => ({
			value: item.id,
			label: `${item.name || item.database} · ${item.driver} · ${item.database}`
		}))
	);
	const selectedPrincipal = $derived(
		overview?.principals.find((principal) => principalKey(principal) === selectedKey) ?? null
	);
	const filteredPrincipals = $derived(
		(overview?.principals ?? []).filter((principal) => {
			const term = search.trim().toLowerCase();
			if (!term) return true;
			return `${principal.name} ${principal.host ?? ''} ${principal.kind}`
				.toLowerCase()
				.includes(term);
		})
	);
	const canApply = $derived(
		Boolean(
			changePreview &&
				previewRequest &&
				reviewed &&
				(!changePreview.destructive || destructiveConfirmation === confirmationValue()) &&
				!applying
		)
	);
	const isPostgres = $derived(overview?.engine === 'postgres');
	const principalKindOptions = [
		{ value: 'user', label: 'User / login' },
		{ value: 'role', label: 'Role' }
	];
	const objectTypeOptions = $derived(
		isPostgres
			? [
					{ value: 'database', label: 'Database' },
					{ value: 'schema', label: 'Schema' },
					{ value: 'table', label: 'Table' },
					{ value: 'all_tables_in_schema', label: 'All tables in schema' },
					{ value: 'sequence', label: 'Sequence' }
				]
			: [
					{ value: 'global', label: 'Global' },
					{ value: 'database', label: 'Database' },
					{ value: 'table', label: 'Table' }
				]
	);
	const privilegeOptions = $derived(
		isPostgres
			? objectType === 'database'
				? ['CONNECT', 'CREATE', 'TEMPORARY']
				: objectType === 'schema'
					? ['USAGE', 'CREATE']
					: objectType === 'sequence'
						? ['USAGE', 'SELECT', 'UPDATE', 'ALL']
						: ['SELECT', 'INSERT', 'UPDATE', 'DELETE', 'TRUNCATE', 'REFERENCES', 'TRIGGER', 'ALL']
			: [
					'SELECT',
					'INSERT',
					'UPDATE',
					'DELETE',
					'CREATE',
					'DROP',
					'ALTER',
					'INDEX',
					'REFERENCES',
					'EXECUTE',
					'CREATE VIEW',
					'SHOW VIEW',
					'TRIGGER',
					'EVENT',
					'CREATE ROUTINE',
					'ALTER ROUTINE',
					'PROCESS',
					'ALL'
				]
	);
	const privilegeSelectOptions = $derived(
		privilegeOptions.map((item) => ({ value: item, label: item }))
	);
	const roleOptions = $derived(
		(overview?.principals ?? [])
			.filter((principal) => principal.kind === 'role' && principal.name !== grantee)
			.map((principal) => ({ value: principal.name, label: principal.name }))
	);

	$effect(() => {
		if (initialized || connections.length === 0) return;
		initialized = true;
		connectionId = (connectionStore.activeConnection ?? connections[0]).id;
		void loadOverview();
	});

	function principalKey(principal: database.DatabasePrincipal): string {
		return `${principal.name}\u0000${principal.host ?? ''}`;
	}

	async function loadOverview(preferredName = '', preferredHost = ''): Promise<void> {
		if (!connectionId) return;
		loading = true;
		error = '';
		try {
			let response = await GetSecurityOverview(connectionId, preferredName, preferredHost);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load database security');
			}
			let loaded = response.data ?? null;
			if (loaded?.supported && !preferredName && loaded.principals.length > 0) {
				const existing = loaded.principals.find(
					(principal) => principalKey(principal) === selectedKey
				);
				const first = existing ?? loaded.principals[0];
				response = await GetSecurityOverview(connectionId, first.name, first.host ?? '');
				if (response.errors?.length) {
					throw createServiceError(response.errors[0], 'Could not load grants');
				}
				loaded = response.data ?? loaded;
				selectedKey = principalKey(first);
			} else if (preferredName && loaded) {
				const selected = loaded.principals.find(
					(principal) =>
						principal.name === preferredName && (principal.host ?? '') === preferredHost
				);
				if (selected) selectedKey = principalKey(selected);
			}
			overview = loaded;
		} catch (loadError: any) {
			error = loadError?.message ?? 'Could not load database security.';
			overview = null;
		} finally {
			loading = false;
		}
	}

	async function selectPrincipal(principal: database.DatabasePrincipal): Promise<void> {
		selectedKey = principalKey(principal);
		await loadOverview(principal.name, principal.host ?? '');
	}

	function resetPreview(): void {
		changePreview = null;
		previewRequest = null;
		reviewed = false;
		destructiveConfirmation = '';
		editorError = '';
	}

	function openAccountEditor(
		action: 'create_principal' | 'alter_principal',
		principal?: database.DatabasePrincipal | null
	): void {
		editorOpen = true;
		editorKind = 'account';
		editorAction = action;
		principalName = principal?.name ?? '';
		principalHost = principal?.host ?? '%';
		principalKind = principal?.kind ?? 'user';
		password = '';
		canLogin = principal?.canLogin ?? true;
		superuser = principal?.superuser ?? false;
		createDb = principal?.createDb ?? false;
		createRole = principal?.createRole ?? false;
		inherit = principal?.inherit ?? true;
		replication = principal?.replication ?? false;
		bypassRls = principal?.bypassRls ?? false;
		locked = principal?.locked ?? false;
		resetPreview();
	}

	function openDropEditor(): void {
		if (!selectedPrincipal) return;
		openAccountEditor('alter_principal', selectedPrincipal);
		editorKind = 'drop';
		editorAction = 'drop_principal';
	}

	function openPrivilegeEditor(
		action: 'grant_privilege' | 'revoke_privilege' = 'grant_privilege',
		existing?: database.DatabaseGrant
	): void {
		if (!selectedPrincipal) return;
		editorOpen = true;
		editorKind = 'privilege';
		editorAction = action;
		grantee = selectedPrincipal.name;
		granteeHost = selectedPrincipal.host ?? '';
		objectType =
			existing?.objectType && existing.objectType !== 'statement'
				? existing.objectType
				: isPostgres
					? 'table'
					: 'database';
		grantSchema = existing?.schema ?? '';
		grantObject = existing?.object ?? '';
		privilege =
			existing?.privilege && existing.privilege !== 'GRANT' ? existing.privilege : 'SELECT';
		grantable = existing?.grantable ?? false;
		resetPreview();
	}

	function openMembershipEditor(
		action: 'grant_role' | 'revoke_role' = 'grant_role',
		existing?: database.DatabaseGrant
	): void {
		if (!selectedPrincipal) return;
		editorOpen = true;
		editorKind = 'membership';
		editorAction = action;
		grantee = selectedPrincipal.name;
		granteeHost = selectedPrincipal.host ?? '';
		role = existing?.role ?? '';
		roleHost = '';
		grantable = existing?.grantable ?? false;
		resetPreview();
	}

	function closeEditor(): void {
		if (applying) return;
		editorOpen = false;
		password = '';
		resetPreview();
	}

	function buildRequest(): database.SecurityChangeRequest {
		return new database.SecurityChangeRequest({
			action: editorAction,
			principal: new database.PrincipalOptions({
				name: principalName.trim(),
				host: principalHost.trim(),
				kind: principalKind,
				password,
				canLogin,
				superuser,
				createDb,
				createRole,
				inherit,
				replication,
				bypassRls,
				locked
			}),
			grant: new database.GrantOptions({
				grantee: grantee.trim(),
				granteeHost: granteeHost.trim(),
				role: role.trim(),
				roleHost: roleHost.trim(),
				objectType,
				schema: grantSchema.trim(),
				object: grantObject.trim(),
				privilege,
				grantable
			})
		});
	}

	async function previewChange(): Promise<void> {
		if (!hasBackendMethod('PreviewSecurityChange')) {
			editorError = BACKEND_RESTART_MESSAGE;
			return;
		}
		previewing = true;
		resetPreview();
		try {
			const request = buildRequest();
			const response = await PreviewSecurityChange(connectionId, request);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not preview security change');
			}
			changePreview = response.data ?? null;
			previewRequest = request;
			if (!changePreview) throw new Error('Security preview returned no data.');
		} catch (previewError: any) {
			editorError = previewError?.message ?? 'Could not preview security change.';
		} finally {
			previewing = false;
		}
	}

	function confirmationValue(): string {
		if (editorKind === 'privilege' || editorKind === 'membership') return grantee;
		return principalName;
	}

	async function applyChange(): Promise<void> {
		if (!changePreview || !previewRequest || !canApply) return;
		applying = true;
		editorError = '';
		try {
			const response = await ApplySecurityChange(
				connectionId,
				new database.ApplySecurityChangeRequest({
					change: previewRequest,
					fingerprint: changePreview.fingerprint
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Security change failed');
			}
			const summary = changePreview.summary;
			updateStatus(summary, 'success');
			addConsoleLog(`Security change applied: ${summary}`, 'success');
			const reloadName =
				editorAction === 'drop_principal' ? '' : editorKind === 'account' ? principalName : grantee;
			const reloadHost = editorKind === 'account' ? principalHost : granteeHost;
			closeEditor();
			await loadOverview(reloadName, reloadHost);
		} catch (applyError: any) {
			editorError = applyError?.message ?? 'Security change failed.';
			updateStatus(editorError, 'error');
		} finally {
			applying = false;
		}
	}

	function objectLabel(grant: database.DatabaseGrant): string {
		if (grant.statement) return grant.statement;
		if (grant.objectType === 'role') return grant.role || grant.object;
		const qualified = [grant.schema, grant.object].filter(Boolean).join('.');
		return qualified || grant.objectType;
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
	<div class="flex h-13 shrink-0 items-center gap-3 border-b bg-[var(--surface-sunken)] px-4">
		<label class="flex min-w-0 flex-1 items-center gap-2">
			<span class="text-muted-foreground text-[8px] font-bold">Connection</span>
			<FilterCombobox
				id="security-connection"
				class="max-w-sm min-w-0 flex-1"
				options={connectionOptions}
				value={connectionId}
				onChange={(value) => {
					connectionId = value;
					selectedKey = '';
					void loadOverview();
				}}
				searchable={connections.length > 8}
				searchPlaceholder="Find connection…"
				triggerClass="h-8 px-2 text-[9px]"
			/>
		</label>
		{#if overview}
			<span class="text-muted-foreground text-[8px]">
				Connected as <strong class="text-foreground">{overview.currentUser || 'unknown'}</strong>
			</span>
		{/if}
		<button
			type="button"
			class="rt-toolbar-button h-8 w-8 cursor-pointer"
			onclick={() => loadOverview(selectedPrincipal?.name, selectedPrincipal?.host ?? '')}
			disabled={loading}
			title="Refresh security"
		>
			<RefreshCw class="h-3.5 w-3.5 {loading ? 'animate-spin' : ''}" />
		</button>
	</div>

	{#if error}
		<div
			class="border-danger-border bg-danger-soft text-danger m-4 mb-0 flex items-start gap-2 rounded-lg border px-3 py-2 text-[8px]"
		>
			<CircleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
			{error}
		</div>
	{/if}

	{#if loading && !overview}
		<div class="flex flex-1 items-center justify-center">
			<Loader2 class="text-muted-foreground h-5 w-5 animate-spin" />
		</div>
	{:else if overview && !overview.supported}
		<div class="flex flex-1 items-center justify-center p-8 text-center">
			<div class="max-w-sm">
				<LockKeyhole class="text-muted-foreground mx-auto h-7 w-7" />
				<h3 class="mt-3 text-[11px] font-bold">Database principals are not available</h3>
				<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">{overview.message}</p>
			</div>
		</div>
	{:else if overview}
		<div class="grid min-h-0 flex-1 grid-cols-[270px_minmax(0,1fr)] overflow-hidden">
			<aside class="flex min-h-0 flex-col border-r bg-[var(--surface-sunken)]">
				<div class="space-y-2 border-b p-3">
					<div class="rt-input flex h-8 items-center gap-2 px-2">
						<Search class="text-muted-foreground h-3.5 w-3.5" />
						<input
							class="min-w-0 flex-1 bg-transparent text-[8px] outline-none"
							bind:value={search}
							placeholder="Filter users and roles"
						/>
					</div>
					<button
						type="button"
						class="rt-primary-button flex h-8 w-full cursor-pointer items-center justify-center gap-2 rounded-md text-[8px] font-bold"
						onclick={() => openAccountEditor('create_principal')}
					>
						<Plus class="h-3.5 w-3.5" />
						New user or role
					</button>
				</div>
				<div class="min-h-0 flex-1 overflow-y-auto p-2">
					{#each filteredPrincipals as principal (principalKey(principal))}
						<button
							type="button"
							class="mb-1 flex w-full cursor-pointer items-center gap-2.5 rounded-lg border px-2.5 py-2 text-left {principalKey(
								principal
							) === selectedKey
								? 'border-border bg-[var(--surface-raised)] shadow-sm'
								: 'border-transparent hover:bg-[var(--surface-hover)]'}"
							onclick={() => selectPrincipal(principal)}
						>
							<span
								class="bg-muted text-muted-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
							>
								{#if principal.kind === 'role'}
									<UsersRound class="h-3.5 w-3.5" />
								{:else}
									<UserRound class="h-3.5 w-3.5" />
								{/if}
							</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-[9px] font-bold">{principal.name}</span>
								<span class="text-muted-foreground mt-0.5 block truncate text-[7px]">
									{principal.host ? `@${principal.host} · ` : ''}{principal.kind}
								</span>
							</span>
							{#if principal.locked}
								<LockKeyhole class="text-warning h-3 w-3" />
							{/if}
							<ChevronRight class="text-muted-foreground h-3 w-3" />
						</button>
					{/each}
				</div>
			</aside>

			<section class="flex min-h-0 min-w-0 flex-col overflow-hidden">
				{#if selectedPrincipal}
					<header class="flex shrink-0 items-center gap-3 border-b px-4 py-3">
						<span
							class="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--surface-sunken)]"
						>
							<Shield class="text-muted-foreground h-4 w-4" />
						</span>
						<div class="min-w-0 flex-1">
							<h3 class="truncate text-[11px] font-bold">
								{selectedPrincipal.name}{selectedPrincipal.host ? `@${selectedPrincipal.host}` : ''}
							</h3>
							<p class="text-muted-foreground mt-0.5 text-[8px]">
								{selectedPrincipal.kind} · {selectedPrincipal.canLogin
									? 'login enabled'
									: 'no login'}{selectedPrincipal.authMethod
									? ` · ${selectedPrincipal.authMethod}`
									: ''}
							</p>
						</div>
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer gap-2 px-2.5 text-[8px] font-bold"
							onclick={() => openMembershipEditor()}
						>
							<UsersRound class="h-3.5 w-3.5" />
							Role
						</button>
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer gap-2 px-2.5 text-[8px] font-bold"
							onclick={() => openPrivilegeEditor()}
						>
							<KeyRound class="h-3.5 w-3.5" />
							Grant
						</button>
						<button
							type="button"
							class="rt-toolbar-button h-8 cursor-pointer px-2.5 text-[8px] font-bold"
							onclick={() => openAccountEditor('alter_principal', selectedPrincipal)}
						>
							Edit
						</button>
						<button
							type="button"
							class="rt-toolbar-button hover:text-destructive h-8 w-8 cursor-pointer"
							onclick={openDropEditor}
							title="Drop principal"
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</header>

					<div class="min-h-0 flex-1 overflow-y-auto p-4">
						<div class="mb-4 flex flex-wrap gap-1.5">
							{#if selectedPrincipal.superuser}
								<span class="bg-danger-soft text-danger rounded-full px-2 py-1 text-[7px] font-bold"
									>SUPERUSER</span
								>
							{/if}
							{#if selectedPrincipal.createDb}
								<span class="bg-muted rounded-full px-2 py-1 text-[7px] font-bold">CREATEDB</span>
							{/if}
							{#if selectedPrincipal.createRole}
								<span class="bg-muted rounded-full px-2 py-1 text-[7px] font-bold">CREATEROLE</span>
							{/if}
							{#if selectedPrincipal.replication}
								<span class="bg-muted rounded-full px-2 py-1 text-[7px] font-bold">REPLICATION</span
								>
							{/if}
							{#if selectedPrincipal.bypassRls}
								<span class="bg-muted rounded-full px-2 py-1 text-[7px] font-bold">BYPASSRLS</span>
							{/if}
							{#if !selectedPrincipal.superuser && !selectedPrincipal.createDb && !selectedPrincipal.createRole && !selectedPrincipal.replication && !selectedPrincipal.bypassRls}
								<span class="text-muted-foreground text-[8px]">No elevated role attributes</span>
							{/if}
						</div>

						<div class="mb-2 flex items-center justify-between">
							<div>
								<h4 class="text-[9px] font-bold">Effective grants shown by the engine</h4>
								<p class="text-muted-foreground mt-0.5 text-[7px]">
									{overview.grants.length} grant{overview.grants.length === 1 ? '' : 's'}
								</p>
							</div>
						</div>
						{#if overview.grants.length === 0}
							<div class="rounded-xl border border-dashed p-8 text-center">
								<KeyRound class="text-muted-foreground mx-auto h-5 w-5" />
								<p class="mt-2 text-[8px] font-bold">No direct grants found</p>
							</div>
						{:else}
							<div class="overflow-hidden rounded-xl border">
								{#each overview.grants as grant, index}
									<div class="flex items-center gap-3 px-3 py-2.5 {index > 0 ? 'border-t' : ''}">
										<span
											class="bg-muted flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
										>
											<KeyRound class="h-3.5 w-3.5" />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate font-mono text-[8px]">
												{grant.statement || `${grant.privilege} · ${objectLabel(grant)}`}
											</span>
											{#if !grant.statement}
												<span class="text-muted-foreground mt-0.5 block text-[7px]">
													{grant.objectType}{grant.grantable ? ' · grant option' : ''}
												</span>
											{/if}
										</span>
										{#if !grant.statement}
											<button
												type="button"
												class="rt-toolbar-button h-7 cursor-pointer px-2 text-[7px] font-bold"
												onclick={() =>
													grant.objectType === 'role'
														? openMembershipEditor('revoke_role', grant)
														: openPrivilegeEditor('revoke_privilege', grant)}
											>
												Revoke
											</button>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{:else}
					<div class="flex flex-1 items-center justify-center text-center">
						<div>
							<Shield class="text-muted-foreground mx-auto h-6 w-6" />
							<p class="mt-2 text-[9px] font-bold">Select a user or role</p>
						</div>
					</div>
				{/if}
			</section>
		</div>
	{/if}
</div>

{#if editorOpen}
	<div class="bg-overlay/25 absolute inset-0 z-20 flex justify-end backdrop-blur-[1px]">
		<button
			type="button"
			class="absolute inset-0 cursor-default"
			onclick={closeEditor}
			aria-label="Close security editor"
		></button>
		<aside
			class="relative flex h-full w-[440px] flex-col border-l bg-[var(--surface-raised)] shadow-2xl"
		>
			<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
				<span
					class="bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-lg"
				>
					{#if editorKind === 'account'}
						<UserRound class="h-4 w-4" />
					{:else if editorKind === 'drop'}
						<ShieldAlert class="text-danger h-4 w-4" />
					{:else}
						<KeyRound class="h-4 w-4" />
					{/if}
				</span>
				<div class="min-w-0 flex-1">
					<h3 class="text-[10px] font-bold">
						{editorAction.replaceAll('_', ' ')}
					</h3>
					<p class="text-muted-foreground mt-0.5 text-[7px]">
						SQL preview and explicit review are required
					</p>
				</div>
				<button
					type="button"
					class="rt-toolbar-button h-7 w-7 cursor-pointer"
					onclick={closeEditor}
				>
					<X class="h-3.5 w-3.5" />
				</button>
			</header>

			<div class="min-h-0 flex-1 overflow-y-auto p-4">
				{#if editorError}
					<div
						class="border-danger-border bg-danger-soft text-danger mb-3 rounded-lg border px-3 py-2 text-[8px]"
					>
						{editorError}
					</div>
				{/if}

				{#if editorKind === 'account' || editorKind === 'drop'}
					<div class="grid grid-cols-2 gap-3">
						<label class="col-span-2">
							<span class="text-muted-foreground mb-1 block text-[8px]">Name</span>
							<input
								class="rt-input h-9 w-full px-2 text-[9px]"
								bind:value={principalName}
								disabled={editorAction !== 'create_principal'}
								oninput={resetPreview}
							/>
						</label>
						{#if !isPostgres}
							<label>
								<span class="text-muted-foreground mb-1 block text-[8px]">Host</span>
								<input
									class="rt-input h-9 w-full px-2 text-[9px]"
									bind:value={principalHost}
									disabled={editorAction !== 'create_principal'}
									oninput={resetPreview}
								/>
							</label>
						{/if}
						<label class={isPostgres ? 'col-span-2' : ''}>
							<span class="text-muted-foreground mb-1 block text-[8px]">Kind</span>
							<FilterCombobox
								id="security-principal-kind"
								options={principalKindOptions}
								value={principalKind}
								onChange={(value) => {
									principalKind = value;
									resetPreview();
								}}
								disabled={editorAction !== 'create_principal'}
								searchable={false}
								triggerClass="h-9 px-2 text-[9px]"
							/>
						</label>
						{#if editorKind !== 'drop' && principalKind === 'user'}
							<label class="col-span-2">
								<span class="text-muted-foreground mb-1 block text-[8px]">
									Password {editorAction === 'alter_principal' ? '(leave blank to keep)' : ''}
								</span>
								<input
									type="password"
									class="rt-input h-9 w-full px-2 text-[9px]"
									bind:value={password}
									oninput={resetPreview}
									autocomplete="new-password"
								/>
							</label>
						{/if}
					</div>

					{#if editorKind === 'drop'}
						<div class="border-danger-border bg-danger-soft mt-4 rounded-xl border p-3">
							<p class="text-danger text-[8px] font-bold">
								This removes the account and its grants.
							</p>
							<p class="text-muted-foreground mt-1 text-[7px]">
								Owned PostgreSQL objects must be reassigned before the role can be dropped.
							</p>
						</div>
					{:else if isPostgres}
						<div class="mt-4 grid grid-cols-2 gap-2">
							{#each [['Can login', 'canLogin'], ['Superuser', 'superuser'], ['Create database', 'createDb'], ['Create roles', 'createRole'], ['Inherit grants', 'inherit'], ['Replication', 'replication'], ['Bypass RLS', 'bypassRls']] as option}
								<label
									class="flex cursor-pointer items-center gap-2 rounded-lg border p-2.5 text-[8px]"
								>
									<input
										type="checkbox"
										checked={option[1] === 'canLogin'
											? canLogin
											: option[1] === 'superuser'
												? superuser
												: option[1] === 'createDb'
													? createDb
													: option[1] === 'createRole'
														? createRole
														: option[1] === 'inherit'
															? inherit
															: option[1] === 'replication'
																? replication
																: bypassRls}
										onchange={(event) => {
											const value = event.currentTarget.checked;
											if (option[1] === 'canLogin') canLogin = value;
											else if (option[1] === 'superuser') superuser = value;
											else if (option[1] === 'createDb') createDb = value;
											else if (option[1] === 'createRole') createRole = value;
											else if (option[1] === 'inherit') inherit = value;
											else if (option[1] === 'replication') replication = value;
											else bypassRls = value;
											resetPreview();
										}}
									/>
									{option[0]}
								</label>
							{/each}
						</div>
					{:else}
						<label
							class="mt-4 flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-[8px]"
						>
							<input type="checkbox" bind:checked={locked} onchange={resetPreview} />
							Lock this account
						</label>
					{/if}
				{:else if editorKind === 'privilege'}
					<div class="grid grid-cols-2 gap-3">
						<label class="col-span-2">
							<span class="text-muted-foreground mb-1 block text-[8px]">Grantee</span>
							<input class="rt-input h-9 w-full px-2 text-[9px]" bind:value={grantee} disabled />
						</label>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Scope</span>
							<FilterCombobox
								id="security-object-type"
								options={objectTypeOptions}
								value={objectType}
								onChange={(value) => {
									objectType = value;
									if (!privilegeOptions.includes(privilege)) {
										privilege = privilegeOptions[0];
									}
									resetPreview();
								}}
								searchable={false}
								triggerClass="h-9 px-2 text-[9px]"
							/>
						</label>
						<label>
							<span class="text-muted-foreground mb-1 block text-[8px]">Privilege</span>
							<FilterCombobox
								id="security-privilege"
								options={privilegeSelectOptions}
								value={privilege}
								onChange={(value) => {
									privilege = value;
									resetPreview();
								}}
								searchable={privilegeSelectOptions.length > 8}
								searchPlaceholder="Find privilege…"
								triggerClass="h-9 px-2 text-[9px]"
							/>
						</label>
						{#if objectType !== 'global' && objectType !== 'database'}
							<label>
								<span class="text-muted-foreground mb-1 block text-[8px]"> Schema / database </span>
								<input
									class="rt-input h-9 w-full px-2 text-[9px]"
									bind:value={grantSchema}
									oninput={resetPreview}
								/>
							</label>
						{/if}
						{#if objectType === 'database'}
							<label class="col-span-2">
								<span class="text-muted-foreground mb-1 block text-[8px]">Database</span>
								<input
									class="rt-input h-9 w-full px-2 text-[9px]"
									bind:value={grantObject}
									oninput={resetPreview}
								/>
							</label>
						{:else if objectType === 'table' || objectType === 'sequence'}
							<label>
								<span class="text-muted-foreground mb-1 block text-[8px]">Object</span>
								<input
									class="rt-input h-9 w-full px-2 text-[9px]"
									bind:value={grantObject}
									oninput={resetPreview}
								/>
							</label>
						{/if}
						{#if editorAction === 'grant_privilege'}
							<label
								class="col-span-2 flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-[8px]"
							>
								<input type="checkbox" bind:checked={grantable} onchange={resetPreview} />
								Allow this principal to grant the privilege to others
							</label>
						{/if}
					</div>
				{:else if editorKind === 'membership'}
					<div class="space-y-3">
						<label class="block">
							<span class="text-muted-foreground mb-1 block text-[8px]">Grantee</span>
							<input class="rt-input h-9 w-full px-2 text-[9px]" bind:value={grantee} disabled />
						</label>
						<label class="block">
							<span class="text-muted-foreground mb-1 block text-[8px]">Role</span>
							<FilterCombobox
								id="security-role"
								options={roleOptions}
								value={role}
								onChange={(value) => {
									role = value;
									resetPreview();
								}}
								placeholder="Choose a role"
								searchable={roleOptions.length > 8}
								searchPlaceholder="Find role…"
								emptyText="No roles available"
								triggerClass="h-9 px-2 text-[9px]"
							/>
						</label>
						{#if editorAction === 'grant_role'}
							<label
								class="flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-[8px]"
							>
								<input type="checkbox" bind:checked={grantable} onchange={resetPreview} />
								Allow administering this role membership
							</label>
						{/if}
					</div>
				{/if}

				{#if changePreview}
					<div class="mt-4 overflow-hidden rounded-xl border">
						<div class="flex items-center justify-between border-b px-3 py-2">
							<span class="text-[8px] font-bold">Reviewed SQL</span>
							{#if changePreview.destructive}
								<span class="text-danger text-[7px] font-bold">DESTRUCTIVE</span>
							{/if}
						</div>
						<pre
							class="rt-code-surface overflow-x-auto p-3 font-mono text-[8px] leading-relaxed whitespace-pre-wrap">{changePreview.sql}</pre>
					</div>
					{#each changePreview.warnings as warning}
						<p class="text-warning mt-2 text-[7px] leading-relaxed">
							{warning}
						</p>
					{/each}
				{/if}
			</div>

			<footer class="shrink-0 space-y-2 border-t p-4">
				{#if changePreview}
					<label class="flex cursor-pointer items-center gap-2 text-[8px]">
						<input type="checkbox" bind:checked={reviewed} />
						I reviewed the SQL and resulting access
					</label>
					{#if changePreview.destructive}
						<label class="block">
							<span class="text-muted-foreground mb-1 block text-[7px]">
								Type <strong>{confirmationValue()}</strong> to confirm
							</span>
							<input
								class="rt-input h-8 w-full px-2 text-[8px]"
								bind:value={destructiveConfirmation}
							/>
						</label>
					{/if}
				{/if}
				<div class="flex gap-2">
					<button
						type="button"
						class="rt-toolbar-button h-9 flex-1 cursor-pointer text-[8px] font-bold"
						onclick={closeEditor}
					>
						Cancel
					</button>
					{#if changePreview}
						<button
							type="button"
							class="rt-primary-button h-9 flex-1 cursor-pointer rounded-md text-[8px] font-bold"
							onclick={applyChange}
							disabled={!canApply}
						>
							{#if applying}<Loader2 class="mr-2 inline h-3.5 w-3.5 animate-spin" />{/if}
							Apply change
						</button>
					{:else}
						<button
							type="button"
							class="rt-primary-button h-9 flex-1 cursor-pointer rounded-md text-[8px] font-bold"
							onclick={previewChange}
							disabled={previewing}
						>
							{#if previewing}<Loader2 class="mr-2 inline h-3.5 w-3.5 animate-spin" />{/if}
							Review SQL
						</button>
					{/if}
				</div>
			</footer>
		</aside>
	</div>
{/if}
