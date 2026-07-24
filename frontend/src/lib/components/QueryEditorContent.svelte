<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		Play,
		Loader2,
		History,
		Trash2,
		Clock,
		X,
		RefreshCw,
		TriangleAlert
	} from 'lucide-svelte';
	import { ExecuteQuery, ExportQueryResults } from '$lib/wailsjs/go/db/Service';
	import { updateStatus, addConsoleLog } from '$lib/stores/status.svelte';
	import {
		addQueryToHistory,
		getQueryHistory,
		clearQueryHistory,
		deleteQueryHistoryItem,
		type QueryHistoryItem
	} from '$lib/stores/queryHistory.svelte';
	import { getSqlAutocompleteMetadata, loadSchemaInfo } from '$lib/stores/schema.svelte';
	import { registerSqlCompletionProvider } from '$lib/sql/autocomplete';
	import { getQueryResultPage, QUERY_RESULT_PAGE_SIZE } from '$lib/query/results';
	import DataGrid from '$lib/components/database/DataGrid.svelte';
	import ExportDialog from '$lib/components/database/ExportDialog.svelte';
	import {
		buildExportOptions,
		formatExportBytes,
		getExportExtension,
		type ExportSettings
	} from '$lib/export/options';
	import { database } from '$lib/wailsjs/go/models';
	import type * as Monaco from 'monaco-editor';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import type { Tab } from '$lib/models/Tab';

	interface Props {
		tab: Tab;
	}

	let { tab }: Props = $props();

	let editorContainer: HTMLDivElement;
	let editor: Monaco.editor.IStandaloneCodeEditor | null = null;
	let editorModel: Monaco.editor.ITextModel | null = null;
	let monaco: typeof Monaco | null = null;
	let themeObserver: MutationObserver | null = null;
	let completionRegistration: Monaco.IDisposable | null = null;
	let contentChangeRegistration: Monaco.IDisposable | null = null;
	let focusRegistration: Monaco.IDisposable | null = null;
	let connectionSwitchHandler: (() => void) | null = null;
	let destroyed = false;

	let isRunning = $state(false);
	let queryResults = $state<Record<string, any>[]>([]);
	let queryResultTruncated = $state(false);
	let queryResultLimit = $state(0);
	let resultPage = $state(0);
	let resultColumns = $state<database.Structure[]>([]);
	let errorMessage = $state<string>('');
	let executedQuery = $state<string>('');
	let showHistory = $state(false);
	let autocompleteRefreshing = $state(false);
	let exportDialogOpen = $state(false);
	let exporting = $state(false);
	const visibleQueryResults = $derived(getQueryResultPage(queryResults, resultPage));
	const autocompleteMetadata = $derived(getSqlAutocompleteMetadata(tab.connectionId));

	async function refreshAutocomplete(force = false, showSuggestions = false) {
		autocompleteRefreshing = true;
		try {
			await loadSchemaInfo(tab.connectionId, force);
			if (showSuggestions) {
				editor?.trigger('metadata-refresh', 'editor.action.triggerSuggest', {});
			}
		} finally {
			autocompleteRefreshing = false;
		}
	}

	onMount(async () => {
		const [, , monacoModule] = await Promise.all([
			refreshAutocomplete(),
			import('monaco-editor/esm/vs/basic-languages/sql/sql.contribution'),
			import('monaco-editor')
		]);
		if (destroyed) return;

		monaco = monacoModule;
		completionRegistration = registerSqlCompletionProvider(monaco);

		// Get initial SQL from tab if present
		const initialSql =
			tab.sql || '-- Press Ctrl+Space for schema-aware suggestions\n\nSELECT * FROM ';

		const editorTheme = document.documentElement.classList.contains('dark') ? 'vs-dark' : 'vs';
		const modelUri = monaco.Uri.parse(
			`inmemory://rollingthunder/query/${encodeURIComponent(tab.connectionId)}/${tab.id}.sql`
		);
		monaco.editor.getModel(modelUri)?.dispose();
		editorModel = monaco.editor.createModel(initialSql, 'sql', modelUri);

		editor = monaco.editor.create(editorContainer, {
			model: editorModel,
			theme: editorTheme,
			minimap: { enabled: false },
			fontSize: 12,
			fontFamily: 'JetBrains Mono, monospace',
			lineHeight: 20,
			lineNumbers: 'on',
			automaticLayout: true,
			scrollBeyondLastLine: false,
			wordWrap: 'on',
			padding: { top: 8, bottom: 8 },
			quickSuggestions: {
				other: true,
				comments: false,
				strings: false
			},
			quickSuggestionsDelay: 80,
			suggestOnTriggerCharacters: true,
			wordBasedSuggestions: 'off',
			acceptSuggestionOnEnter: 'on',
			snippetSuggestions: 'top',
			suggestSelection: 'recentlyUsedByPrefix',
			parameterHints: { enabled: true },
			suggest: {
				preview: true,
				showKeywords: true,
				showFunctions: true,
				showFields: true,
				showStructs: true,
				showModules: true,
				showSnippets: true
			}
		});

		contentChangeRegistration = editor.onDidChangeModelContent(() => {
			tabsStore.updateTab(tab.id, { sql: editor?.getValue() || '' });
		});
		focusRegistration = editor.onDidFocusEditorText(() => {
			if (autocompleteMetadata.error || autocompleteMetadata.tables.length === 0) {
				void refreshAutocomplete();
			}
		});

		connectionSwitchHandler = () => {
			void refreshAutocomplete(true);
		};
		window.addEventListener('connection-switched', connectionSwitchHandler);

		themeObserver = new MutationObserver(() => {
			monaco?.editor.setTheme(
				document.documentElement.classList.contains('dark') ? 'vs-dark' : 'vs'
			);
		});
		themeObserver.observe(document.documentElement, {
			attributes: true,
			attributeFilter: ['class']
		});

		// Add keyboard shortcut for run (Ctrl+Enter or Cmd+Enter)
		editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
			handleRun();
		});
	});

	onDestroy(() => {
		destroyed = true;
		if (connectionSwitchHandler) {
			window.removeEventListener('connection-switched', connectionSwitchHandler);
		}
		themeObserver?.disconnect();
		focusRegistration?.dispose();
		contentChangeRegistration?.dispose();
		completionRegistration?.dispose();
		editor?.dispose();
		editorModel?.dispose();
	});

	// Get the query to execute - either selected text or current statement
	function getQueryToExecute(): string {
		if (!editor) return '';

		// First check if there's selected text
		const selection = editor.getSelection();
		if (selection && !selection.isEmpty()) {
			return editor.getModel()?.getValueInRange(selection) || '';
		}

		// Otherwise, get the statement at cursor position (delimited by ;)
		const fullText = editor.getValue();
		const position = editor.getPosition();
		if (!position) return fullText;

		// Find statement boundaries
		const offset = editor.getModel()?.getOffsetAt(position) || 0;
		const statements = splitStatements(fullText);

		let currentOffset = 0;
		for (const stmt of statements) {
			const stmtEnd = currentOffset + stmt.length;
			if (offset >= currentOffset && offset <= stmtEnd) {
				return stmt.trim();
			}
			currentOffset = stmtEnd + 1; // +1 for the semicolon
		}

		// Fallback to full text
		return fullText.trim();
	}

	// Split SQL text by semicolons (respecting strings and comments)
	function splitStatements(sql: string): string[] {
		const statements: string[] = [];
		let current = '';
		let inString = false;
		let stringChar = '';
		let inLineComment = false;
		let inBlockComment = false;

		for (let i = 0; i < sql.length; i++) {
			const char = sql[i];
			const nextChar = sql[i + 1] || '';

			// Handle line comments
			if (!inString && !inBlockComment && char === '-' && nextChar === '-') {
				inLineComment = true;
				current += char;
				continue;
			}
			if (inLineComment && char === '\n') {
				inLineComment = false;
				current += char;
				continue;
			}

			// Handle block comments
			if (!inString && !inLineComment && char === '/' && nextChar === '*') {
				inBlockComment = true;
				current += char;
				continue;
			}
			if (inBlockComment && char === '*' && nextChar === '/') {
				inBlockComment = false;
				current += char;
				continue;
			}

			// Handle strings
			if (!inLineComment && !inBlockComment && (char === "'" || char === '"')) {
				if (!inString) {
					inString = true;
					stringChar = char;
				} else if (char === stringChar) {
					inString = false;
				}
			}

			// Handle semicolons
			if (!inString && !inLineComment && !inBlockComment && char === ';') {
				if (current.trim()) {
					statements.push(current);
				}
				current = '';
				continue;
			}

			current += char;
		}

		// Don't forget the last statement
		if (current.trim()) {
			statements.push(current);
		}

		return statements;
	}

	async function handleRun() {
		if (!editor || isRunning) return;

		const query = getQueryToExecute();
		if (!query) {
			updateStatus('Please enter a valid SQL query', 'warn');
			return;
		}

		// Check if query is only comments
		const strippedQuery = query
			.replace(/--.*$/gm, '') // Remove line comments
			.replace(/\/\*[\s\S]*?\*\//g, '') // Remove block comments
			.trim();

		if (!strippedQuery) {
			updateStatus('Query contains only comments', 'warn');
			return;
		}

		isRunning = true;
		errorMessage = '';
		queryResults = [];
		queryResultTruncated = false;
		queryResultLimit = 0;
		resultPage = 0;
		resultColumns = [];
		executedQuery = query;
		updateStatus('Executing query...', 'info');

		// Log query to console
		addConsoleLog(
			`Executing: ${query.replace(/\n/g, ' ').substring(0, 100)}${query.length > 100 ? '...' : ''}`,
			'info'
		);

		const startTime = Date.now();

		try {
			const response = await ExecuteQuery(tab.connectionId, query);

			if (response.errors?.length) {
				throw new Error(response.errors[0].detail);
			}

			queryResults = response.data?.rows || [];
			queryResultTruncated = Boolean(response.data?.truncated);
			queryResultLimit = response.data?.rowLimit || 0;

			// Generate columns from first result row
			if (queryResults.length > 0) {
				const firstRow = queryResults[0];
				resultColumns = Object.keys(firstRow).map((key) => ({
					name: key,
					data_type: typeof firstRow[key] === 'number' ? 'number' : 'text',
					nullable: true
				})) as database.Structure[];
			}

			const executionTime = Date.now() - startTime;
			if (queryResultTruncated) {
				const limit = queryResultLimit || queryResults.length;
				updateStatus(
					`Showing the first ${limit.toLocaleString()} rows in ${executionTime}ms — result limit reached`,
					'warn'
				);
				addConsoleLog(
					`Query returned more than ${limit.toLocaleString()} rows; only the first ${limit.toLocaleString()} are shown`,
					'warn'
				);
			} else {
				updateStatus(`Query returned ${queryResults.length} rows in ${executionTime}ms`, 'info');
				addConsoleLog(`✓ Query returned ${queryResults.length} rows in ${executionTime}ms`, 'info');
			}
			addQueryToHistory(query, 'success', queryResults.length, undefined, executionTime);
		} catch (e: any) {
			const executionTime = Date.now() - startTime;
			errorMessage = e?.message ?? 'Query execution failed';
			updateStatus(errorMessage, 'error');
			addConsoleLog(`✗ Error: ${errorMessage}`, 'error');
			addQueryToHistory(query, 'error', undefined, errorMessage, executionTime);
		} finally {
			isRunning = false;
		}
	}

	async function handleExport(settings: ExportSettings) {
		if (queryResults.length === 0 || exporting) return;

		exporting = true;
		const extension = getExportExtension(settings.format);
		const request = new database.RowsExportRequest({
			columns: resultColumns.map((column) => column.name),
			rows: queryResults,
			suggestedName: `query-results.${extension}`,
			options: new database.ExportOptions(buildExportOptions(settings))
		});

		try {
			updateStatus(
				`Exporting ${queryResults.length.toLocaleString()} loaded query rows as ${settings.format.toUpperCase()}…`,
				'info'
			);
			const response = await ExportQueryResults(request);
			if (response.errors?.length) throw new Error(response.errors[0].detail);

			if (response.data?.cancelled) {
				updateStatus('Export cancelled', 'info');
			} else if (response.data) {
				updateStatus(
					`Exported ${response.data.rows.toLocaleString()} rows (${formatExportBytes(response.data.bytes)}) to ${response.data.path}`,
					'success'
				);
			}
			exportDialogOpen = false;
		} catch (error: any) {
			updateStatus(error?.message ?? 'Failed to export query results', 'error');
		} finally {
			exporting = false;
		}
	}

	function loadQueryFromHistory(item: QueryHistoryItem) {
		if (editor) {
			editor.setValue(item.query);
			showHistory = false;
		}
	}

	function formatTimestamp(date: Date): string {
		const now = new Date();
		const diff = now.getTime() - date.getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		return date.toLocaleDateString();
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--background)] p-3">
	<!-- Toolbar -->
	<div class="mb-2 flex h-8 shrink-0 items-center justify-between">
		<div class="flex items-center gap-2">
			<span class="bg-primary/10 text-primary flex h-6 w-6 items-center justify-center rounded-md">
				<Play class="h-3 w-3" fill="currentColor" />
			</span>
			<div>
				<h3 class="text-[11px] font-bold">SQL editor</h3>
				<p
					class="text-[9px] {autocompleteMetadata.error
						? 'text-destructive'
						: 'text-muted-foreground'}"
					title={autocompleteMetadata.error || 'Schema-aware SQL autocomplete'}
				>
					{#if autocompleteRefreshing || autocompleteMetadata.isLoading}
						Indexing database metadata…
					{:else if autocompleteMetadata.error}
						Autocomplete metadata unavailable
					{:else}
						{autocompleteMetadata.engine} autocomplete · {autocompleteMetadata.tables.length} tables
					{/if}
				</p>
			</div>
		</div>
		<div class="flex items-center gap-1">
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer disabled:cursor-wait disabled:opacity-50"
				onclick={() => refreshAutocomplete(true, true)}
				disabled={autocompleteRefreshing || autocompleteMetadata.isLoading}
				title="Refresh SQL autocomplete metadata"
				aria-label="Refresh SQL autocomplete metadata"
			>
				<RefreshCw
					class="h-3 w-3 {autocompleteRefreshing || autocompleteMetadata.isLoading
						? 'animate-spin'
						: ''}"
				/>
			</button>
			<button
				class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2.5 text-[10px]"
				onclick={() => (showHistory = !showHistory)}
			>
				<History class="h-3 w-3" />
				History
			</button>
			<button
				class="rt-primary-button inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[10px] font-bold disabled:pointer-events-none disabled:opacity-50"
				onclick={handleRun}
				disabled={isRunning}
			>
				{#if isRunning}
					<Loader2 class="h-3 w-3 animate-spin" />
					Running…
				{:else}
					<Play class="h-3 w-3" fill="currentColor" />
					Run <span class="text-primary-foreground/70 text-[9px]">⌘ ↵</span>
				{/if}
			</button>
		</div>
	</div>

	<!-- History Panel (slide-out) -->
	{#if showHistory}
		<div
			class="mb-2 max-h-52 overflow-auto rounded-lg border bg-[var(--surface-raised)] p-2 shadow-sm"
		>
			<div class="mb-2 flex items-center justify-between">
				<span class="text-[11px] font-bold">Query history</span>
				<div class="flex items-center gap-1">
					{#if getQueryHistory().length > 0}
						<button
							class="hover:bg-destructive/10 hover:text-destructive rounded p-1 text-xs transition-colors"
							onclick={clearQueryHistory}
							title="Clear all history"
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					{/if}
					<button
						class="hover:bg-accent rounded p-1 transition-colors"
						onclick={() => (showHistory = false)}
					>
						<X class="h-3.5 w-3.5" />
					</button>
				</div>
			</div>
			{#if getQueryHistory().length === 0}
				<div class="text-muted-foreground py-4 text-center text-[11px]">No query history yet</div>
			{:else}
				<div class="space-y-1">
					{#each getQueryHistory() as item (item.id)}
						<div
							role="button"
							tabindex="0"
							class="hover:bg-accent group flex w-full cursor-pointer items-start gap-2 rounded-md p-2 text-left transition-colors"
							onclick={() => loadQueryFromHistory(item)}
							onkeydown={(e) => e.key === 'Enter' && loadQueryFromHistory(item)}
						>
							<div class="mt-0.5 shrink-0">
								{#if item.status === 'success'}
									<div class="h-2 w-2 rounded-full bg-green-500"></div>
								{:else}
									<div class="h-2 w-2 rounded-full bg-red-500"></div>
								{/if}
							</div>
							<div class="min-w-0 flex-1">
								<div class="truncate font-mono text-[10px]">
									{item.query.substring(0, 80)}{item.query.length > 80 ? '...' : ''}
								</div>
								<div class="text-muted-foreground mt-0.5 flex items-center gap-2 text-[9px]">
									<span class="flex items-center gap-1">
										<Clock class="h-3 w-3" />
										{formatTimestamp(item.timestamp)}
									</span>
									{#if item.rowCount !== undefined}
										<span>{item.rowCount} rows</span>
									{/if}
									{#if item.executionTime !== undefined}
										<span>{item.executionTime}ms</span>
									{/if}
								</div>
							</div>
							<button
								class="invisible shrink-0 rounded p-1 group-hover:visible hover:bg-red-100 hover:text-red-600"
								onclick={(e) => {
									e.stopPropagation();
									deleteQueryHistoryItem(item.id);
								}}
								title="Delete"
							>
								<Trash2 class="h-3 w-3" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Editor -->
	<div class="h-52 flex-shrink-0 overflow-hidden rounded-lg border bg-[var(--code-bg)] shadow-sm">
		<div bind:this={editorContainer} class="h-full w-full"></div>
	</div>

	<!-- Results -->
	<div class="mt-3 flex min-h-0 flex-1 flex-col">
		<div class="mb-2 flex h-6 items-center justify-between">
			<h4 class="text-[11px] font-bold">
				Results
				{#if queryResults.length > 0}
					<span class="bg-muted text-muted-foreground ml-1 rounded px-1.5 py-0.5 text-[9px]"
						>{queryResults.length.toLocaleString()}{queryResultTruncated ? '+' : ''} rows</span
					>
					{#if queryResultTruncated}
						<span
							class="ml-1 rounded bg-amber-500/10 px-1.5 py-0.5 text-[9px] font-semibold text-amber-600 dark:text-amber-400"
							title="Rolling Thunder caps interactive query results to keep the workspace responsive"
						>
							Limited to {queryResultLimit.toLocaleString()}
						</span>
					{/if}
				{/if}
			</h4>
			{#if executedQuery && !errorMessage}
				<span class="text-muted-foreground max-w-md truncate font-mono text-[9px]">
					{executedQuery.split('\n')[0]}…
				</span>
			{/if}
		</div>

		{#if errorMessage}
			<div class="rounded-lg border border-red-500/40 bg-red-500/10 p-3 text-xs text-red-500">
				<strong>Error:</strong>
				{errorMessage}
			</div>
		{:else if queryResults.length > 0}
			<div class="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
				{#if queryResultTruncated}
					<div
						class="flex shrink-0 items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[9px] text-amber-700 dark:text-amber-300"
					>
						<TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" />
						<span>
							Showing the first {queryResultLimit.toLocaleString()} rows. Add a smaller
							<code class="font-mono font-semibold">LIMIT</code> to the query for faster, deterministic
							exploration. Interactive results remain capped even if the SQL asks for more rows.
						</span>
					</div>
				{/if}
				<div class="min-h-0 flex-1 overflow-hidden">
					<DataGrid
						tabId={tab.id}
						columns={resultColumns}
						data={visibleQueryResults}
						totalRows={queryResults.length}
						currentPage={resultPage}
						pageSize={QUERY_RESULT_PAGE_SIZE}
						onPageChange={(page) => (resultPage = page)}
						onExport={() => (exportDialogOpen = true)}
						{exporting}
						gridTitle="Query results"
						detailTitle="Query result"
						readonly={true}
					/>
				</div>
			</div>
		{:else}
			<div
				class="text-muted-foreground relative flex h-32 items-center justify-center overflow-hidden rounded-lg border border-dashed bg-[var(--surface-raised)] text-xs"
			>
				<div class="rt-empty-grid pointer-events-none absolute inset-0 opacity-60"></div>
				<div class="relative flex flex-col items-center gap-2">
					{#if isRunning}
						<Loader2 class="h-4 w-4 animate-spin opacity-50" />
						<span>Query is running…</span>
					{:else}
						<Play class="h-4 w-4 opacity-50" />
						<span
							>{executedQuery ? 'Query completed with no rows' : 'Run a query to see results'}</span
						>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>

<ExportDialog
	open={exportDialogOpen}
	source="query"
	pageRows={visibleQueryResults.length}
	totalRows={queryResults.length}
	truncated={queryResultTruncated}
	{exporting}
	onClose={() => (exportDialogOpen = false)}
	onExport={handleExport}
/>
