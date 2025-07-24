<script>
    import { onMount } from "svelte";
    import { EditorView, basicSetup } from "codemirror";
    import { EditorState } from "@codemirror/state";
    import { sql } from "@codemirror/lang-sql";

    let editorContainer;

    onMount(() => {
        const state = EditorState.create({
            extensions: [basicSetup, sql()]
        });

        const editor = new EditorView({
            state,
            parent: editorContainer
        });
    });

    function handleRun() {

    }
</script>

<form class="flex flex-col p-2" onsubmit={(e) => {
    e.preventDefault()
    handleRun()
}}>
    <!-- Editor container with fixed height and scroll -->
    <div class="h-[50vh] overflow-y-auto border border-zinc-300 rounded">
        <div bind:this={editorContainer}></div>
    </div>

    <!-- Button aligned to the right -->
    <div class="flex justify-end mt-2">
        <button class="btn btn-soft">Run</button>
    </div>
</form>


