import {
	parseSavedQueries,
	SAVED_QUERY_STORAGE_KEY,
	upsertSavedQuery,
	type SavedQuery
} from '$lib/query/snippets';

let queries = $state<SavedQuery[]>([]);
let hydrated = false;

function hydrate(): void {
	if (hydrated || typeof window === 'undefined') return;
	hydrated = true;
	queries = parseSavedQueries(localStorage.getItem(SAVED_QUERY_STORAGE_KEY)).queries;
}

function persist(): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(
		SAVED_QUERY_STORAGE_KEY,
		JSON.stringify({
			version: 1,
			queries
		})
	);
}

export function getSavedQueries(): SavedQuery[] {
	hydrate();
	return queries;
}

export function saveNamedQuery(input: {
	id?: string;
	name: string;
	query: string;
	engine: string;
	tags?: string[];
}): SavedQuery {
	hydrate();
	queries = upsertSavedQuery(queries, {
		...input,
		tags: input.tags || []
	});
	persist();
	return queries.find((query) => query.id === input.id) || queries[0];
}

export function deleteSavedQuery(id: string): void {
	hydrate();
	queries = queries.filter((query) => query.id !== id);
	persist();
}
