export type FilterOperator =
	| 'eq'
	| 'neq'
	| 'gt'
	| 'lt'
	| 'gte'
	| 'lte'
	| 'contains'
	| 'is_null'
	| 'is_not_null';

export interface FilterCondition {
	id: string;
	column: string;
	operator: FilterOperator;
	value: string;
	enabled: boolean;
}

export interface DatabaseFilter {
	Column: string;
	Operator: FilterOperator;
	Value: string | null;
}

export const FILTER_OPERATORS: Array<{ value: FilterOperator; label: string }> = [
	{ value: 'eq', label: 'equals' },
	{ value: 'neq', label: 'not equals' },
	{ value: 'gt', label: 'greater than' },
	{ value: 'lt', label: 'less than' },
	{ value: 'gte', label: 'greater or equal' },
	{ value: 'lte', label: 'less or equal' },
	{ value: 'contains', label: 'contains' },
	{ value: 'is_null', label: 'is null' },
	{ value: 'is_not_null', label: 'is not null' }
];

export function filterNeedsValue(operator: FilterOperator): boolean {
	return operator !== 'is_null' && operator !== 'is_not_null';
}

export function buildDatabaseFilters(filters: FilterCondition[]): DatabaseFilter[] {
	return filters
		.filter(
			(filter) =>
				filter.enabled &&
				Boolean(filter.column.trim()) &&
				(!filterNeedsValue(filter.operator) || Boolean(filter.value))
		)
		.map((filter) => ({
			Column: filter.column.trim(),
			Operator: filter.operator,
			Value: filterNeedsValue(filter.operator) ? filter.value : null
		}));
}
