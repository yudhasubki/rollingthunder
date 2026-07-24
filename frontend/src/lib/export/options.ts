export type ExportScope = 'page' | 'all' | 'loaded';
export type ExportFormat = 'csv' | 'json' | 'sql';

export interface ExportSettings {
	scope: ExportScope;
	format: ExportFormat;
	delimiter: ',' | ';' | '\t';
	includeHeader: boolean;
	nullValue: string;
	prettyJSON: boolean;
	sqlBatchSize: number;
	includeTransaction: boolean;
}

export function buildExportOptions(settings: ExportSettings) {
	if (settings.format === 'sql') {
		return {
			format: 'sql',
			sql: {
				batchSize: settings.sqlBatchSize,
				includeTransaction: settings.includeTransaction
			}
		};
	}

	if (settings.format === 'json') {
		return {
			format: 'json',
			json: {
				pretty: settings.prettyJSON
			}
		};
	}

	return {
		format: 'csv',
		csv: {
			delimiter: settings.delimiter,
			includeHeader: settings.includeHeader,
			nullValue: settings.nullValue
		}
	};
}

export function getExportExtension(format: ExportFormat): 'csv' | 'json' | 'sql' {
	return format;
}

export function formatExportBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
	if (bytes < 1024) return `${Math.round(bytes)} B`;
	if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
	if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}
