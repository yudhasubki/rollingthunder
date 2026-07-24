export type ExportScope = 'page' | 'all' | 'loaded';

export interface CSVExportSettings {
	scope: ExportScope;
	delimiter: ',' | ';' | '\t';
	includeHeader: boolean;
	nullValue: string;
}

export function buildCSVExportOptions(settings: CSVExportSettings) {
	return {
		format: 'csv',
		csv: {
			delimiter: settings.delimiter,
			includeHeader: settings.includeHeader,
			nullValue: settings.nullValue
		}
	};
}

export function formatExportBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
	if (bytes < 1024) return `${Math.round(bytes)} B`;
	if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
	if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
