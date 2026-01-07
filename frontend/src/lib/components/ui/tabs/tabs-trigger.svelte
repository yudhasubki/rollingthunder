<script>
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import { cn } from '$lib/utils.js';

	let {
		ref = $bindable(null),
		class: className,
		variant = 'default', // "default" | "underline" | "lifted"
		...restProps
	} = $props();

	const baseClasses =
		"relative inline-flex items-center justify-center gap-1.5 text-sm font-medium transition-colors focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4";

	const underlineClasses =
		'text-muted-foreground data-[state=active]:text-foreground border-b-2 border-transparent px-3 py-1.5 hover:text-foreground data-[state=active]:border-foreground';

	const defaultClasses =
		'text-muted-foreground data-[state=active]:text-foreground data-[state=active]:bg-background px-3 py-1.5 rounded-md hover:text-foreground hover:bg-muted/50';

	// DaisyUI-style lifted tabs: active tab breaks the bottom border of content
	const liftedClasses =
		'text-muted-foreground data-[state=active]:text-foreground rounded-t-lg border border-transparent px-4 py-2 hover:text-foreground data-[state=active]:border-border data-[state=active]:border-b-transparent data-[state=active]:bg-background data-[state=active]:-mb-px';
</script>

<TabsPrimitive.Trigger
	bind:ref
	data-slot="tabs-trigger"
	data-variant={variant}
	class={cn(
		baseClasses,
		variant === 'underline' && underlineClasses,
		variant === 'lifted' && liftedClasses,
		variant === 'default' && defaultClasses,
		className
	)}
	{...restProps}
/>
