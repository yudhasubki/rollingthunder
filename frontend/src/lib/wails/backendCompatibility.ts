type WailsService = Record<string, unknown>;

type WailsWindow = Window & {
	go?: {
		db?: {
			Service?: WailsService;
		};
	};
};

export const BACKEND_RESTART_MESSAGE = `${APPLICATION.name} was updated while this window was open. Quit the app completely, stop the old Wails process, and start it again.`;

export function hasBackendMethod(method: string): boolean {
	if (typeof window === 'undefined') return false;
	return typeof (window as WailsWindow).go?.db?.Service?.[method] === 'function';
}

export function isBackendVersionMismatch(error: unknown): boolean {
	const message =
		error instanceof Error
			? error.message
			: typeof error === 'string'
				? error
				: String(error || '');
	return (
		message.includes('is not a function') ||
		message.includes('cannot unmarshal object into Go value of type []db.SavedConnection')
	);
}
import { APPLICATION } from '../config/application.ts';
