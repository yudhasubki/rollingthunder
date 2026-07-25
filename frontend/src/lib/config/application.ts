export const APPLICATION = Object.freeze({
	name: 'Rolling Thunder',
	id: 'rollingthunder'
});

export const APPLICATION_STORAGE = Object.freeze({
	savedQueries: `${APPLICATION.id}.saved-queries`,
	shortcuts: `${APPLICATION.id}.shortcuts`,
	queryTooling: `${APPLICATION.id}.query-tooling`,
	workspace: `${APPLICATION.id}.workspace`,
	updateSnooze: `${APPLICATION.id}:update-snooze`
});

export const APPLICATION_EVENTS = Object.freeze({
	queryCommand: `${APPLICATION.id}-query-command`
});

export const UI_RUNTIME = Object.freeze({
	connectionTimeoutSeconds: 15,
	elapsedTimerTickMs: 250,
	connectionHealthRefreshMs: 15_000,
	exportProgressPollMs: 150,
	maintenanceProgressPollMs: 1_000,
	persistenceDebounceMs: 180,
	sqlLintDebounceMs: 180,
	copyFeedbackMs: 1_600,
	consoleHistoryLimit: 100
});

export const TIME = Object.freeze({
	millisecondsPerSecond: 1_000
});

export type ProviderId = 'postgres' | 'mysql' | 'sqlite' | 'oracle' | 'sqlserver';
export type ConnectionEnvironment = 'unclassified' | 'development' | 'staging' | 'production';
export type ConnectionAccessMode = 'read-write' | 'read-only';

export const DATABASE_PROVIDERS: ReadonlyArray<{
	id: ProviderId;
	name: string;
	description: string;
	defaultPort: string;
	defaultDatabase: string;
	defaultUser: string;
	databaseLabel: string;
	supportsClientCertificates: boolean;
	available: boolean;
	mark: string;
}> = [
	{
		id: 'postgres',
		name: 'PostgreSQL',
		description: 'Schemas, relations, indexes, and SQL tools.',
		defaultPort: '5432',
		defaultDatabase: 'postgres',
		defaultUser: 'postgres',
		databaseLabel: 'Database name',
		supportsClientCertificates: true,
		available: true,
		mark: 'PG'
	},
	{
		id: 'mysql',
		name: 'MySQL',
		description: 'MySQL and compatible server connections.',
		defaultPort: '3306',
		defaultDatabase: 'app',
		defaultUser: 'root',
		databaseLabel: 'Database name',
		supportsClientCertificates: true,
		available: true,
		mark: 'MY'
	},
	{
		id: 'sqlite',
		name: 'SQLite',
		description: 'Open a local SQLite database file.',
		defaultPort: '',
		defaultDatabase: '',
		defaultUser: '',
		databaseLabel: 'SQLite file path',
		supportsClientCertificates: false,
		available: true,
		mark: 'SQ'
	},
	{
		id: 'oracle',
		name: 'Oracle Database',
		description: 'Oracle schemas, objects, table data, and SQL tools.',
		defaultPort: '1521',
		defaultDatabase: 'FREEPDB1',
		defaultUser: 'system',
		databaseLabel: 'Service name',
		supportsClientCertificates: true,
		available: true,
		mark: 'OR'
	},
	{
		id: 'sqlserver',
		name: 'SQL Server',
		description: 'Microsoft SQL Server schemas, objects, and T-SQL tools.',
		defaultPort: '1433',
		defaultDatabase: 'master',
		defaultUser: 'sa',
		databaseLabel: 'Database name',
		supportsClientCertificates: false,
		available: true,
		mark: 'MS'
	}
];

export const CONNECTION_DEFAULTS = Object.freeze({
	host: '127.0.0.1',
	sshPort: '22',
	sslMode: 'disable',
	provider: 'postgres' as ProviderId,
	environment: 'unclassified' as ConnectionEnvironment,
	accessMode: 'read-write' as ConnectionAccessMode
});

export const CONNECTION_ACCESS_MODES: ReadonlyArray<{
	value: ConnectionAccessMode;
	label: string;
	description: string;
}> = [
	{
		value: 'read-write',
		label: 'Read & write',
		description: 'Allow reviewed data and schema changes.'
	},
	{
		value: 'read-only',
		label: 'Read only',
		description: 'Block every database mutation until temporarily unlocked.'
	}
];

export const SSL_OPTIONS = Object.freeze([
	{ value: 'disable', label: 'Disable' },
	{ value: 'require', label: 'Require' },
	{ value: 'verify-ca', label: 'Verify CA' },
	{ value: 'verify-full', label: 'Verify full' }
]);

export const SSH_AUTH_OPTIONS = Object.freeze([
	{ value: 'agent', label: 'SSH agent' },
	{ value: 'private-key', label: 'Private key' },
	{ value: 'password', label: 'Password' }
]);

export const CONNECTION_ENVIRONMENTS: ReadonlyArray<{
	value: ConnectionEnvironment;
	label: string;
	description: string;
	toneClass: string;
	dotClass: string;
}> = [
	{
		value: 'unclassified',
		label: 'Unclassified',
		description: 'No operational risk level assigned.',
		toneClass: 'border-border bg-muted text-muted-foreground',
		dotClass: 'bg-muted-foreground'
	},
	{
		value: 'development',
		label: 'Development',
		description: 'Safe for iterative development work.',
		toneClass: 'border-info-border bg-info-soft text-info',
		dotClass: 'bg-info'
	},
	{
		value: 'staging',
		label: 'Staging',
		description: 'Production-like data or workflows.',
		toneClass: 'border-warning-border bg-warning-soft text-warning',
		dotClass: 'bg-warning'
	},
	{
		value: 'production',
		label: 'Production',
		description: 'Changes can affect live systems.',
		toneClass: 'border-danger-border bg-danger-soft text-danger',
		dotClass: 'bg-danger'
	}
];

export function normalizeConnectionEnvironment(value?: string): ConnectionEnvironment {
	const normalized = value?.trim().toLowerCase();
	return CONNECTION_ENVIRONMENTS.some((option) => option.value === normalized)
		? (normalized as ConnectionEnvironment)
		: CONNECTION_DEFAULTS.environment;
}

export function connectionEnvironmentOption(value?: string) {
	const normalized = normalizeConnectionEnvironment(value);
	return CONNECTION_ENVIRONMENTS.find((option) => option.value === normalized)!;
}

export function normalizeConnectionAccessMode(
	value?: string,
	environment?: string
): ConnectionAccessMode {
	const normalized = value?.trim().toLowerCase();
	if (normalized === 'read-only' || normalized === 'read-write') return normalized;
	return normalizeConnectionEnvironment(environment) === 'production'
		? 'read-only'
		: CONNECTION_DEFAULTS.accessMode;
}

export function providerOption(value?: string) {
	const normalized =
		value?.trim().toLowerCase() === 'mariadb'
			? 'mysql'
			: value?.trim().toLowerCase() === 'postgresql'
				? 'postgres'
				: value?.trim().toLowerCase();
	return (
		DATABASE_PROVIDERS.find((provider) => provider.id === normalized) ??
		DATABASE_PROVIDERS.find((provider) => provider.id === CONNECTION_DEFAULTS.provider)!
	);
}
