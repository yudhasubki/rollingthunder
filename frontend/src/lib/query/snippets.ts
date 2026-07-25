import { APPLICATION_STORAGE } from '../config/application.ts';

export const SAVED_QUERY_STORAGE_KEY = APPLICATION_STORAGE.savedQueries;
export const SAVED_QUERY_STORAGE_VERSION = 1;

export interface SavedQuery {
	id: string;
	name: string;
	query: string;
	engine: string;
	tags: string[];
	createdAt: string;
	updatedAt: string;
}

export interface SavedQueryEnvelope {
	version: number;
	queries: SavedQuery[];
}

export function parseSavedQueries(raw: string | null): SavedQueryEnvelope {
	if (!raw) return { version: SAVED_QUERY_STORAGE_VERSION, queries: [] };
	try {
		const parsed = JSON.parse(raw);
		if (parsed?.version !== SAVED_QUERY_STORAGE_VERSION || !Array.isArray(parsed.queries)) {
			return { version: SAVED_QUERY_STORAGE_VERSION, queries: [] };
		}
		return {
			version: SAVED_QUERY_STORAGE_VERSION,
			queries: parsed.queries
				.filter(
					(query: any) =>
						query &&
						typeof query.id === 'string' &&
						typeof query.name === 'string' &&
						typeof query.query === 'string'
				)
				.map(
					(query: any): SavedQuery => ({
						id: query.id,
						name: query.name.trim() || 'Untitled query',
						query: query.query,
						engine: typeof query.engine === 'string' ? query.engine : 'sql',
						tags: Array.isArray(query.tags)
							? query.tags.filter((tag: unknown): tag is string => typeof tag === 'string')
							: [],
						createdAt:
							typeof query.createdAt === 'string' ? query.createdAt : new Date(0).toISOString(),
						updatedAt:
							typeof query.updatedAt === 'string' ? query.updatedAt : new Date(0).toISOString()
					})
				)
		};
	} catch {
		return { version: SAVED_QUERY_STORAGE_VERSION, queries: [] };
	}
}

export function upsertSavedQuery(
	queries: SavedQuery[],
	input: Pick<SavedQuery, 'name' | 'query' | 'engine' | 'tags'> & { id?: string },
	now = new Date()
): SavedQuery[] {
	const timestamp = now.toISOString();
	const existing = input.id ? queries.find((query) => query.id === input.id) : undefined;
	const saved: SavedQuery = {
		id: existing?.id || input.id || crypto.randomUUID(),
		name: input.name.trim() || 'Untitled query',
		query: input.query,
		engine: input.engine || 'sql',
		tags: [...new Set(input.tags.map((tag) => tag.trim()).filter(Boolean))],
		createdAt: existing?.createdAt || timestamp,
		updatedAt: timestamp
	};
	return [saved, ...queries.filter((query) => query.id !== saved.id)].sort((left, right) =>
		right.updatedAt.localeCompare(left.updatedAt)
	);
}
