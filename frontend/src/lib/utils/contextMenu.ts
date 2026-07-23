export interface ContextMenuPosition {
	x: number;
	y: number;
}

export function getContextMenuPosition(
	event: MouseEvent,
	width = 236,
	height = 196,
	viewportPadding = 8
): ContextMenuPosition {
	const maxX = Math.max(viewportPadding, window.innerWidth - width - viewportPadding);
	const maxY = Math.max(viewportPadding, window.innerHeight - height - viewportPadding);

	return {
		x: Math.max(viewportPadding, Math.min(event.clientX, maxX)),
		y: Math.max(viewportPadding, Math.min(event.clientY, maxY))
	};
}
