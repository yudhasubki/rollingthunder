<script>
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import { cn } from '$lib/utils.js';

	let {
		ref = $bindable(null),
		class: className,
		variant = 'default', // "default" | "underline" | "lifted"
		...restProps
	} = $props();
</script>

<TabsPrimitive.List
	bind:ref
	data-slot="tabs-list"
	data-variant={variant}
	class={cn(
		'inline-flex items-end',
		variant === 'underline' && 'h-9 gap-0 border-b',
		variant === 'lifted' && 'relative',
		variant === 'default' && 'h-9 gap-1',
		className
	)}
	{...restProps}
/>
