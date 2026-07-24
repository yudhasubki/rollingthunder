import { removeSqlNoise } from './context.ts';
import type { SqlDialect } from './dialects.ts';

export interface SqlFormatSettings {
	keywordCase: 'upper' | 'lower' | 'preserve';
	indentSize: 2 | 4;
}

export interface SqlLintSettings {
	requireWhereForMutations: boolean;
	disallowSelectStar: boolean;
	requireSemicolon: boolean;
	maxLineLength: number;
}

export interface SqlLintIssue {
	rule: string;
	message: string;
	severity: 'warning' | 'error';
	start: number;
	end: number;
}

export const defaultSqlFormatSettings: SqlFormatSettings = {
	keywordCase: 'upper',
	indentSize: 2
};

export const defaultSqlLintSettings: SqlLintSettings = {
	requireWhereForMutations: true,
	disallowSelectStar: false,
	requireSemicolon: false,
	maxLineLength: 120
};

const SQL_KEYWORDS = new Set(
	[
		'select',
		'from',
		'where',
		'join',
		'left',
		'right',
		'full',
		'inner',
		'outer',
		'cross',
		'on',
		'and',
		'or',
		'group',
		'by',
		'order',
		'having',
		'limit',
		'offset',
		'fetch',
		'union',
		'all',
		'insert',
		'into',
		'values',
		'update',
		'set',
		'delete',
		'returning',
		'as',
		'with',
		'recursive',
		'distinct',
		'case',
		'when',
		'then',
		'else',
		'end',
		'is',
		'not',
		'null',
		'true',
		'false',
		'like',
		'in',
		'exists',
		'between',
		'asc',
		'desc',
		'nulls',
		'first',
		'last',
		'over',
		'partition',
		'window',
		'conflict',
		'duplicate',
		'key',
		'do',
		'nothing'
	].map((keyword) => keyword.toLowerCase())
);

const BREAK_BEFORE = new Set([
	'SELECT',
	'FROM',
	'WHERE',
	'GROUP BY',
	'ORDER BY',
	'HAVING',
	'LIMIT',
	'OFFSET',
	'FETCH',
	'UNION',
	'INSERT',
	'UPDATE',
	'DELETE',
	'VALUES',
	'RETURNING',
	'LEFT JOIN',
	'RIGHT JOIN',
	'FULL JOIN',
	'INNER JOIN',
	'CROSS JOIN',
	'JOIN'
]);

interface SqlToken {
	value: string;
	kind: 'word' | 'quoted' | 'comment' | 'punctuation' | 'operator';
}

function tokenizeForFormatting(sql: string): SqlToken[] {
	const tokens: SqlToken[] = [];
	for (let index = 0; index < sql.length; ) {
		const character = sql[index];
		const next = sql[index + 1] || '';
		if (/\s/.test(character)) {
			index += 1;
			continue;
		}
		if (character === '-' && next === '-') {
			const end = sql.indexOf('\n', index);
			const stop = end < 0 ? sql.length : end;
			tokens.push({ value: sql.slice(index, stop), kind: 'comment' });
			index = stop;
			continue;
		}
		if (character === '/' && next === '*') {
			const end = sql.indexOf('*/', index + 2);
			const stop = end < 0 ? sql.length : end + 2;
			tokens.push({ value: sql.slice(index, stop), kind: 'comment' });
			index = stop;
			continue;
		}
		if (character === "'" || character === '"' || character === '`' || character === '[') {
			const close = character === '[' ? ']' : character;
			let stop = index + 1;
			while (stop < sql.length) {
				if (sql[stop] === close && sql[stop + 1] === close) {
					stop += 2;
					continue;
				}
				if (sql[stop] === close) {
					stop += 1;
					break;
				}
				stop += 1;
			}
			tokens.push({ value: sql.slice(index, stop), kind: 'quoted' });
			index = stop;
			continue;
		}
		if (character === '$') {
			const delimiter = sql.slice(index).match(/^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$/)?.[0];
			if (delimiter) {
				const bodyEnd = sql.indexOf(delimiter, index + delimiter.length);
				const stop = bodyEnd < 0 ? sql.length : bodyEnd + delimiter.length;
				tokens.push({ value: sql.slice(index, stop), kind: 'quoted' });
				index = stop;
				continue;
			}
		}
		const word = sql.slice(index).match(/^[A-Za-z_][A-Za-z0-9_$]*/)?.[0];
		if (word) {
			tokens.push({ value: word, kind: 'word' });
			index += word.length;
			continue;
		}
		if ('(),.;'.includes(character)) {
			tokens.push({ value: character, kind: 'punctuation' });
			index += 1;
			continue;
		}
		const operator = sql.slice(index).match(/^(?:<>|!=|<=|>=|::|->>|->|:=|[-+*/%=<>])/)?.[0];
		tokens.push({ value: operator || character, kind: 'operator' });
		index += (operator || character).length;
	}
	return tokens;
}

export function formatSql(
	sql: string,
	_dialect: SqlDialect,
	settings: SqlFormatSettings = defaultSqlFormatSettings
): string {
	const tokens = tokenizeForFormatting(sql);
	const indent = ' '.repeat(settings.indentSize);
	const lines: string[] = [];
	let current = '';
	let depth = 0;

	const writeLine = () => {
		if (current.trim()) lines.push(indent.repeat(Math.max(0, depth)) + current.trim());
		current = '';
	};
	const append = (value: string, compact = false) => {
		if (!current || compact) current += value;
		else current += ` ${value}`;
	};

	for (let index = 0; index < tokens.length; index += 1) {
		const token = tokens[index];
		let value = token.value;
		if (token.kind === 'word' && SQL_KEYWORDS.has(value.toLowerCase())) {
			if (settings.keywordCase === 'upper') value = value.toUpperCase();
			if (settings.keywordCase === 'lower') value = value.toLowerCase();
		}
		const upper = value.toUpperCase();
		const nextUpper = tokens[index + 1]?.value.toUpperCase();
		const compound =
			['GROUP', 'ORDER'].includes(upper) && nextUpper === 'BY'
				? `${upper} BY`
				: ['LEFT', 'RIGHT', 'FULL', 'INNER', 'CROSS'].includes(upper) && nextUpper === 'JOIN'
					? `${upper} JOIN`
					: upper;
		if (compound !== upper) {
			value = settings.keywordCase === 'lower' ? compound.toLowerCase() : compound.toUpperCase();
			index += 1;
		}

		if (BREAK_BEFORE.has(compound)) {
			writeLine();
		} else if ((upper === 'AND' || upper === 'OR') && current) {
			writeLine();
		}

		if (value === '(') {
			append(value);
			writeLine();
			depth += 1;
		} else if (value === ')') {
			writeLine();
			depth = Math.max(0, depth - 1);
			append(value);
		} else if (value === ',') {
			append(value, true);
			if (depth > 0) writeLine();
		} else if (value === ';') {
			append(value, true);
			writeLine();
		} else if (value === '.') {
			append(value, true);
		} else if (token.kind === 'comment') {
			writeLine();
			append(value);
			writeLine();
		} else {
			append(value);
		}
	}
	writeLine();
	return lines.join('\n').trim();
}

export function lintSql(
	sql: string,
	settings: SqlLintSettings = defaultSqlLintSettings
): SqlLintIssue[] {
	const issues: SqlLintIssue[] = [];
	const clean = removeSqlNoise(sql);
	if (settings.requireWhereForMutations) {
		const mutation = /\b(UPDATE|DELETE)\b/gi;
		for (const match of clean.matchAll(mutation)) {
			const tail = clean.slice(match.index);
			const statementEnd = tail.indexOf(';');
			const statement = statementEnd < 0 ? tail : tail.slice(0, statementEnd);
			if (!/\bWHERE\b/i.test(statement)) {
				issues.push({
					rule: 'mutation-where',
					message: `${match[1].toUpperCase()} has no WHERE clause`,
					severity: 'error',
					start: match.index,
					end: match.index + match[0].length
				});
			}
		}
	}
	if (settings.disallowSelectStar) {
		for (const match of clean.matchAll(/\bSELECT\s+\*/gi)) {
			issues.push({
				rule: 'select-star',
				message: 'Prefer explicit columns over SELECT *',
				severity: 'warning',
				start: match.index,
				end: match.index + match[0].length
			});
		}
	}
	if (settings.requireSemicolon && clean.trim() && !clean.trimEnd().endsWith(';')) {
		issues.push({
			rule: 'terminal-semicolon',
			message: 'Statement does not end with a semicolon',
			severity: 'warning',
			start: Math.max(0, sql.trimEnd().length - 1),
			end: sql.trimEnd().length
		});
	}
	if (settings.maxLineLength > 0) {
		let offset = 0;
		for (const line of sql.split('\n')) {
			if (line.length > settings.maxLineLength) {
				issues.push({
					rule: 'line-length',
					message: `Line exceeds ${settings.maxLineLength} characters`,
					severity: 'warning',
					start: offset + settings.maxLineLength,
					end: offset + line.length
				});
			}
			offset += line.length + 1;
		}
	}
	return issues;
}
