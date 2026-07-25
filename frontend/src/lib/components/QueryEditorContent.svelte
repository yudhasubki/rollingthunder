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
		TriangleAlert,
		CircleDot,
		CheckCheck,
		Undo2,
		Square,
		Bookmark,
		ChartNoAxesCombined,
		ListChecks,
		LocateFixed,
		Settings2,
		WandSparkles,
		FolderOpen,
		Save,
		BarChart3,
		Table2
	} from 'lucide-svelte';
	import {
		BeginTransaction,
		CancelQuery,
		CommitTransaction,
		ExecuteQuery,
		ExplainQuery,
		ExportQueryResults,
		OpenSQLFile,
		RollbackTransaction,
		SaveSQLFile
	} from '$lib/wailsjs/go/db/Service';
	import { createServiceError } from '$lib/errors/service';
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
		type ExportScope,
		type ExportSettings
	} from '$lib/export/options';
	import {
		cancelExportJob,
		createInitialExportProgress,
		startExportProgressPolling
	} from '$lib/export/progress';
	import { database, db } from '$lib/wailsjs/go/models';
	import type * as Monaco from 'monaco-editor';
	import { tabsStore } from '$lib/stores/tabs.svelte';
	import type { Tab } from '$lib/models/Tab';
	import ExplainPlanViewer from '$lib/components/query/ExplainPlanViewer.svelte';
	import QueryVariablesDialog from '$lib/components/query/QueryVariablesDialog.svelte';
	import SavedQueriesDrawer from '$lib/components/query/SavedQueriesDrawer.svelte';
	import QueryToolingSettings from '$lib/components/query/QueryToolingSettings.svelte';
	import QueryResultChart from '$lib/components/query/QueryResultChart.svelte';
	import { extractQueryVariableNames, type QueryVariableInput } from '$lib/query/variables';
	import { formatSql, lintSql } from '$lib/sql/tooling';
	import { queryToolingStore } from '$lib/stores/queryTooling.svelte';
	import { getSqlIdentifierAtOffset, resolveSqlObjectTarget } from '$lib/sql/navigation';
	import { ensureColumnsForTables } from '$lib/stores/schema.svelte';
	import { getStatementAtCursor, parseTableReferences } from '$lib/sql/context';
	import type { SavedQuery } from '$lib/query/snippets';
	import { focusTrap } from '$lib/actions/focusTrap';
	import { APPLICATION, APPLICATION_EVENTS, TIME, UI_RUNTIME } from '$lib/config/application';

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
	let currentSql = $state(tab.sql || '');
	let sqlFileToken = $state(tab.sqlFileToken || '');
	let sqlFilePath = $state(tab.sqlFilePath || '');
	let sqlFileName = $state(tab.sqlFileName || '');
	let sqlFileSavedContent = $state(tab.sqlFileSavedContent ?? tab.sql ?? '');
	let sqlFileBusy = $state(false);
	const sqlFileDirty = $derived(Boolean(sqlFileName) && currentSql !== sqlFileSavedContent);

	let isRunning = $state(false);
	let queryCancelling = $state(false);
	let queryAttemptID = $state('');
	let queryElapsedSeconds = $state(0);
	let queryElapsedTimer: ReturnType<typeof setInterval> | null = null;
	let queryCancelled = $state(false);
	let queryResults = $state<Record<string, any>[]>([]);
	let queryResultSets = $state<database.QueryResultSet[]>([]);
	let activeResultSetIndex = $state(0);
	let queryResultTruncated = $state(false);
	let queryResultLimit = $state(0);
	let resultPage = $state(0);
	let resultColumns = $state<database.Structure[]>([]);
	let resultView = $state<'grid' | 'chart'>('grid');
	let errorMessage = $state<string>('');
	let errorCode = $state<string>('');
	let errorHint = $state<string>('');
	let executedQuery = $state<string>('');
	let unsafeQueryPending = $state<string>('');
	let unsafeQueryVariables = $state<database.QueryVariable[]>([]);
	let unsafeMutationDetail = $state('');
	let transactionID = $state('');
	let transactionState = $state<
		'idle' | 'starting' | 'active' | 'failed' | 'committing' | 'rolling_back'
	>('idle');
	let showHistory = $state(false);
	let autocompleteRefreshing = $state(false);
	let exportDialogOpen = $state(false);
	let exporting = $state(false);
	let exportCancelling = $state(false);
	let exportProgress = $state<database.ExportProgress | null>(null);
	let exportJobID = $state('');
	let stopExportProgressPolling: (() => void) | null = null;
	let exportInitialScope = $state<ExportScope>('loaded');
	let selectedRows = $state<Record<string, any>[]>([]);
	let selectedRowIndexes = $state<number[]>([]);
	let explainPlan = $state<database.ExplainPlan | null>(null);
	let explainLoading = $state(false);
	let savedQueriesOpen = $state(false);
	let toolingSettingsOpen = $state(false);
	let variableDialogOpen = $state(false);
	let variableNames = $state<string[]>([]);
	let pendingQueryAction = $state<'run' | 'explain' | null>(null);
	let pendingVariableQuery = $state('');
	let lintTimer: ReturnType<typeof setTimeout> | null = null;
	let queryCommandHandler: ((event: Event) => void) | null = null;
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
		const initialSql = tab.sqlFileToken
			? (tab.sql ?? '')
			: tab.sql || '-- Press Ctrl+Space for schema-aware suggestions\n\nSELECT * FROM ';
		currentSql = initialSql;

		const editorTheme = document.documentElement.classList.contains('dark') ? 'vs-dark' : 'vs';
		const modelUri = monaco.Uri.parse(
			`inmemory://${APPLICATION.id}/query/${encodeURIComponent(tab.connectionId)}/${tab.id}.sql`
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
			currentSql = editor?.getValue() || '';
			tabsStore.updateTab(tab.id, { sql: currentSql });
			scheduleLint();
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
		editor.addCommand(monaco.KeyMod.Shift | monaco.KeyMod.Alt | monaco.KeyCode.KeyF, () => {
			formatEditor();
		});
		editor.addCommand(monaco.KeyCode.F12, () => {
			void jumpToIdentifier();
		});
		editor.addCommand(monaco.KeyMod.Shift | monaco.KeyCode.F12, () => {
			findIdentifierReferences();
		});

		queryCommandHandler = (event: Event) => {
			const detail = (
				event as CustomEvent<{
					tabId?: string;
					command?: string;
				}>
			).detail;
			if (detail?.tabId && detail.tabId !== tab.id) return;
			switch (detail?.command) {
				case 'runQuery':
					void handleRun();
					break;
				case 'formatQuery':
					formatEditor();
					break;
				case 'explainQuery':
					void handleExplain();
					break;
				case 'saveQuery':
					savedQueriesOpen = true;
					break;
				case 'findReferences':
					findIdentifierReferences();
					break;
				case 'jumpToObject':
					void jumpToIdentifier();
					break;
			}
		};
		window.addEventListener(APPLICATION_EVENTS.queryCommand, queryCommandHandler);
		scheduleLint();
	});

	onDestroy(() => {
		destroyed = true;
		if (connectionSwitchHandler) {
			window.removeEventListener('connection-switched', connectionSwitchHandler);
		}
		if (queryCommandHandler) {
			window.removeEventListener(APPLICATION_EVENTS.queryCommand, queryCommandHandler);
		}
		themeObserver?.disconnect();
		focusRegistration?.dispose();
		contentChangeRegistration?.dispose();
		completionRegistration?.dispose();
		editor?.dispose();
		editorModel?.dispose();
		stopExportProgressPolling?.();
		if (queryElapsedTimer) globalThis.clearInterval(queryElapsedTimer);
		if (lintTimer) globalThis.clearTimeout(lintTimer);
		if (queryAttemptID) {
			void CancelQuery(queryAttemptID).catch(() => {});
		}
		if (transactionID) {
			void RollbackTransaction(transactionID).catch(() => {});
		}
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

		const offset = editor.getModel()?.getOffsetAt(position) || 0;
		return getStatementAtCursor(fullText, offset).text.trim();
	}

	function scheduleLint() {
		if (lintTimer) globalThis.clearTimeout(lintTimer);
		lintTimer = globalThis.setTimeout(() => {
			if (!editorModel || !monaco) return;
			const issues = lintSql(editorModel.getValue(), queryToolingStore.lint);
			monaco.editor.setModelMarkers(
				editorModel,
				`${APPLICATION.id}-sql-lint-${tab.id}`,
				issues.map((issue) => {
					const start = editorModel!.getPositionAt(issue.start);
					const end = editorModel!.getPositionAt(Math.max(issue.end, issue.start + 1));
					return {
						startLineNumber: start.lineNumber,
						startColumn: start.column,
						endLineNumber: end.lineNumber,
						endColumn: end.column,
						message: issue.message,
						code: issue.rule,
						source: `${APPLICATION.name} SQL lint`,
						severity:
							issue.severity === 'error'
								? monaco!.MarkerSeverity.Error
								: monaco!.MarkerSeverity.Warning
					};
				})
			);
		}, UI_RUNTIME.sqlLintDebounceMs);
	}

	function formatEditor() {
		if (!editor || !editorModel) return;
		const selection = editor.getSelection();
		const range = selection && !selection.isEmpty() ? selection : editorModel.getFullModelRange();
		const source = editorModel.getValueInRange(range);
		if (!source.trim()) return;
		const formatted = formatSql(source, autocompleteMetadata.dialect, queryToolingStore.format);
		editor.executeEdits(`${APPLICATION.id}-format`, [
			{
				range,
				text: formatted,
				forceMoveMarkers: true
			}
		]);
		editor.pushUndoStop();
		updateStatus('SQL formatted with the active dialect settings', 'success');
	}

	async function openSQLWorkspaceFile() {
		if (sqlFileBusy) return;
		sqlFileBusy = true;
		try {
			const response = await OpenSQLFile();
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not open SQL file');
			}
			if (!response.data?.token) return;
			const file = response.data;
			const newTabID = tabsStore.newQueryTabWithContent(tab.connectionId, file.content, file.name);
			tabsStore.updateTab(newTabID, {
				sqlFileToken: file.token,
				sqlFilePath: file.path,
				sqlFileName: file.name,
				sqlFileSavedContent: file.content
			});
			updateStatus(`Opened ${file.name}`, 'success');
		} catch (error: any) {
			updateStatus(error?.message || 'Could not open SQL file', 'error');
		} finally {
			sqlFileBusy = false;
		}
	}

	async function saveSQLWorkspaceFile(saveAs = false) {
		if (sqlFileBusy) return;
		sqlFileBusy = true;
		try {
			const content = editor?.getValue() ?? currentSql;
			const response = await SaveSQLFile(
				new db.SaveSQLFileRequest({
					token: sqlFileToken,
					content,
					saveAs,
					suggestedName: sqlFileName || `${tab.title || 'query'}.sql`
				})
			);
			if (response.errors?.length) {
				throw createServiceError(response.errors[0], 'Could not save SQL file');
			}
			if (!response.data?.token) return;
			const file = response.data;
			sqlFileToken = file.token;
			sqlFilePath = file.path;
			sqlFileName = file.name;
			sqlFileSavedContent = content;
			tabsStore.updateTab(tab.id, {
				title: file.name,
				sql: content,
				sqlFileToken: file.token,
				sqlFilePath: file.path,
				sqlFileName: file.name,
				sqlFileSavedContent: content
			});
			updateStatus(`Saved ${file.name}`, 'success');
		} catch (error: any) {
			updateStatus(error?.message || 'Could not save SQL file', 'error');
		} finally {
			sqlFileBusy = false;
		}
	}

	function findIdentifierReferences() {
		if (!editor || !editorModel) return;
		const position = editor.getPosition();
		if (!position) return;
		const identifier = getSqlIdentifierAtOffset(
			editorModel.getValue(),
			editorModel.getOffsetAt(position)
		);
		if (!identifier) {
			updateStatus('Place the cursor on an identifier first', 'warn');
			return;
		}
		const start = editorModel.getPositionAt(identifier.start);
		const end = editorModel.getPositionAt(identifier.end);
		editor.setSelection({
			startLineNumber: start.lineNumber,
			startColumn: start.column,
			endLineNumber: end.lineNumber,
			endColumn: end.column
		});
		void editor.getAction('actions.find')?.run();
		updateStatus(`Finding references to ${identifier.text}`, 'info');
	}

	async function jumpToIdentifier() {
		if (!editor || !editorModel) return;
		const position = editor.getPosition();
		if (!position) return;
		const sql = editorModel.getValue();
		const identifier = getSqlIdentifierAtOffset(sql, editorModel.getOffsetAt(position));
		if (!identifier) {
			updateStatus('Place the cursor on a table or column first', 'warn');
			return;
		}
		await ensureColumnsForTables(tab.connectionId, parseTableReferences(sql));
		const target = resolveSqlObjectTarget(
			sql,
			identifier,
			getSqlAutocompleteMetadata(tab.connectionId)
		);
		if (!target) {
			updateStatus(`No unique database object matches ${identifier.text}`, 'warn');
			return;
		}
		const targetTabID = tabsStore.newTableTab(tab.connectionId, target.schema, target.table);
		tabsStore.updateTab(targetTabID, { activeSubTab: 'structure' });
		updateStatus(
			`Opened ${target.schema}.${target.table}${target.column ? ` · ${target.column}` : ''}`,
			'success'
		);
	}

	function selectResultSet(index: number) {
		const result = queryResultSets[index];
		if (!result) return;
		activeResultSetIndex = index;
		queryResults = result.rows || [];
		queryResultTruncated = Boolean(result.truncated);
		queryResultLimit = result.rowLimit || 0;
		resultPage = 0;
		selectedRows = [];
		selectedRowIndexes = [];
		resultView = 'grid';
		executedQuery = result.statement;
		const columnNames =
			result.columns?.length > 0
				? result.columns
				: queryResults.length > 0
					? Object.keys(queryResults[0])
					: [];
		resultColumns = columnNames.map((key) => ({
			name: key,
			data_type:
				queryResults.length > 0 && typeof queryResults[0]?.[key] === 'number' ? 'number' : 'text',
			nullable: true
		})) as database.Structure[];
	}

	function prepareQueryAction(query: string, action: 'run' | 'explain') {
		const names = extractQueryVariableNames(query);
		if (names.length === 0) {
			if (action === 'run') void executeQuery(query, false, []);
			else void executeExplain(query, []);
			return;
		}
		variableNames = names;
		pendingVariableQuery = query;
		pendingQueryAction = action;
		variableDialogOpen = true;
	}

	async function submitQueryVariables(inputs: QueryVariableInput[]) {
		const variables = inputs.map(
			(input) =>
				new database.QueryVariable({
					name: input.name,
					value: input.value,
					type: input.type
				})
		);
		variableDialogOpen = false;
		const query = pendingVariableQuery;
		const action = pendingQueryAction;
		pendingVariableQuery = '';
		pendingQueryAction = null;
		if (action === 'run') await executeQuery(query, false, variables);
		if (action === 'explain') await executeExplain(query, variables);
	}

	function closeVariableDialog() {
		if (isRunning || explainLoading) return;
		variableDialogOpen = false;
		pendingVariableQuery = '';
		pendingQueryAction = null;
	}

	async function handleExplain() {
		if (!editor || isRunning || explainLoading) return;
		const query = getQueryToExecute();
		if (!query.trim()) {
			updateStatus('Enter or select one statement to explain', 'warn');
			return;
		}
		prepareQueryAction(query, 'explain');
	}

	async function executeExplain(query: string, variables: database.QueryVariable[]) {
		explainLoading = true;
		errorMessage = '';
		errorCode = '';
		errorHint = '';
		updateStatus('Building a safe estimated query plan…', 'info');
		try {
			const response = await ExplainQuery(
				new database.QueryRequest({
					connectionId: tab.connectionId,
					query,
					variables
				})
			);
			if (response.errors?.length) {
				const serviceError = response.errors[0];
				throw {
					message: serviceError.detail,
					code: serviceError.code,
					hint: serviceError.hint
				};
			}
			explainPlan = response.data || null;
			if (!explainPlan) throw new Error('The driver returned no explain plan.');
			updateStatus(
				`${explainPlan.engine} plan ready · estimates only, query was not executed`,
				'success'
			);
			addConsoleLog(`Explain plan ready: ${explainPlan.summary}`, 'info');
		} catch (error: any) {
			errorCode = error?.code || 'QUERY_FAILED';
			errorMessage = error?.message || 'Could not build explain plan';
			errorHint = error?.hint || '';
			updateStatus(errorMessage, 'error');
		} finally {
			explainLoading = false;
		}
	}

	function startQueryTimer() {
		if (queryElapsedTimer) globalThis.clearInterval(queryElapsedTimer);
		const startedAt = Date.now();
		queryElapsedSeconds = 0;
		queryElapsedTimer = globalThis.setInterval(() => {
			queryElapsedSeconds = Math.floor((Date.now() - startedAt) / TIME.millisecondsPerSecond);
		}, UI_RUNTIME.elapsedTimerTickMs);
	}

	function stopQueryTimer() {
		if (queryElapsedTimer) {
			globalThis.clearInterval(queryElapsedTimer);
			queryElapsedTimer = null;
		}
	}

	async function handleRun() {
		if (!editor || isRunning) return;
		if (transactionState === 'failed') {
			updateStatus('Roll back the failed transaction before running another query', 'warn');
			return;
		}
		if (transactionState !== 'idle' && transactionState !== 'active') {
			updateStatus('Wait for the current transaction action to finish', 'warn');
			return;
		}

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

		prepareQueryAction(query, 'run');
	}

	async function executeQuery(
		query: string,
		allowUnfilteredMutation: boolean,
		variables: database.QueryVariable[]
	) {
		if (isRunning) return;

		const attemptID = crypto.randomUUID();
		isRunning = true;
		queryCancelling = false;
		queryAttemptID = attemptID;
		queryCancelled = false;
		startQueryTimer();
		errorMessage = '';
		errorCode = '';
		errorHint = '';
		queryResults = [];
		queryResultSets = [];
		activeResultSetIndex = 0;
		explainPlan = null;
		queryResultTruncated = false;
		queryResultLimit = 0;
		resultPage = 0;
		resultColumns = [];
		selectedRows = [];
		selectedRowIndexes = [];
		executedQuery = query;
		updateStatus(
			transactionID
				? 'Executing query inside transaction…'
				: 'Executing query in auto-commit mode…',
			'info'
		);

		addConsoleLog(
			`${transactionID ? '[TX] ' : ''}Executing: ${query.replace(/\n/g, ' ').substring(0, 100)}${query.length > 100 ? '...' : ''}`,
			'info'
		);

		const startTime = Date.now();

		try {
			const response = await ExecuteQuery(
				new database.QueryRequest({
					connectionId: tab.connectionId,
					query,
					attemptId: attemptID,
					transactionId: transactionID || undefined,
					allowUnfilteredMutation,
					variables
				})
			);

			if (response.errors?.length) {
				const serviceError = response.errors[0];
				if (
					serviceError.code === 'UNFILTERED_MUTATION_REQUIRES_CONFIRMATION' &&
					!allowUnfilteredMutation
				) {
					unsafeQueryPending = query;
					unsafeQueryVariables = variables;
					unsafeMutationDetail = serviceError.detail;
					updateStatus(serviceError.detail, 'warn');
					addConsoleLog(`Safety check: ${serviceError.detail}`, 'warn');
					return;
				}
				throw {
					message: serviceError.detail,
					code: serviceError.code,
					hint: serviceError.hint
				};
			}

			const returnedSets = response.data?.resultSets || [];
			queryResultSets =
				returnedSets.length > 0
					? returnedSets
					: [
							new database.QueryResultSet({
								index: 0,
								statement: query,
								columns: response.data?.columns || [],
								rows: response.data?.rows || [],
								truncated: Boolean(response.data?.truncated),
								rowLimit: response.data?.rowLimit || 0
							})
						];
			let preferredSet = 0;
			queryResultSets.forEach((result, index) => {
				if (result.rows?.length > 0 || result.columns?.length > 0) preferredSet = index;
			});
			selectResultSet(preferredSet);

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
			} else if (queryResultSets.length > 1) {
				const totalRows = queryResultSets.reduce(
					(total, result) => total + (result.rows?.length || 0),
					0
				);
				updateStatus(
					`${queryResultSets.length} statements completed · ${totalRows.toLocaleString()} loaded rows · ${executionTime}ms`,
					'success'
				);
				addConsoleLog(
					`✓ ${queryResultSets.length} statements completed in ${executionTime}ms`,
					'info'
				);
			} else {
				updateStatus(`Query returned ${queryResults.length} rows in ${executionTime}ms`, 'info');
				addConsoleLog(`✓ Query returned ${queryResults.length} rows in ${executionTime}ms`, 'info');
			}
			addQueryToHistory(
				query,
				'success',
				queryResultSets.reduce((total, result) => total + (result.rows?.length || 0), 0),
				undefined,
				executionTime
			);
		} catch (e: any) {
			const executionTime = Date.now() - startTime;
			errorCode = e?.code || 'QUERY_FAILED';
			errorHint = e?.hint || '';
			if (errorCode === 'QUERY_CANCELLED') {
				queryCancelled = true;
				errorMessage = '';
				if (transactionID) {
					transactionState = 'failed';
				}
				updateStatus(
					transactionID
						? 'Query cancelled. Roll back the current transaction before continuing.'
						: 'Query cancelled',
					'info'
				);
				addConsoleLog(`Query cancelled after ${executionTime}ms`, 'warn');
				return;
			}

			errorMessage = e?.message ?? 'Query execution failed';
			if (
				transactionID &&
				errorCode !== 'UNFILTERED_MUTATION_REQUIRES_CONFIRMATION' &&
				errorCode !== 'TRANSACTION_CONTROL_REQUIRES_MODE'
			) {
				transactionState = 'failed';
			}
			updateStatus(errorMessage, 'error');
			addConsoleLog(
				`✗ ${errorCode}: ${errorMessage}${errorHint ? ` — ${errorHint}` : ''}`,
				'error'
			);
			addQueryToHistory(query, 'error', undefined, errorMessage, executionTime);
		} finally {
			if (queryAttemptID === attemptID) {
				queryAttemptID = '';
			}
			stopQueryTimer();
			queryCancelling = false;
			isRunning = false;
		}
	}

	async function cancelRunningQuery() {
		if (!queryAttemptID || !isRunning || queryCancelling) return;
		queryCancelling = true;
		updateStatus('Cancelling query safely…', 'info');

		try {
			const response = await CancelQuery(queryAttemptID);
			if (response.errors?.length && response.errors[0].code !== 'QUERY_NOT_RUNNING') {
				throw new Error(response.errors[0].detail);
			}
		} catch (error: any) {
			queryCancelling = false;
			updateStatus(error?.message ?? 'Failed to cancel query', 'error');
		}
	}

	async function beginTransaction() {
		if (transactionID || transactionState !== 'idle' || isRunning) return;
		const requestedID = crypto.randomUUID();
		transactionState = 'starting';
		errorMessage = '';
		errorCode = '';
		errorHint = '';
		updateStatus('Starting explicit transaction…', 'info');

		try {
			const response = await BeginTransaction(tab.connectionId, requestedID);
			if (response.errors?.length) {
				const serviceError = response.errors[0];
				throw {
					message: serviceError.detail,
					code: serviceError.code,
					hint: serviceError.hint
				};
			}
			const startedID = response.data?.id || requestedID;
			if (destroyed) {
				void RollbackTransaction(startedID).catch(() => {});
				return;
			}
			transactionID = startedID;
			transactionState = 'active';
			updateStatus('Transaction started. Changes remain pending until Commit.', 'warn');
			addConsoleLog(`Transaction ${transactionID.slice(0, 8)} started`, 'info');
		} catch (error: any) {
			transactionState = 'idle';
			errorCode = error?.code || 'TRANSACTION_FAILED';
			errorMessage = error?.message || 'Failed to start transaction';
			errorHint = error?.hint || '';
			updateStatus(errorMessage, 'error');
		}
	}

	async function commitTransaction() {
		if (!transactionID || transactionState !== 'active' || isRunning) return;
		const finishingID = transactionID;
		transactionState = 'committing';
		updateStatus('Committing transaction…', 'info');

		try {
			const response = await CommitTransaction(finishingID);
			if (response.errors?.length) {
				const serviceError = response.errors[0];
				throw {
					message: serviceError.detail,
					code: serviceError.code,
					hint: serviceError.hint
				};
			}
			updateStatus('Transaction committed', 'success');
			addConsoleLog(`Transaction ${finishingID.slice(0, 8)} committed`, 'success');
			errorMessage = '';
			errorCode = '';
			errorHint = '';
		} catch (error: any) {
			errorCode = error?.code || 'TRANSACTION_FAILED';
			errorMessage = error?.message || 'Failed to commit transaction';
			errorHint = error?.hint || '';
			updateStatus(errorMessage, 'error');
		} finally {
			transactionID = '';
			transactionState = 'idle';
		}
	}

	async function rollbackTransaction() {
		if (!transactionID || isRunning) return;
		const finishingID = transactionID;
		transactionState = 'rolling_back';
		updateStatus('Rolling back transaction…', 'info');

		try {
			const response = await RollbackTransaction(finishingID);
			if (response.errors?.length && response.errors[0].code !== 'TRANSACTION_NOT_FOUND') {
				const serviceError = response.errors[0];
				throw {
					message: serviceError.detail,
					code: serviceError.code,
					hint: serviceError.hint
				};
			}
			updateStatus('Transaction rolled back', 'success');
			addConsoleLog(`Transaction ${finishingID.slice(0, 8)} rolled back`, 'warn');
			errorMessage = '';
			errorCode = '';
			errorHint = '';
		} catch (error: any) {
			errorCode = error?.code || 'TRANSACTION_FAILED';
			errorMessage = error?.message || 'Failed to roll back transaction';
			errorHint = error?.hint || '';
			updateStatus(errorMessage, 'error');
		} finally {
			transactionID = '';
			transactionState = 'idle';
		}
	}

	function cancelUnsafeMutation() {
		unsafeQueryPending = '';
		unsafeQueryVariables = [];
		unsafeMutationDetail = '';
		updateStatus('Unfiltered mutation was not executed', 'info');
	}

	function confirmUnsafeMutation() {
		const query = unsafeQueryPending;
		const variables = unsafeQueryVariables;
		unsafeQueryPending = '';
		unsafeQueryVariables = [];
		unsafeMutationDetail = '';
		if (query) void executeQuery(query, true, variables);
	}

	function openExportDialog(preferredScope?: 'selected') {
		exportInitialScope =
			preferredScope === 'selected' && selectedRows.length > 0 ? 'selected' : 'loaded';
		exportDialogOpen = true;
	}

	function handleExportSelection(rows: Record<string, any>[], indexes: number[]) {
		selectedRows = rows;
		selectedRowIndexes = indexes;
	}

	function beginExportProgress(jobID: string, expectedRows: number) {
		stopExportProgressPolling?.();
		exportJobID = jobID;
		exportProgress = createInitialExportProgress(jobID, expectedRows);
		stopExportProgressPolling = startExportProgressPolling(jobID, (progress) => {
			exportProgress = progress;
		});
	}

	function finishExportProgress() {
		stopExportProgressPolling?.();
		stopExportProgressPolling = null;
		exportJobID = '';
		exportProgress = null;
		exportCancelling = false;
	}

	async function cancelRunningExport() {
		if (!exportJobID || !exporting || exportCancelling) return;

		exportCancelling = true;
		if (exportProgress) {
			exportProgress = new database.ExportProgress({
				...exportProgress,
				status: 'cancelling',
				cancellable: false
			});
		}

		try {
			await cancelExportJob(exportJobID);
			updateStatus('Stopping export safely…', 'info');
		} catch (error: any) {
			exportCancelling = false;
			updateStatus(error?.message ?? 'Failed to cancel export', 'error');
		}
	}

	async function handleExport(settings: ExportSettings) {
		if (queryResults.length === 0 || exporting) return;

		const rows = settings.scope === 'selected' ? selectedRows : queryResults;
		const expectedRows =
			settings.scope === 'selected' ? selectedRowIndexes.length : queryResults.length;
		if (expectedRows === 0) {
			updateStatus('Select at least one query row to export', 'warn');
			return;
		}

		exporting = true;
		exportCancelling = false;
		const extension = getExportExtension(settings.format);
		const jobID = crypto.randomUUID();
		beginExportProgress(jobID, expectedRows);
		const request = new database.RowsExportRequest({
			columns: resultColumns.map((column) => column.name),
			rows,
			jobId: jobID,
			expectedRows,
			suggestedName:
				settings.scope === 'selected'
					? `query-results-selected.${extension}`
					: `query-results.${extension}`,
			options: new database.ExportOptions(buildExportOptions(settings))
		});

		try {
			updateStatus(
				`Exporting ${expectedRows.toLocaleString()} ${
					settings.scope === 'selected' ? 'selected' : 'loaded'
				} query rows as ${settings.format.toUpperCase()}…`,
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
			finishExportProgress();
		}
	}

	function loadQueryFromHistory(item: QueryHistoryItem) {
		if (editor) {
			editor.setValue(item.query);
			showHistory = false;
		}
	}

	function loadSavedQuery(query: SavedQuery) {
		editor?.setValue(query.query);
		tabsStore.updateTab(tab.id, {
			title: query.name,
			sql: query.query,
			savedQueryId: query.id
		});
		savedQueriesOpen = false;
		updateStatus(`Loaded named query “${query.name}”`, 'info');
	}

	function handleNamedQuerySaved(query: SavedQuery) {
		tabsStore.updateTab(tab.id, {
			title: query.name,
			savedQueryId: query.id,
			sql: editor?.getValue() || query.query
		});
		updateStatus(`Saved named query “${query.name}”`, 'success');
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

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && unsafeQueryPending) cancelUnsafeMutation();
		if (tabsStore.activeTab?.id !== tab.id || !(event.metaKey || event.ctrlKey)) return;
		if (event.key.toLowerCase() === 's') {
			event.preventDefault();
			void saveSQLWorkspaceFile(event.shiftKey);
		} else if (event.key.toLowerCase() === 'o') {
			event.preventDefault();
			void openSQLWorkspaceFile();
		}
	}}
/>

<div class="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--background)] p-3">
	<!-- Toolbar -->
	<div class="mb-2 flex min-h-8 shrink-0 items-center justify-between gap-3">
		<div class="flex min-w-0 items-center gap-2">
			<span class="bg-primary/10 text-primary flex h-6 w-6 items-center justify-center rounded-md">
				<Play class="h-3 w-3" fill="currentColor" />
			</span>
			<div class="min-w-0">
				<h3 class="text-[11px] font-bold">SQL editor</h3>
				<p
					class="truncate text-[9px] {autocompleteMetadata.error
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
		<div class="flex shrink-0 items-center gap-1">
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
				class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px]"
				onclick={() => (savedQueriesOpen = true)}
				title="Save or open a named query"
			>
				<Bookmark class="h-3 w-3" />
				Saved
			</button>
			<span class="bg-border mx-0.5 h-4 w-px"></span>
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={() => void openSQLWorkspaceFile()}
				disabled={sqlFileBusy}
				title="Open SQL file (⌘O)"
				aria-label="Open SQL file"
			>
				<FolderOpen class="h-3.5 w-3.5" />
			</button>
			<button
				class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px]"
				onclick={(event) => void saveSQLWorkspaceFile(event.shiftKey)}
				disabled={sqlFileBusy}
				title="Save SQL file (⌘S). Shift-click for Save As."
			>
				{#if sqlFileBusy}
					<Loader2 class="h-3 w-3 animate-spin" />
				{:else}
					<Save class="h-3 w-3" />
				{/if}
				<span class="max-w-24 truncate">
					{sqlFileName ? `${sqlFileName}${sqlFileDirty ? ' •' : ''}` : 'Save file'}
				</span>
			</button>
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={formatEditor}
				title="Format SQL (Shift+Alt+F)"
				aria-label="Format SQL"
			>
				<WandSparkles class="h-3.5 w-3.5" />
			</button>
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={findIdentifierReferences}
				title="Find identifier references (Shift+F12)"
				aria-label="Find identifier references"
			>
				<ListChecks class="h-3.5 w-3.5" />
			</button>
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={() => void jumpToIdentifier()}
				title="Jump to database object (F12)"
				aria-label="Jump to database object"
			>
				<LocateFixed class="h-3.5 w-3.5" />
			</button>
			<button
				class="rt-toolbar-button h-7 w-7 cursor-pointer"
				onclick={() => (toolingSettingsOpen = true)}
				title="SQL tooling settings"
				aria-label="SQL tooling settings"
			>
				<Settings2 class="h-3.5 w-3.5" />
			</button>
			<span class="bg-border mx-0.5 h-4 w-px"></span>

			{#if transactionID}
				<span
					class="inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[9px] font-semibold {transactionState ===
					'failed'
						? 'border-danger-border bg-danger-soft text-danger'
						: 'border-warning-border bg-warning-soft text-warning'}"
					title={`Transaction ${transactionID}`}
				>
					{#if transactionState === 'committing' || transactionState === 'rolling_back'}
						<Loader2 class="h-3 w-3 animate-spin" />
					{:else}
						<CircleDot class="h-3 w-3" />
					{/if}
					{#if transactionState === 'failed'}
						TX failed
					{:else if transactionState === 'committing'}
						Committing
					{:else if transactionState === 'rolling_back'}
						Rolling back
					{:else}
						TX active
					{/if}
				</span>
				<button
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold disabled:pointer-events-none disabled:opacity-35"
					onclick={commitTransaction}
					disabled={isRunning || transactionState !== 'active'}
					title={transactionState === 'failed'
						? 'A failed transaction must be rolled back'
						: 'Commit all pending changes'}
				>
					<CheckCheck class="h-3 w-3" />
					Commit
				</button>
				<button
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold disabled:pointer-events-none disabled:opacity-35"
					onclick={rollbackTransaction}
					disabled={isRunning ||
						transactionState === 'committing' ||
						transactionState === 'rolling_back'}
					title="Discard all pending changes"
				>
					<Undo2 class="h-3 w-3" />
					Rollback
				</button>
			{:else}
				<button
					class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold disabled:pointer-events-none disabled:opacity-35"
					onclick={beginTransaction}
					disabled={isRunning || transactionState !== 'idle'}
					title="Start an explicit transaction for this query tab"
				>
					{#if transactionState === 'starting'}
						<Loader2 class="h-3 w-3 animate-spin" />
						Starting
					{:else}
						<CircleDot class="h-3 w-3" />
						Begin
					{/if}
				</button>
			{/if}

			<button
				class="rt-toolbar-button h-7 cursor-pointer gap-1.5 px-2 text-[9px] font-semibold disabled:pointer-events-none disabled:opacity-45"
				onclick={() => void handleExplain()}
				disabled={isRunning || explainLoading}
				title="Build an estimated explain plan without executing the query"
			>
				{#if explainLoading}
					<Loader2 class="h-3 w-3 animate-spin" />
				{:else}
					<ChartNoAxesCombined class="h-3 w-3" />
				{/if}
				Explain
			</button>

			{#if isRunning}
				<button
					class="border-danger-border bg-danger-soft text-danger hover:bg-danger-soft inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border px-3 text-[10px] font-bold transition-colors disabled:cursor-wait disabled:opacity-60"
					onclick={cancelRunningQuery}
					disabled={queryCancelling}
					title="Cancel the running query"
				>
					{#if queryCancelling}
						<Loader2 class="h-3 w-3 animate-spin" />
						Cancelling…
					{:else}
						<Square class="h-3 w-3" fill="currentColor" />
						Cancel <span class="font-mono text-[9px] opacity-75">{queryElapsedSeconds}s</span>
					{/if}
				</button>
			{:else}
				<button
					class="rt-primary-button inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[10px] font-bold disabled:pointer-events-none disabled:opacity-50"
					onclick={handleRun}
					disabled={transactionState !== 'idle' && transactionState !== 'active'}
					title={transactionState === 'failed'
						? 'Roll back the failed transaction before running another query'
						: 'Run selected or current statement'}
				>
					<Play class="h-3 w-3" fill="currentColor" />
					Run <span class="text-primary-foreground/70 text-[9px]">⌘ ↵</span>
				</button>
			{/if}
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
									<div class="bg-success h-2 w-2 rounded-full"></div>
								{:else}
									<div class="bg-danger h-2 w-2 rounded-full"></div>
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
								class="hover:bg-danger-soft hover:text-danger invisible shrink-0 rounded p-1 group-hover:visible"
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
				{explainPlan ? 'Explain plan' : 'Results'}
				{#if !explainPlan && queryResults.length > 0}
					<span class="bg-muted text-muted-foreground ml-1 rounded px-1.5 py-0.5 text-[9px]"
						>{queryResults.length.toLocaleString()}{queryResultTruncated ? '+' : ''} rows</span
					>
					{#if queryResultTruncated}
						<span
							class="bg-warning-soft text-warning ml-1 rounded px-1.5 py-0.5 text-[9px] font-semibold"
							title={`${APPLICATION.name} caps interactive query results to keep the workspace responsive`}
						>
							Limited to {queryResultLimit.toLocaleString()}
						</span>
					{/if}
				{/if}
			</h4>
			<div class="flex min-w-0 items-center gap-2">
				{#if !explainPlan && queryResults.length > 0}
					<div class="flex h-7 items-center rounded-md border bg-[var(--surface-sunken)] p-0.5">
						<button
							type="button"
							class="flex h-6 items-center gap-1.5 rounded px-2 text-[8px] font-semibold {resultView ===
							'grid'
								? 'bg-[var(--surface-raised)] shadow-sm'
								: 'text-muted-foreground'}"
							onclick={() => (resultView = 'grid')}
						>
							<Table2 class="h-3 w-3" />
							Grid
						</button>
						<button
							type="button"
							class="flex h-6 items-center gap-1.5 rounded px-2 text-[8px] font-semibold {resultView ===
							'chart'
								? 'bg-[var(--surface-raised)] shadow-sm'
								: 'text-muted-foreground'}"
							onclick={() => (resultView = 'chart')}
						>
							<BarChart3 class="h-3 w-3" />
							Chart
						</button>
					</div>
				{/if}
				{#if executedQuery && !errorMessage}
					<span class="text-muted-foreground max-w-56 truncate font-mono text-[9px]">
						{executedQuery.split('\n')[0]}…
					</span>
				{/if}
			</div>
		</div>

		{#if !explainPlan && queryResultSets.length > 1}
			<nav
				class="mb-2 flex shrink-0 items-center gap-1 overflow-x-auto rounded-lg border bg-[var(--surface-sunken)] p-1"
				aria-label="Query result sets"
			>
				{#each queryResultSets as result, index (`${result.index}-${index}`)}
					<button
						type="button"
						class="flex h-7 min-w-24 shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 text-left text-[8px] transition-colors {activeResultSetIndex ===
						index
							? 'text-foreground bg-[var(--surface-raised)] shadow-sm'
							: 'text-muted-foreground hover:text-foreground'}"
						onclick={() => selectResultSet(index)}
						title={result.statement}
					>
						<span
							class="bg-muted flex h-4 min-w-4 items-center justify-center rounded px-1 font-bold"
						>
							{index + 1}
						</span>
						<span class="max-w-28 truncate font-mono">
							{result.statement.trim().split(/\s+/).slice(0, 3).join(' ')}
						</span>
						<span class="ml-auto tabular-nums">{result.rows?.length || 0}</span>
					</button>
				{/each}
			</nav>
		{/if}

		{#if errorMessage}
			<div
				class="border-danger-border bg-danger-soft text-danger flex items-start gap-2.5 rounded-lg border p-3"
			>
				<TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" />
				<div class="min-w-0">
					<div class="flex flex-wrap items-center gap-2">
						<strong class="text-xs">{errorMessage}</strong>
						{#if errorCode}
							<span
								class="border-danger-border bg-danger-soft rounded border px-1.5 py-0.5 font-mono text-[8px] font-semibold tracking-wide"
								>{errorCode}</span
							>
						{/if}
					</div>
					{#if errorHint}
						<p class="mt-1 text-[10px] leading-relaxed opacity-85">{errorHint}</p>
					{/if}
					{#if transactionState === 'failed'}
						<p class="mt-1.5 text-[10px] font-semibold">
							This transaction cannot continue. Roll it back before running another query.
						</p>
					{/if}
				</div>
			</div>
		{:else if explainPlan}
			<ExplainPlanViewer plan={explainPlan} />
		{:else if queryResults.length > 0}
			<div class="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
				{#if queryResultTruncated}
					<div
						class="border-warning-border bg-warning-soft text-warning flex shrink-0 items-start gap-2 rounded-lg border px-3 py-2 text-[9px]"
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
					{#if resultView === 'chart'}
						<QueryResultChart data={queryResults} columns={resultColumns} />
					{:else}
						<DataGrid
							tabId={tab.id}
							columns={resultColumns}
							data={visibleQueryResults}
							totalRows={queryResults.length}
							currentPage={resultPage}
							pageSize={QUERY_RESULT_PAGE_SIZE}
							onPageChange={(page) => (resultPage = page)}
							onExport={openExportDialog}
							onSelectionChange={handleExportSelection}
							{exporting}
							gridTitle="Query results"
							detailTitle="Query result"
							readonly={true}
						/>
					{/if}
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
						<span>Query is running · {queryElapsedSeconds}s</span>
					{:else if queryCancelled}
						<Square class="h-4 w-4 opacity-50" />
						<span>Query cancelled after {queryElapsedSeconds}s</span>
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

{#if unsafeQueryPending}
	<div class="fixed inset-0 z-[90] flex items-center justify-center p-6">
		<button
			type="button"
			class="bg-overlay/45 absolute inset-0 cursor-default backdrop-blur-[1px]"
			onclick={cancelUnsafeMutation}
			aria-label="Cancel unfiltered mutation"
		></button>
		<div
			use:focusTrap
			class="rt-popover relative w-full max-w-md rounded-xl p-4"
			role="dialog"
			aria-modal="true"
			aria-labelledby="unsafe-mutation-title"
		>
			<div class="flex items-start gap-3">
				<span
					class="bg-danger-soft text-danger flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
				>
					<TriangleAlert class="h-4 w-4" />
				</span>
				<div class="min-w-0">
					<h3 id="unsafe-mutation-title" class="text-sm font-bold">Run unfiltered mutation?</h3>
					<p class="text-muted-foreground mt-1 text-[10px] leading-relaxed">
						{unsafeMutationDetail}
					</p>
				</div>
			</div>

			<pre
				class="bg-muted/60 mt-3 max-h-32 overflow-auto rounded-lg border p-3 font-mono text-[10px] leading-relaxed whitespace-pre-wrap"><code
					>{unsafeQueryPending}</code
				></pre>

			<div
				class="border-warning-border bg-warning-soft text-warning mt-3 rounded-lg border px-3 py-2 text-[9px] leading-relaxed"
			>
				{#if transactionID}
					The statement will run inside the active transaction. You can inspect the result and roll
					it back before committing.
				{:else}
					Auto-commit is active, so affected rows cannot be restored by {APPLICATION.name} after this
					statement succeeds.
				{/if}
			</div>

			<div class="mt-4 flex justify-end gap-2">
				<button
					type="button"
					class="rt-toolbar-button h-8 cursor-pointer px-3 text-[10px] font-semibold"
					onclick={cancelUnsafeMutation}
				>
					Cancel
				</button>
				<button
					type="button"
					class="bg-danger text-on-solid hover:bg-danger/90 inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md px-3 text-[10px] font-bold transition-colors"
					onclick={confirmUnsafeMutation}
				>
					<TriangleAlert class="h-3 w-3" />
					Run anyway
				</button>
			</div>
		</div>
	</div>
{/if}

<ExportDialog
	open={exportDialogOpen}
	source="query"
	pageRows={visibleQueryResults.length}
	totalRows={queryResults.length}
	selectedRows={selectedRows.length}
	initialScope={exportInitialScope}
	truncated={queryResultTruncated}
	{exporting}
	cancelling={exportCancelling}
	progress={exportProgress}
	onClose={() => (exportDialogOpen = false)}
	onCancelExport={cancelRunningExport}
	onExport={handleExport}
/>

<QueryVariablesDialog
	open={variableDialogOpen}
	names={variableNames}
	busy={isRunning || explainLoading}
	actionLabel={pendingQueryAction === 'explain' ? 'Explain query' : 'Run query'}
	onClose={closeVariableDialog}
	onSubmit={submitQueryVariables}
/>

<QueryToolingSettings
	open={toolingSettingsOpen}
	onClose={() => (toolingSettingsOpen = false)}
	onChanged={scheduleLint}
/>

<SavedQueriesDrawer
	open={savedQueriesOpen}
	currentSql={tab.sql || ''}
	engine={autocompleteMetadata.dialect}
	savedQueryId={tab.savedQueryId}
	onClose={() => (savedQueriesOpen = false)}
	onLoad={loadSavedQuery}
	onSaved={handleNamedQuerySaved}
/>
