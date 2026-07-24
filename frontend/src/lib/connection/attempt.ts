import { CancelConnectionAttempt } from '$lib/wailsjs/go/db/Service';

export const CONNECTION_TIMEOUT_SECONDS = 15;

export function createConnectionAttemptID(): string {
	return crypto.randomUUID();
}

export function startConnectionElapsedTimer(onElapsed: (seconds: number) => void): () => void {
	const startedAt = Date.now();
	onElapsed(0);

	const timer = globalThis.setInterval(() => {
		onElapsed(Math.floor((Date.now() - startedAt) / 1000));
	}, 250);

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
