import { CancelExport, GetExportProgress } from '$lib/wailsjs/go/db/Service';
import { database } from '$lib/wailsjs/go/models';

export function createInitialExportProgress(
	jobId: string,
	totalRows: number
): database.ExportProgress {
	return new database.ExportProgress({
		jobId,
		status: 'preparing',
		rows: 0,
		bytes: 0,
		totalRows: Math.max(0, totalRows),
		elapsedMs: 0,
		cancellable: true
	});
}

export function startExportProgressPolling(
	jobId: string,
	onProgress: (progress: database.ExportProgress) => void,
	intervalMilliseconds = 150
): () => void {
	let stopped = false;
	let requestInFlight = false;

	const poll = async () => {
		if (stopped || requestInFlight) return;
		requestInFlight = true;
		try {
			const response = await GetExportProgress(jobId);
			if (!stopped && !response.errors?.length && response.data) {
				onProgress(response.data);
			}
		} catch {
			// A job may finish between polling ticks. The export response is authoritative.
		} finally {
			requestInFlight = false;
		}
	};

	void poll();
	const timer = globalThis.setInterval(poll, intervalMilliseconds);

	return () => {
		stopped = true;
		globalThis.clearInterval(timer);
	};
}

export async function cancelExportJob(jobId: string): Promise<void> {
	const response = await CancelExport(jobId);
	if (response.errors?.length) {
		const message = response.errors[0]?.detail || 'Failed to cancel export';
		if (!message.toLowerCase().includes('not running')) {
			throw new Error(message);
		}
	}
}
