const focusableSelector = [
	'a[href]',
	'button:not([disabled])',
	'input:not([disabled])',
	'select:not([disabled])',
	'textarea:not([disabled])',
	'[tabindex]:not([tabindex="-1"])'
].join(',');

export function focusTrap(node: HTMLElement) {
	const previousFocus =
		document.activeElement instanceof HTMLElement ? document.activeElement : null;

	function focusableElements(): HTMLElement[] {
		return Array.from(node.querySelectorAll<HTMLElement>(focusableSelector)).filter(
			(element) => element.getAttribute('aria-hidden') !== 'true' && element.offsetParent !== null
		);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (event.key !== 'Tab') return;
		const focusable = focusableElements();
		if (focusable.length === 0) {
			event.preventDefault();
			node.focus();
			return;
		}
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}

	node.addEventListener('keydown', handleKeydown);
	queueMicrotask(() => {
		const [first] = focusableElements();
		if (!node.contains(document.activeElement)) {
			(first || node).focus();
		}
	});

	return {
		destroy() {
			node.removeEventListener('keydown', handleKeydown);
			if (previousFocus?.isConnected) previousFocus.focus();
		}
	};
}
