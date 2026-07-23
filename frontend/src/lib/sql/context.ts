export interface ParsedTableReference {
	schema?: string;
	table: string;
	alias?: string;
}

export interface StatementAtCursor {
	text: string;
	cursorOffset: number;
}

export type CompletionContextKind = 'table' | 'column' | 'general';

export interface CompletionContext {
	kind: CompletionContextKind;
	qualifier?: string;
	tableReferences: ParsedTableReference[];
	ctes: string[];
	useLowercaseKeywords: boolean;
}

type SqlLexicalState =
	| 'code'
	| 'single-quote'
	| 'double-quote'
	| 'backtick'
	| 'bracket'
	| 'line-comment'
	| 'block-comment'
	| 'dollar-quote';

interface MaskedSql {
	text: string;
	state: SqlLexicalState;
}

const IDENTIFIER = '(?:"(?:[^"]|"")*"|`(?:[^`]|``)*`|\\[[^\\]]+\\]|[A-Za-z_][A-Za-z0-9_$]*)';

const CLAUSE_WORDS = new Set([
	'WHERE',
	'JOIN',
	'LEFT',
	'RIGHT',
	'INNER',
	'OUTER',
	'FULL',
	'CROSS',
	'ON',
	'GROUP',
	'ORDER',
	'HAVING',
	'LIMIT',
	'OFFSET',
	'UNION',
	'RETURNING',
	'SET',
	'VALUES',
	'WINDOW',
	'FETCH',
	'FOR'
]);

function readDollarQuoteDelimiter(sql: string, index: number): string | null {
	if (sql[index] !== '$') return null;
	if (index > 0 && /[A-Za-z0-9_$]/.test(sql[index - 1])) return null;
	return sql.slice(index).match(/^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$/)?.[0] ?? null;
}

function maskSql(sql: string, maskQuotedIdentifiers: boolean): MaskedSql {
	// Keep UTF-16 code-unit indexes aligned with Monaco's cursor offsets.
	const masked = sql.split('');
	let state: SqlLexicalState = 'code';
	let blockCommentDepth = 0;
	let dollarQuoteDelimiter = '';

	const hide = (index: number, shouldHide = true) => {
		if (shouldHide && masked[index] !== '\n' && masked[index] !== '\r') {
			masked[index] = ' ';
		}
	};

	for (let index = 0; index < sql.length; index += 1) {
		const character = sql[index];
		const next = sql[index + 1] || '';

		if (state === 'line-comment') {
			if (character === '\n' || character === '\r') {
				state = 'code';
			} else {
				hide(index);
			}
			continue;
		}

		if (state === 'block-comment') {
			hide(index);
			if (character === '/' && next === '*') {
				hide(index + 1);
				blockCommentDepth += 1;
				index += 1;
			} else if (character === '*' && next === '/') {
				hide(index + 1);
				blockCommentDepth -= 1;
				index += 1;
				if (blockCommentDepth === 0) state = 'code';
			}
			continue;
		}

		if (state === 'dollar-quote') {
			if (sql.startsWith(dollarQuoteDelimiter, index)) {
				for (let offset = 0; offset < dollarQuoteDelimiter.length; offset += 1) {
					hide(index + offset);
				}
				index += dollarQuoteDelimiter.length - 1;
				dollarQuoteDelimiter = '';
				state = 'code';
			} else {
				hide(index);
			}
			continue;
		}

		if (state === 'single-quote') {
			hide(index);
			if (character === '\\' && next) {
				hide(index + 1);
				index += 1;
			} else if (character === "'" && next === "'") {
				hide(index + 1);
				index += 1;
			} else if (character === "'") {
				state = 'code';
			}
			continue;
		}

		if (state === 'double-quote') {
			hide(index, maskQuotedIdentifiers);
			if (character === '"' && next === '"') {
				hide(index + 1, maskQuotedIdentifiers);
				index += 1;
			} else if (character === '"') {
				state = 'code';
			}
			continue;
		}

		if (state === 'backtick') {
			hide(index, maskQuotedIdentifiers);
			if (character === '`' && next === '`') {
				hide(index + 1, maskQuotedIdentifiers);
				index += 1;
			} else if (character === '`') {
				state = 'code';
			}
			continue;
		}

		if (state === 'bracket') {
			hide(index, maskQuotedIdentifiers);
			if (character === ']' && next === ']') {
				hide(index + 1, maskQuotedIdentifiers);
				index += 1;
			} else if (character === ']') {
				state = 'code';
			}
			continue;
		}

		if (character === '-' && next === '-') {
			hide(index);
			hide(index + 1);
			state = 'line-comment';
			index += 1;
			continue;
		}
		if (character === '#') {
			hide(index);
			state = 'line-comment';
			continue;
		}
		if (character === '/' && next === '*') {
			hide(index);
			hide(index + 1);
			state = 'block-comment';
			blockCommentDepth = 1;
			index += 1;
			continue;
		}
		if (character === "'") {
			hide(index);
			state = 'single-quote';
			continue;
		}
		if (character === '"') {
			hide(index, maskQuotedIdentifiers);
			state = 'double-quote';
			continue;
		}
		if (character === '`') {
			hide(index, maskQuotedIdentifiers);
			state = 'backtick';
			continue;
		}
		if (character === '[') {
			hide(index, maskQuotedIdentifiers);
			state = 'bracket';
			continue;
		}

		const delimiter = readDollarQuoteDelimiter(sql, index);
		if (delimiter) {
			for (let offset = 0; offset < delimiter.length; offset += 1) {
				hide(index + offset);
			}
			index += delimiter.length - 1;
			dollarQuoteDelimiter = delimiter;
			state = 'dollar-quote';
		}
	}

	return { text: masked.join(''), state };
}

export function unquoteIdentifier(value: string): string {
	const trimmed = value.trim();
	if (
		(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
		(trimmed.startsWith('`') && trimmed.endsWith('`')) ||
		(trimmed.startsWith('[') && trimmed.endsWith(']'))
	) {
		return trimmed.slice(1, -1).replace(/""/g, '"').replace(/``/g, '`').replace(/]]/g, ']');
	}
	return trimmed;
}

export function normalizeIdentifier(value: string): string {
	return unquoteIdentifier(value).toLowerCase();
}

function splitQualifiedIdentifier(value: string): string[] {
	return value
		.split(/\s*\.\s*/)
		.map(unquoteIdentifier)
		.filter(Boolean);
}

export function removeSqlNoise(sql: string): string {
	return maskSql(sql, false).text;
}

export function getStatementAtCursor(sql: string, cursorOffset: number): StatementAtCursor {
	const clampedCursorOffset = Math.max(0, Math.min(cursorOffset, sql.length));
	const statementSql = maskSql(sql, true).text;
	let statementStart = 0;
	let statementEnd = sql.length;

	for (let index = 0; index < statementSql.length; index += 1) {
		if (statementSql[index] !== ';') continue;

		if (index < clampedCursorOffset) {
			statementStart = index + 1;
		} else {
			statementEnd = index;
			break;
		}
	}

	return {
		text: sql.slice(statementStart, statementEnd),
		cursorOffset: Math.max(0, clampedCursorOffset - statementStart)
	};
}

export function isCursorInCommentOrString(sqlBeforeCursor: string): boolean {
	const { state } = maskSql(sqlBeforeCursor, false);
	return ['single-quote', 'line-comment', 'block-comment', 'dollar-quote'].includes(state);
}

export function parseTableReferences(statement: string): ParsedTableReference[] {
	const clean = removeSqlNoise(statement);
	const expression = new RegExp(
		`\\b(?:FROM|JOIN|UPDATE|INTO|USING)\\s+(${IDENTIFIER}(?:\\s*\\.\\s*${IDENTIFIER})?)(?:\\s+(?:AS\\s+)?(${IDENTIFIER}))?`,
		'gi'
	);
	const references: ParsedTableReference[] = [];

	for (const match of clean.matchAll(expression)) {
		const parts = splitQualifiedIdentifier(match[1]);
		if (parts.length === 0) continue;

		const possibleAlias = match[2] ? unquoteIdentifier(match[2]) : undefined;
		const alias =
			possibleAlias && !CLAUSE_WORDS.has(possibleAlias.toUpperCase()) ? possibleAlias : undefined;

		references.push({
			schema: parts.length > 1 ? parts.at(-2) : undefined,
			table: parts.at(-1) || '',
			alias
		});
	}

	return references.filter((reference) => reference.table);
}

export function parseCteNames(statement: string): string[] {
	const clean = removeSqlNoise(statement);
	const expression = new RegExp(
		`(?:\\bWITH(?:\\s+RECURSIVE)?|,)\\s*(${IDENTIFIER})\\s*(?:\\([^)]*\\))?\\s+AS\\s*\\(`,
		'gi'
	);
	return [...clean.matchAll(expression)].map((match) => unquoteIdentifier(match[1]));
}

export function inferLowercaseKeywords(sqlBeforeCursor: string): boolean {
	const currentWord = sqlBeforeCursor.match(/[A-Za-z]+$/)?.[0];
	if (currentWord && currentWord.length > 1) {
		if (currentWord === currentWord.toLowerCase()) return true;
		if (currentWord === currentWord.toUpperCase()) return false;
	}

	const samples =
		sqlBeforeCursor.match(/\b(?:select|from|where|join|insert|update|delete)\b/gi) || [];
	const lowercase = samples.filter((word) => word === word.toLowerCase()).length;
	const uppercase = samples.filter((word) => word === word.toUpperCase()).length;
	return lowercase > uppercase;
}

export function analyzeCompletionContext(statement: StatementAtCursor): CompletionContext {
	const cleanStatement = removeSqlNoise(statement.text);
	const beforeCursor = cleanStatement.slice(0, statement.cursorOffset);
	const tableReferences = parseTableReferences(cleanStatement);
	const ctes = parseCteNames(cleanStatement);
	const qualifierMatch = beforeCursor.match(
		new RegExp(`(${IDENTIFIER})\\s*\\.\\s*[A-Za-z0-9_$]*$`, 'i')
	);

	if (qualifierMatch) {
		return {
			kind: 'column',
			qualifier: unquoteIdentifier(qualifierMatch[1]),
			tableReferences,
			ctes,
			useLowercaseKeywords: inferLowercaseKeywords(beforeCursor)
		};
	}

	const tableContextMatch = beforeCursor.match(
		new RegExp(
			`\\b(?:FROM|JOIN|UPDATE|INTO|TABLE|REFERENCES|USING)\\s+(?:(${IDENTIFIER})\\s*\\.\\s*)?[A-Za-z0-9_$"\\x60\\[\\]]*$`,
			'i'
		)
	);
	if (tableContextMatch) {
		return {
			kind: 'table',
			qualifier: tableContextMatch[1] ? unquoteIdentifier(tableContextMatch[1]) : undefined,
			tableReferences,
			ctes,
			useLowercaseKeywords: inferLowercaseKeywords(beforeCursor)
		};
	}

	const insertColumnsExpression = new RegExp(
		`\\bINSERT\\s+INTO\\s+${IDENTIFIER}(?:\\s*\\.\\s*${IDENTIFIER})?\\s*\\([^)]*$`,
		'i'
	);
	const lastClause =
		[
			...beforeCursor.matchAll(
				/\b(SELECT|WHERE|ON|HAVING|RETURNING|SET|GROUP\s+BY|ORDER\s+BY|FROM|JOIN|VALUES)\b/gi
			)
		]
			.at(-1)?.[1]
			.toUpperCase() || '';

	const isColumnContext =
		insertColumnsExpression.test(beforeCursor) ||
		['SELECT', 'WHERE', 'ON', 'HAVING', 'RETURNING', 'SET', 'GROUP BY', 'ORDER BY'].includes(
			lastClause
		);

	return {
		kind: isColumnContext ? 'column' : 'general',
		tableReferences,
		ctes,
		useLowercaseKeywords: inferLowercaseKeywords(beforeCursor)
	};
}
