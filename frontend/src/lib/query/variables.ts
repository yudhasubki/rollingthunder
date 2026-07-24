export type QueryVariableType = 'text' | 'number' | 'boolean' | 'date' | 'null';

export interface QueryVariableInput {
	name: string;
	value: string | number | boolean | null;
	type: QueryVariableType;
}

export function extractQueryVariableNames(sql: string): string[] {
	const names: string[] = [];
	const seen = new Set<string>();
	let state:
		| 'code'
		| 'single'
		| 'double'
		| 'backtick'
		| 'bracket'
		| 'line-comment'
		| 'block-comment'
		| 'dollar' = 'code';
	let dollarDelimiter = '';
	let blockDepth = 0;

	for (let index = 0; index < sql.length; index += 1) {
		const character = sql[index];
		const next = sql[index + 1] || '';

		if (state === 'line-comment') {
			if (character === '\n') state = 'code';
			continue;
		}
		if (state === 'block-comment') {
			if (character === '/' && next === '*') {
				blockDepth += 1;
				index += 1;
			} else if (character === '*' && next === '/') {
				blockDepth -= 1;
				index += 1;
				if (blockDepth === 0) state = 'code';
			}
			continue;
		}
		if (state === 'dollar') {
			if (sql.startsWith(dollarDelimiter, index)) {
				index += dollarDelimiter.length - 1;
				state = 'code';
			}
			continue;
		}
		if (state === 'single' || state === 'double' || state === 'backtick') {
			const quote = state === 'single' ? "'" : state === 'double' ? '"' : '`';
			if (character === quote && next === quote) {
				index += 1;
			} else if (character === '\\' && next) {
				index += 1;
			} else if (character === quote) {
				state = 'code';
			}
			continue;
		}
		if (state === 'bracket') {
			if (character === ']' && next === ']') index += 1;
			else if (character === ']') state = 'code';
			continue;
		}

		if (character === '-' && next === '-') {
			state = 'line-comment';
			index += 1;
			continue;
		}
		if (character === '#') {
			state = 'line-comment';
			continue;
		}
		if (character === '/' && next === '*') {
			state = 'block-comment';
			blockDepth = 1;
			index += 1;
			continue;
		}
		if (character === "'" || character === '"' || character === '`') {
			state = character === "'" ? 'single' : character === '"' ? 'double' : 'backtick';
			continue;
		}
		if (character === '[') {
			state = 'bracket';
			continue;
		}
		if (character === '$') {
			const delimiter = sql.slice(index).match(/^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$/)?.[0];
			if (delimiter) {
				dollarDelimiter = delimiter;
				state = 'dollar';
				index += delimiter.length - 1;
				continue;
			}
		}
		if (character !== '{' || next !== '{') continue;
		const closeOffset = sql.indexOf('}}', index + 2);
		if (closeOffset < 0) continue;
		const name = sql.slice(index + 2, closeOffset).trim();
		if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) && !seen.has(name)) {
			seen.add(name);
			names.push(name);
		}
		index = closeOffset + 1;
	}
	return names;
}

export function coerceQueryVariable(input: QueryVariableInput): QueryVariableInput {
	switch (input.type) {
		case 'null':
			return { ...input, value: null };
		case 'boolean':
			return {
				...input,
				value:
					typeof input.value === 'boolean'
						? input.value
						: String(input.value).toLowerCase() === 'true'
			};
		case 'number': {
			const parsed = Number(input.value);
			if (!Number.isFinite(parsed)) throw new Error(`${input.name} must be a valid number`);
			return { ...input, value: parsed };
		}
		default:
			return { ...input, value: input.value == null ? '' : String(input.value) };
	}
}
