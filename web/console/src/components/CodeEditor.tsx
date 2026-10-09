import { useEffect, useRef } from "react";
import {
  EditorView,
  lineNumbers,
  highlightActiveLineGutter,
  drawSelection,
  keymap,
} from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import {
  syntaxHighlighting,
  HighlightStyle,
  bracketMatching,
  indentOnInput,
} from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { yaml } from "@codemirror/lang-yaml";
export default function CodeEditor({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const parent = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const change = useRef(onChange);
  change.current = onChange;
  useEffect(() => {
    if (!parent.current) return;
    const v = new EditorView({
      parent: parent.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(),
          highlightActiveLineGutter(),
          drawSelection(),
          history(),
          bracketMatching(),
          indentOnInput(),
          yaml(),
          syntaxHighlighting(
            HighlightStyle.define([
              {
                tag: [tags.keyword, tags.bool, tags.null, tags.number],
                color: "var(--green)",
              },
              { tag: [tags.string, tags.propertyName], color: "var(--ink)" },
              { tag: tags.comment, color: "var(--muted)", fontStyle: "italic" },
            ]),
          ),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.contentAttributes.of({
            "aria-label": "Manifest syntax editor",
            spellcheck: "false",
          }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) change.current(u.state.doc.toString());
          }),
          EditorView.theme({
            "&": {
              fontSize: "12px",
              backgroundColor: "var(--panel)",
              color: "var(--ink)",
            },
            ".cm-scroller": {
              fontFamily: "ui-monospace,monospace",
              lineHeight: "1.9",
              overflow: "auto",
            },
            ".cm-content": { minHeight: "540px", padding: "15px 0" },
            ".cm-gutters": {
              backgroundColor: "var(--soft)",
              color: "var(--muted)",
              borderRight: "1px solid var(--line)",
            },
            ".cm-activeLineGutter": { backgroundColor: "var(--green-soft)" },
            ".cm-cursor": { borderLeftColor: "var(--ink)" },
            "&.cm-focused": {
              outline: "2px solid var(--green)",
              outlineOffset: "-2px",
            },
          }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
  }, []);
  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value)
      v.dispatch({
        changes: { from: 0, to: v.state.doc.length, insert: value },
      });
  }, [value]);
  return <div className="syntax-editor" ref={parent} />;
}
