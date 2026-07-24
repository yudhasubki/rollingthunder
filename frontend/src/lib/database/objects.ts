import type { database } from '$lib/wailsjs/go/models';

export interface DatabaseObjectGroup {
	id: string;
	label: string;
	kinds: string[];
	objects: database.DatabaseObject[];
}

export const DATABASE_OBJECT_GROUPS: Array<Omit<DatabaseObjectGroup, 'objects'>> = [
	{ id: 'tables', label: 'Tables', kinds: ['table'] },
	{ id: 'views', label: 'Views', kinds: ['view'] },
	{ id: 'materialized-views', label: 'Materialized views', kinds: ['materialized_view'] },
	{ id: 'functions', label: 'Functions', kinds: ['function'] },
	{ id: 'procedures', label: 'Procedures', kinds: ['procedure'] },
	{ id: 'triggers', label: 'Triggers', kinds: ['trigger'] },
	{ id: 'sequences', label: 'Sequences', kinds: ['sequence'] },
	{ id: 'types', label: 'Types', kinds: ['type', 'enum', 'domain'] },
	{ id: 'constraints', label: 'Constraints', kinds: ['constraint'] },
	{ id: 'extensions', label: 'Extensions', kinds: ['extension'] },
	{ id: 'indexes', label: 'Indexes', kinds: ['index'] }
];

export function databaseObjectKey(reference: database.ObjectReference): string {
	if (reference.id) return reference.id;
	return [
		reference.kind,
		reference.schema || '',
		reference.name,
		reference.signature || '',
		reference.parentSchema || '',
		reference.parentName || ''
	].join(':');
}

export function databaseObjectQualifiedName(reference: database.ObjectReference): string {
	const name = reference.schema ? `${reference.schema}.${reference.name}` : reference.name;
	return reference.signature ? `${name}(${reference.signature})` : name;
}

export function databaseObjectKindLabel(kind: string): string {
	switch (kind) {
		case 'materialized_view':
			return 'Materialized view';
		case 'function':
			return 'Function';
		case 'procedure':
			return 'Procedure';
		case 'trigger':
			return 'Trigger';
		case 'sequence':
			return 'Sequence';
		case 'enum':
			return 'Enum';
		case 'domain':
			return 'Domain';
		case 'constraint':
			return 'Constraint';
		case 'extension':
			return 'Extension';
		case 'index':
			return 'Index';
		case 'type':
			return 'Type';
		case 'view':
			return 'View';
		case 'table':
			return 'Table';
		default:
			return 'Database object';
	}
}

export function groupDatabaseObjects(
	objects: database.DatabaseObject[],
	search: string
): DatabaseObjectGroup[] {
	const query = search.trim().toLowerCase();
	const filtered = query
		? objects.filter((object) => {
				const reference = object.reference;
				return [
					object.displayName,
					object.description || '',
					reference.schema || '',
					reference.name,
					reference.signature || '',
					reference.parentName || ''
				]
					.join(' ')
					.toLowerCase()
					.includes(query);
			})
		: objects;

	return DATABASE_OBJECT_GROUPS.map((group) => ({
		...group,
		objects: filtered
			.filter((object) => group.kinds.includes(object.reference.kind))
			.sort((left, right) =>
				left.displayName.localeCompare(right.displayName, undefined, {
					numeric: true,
					sensitivity: 'base'
				})
			)
	})).filter((group) => group.objects.length > 0);
}

export function countGroupedObjects(groups: DatabaseObjectGroup[]): number {
	return groups.reduce((total, group) => total + group.objects.length, 0);
}
