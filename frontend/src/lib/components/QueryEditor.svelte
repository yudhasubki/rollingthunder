<script>
    import { EditorView, basicSetup } from "codemirror";
    import { EditorState } from "@codemirror/state"
    import { sql } from "@codemirror/lang-sql"

    const state = new EditorState.create({
        extensions: [basicSetup, sql()]
    })

    const editor = new EditorView({
        state:state,
    })
</script>

<div id="editor"></div>

