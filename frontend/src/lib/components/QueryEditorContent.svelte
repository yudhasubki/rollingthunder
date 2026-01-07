<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Play, Loader2 } from 'lucide-svelte';
	import { ExecuteQuery } from '$lib/wailsjs/go/db/Service';
	import { updateStatus, addConsoleLog } from '$lib/stores/status.svelte';
	import {
		getAllTables,
		getAllColumns,
		getSQLKeywords,
		loadSchemaInfo
	} from '$lib/stores/schema.svelte';
	import DataGrid from '$lib/components/database/DataGrid.svelte';
	import { database } from '$lib/wailsjs/go/models';
	import type * as Monaco from 'monaco-editor';

	let editorContainer: HTMLDivElement;
	let editor: Monaco.editor.IStandaloneCodeEditor | null = null;
	let monaco: typeof Monaco | null = null;

	let isRunning = $state(false);
	let queryResults = $state<Record<string, any>[]>([]);
	let resultColumns = $state<database.Structure[]>([]);
	let errorMessage = $state<string>('');
	let executedQuery = $state<string>('');

	onMount(async () => {
		// Load schema info for autocomplete
		loadSchemaInfo();

		// Dynamic import Monaco to avoid SSR issues
		monaco = await import('monaco-editor');

		// Configure SQL language with custom completions
		monaco.languages.registerCompletionItemProvider('sql', {
			provideCompletionItems: (model, position) => {
				const word = model.getWordUntilPosition(position);
				const range = {
					startLineNumber: position.lineNumber,
					endLineNumber: position.lineNumber,
					startColumn: word.startColumn,
					endColumn: word.endColumn
				};

				const suggestions: Monaco.languages.CompletionItem[] = [];

				// Add SQL keywords
				for (const keyword of getSQLKeywords()) {
					suggestions.push({
						label: keyword,
						kind: monaco!.languages.CompletionItemKind.Keyword,
						insertText: keyword,
						range
					});
				}

				// Add table names
				for (const table of getAllTables()) {
					suggestions.push({
						label: table,
						kind: monaco!.languages.CompletionItemKind.Class,
						insertText: table,
						detail: 'Table',
						range
					});
				}

				// Add column names
				for (const column of getAllColumns()) {
					suggestions.push({
						label: column,
						kind: monaco!.languages.CompletionItemKind.Field,
						insertText: column,
						detail: 'Column',
						range
					});
				}

				return { suggestions };
			}
		});

		// Create editor with dark theme
		editor = monaco.editor.create(editorContainer, {
			value:
				'-- Write your SQL query here\n-- Use semicolons to separate multiple queries\n-- Select text to run only that portion\n\nSELECT * FROM ',
			language: 'sql',
			theme: 'vs-dark',
			minimap: { enabled: false },
			fontSize: 14,
			fontFamily: 'JetBrains Mono, monospace',
			lineNumbers: 'on',
			automaticLayout: true,
			scrollBeyondLastLine: false,
			wordWrap: 'on',
			padding: { top: 8, bottom: 8 }
		});

		// Add keyboard shortcut for run (Ctrl+Enter or Cmd+Enter)
		editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
			handleRun();
		});
	});

	onDestroy(() => {
		editor?.dispose();
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
		resultColumns = [];
		executedQuery = query;
		updateStatus('Executing query...', 'info');

		// Log query to console
		addConsoleLog(
			`Executing: ${query.replace(/\n/g, ' ').substring(0, 100)}${query.length > 100 ? '...' : ''}`,
			'info'
		);

		try {
			const response = await ExecuteQuery(query);

			if (response.errors?.length) {
				throw new Error(response.errors[0].detail);
			}

			queryResults = response.data || [];

			// Generate columns from first result row
			if (queryResults.length > 0) {
				const firstRow = queryResults[0];
				resultColumns = Object.keys(firstRow).map((key) => ({
					name: key,
					data_type: typeof firstRow[key] === 'number' ? 'number' : 'text',
					nullable: true
				})) as database.Structure[];
			}

			updateStatus(`Query returned ${queryResults.length} rows`, 'info');
			addConsoleLog(`✓ Query returned ${queryResults.length} rows`, 'info');
		} catch (e: any) {
			errorMessage = e?.message ?? 'Query execution failed';
			updateStatus(errorMessage, 'error');
			addConsoleLog(`✗ Error: ${errorMessage}`, 'error');
		} finally {
			isRunning = false;
		}
	}
</script>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden p-4">
	<!-- Toolbar -->
	<div class="mb-2 flex items-center justify-between">
		<div class="flex items-center gap-2">
			<h3 class="text-sm font-medium">SQL Query</h3>
			<span class="text-muted-foreground text-xs"> (select text to run specific query) </span>
		</div>
		<Button size="sm" class="gap-1.5" onclick={handleRun} disabled={isRunning}>
			{#if isRunning}
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
				Running...
			{:else}
				<Play class="h-3.5 w-3.5" />
				Run <span class="text-muted-foreground text-xs">(Ctrl+Enter)</span>
			{/if}
		</Button>
	</div>

	<!-- Editor -->
	<div class="h-48 flex-shrink-0 overflow-hidden rounded-md border">
		<div bind:this={editorContainer} class="h-full w-full"></div>
	</div>

	<!-- Results -->
	<div class="mt-4 flex min-h-0 flex-1 flex-col">
		<div class="mb-2 flex items-center justify-between">
			<h4 class="text-muted-foreground text-sm font-medium">
				Results
				{#if queryResults.length > 0}
					<span class="text-xs">({queryResults.length} rows)</span>
				{/if}
			</h4>
			{#if executedQuery && !errorMessage}
				<span class="text-muted-foreground max-w-md truncate font-mono text-xs">
					{executedQuery.split('\n')[0]}...
				</span>
			{/if}
		</div>

		{#if errorMessage}
			<div class="rounded-md border border-red-500/50 bg-red-500/10 p-4 text-sm text-red-500">
				<strong>Error:</strong>
				{errorMessage}
			</div>
		{:else if queryResults.length > 0}
			<div class="min-h-0 flex-1 overflow-hidden">
				<DataGrid
					columns={resultColumns}
					data={queryResults}
					totalRows={queryResults.length}
					currentPage={0}
					pageSize={100}
					onPageChange={() => {}}
					readonly={true}
				/>
			</div>
		{:else}
			<div
				class="text-muted-foreground flex h-32 items-center justify-center rounded-md border border-dashed"
			>
				Run a query to see results
			</div>
		{/if}
	</div>
</div>
