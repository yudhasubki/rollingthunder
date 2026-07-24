import {
	defaultSqlFormatSettings,
	defaultSqlLintSettings,
	type SqlFormatSettings,
	type SqlLintSettings
} from '$lib/sql/tooling';

const STORAGE_KEY = 'rollingthunder.query-tooling';

let hydrated = false;
const state = $state<{
	format: SqlFormatSettings;
	lint: SqlLintSettings;
}>({
	format: { ...defaultSqlFormatSettings },
	lint: { ...defaultSqlLintSettings }
});

function hydrate(): void {
	if (hydrated || typeof window === 'undefined') return;
	hydrated = true;
	try {
		const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
		state.format = {
			...defaultSqlFormatSettings,
			...(parsed.format || {})
		};
		state.lint = {
			...defaultSqlLintSettings,
			...(parsed.lint || {})
		};
	} catch {
		state.format = { ...defaultSqlFormatSettings };
		state.lint = { ...defaultSqlLintSettings };
	}
}

function persist(): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export const queryToolingStore = {
	get format(): SqlFormatSettings {
		hydrate();
		return state.format;
	},
	get lint(): SqlLintSettings {
		hydrate();
		return state.lint;
	},
	updateFormat(patch: Partial<SqlFormatSettings>) {
		hydrate();
		state.format = { ...state.format, ...patch };
		persist();
	},
	updateLint(patch: Partial<SqlLintSettings>) {
		hydrate();
		state.lint = { ...state.lint, ...patch };
		persist();
	},
	reset() {
		state.format = { ...defaultSqlFormatSettings };
		state.lint = { ...defaultSqlLintSettings };
		persist();
	}
};
