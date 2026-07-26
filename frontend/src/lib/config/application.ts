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
export type OracleConnectionMode = 'direct' | 'tns';
export type TLSMode = 'disable' | 'require' | 'verify-ca' | 'verify-full' | 'strict';
export type SQLServerAuthMode =
	| 'sql'
	| 'integrated'
	| 'entra-default'
	| 'entra-password'
	| 'entra-service-principal'
	| 'entra-managed-identity'
	| 'entra-azure-cli';

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
	sslMode: 'disable' as TLSMode,
	provider: 'postgres' as ProviderId,
	oracleConnectionMode: 'direct' as OracleConnectionMode,
	sqlServerAuthMode: 'sql' as SQLServerAuthMode,
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

export const SSL_OPTIONS: ReadonlyArray<{ value: TLSMode; label: string }> = Object.freeze([
	{ value: 'disable', label: 'Disable' },
	{ value: 'require', label: 'Require' },
	{ value: 'verify-ca', label: 'Verify CA' },
	{ value: 'verify-full', label: 'Verify full' },
	{ value: 'strict', label: 'Strict (TDS 8.0)' }
]);

export function tlsModeAvailableForProvider(mode: TLSMode, provider?: ProviderId | ''): boolean {
	return mode !== 'strict' || provider === 'sqlserver';
}

export function normalizeTLSModeForProvider(
	value: string | undefined,
	provider?: ProviderId | ''
): TLSMode {
	const normalized = value?.trim().toLowerCase();
	const mode = SSL_OPTIONS.find((option) => option.value === normalized)?.value;
	if (!mode) return CONNECTION_DEFAULTS.sslMode;
	if (!tlsModeAvailableForProvider(mode, provider)) return 'verify-full';
	return mode;
}

export function tlsModeVerifiesServerCertificate(mode: TLSMode): boolean {
	return mode === 'verify-ca' || mode === 'verify-full' || mode === 'strict';
}

export const SSH_AUTH_OPTIONS = Object.freeze([
	{ value: 'agent', label: 'SSH agent' },
	{ value: 'private-key', label: 'Private key' },
	{ value: 'password', label: 'Password' }
]);

export const ORACLE_CONNECTION_MODES: ReadonlyArray<{
	value: OracleConnectionMode;
	label: string;
}> = [
	{ value: 'direct', label: 'Direct endpoint' },
	{ value: 'tns', label: 'TNS alias' }
];

export const SQL_SERVER_AUTH_MODES: ReadonlyArray<{
	value: SQLServerAuthMode;
	label: string;
	description: string;
}> = [
	{
		value: 'sql',
		label: 'SQL password',
		description: 'Authenticate with a SQL Server login and password.'
	},
	{
		value: 'integrated',
		label: 'Windows Integrated',
		description: 'Use the current Windows identity through SSPI.'
	},
	{
		value: 'entra-default',
		label: 'Microsoft Entra Default',
		description: 'Use the Azure Identity default credential chain.'
	},
	{
		value: 'entra-password',
		label: 'Microsoft Entra password',
		description: 'Use an Entra user, password, and approved application client ID.'
	},
	{
		value: 'entra-service-principal',
		label: 'Entra service principal',
		description: 'Use an application client ID, tenant ID, and client secret.'
	},
	{
		value: 'entra-managed-identity',
		label: 'Entra managed identity',
		description: 'Use a system-assigned or optional user-assigned managed identity.'
	},
	{
		value: 'entra-azure-cli',
		label: 'Azure CLI session',
		description: 'Reuse the identity currently signed in through the Azure CLI.'
	}
];

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
