import { CancelConnectionAttempt } from '$lib/wailsjs/go/db/Service';
import { TIME, UI_RUNTIME } from '$lib/config/application';

export const CONNECTION_TIMEOUT_SECONDS = UI_RUNTIME.connectionTimeoutSeconds;

export function createConnectionAttemptID(): string {
	return crypto.randomUUID();
}

export function startConnectionElapsedTimer(onElapsed: (seconds: number) => void): () => void {
	const startedAt = Date.now();
	onElapsed(0);

	const timer = globalThis.setInterval(() => {
		onElapsed(Math.floor((Date.now() - startedAt) / TIME.millisecondsPerSecond));
	}, UI_RUNTIME.elapsedTimerTickMs);

	return () => globalThis.clearInterval(timer);
}

export async function cancelConnectionAttempt(attemptID: string): Promise<void> {
	const response = await CancelConnectionAttempt(attemptID);
	if (!response.errors?.length) return;

	const detail = response.errors[0]?.detail || 'Could not cancel the connection attempt';
	// Completion and cancellation can cross between two Wails calls. In that
	// case the Connect response remains authoritative.
	if (!detail.toLowerCase().includes('not running')) {
		throw new Error(detail);
	}
}
