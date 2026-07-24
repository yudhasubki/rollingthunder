export interface ServiceErrorLike {
	code?: string;
	detail?: string;
	hint?: string;
}

export interface ServiceException extends Error {
	code?: string;
	hint?: string;
}

export function createServiceError(
	error: ServiceErrorLike | null | undefined,
	fallback: string
): ServiceException {
	const detail = error?.detail?.trim() || fallback;
	const hint = error?.hint?.trim() || '';
	const message = `${error?.code ? `[${error.code}] ` : ''}${detail}${hint ? ` — ${hint}` : ''}`;
	const exception = new Error(message) as ServiceException;
	exception.code = error?.code;
	exception.hint = hint || undefined;
	return exception;
}
