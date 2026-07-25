<script lang="ts">
	import {
		Activity,
		Check,
		CircleAlert,
		Clock3,
		Database,
		Loader2,
		LockKeyhole,
		Pause,
		Play,
		RefreshCw,
		Search,
		ServerCrash,
		Square,
		TimerReset,
		UserRound,
		X,
		Zap
	} from 'lucide-svelte';
	import { CancelDatabaseSession, GetDatabaseActivity } from '$lib/wailsjs/go/db/Service';
	import { database } from '$lib/wailsjs/go/models';
	import { connectionStore } from '$lib/stores/connectionStore.svelte';
	import { createServiceError } from '$lib/errors/service';
	import { BACKEND_RESTART_MESSAGE, hasBackendMethod } from '$lib/wails/backendCompatibility';
	import { addConsoleLog, updateStatus } from '$lib/stores/status.svelte';
	import FilterCombobox from '$lib/components/ui/FilterCombobox.svelte';
	import { APPLICATION } from '$lib/config/application';

	let connectionId = $state('');
	let activity = $state<database.DatabaseActivity | null>(null);
	let loading = $state(false);
	let refreshing = $state(false);
	let error = $state('');
	let search = $state('');
	let stateFilter = $state<'all' | 'active' | 'waiting' | 'idle'>('all');
	let refreshSeconds = $state(5);
	let selectedSessionId = $state('');
	let action = $state<'cancel' | 'terminate' | null>(null);
	let actionConfirmed = $state(false);
	let terminateConfirmation = $state('');
	let actionRunning = $state(false);
	let initialized = false;

	const connections = $derived(connectionStore.connections);
	const connectionOptions = $derived(
		connections.map((item) => ({
			value: item.id,
			label: `${item.name || item.database} · ${item.driver} · ${item.database}`
		}))
	);
	const refreshOptions = [
		{ value: '0', label: 'Off' },
		{ value: '5', label: '5 sec' },
		{ value: '10', label: '10 sec' },
		{ value: '30', label: '30 sec' }
	];
	const selectedSession = $derived(
		activity?.sessions.find((session) => session.id === selectedSessionId) ?? null
	);
	const filteredSessions = $derived(
		(activity?.sessions ?? []).filter((session) => {
			const term = search.trim().toLowerCase();
			if (
				term &&
				!`${session.id} ${session.user} ${session.database ?? ''} ${session.client ?? ''} ${session.state} ${session.query ?? ''}`
					.toLowerCase()
					.includes(term)
			) {
				return false;
			}
			if (stateFilter === 'waiting') return session.waiting;
			if (stateFilter === 'active') {
				return (
					!session.waiting &&
					!['idle', 'sleep'].includes(session.state.toLowerCase()) &&
					session.command?.toLowerCase() !== 'sleep'
				);
			}
			if (stateFilter === 'idle') {
				return (
					['idle', 'sleep'].includes(session.state.toLowerCase()) ||
					session.command?.toLowerCase() === 'sleep'
				);
			}
			return true;
		})
	);
	const activeCount = $derived(
		(activity?.sessions ?? []).filter(
			(session) =>
				!session.waiting &&
				!['idle', 'sleep'].includes(session.state.toLowerCase()) &&
				session.command?.toLowerCase() !== 'sleep'
		).length
	);
	const waitingCount = $derived(
		(activity?.sessions ?? []).filter((session) => session.waiting).length
	);
	const idleCount = $derived(
		(activity?.sessions ?? []).filter(
			(session) =>
				['idle', 'sleep'].includes(session.state.toLowerCase()) ||
				session.command?.toLowerCase() === 'sleep'
		).length
	);

	$effect(() => {
		if (initialized || connections.length === 0) return;
		initialized = true;
		connectionId = (connectionStore.activeConnection ?? connections[0]).id;
		void refreshActivity();
	});

	$effect(() => {
		const interval = refreshSeconds;
		if (interval <= 0) return;
		const timer = globalThis.setInterval(() => {
			if (!actionRunning) void refreshActivity(true);
		}, interval * 1000);
		return () => globalThis.clearInterval(timer);
	});

	async function refreshActivity(silent = false): Promise<void> {
		if (!connectionId || refreshing) return;
		if (!hasBackendMethod('GetDatabaseActivity')) {
			error = BACKEND_RESTART_MESSAGE;
			return;
		}
		refreshing = true;
		if (!silent) loading = true;
		error = '';
		try {
			const response = await GetDatabaseActivity(connectionId);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not load database activity');
			}
			activity = response.data ?? null;
			if (
				selectedSessionId &&
				!activity?.sessions.some((session) => session.id === selectedSessionId)
			) {
				closeDetails();
			}
		} catch (loadError: any) {
			error = loadError?.message ?? 'Could not load database activity.';
		} finally {
			if (!silent) loading = false;
			refreshing = false;
		}
	}

	function formatDuration(milliseconds: number): string {
		if (!Number.isFinite(milliseconds) || milliseconds < 1000) {
			return `${Math.max(0, Math.round(milliseconds))}ms`;
		}
		const seconds = Math.floor(milliseconds / 1000);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
		const hours = Math.floor(minutes / 60);
		return `${hours}h ${minutes % 60}m`;
	}

	function sessionState(session: database.DatabaseSession): string {
		if (session.waiting) return 'waiting';
		if (
			['idle', 'sleep'].includes(session.state.toLowerCase()) ||
			session.command?.toLowerCase() === 'sleep'
		) {
			return 'idle';
		}
		return 'active';
	}

	function openDetails(session: database.DatabaseSession): void {
		selectedSessionId = session.id;
		action = null;
		actionConfirmed = false;
		terminateConfirmation = '';
	}

	function closeDetails(): void {
		if (actionRunning) return;
		selectedSessionId = '';
		action = null;
		actionConfirmed = false;
		terminateConfirmation = '';
	}

	async function stopSession(): Promise<void> {
		if (
			!selectedSession ||
			!action ||
			selectedSession.isCurrent ||
			(action === 'cancel' && !actionConfirmed) ||
			(action === 'terminate' && terminateConfirmation !== selectedSession.id)
		) {
			return;
		}
		actionRunning = true;
		error = '';
		try {
			const terminate = action === 'terminate';
			const response = await CancelDatabaseSession(
				new database.CancelSessionRequest({
					connectionId,
					sessionId: selectedSession.id,
					terminate,
					confirmed: true
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not stop database session');
			}
			const message = terminate
				? `Terminated database session ${selectedSession.id}`
				: `Cancelled query on session ${selectedSession.id}`;
			updateStatus(message, 'success');
			addConsoleLog(message, 'success');
			action = null;
			actionConfirmed = false;
			terminateConfirmation = '';
			await refreshActivity(true);
		} catch (actionError: any) {
			error = actionError?.message ?? 'Could not stop database session.';
			updateStatus(error, 'error');
		} finally {
			actionRunning = false;
		}
	}
</script>

<div class="relative flex min-h-0 flex-1 flex-col overflow-hidden">
	<div class="flex h-13 shrink-0 items-center gap-3 border-b bg-[var(--surface-sunken)] px-4">
		<label class="flex min-w-0 flex-1 items-center gap-2">
			<span class="text-muted-foreground text-[8px] font-bold">Connection</span>
			<FilterCombobox
				id="activity-connection"
				class="max-w-sm min-w-0 flex-1"
				options={connectionOptions}
				value={connectionId}
				onChange={(value) => {
					connectionId = value;
					closeDetails();
					void refreshActivity();
				}}
				searchable={connections.length > 8}
				searchPlaceholder="Find connection…"
				triggerClass="h-8 px-2 text-[9px]"
			/>
		</label>
		<label class="flex items-center gap-2">
			<span class="text-muted-foreground text-[8px]">Auto refresh</span>
			<FilterCombobox
				id="activity-refresh"
				class="w-24"
				options={refreshOptions}
				value={String(refreshSeconds)}
				onChange={(value) => (refreshSeconds = Number(value))}
				searchable={false}
				triggerClass="h-8 px-2 text-[8px]"
			/>
		</label>
		<button
			type="button"
			class="rt-toolbar-button h-8 w-8 cursor-pointer"
			onclick={() => refreshActivity()}
			disabled={loading}
			title="Refresh activity"
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

	{#if loading && !activity}
		<div class="flex flex-1 items-center justify-center">
			<Loader2 class="text-muted-foreground h-5 w-5 animate-spin" />
		</div>
	{:else if activity && !activity.supported}
		<div class="flex flex-1 items-center justify-center p-8 text-center">
			<div class="max-w-sm">
				<Activity class="text-muted-foreground mx-auto h-7 w-7" />
				<h3 class="mt-3 text-[11px] font-bold">Server activity is not available</h3>
				<p class="text-muted-foreground mt-1 text-[8px] leading-relaxed">{activity.message}</p>
			</div>
		</div>
	{:else if activity}
		<div class="min-h-0 flex-1 overflow-y-auto p-4">
			<div class="mb-4 grid grid-cols-4 gap-3">
				<button
					type="button"
					class="rounded-xl border p-3 text-left {stateFilter === 'all'
						? 'bg-[var(--surface-sunken)]'
						: 'hover:bg-[var(--surface-hover)]'}"
					onclick={() => (stateFilter = 'all')}
				>
					<div class="flex items-center justify-between">
						<Database class="text-muted-foreground h-4 w-4" />
						<span class="text-[16px] font-bold">{activity.sessions.length}</span>
					</div>
					<p class="text-muted-foreground mt-2 text-[8px]">All sessions</p>
				</button>
				<button
					type="button"
					class="rounded-xl border p-3 text-left {stateFilter === 'active'
						? 'border-info-border bg-info-soft'
						: 'hover:bg-[var(--surface-hover)]'}"
					onclick={() => (stateFilter = 'active')}
				>
					<div class="flex items-center justify-between">
						<Zap class="text-info h-4 w-4" />
						<span class="text-[16px] font-bold">{activeCount}</span>
					</div>
					<p class="text-muted-foreground mt-2 text-[8px]">Active</p>
				</button>
				<button
					type="button"
					class="rounded-xl border p-3 text-left {stateFilter === 'waiting'
						? 'border-warning-border bg-warning-soft'
						: 'hover:bg-[var(--surface-hover)]'}"
					onclick={() => (stateFilter = 'waiting')}
				>
					<div class="flex items-center justify-between">
						<LockKeyhole class="text-warning h-4 w-4" />
						<span class="text-[16px] font-bold">{waitingCount}</span>
					</div>
					<p class="text-muted-foreground mt-2 text-[8px]">Waiting / blocked</p>
				</button>
				<button
					type="button"
					class="rounded-xl border p-3 text-left {stateFilter === 'idle'
						? 'bg-[var(--surface-sunken)]'
						: 'hover:bg-[var(--surface-hover)]'}"
					onclick={() => (stateFilter = 'idle')}
				>
					<div class="flex items-center justify-between">
						<Pause class="text-muted-foreground h-4 w-4" />
						<span class="text-[16px] font-bold">{idleCount}</span>
					</div>
					<p class="text-muted-foreground mt-2 text-[8px]">Idle</p>
				</button>
			</div>

			<div class="mb-3 flex items-center justify-between gap-3">
				<div class="rt-input flex h-8 w-72 items-center gap-2 px-2">
					<Search class="text-muted-foreground h-3.5 w-3.5" />
					<input
						class="min-w-0 flex-1 bg-transparent text-[8px] outline-none"
						bind:value={search}
						placeholder="Search PID, user, database, or query"
					/>
				</div>
				<p class="text-muted-foreground text-[7px]">
					Snapshot {new Date(activity.capturedAt).toLocaleTimeString()}
				</p>
			</div>

			<div class="overflow-hidden rounded-xl border">
				<div
					class="text-muted-foreground grid grid-cols-[72px_130px_105px_95px_minmax(220px,1fr)_84px] gap-2 border-b bg-[var(--surface-sunken)] px-3 py-2 text-[7px] font-bold tracking-[0.06em] uppercase"
				>
					<span>Session</span>
					<span>User / database</span>
					<span>State</span>
					<span>Duration</span>
					<span>Query</span>
					<span class="text-right">Client</span>
				</div>
				{#if filteredSessions.length === 0}
					<div class="text-muted-foreground p-8 text-center text-[8px]">
						No sessions match this view.
					</div>
				{:else}
					{#each filteredSessions as session, index (session.id)}
						<button
							type="button"
							class="grid w-full cursor-pointer grid-cols-[72px_130px_105px_95px_minmax(220px,1fr)_84px] items-center gap-2 px-3 py-2.5 text-left hover:bg-[var(--surface-hover)] {index >
							0
								? 'border-t'
								: ''}"
							onclick={() => openDetails(session)}
						>
							<span class="flex items-center gap-1.5 font-mono text-[8px] font-bold">
								{session.id}
								{#if session.isCurrent}
									<span class="bg-info h-1.5 w-1.5 rounded-full" title={APPLICATION.name}></span>
								{/if}
							</span>
							<span class="min-w-0">
								<span class="block truncate text-[8px] font-bold">{session.user}</span>
								<span class="text-muted-foreground block truncate text-[7px]"
									>{session.database || '-'}</span
								>
							</span>
							<span
								class="w-fit rounded-full px-2 py-1 text-[7px] font-bold {sessionState(session) ===
								'waiting'
									? 'bg-warning-soft text-warning'
									: sessionState(session) === 'active'
										? 'bg-info-soft text-info'
										: 'bg-muted text-muted-foreground'}"
							>
								{sessionState(session)}
							</span>
							<span class="text-muted-foreground font-mono text-[8px]"
								>{formatDuration(session.durationMs)}</span
							>
							<span class="truncate font-mono text-[8px]"
								>{session.query || session.command || '-'}</span
							>
							<span class="text-muted-foreground truncate text-right text-[7px]"
								>{session.client || 'local'}</span
							>
						</button>
					{/each}
				{/if}
			</div>
		</div>
	{/if}

	{#if selectedSession}
		<div class="bg-overlay/25 absolute inset-0 z-20 flex justify-end backdrop-blur-[1px]">
			<button
				type="button"
				class="absolute inset-0 cursor-default"
				onclick={closeDetails}
				aria-label="Close session details"
			></button>
			<aside
				class="relative flex h-full w-[470px] flex-col border-l bg-[var(--surface-raised)] shadow-2xl"
			>
				<header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
					<span
						class="flex h-8 w-8 items-center justify-center rounded-lg {selectedSession.waiting
							? 'bg-warning-soft text-warning'
							: 'bg-primary/10 text-primary'}"
					>
						<Activity class="h-4 w-4" />
					</span>
					<div class="min-w-0 flex-1">
						<h3 class="text-[10px] font-bold">Session {selectedSession.id}</h3>
						<p class="text-muted-foreground mt-0.5 text-[7px]">
							{selectedSession.user} · {selectedSession.database || 'no database'}
						</p>
					</div>
					<button
						type="button"
						class="rt-toolbar-button h-7 w-7 cursor-pointer"
						onclick={closeDetails}
					>
						<X class="h-3.5 w-3.5" />
					</button>
				</header>

				<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
					{#if selectedSession.isCurrent}
						<div
							class="border-info-border bg-info-soft text-info rounded-lg border px-3 py-2 text-[8px]"
						>
							This session belongs to {APPLICATION.name}. Stop its query from the query tab.
						</div>
					{/if}
					<div class="grid grid-cols-2 gap-2">
						{#each [['State', selectedSession.state || '-', Play], ['Duration', formatDuration(selectedSession.durationMs), Clock3], ['Client', selectedSession.client || 'local', UserRound], ['Application', selectedSession.application || selectedSession.command || '-', Database]] as detail}
							{@const DetailIcon = detail[2]}
							<div class="rounded-lg border p-3">
								<DetailIcon class="text-muted-foreground h-3.5 w-3.5" />
								<p class="text-muted-foreground mt-2 text-[7px]">{detail[0]}</p>
								<p class="mt-0.5 truncate text-[8px] font-bold">{detail[1]}</p>
							</div>
						{/each}
					</div>

					{#if selectedSession.waitEvent}
						<div class="border-warning-border bg-warning-soft rounded-lg border p-3">
							<div class="text-warning flex items-center gap-2 text-[8px] font-bold">
								<LockKeyhole class="h-3.5 w-3.5" />
								{selectedSession.waitEvent}
							</div>
							{#if selectedSession.blockedBy?.length}
								<p class="text-muted-foreground mt-1 text-[7px]">
									Blocked by {selectedSession.blockedBy.join(', ')}
								</p>
							{/if}
						</div>
					{/if}

					<section class="overflow-hidden rounded-xl border">
						<div class="flex items-center justify-between border-b px-3 py-2">
							<span class="text-[8px] font-bold">Current statement</span>
							<span class="text-muted-foreground font-mono text-[7px]"
								>{formatDuration(selectedSession.durationMs)}</span
							>
						</div>
						<pre
							class="rt-code-surface max-h-64 overflow-auto p-3 font-mono text-[8px] leading-relaxed whitespace-pre-wrap">{selectedSession.query ||
								'-- No statement text is available for this session.'}</pre>
					</section>

					{#if action === 'cancel'}
						<div class="border-warning-border rounded-xl border p-3">
							<p class="text-[8px] font-bold">Cancel the current statement?</p>
							<p class="text-muted-foreground mt-1 text-[7px] leading-relaxed">
								The connection stays open. Its transaction may become aborted depending on the
								engine.
							</p>
							<label class="mt-3 flex cursor-pointer items-center gap-2 text-[8px]">
								<input type="checkbox" bind:checked={actionConfirmed} />
								I reviewed session {selectedSession.id}
							</label>
						</div>
					{:else if action === 'terminate'}
						<div class="border-danger-border bg-danger-soft rounded-xl border p-3">
							<p class="text-danger text-[8px] font-bold">Terminate the entire connection?</p>
							<p class="text-muted-foreground mt-1 text-[7px] leading-relaxed">
								Any open transaction will roll back and the client will be disconnected.
							</p>
							<label class="mt-3 block">
								<span class="text-muted-foreground mb-1 block text-[7px]">
									Type <strong>{selectedSession.id}</strong> to confirm
								</span>
								<input
									class="rt-input h-8 w-full px-2 text-[8px]"
									bind:value={terminateConfirmation}
								/>
							</label>
						</div>
					{/if}
				</div>

				<footer class="shrink-0 border-t p-4">
					{#if action}
						<div class="flex gap-2">
							<button
								type="button"
								class="rt-toolbar-button h-9 flex-1 cursor-pointer text-[8px] font-bold"
								onclick={() => {
									action = null;
									actionConfirmed = false;
									terminateConfirmation = '';
								}}
							>
								Back
							</button>
							<button
								type="button"
								class="text-on-solid h-9 flex-1 cursor-pointer rounded-md text-[8px] font-bold {action ===
								'terminate'
									? 'bg-danger'
									: 'bg-warning'} disabled:cursor-not-allowed disabled:opacity-35"
								onclick={stopSession}
								disabled={actionRunning ||
									(action === 'cancel'
										? !actionConfirmed
										: terminateConfirmation !== selectedSession.id)}
							>
								{#if actionRunning}<Loader2 class="mr-2 inline h-3.5 w-3.5 animate-spin" />{/if}
								{action === 'terminate' ? 'Terminate session' : 'Cancel statement'}
							</button>
						</div>
					{:else}
						<div class="flex gap-2">
							<button
								type="button"
								class="rt-toolbar-button h-9 flex-1 cursor-pointer gap-2 text-[8px] font-bold"
								onclick={() => (action = 'cancel')}
								disabled={selectedSession.isCurrent}
							>
								<Square class="h-3 w-3 fill-current" />
								Cancel query
							</button>
							<button
								type="button"
								class="border-danger-border text-danger hover:bg-danger-soft h-9 flex-1 cursor-pointer rounded-md border text-[8px] font-bold disabled:cursor-not-allowed disabled:opacity-35"
								onclick={() => (action = 'terminate')}
								disabled={selectedSession.isCurrent}
							>
								<ServerCrash class="mr-2 inline h-3.5 w-3.5" />
								Terminate
							</button>
						</div>
					{/if}
				</footer>
			</aside>
		</div>
	{/if}
</div>
