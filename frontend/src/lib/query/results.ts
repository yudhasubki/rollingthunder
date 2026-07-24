export const QUERY_RESULT_PAGE_SIZE = 100;

export function getQueryResultPage<T>(
	rows: T[],
	page: number,
	pageSize = QUERY_RESULT_PAGE_SIZE
): T[] {
	const safePage = Math.max(0, Math.floor(page));
	const safePageSize = Math.max(1, Math.floor(pageSize));
	const start = safePage * safePageSize;
	return rows.slice(start, start + safePageSize);
}
