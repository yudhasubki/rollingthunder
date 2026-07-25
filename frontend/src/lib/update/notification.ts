import { APPLICATION_STORAGE } from '../config/application.ts';

export const UPDATE_SNOOZE_STORAGE_KEY = APPLICATION_STORAGE.updateSnooze;
export const UPDATE_REMINDER_DELAY_MS = 24 * 60 * 60 * 1000;
export const UPDATE_DOWNLOAD_DELAY_MS = 7 * UPDATE_REMINDER_DELAY_MS;

interface UpdateSnooze {
	version: string;
	until: number;
}

export function isUpdateSnoozed(value: string | null, version: string, now = Date.now()): boolean {
	if (!value || !version) return false;
	try {
		const snooze = JSON.parse(value) as Partial<UpdateSnooze>;
		return (
			snooze.version === version &&
			typeof snooze.until === 'number' &&
			Number.isFinite(snooze.until) &&
			snooze.until > now
		);
	} catch {
		return false;
	}
}

export function createUpdateSnooze(version: string, delay: number, now = Date.now()): string {
	return JSON.stringify({
		version,
		until: now + Math.max(0, delay)
	} satisfies UpdateSnooze);
}

export function displayVersion(value: string): string {
	const normalized = value.trim().replace(/^[vV]/, '');
	return normalized ? `v${normalized}` : 'Unknown';
}
