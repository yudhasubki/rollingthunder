<script>
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import { cn } from '$lib/utils.js';

	let {
		ref = $bindable(null),
		class: className,
		variant = 'default', // "default" | "lifted"
		...restProps
	} = $props();
</script>

<TabsPrimitive.Content
	bind:ref
	data-slot="tabs-content"
	data-variant={variant}
	class={cn(
		'flex-1 outline-none',
		variant === 'lifted' && 'bg-background rounded-lg rounded-tl-none border p-4',
		className
	)}
	{...restProps}
/>
