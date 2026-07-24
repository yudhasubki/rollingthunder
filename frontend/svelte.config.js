import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { preprocessMeltUI, sequence } from '@melt-ui/pp';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: sequence([
		vitePreprocess(),
		preprocessMeltUI() // add to the end!
	]),
	kit: {
		adapter: adapter({
			pages: 'build/app',
			assets: 'build/app',
			fallback: 'index.html'
		})
	}
};

export default config;
