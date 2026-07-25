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

export type ProviderId = 'postgres' | 'mysql' | 'sqlite';
export type ConnectionEnvironment = 'unclassified' | 'development' | 'staging' | 'production';

export const DATABASE_PROVIDERS: ReadonlyArray<{
	id: ProviderId;
	name: string;
	description: string;
	defaultPort: string;
	defaultDatabase: string;
	defaultUser: string;
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
		available: true,
		mark: 'SQ'
	}
];

export const CONNECTION_DEFAULTS = Object.freeze({
	host: '127.0.0.1',
	sshPort: '22',
	sslMode: 'disable',
	provider: 'postgres' as ProviderId,
	environment: 'unclassified' as ConnectionEnvironment
});

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

export function providerOption(value?: string) {
	return (
		DATABASE_PROVIDERS.find((provider) => provider.id === value) ??
		DATABASE_PROVIDERS.find((provider) => provider.id === CONNECTION_DEFAULTS.provider)!
	);
}
