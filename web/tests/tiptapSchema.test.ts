// Round-trip a backend-produced body_doc through the Tiptap schema and
// assert structural invariants survive — specifically the stable data-*
// attrs every node carries (per ADR-0008) and the doc shape required by
// the IssueDocBuilder + UpdateBody contract.

import { Editor, getSchema } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it } from "vitest";

import {
  CoverNode,
  IntroNode,
  IssueDocument,
  StoryNode,
  SCHEMA_VERSION,
} from "../tiptap/schema";

const extensions = [
  StarterKit.configure({
    document: false,
    heading: false,
    codeBlock: false,
    blockquote: false,
    horizontalRule: false,
    bulletList: false,
    orderedList: false,
    listItem: false,
    hardBreak: false,
  }),
  IssueDocument,
  CoverNode,
  IntroNode,
  StoryNode,
];

// What internal/issuedoc/builder.go produces, simplified for the test.
const backendDoc = {
  type: "doc",
  attrs: { version: SCHEMA_VERSION, subject: "Subj", title: "T" },
  content: [
    {
      type: "cover",
      attrs: { block: "cover", src: "https://cdn/x.png", alt: "T" },
    },
    {
      type: "intro",
      attrs: { block: "intro" },
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "Welcome back." }],
        },
      ],
    },
    {
      type: "story",
      attrs: {
        block: "story",
        storyId: "abc-123-def",
        sourceUrl: "https://example.com/p1",
      },
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "Headline one" }],
        },
        {
          type: "paragraph",
          content: [{ type: "text", text: "Body text one." }],
        },
      ],
    },
  ],
};

describe("Tiptap schema mirrors backend issuedoc", () => {
  it("the constructed schema declares the four custom node types", () => {
    const schema = getSchema(extensions);
    expect(schema.nodes.doc).toBeDefined();
    expect(schema.nodes.cover).toBeDefined();
    expect(schema.nodes.intro).toBeDefined();
    expect(schema.nodes.story).toBeDefined();
  });

  it("parses a backend body_doc cleanly", () => {
    const editor = new Editor({ content: backendDoc, extensions });
    const json = editor.getJSON();
    expect(json.type).toBe("doc");
    expect((json.attrs as { version: number }).version).toBe(SCHEMA_VERSION);
    editor.destroy();
  });

  it("preserves story node attrs across a parse/serialise round-trip", () => {
    const editor = new Editor({ content: backendDoc, extensions });
    const json = editor.getJSON();
    const story = (json.content ?? []).find((n) => n.type === "story") as {
      attrs: { storyId: string; sourceUrl: string; block: string };
    };
    expect(story.attrs.storyId).toBe("abc-123-def");
    expect(story.attrs.sourceUrl).toBe("https://example.com/p1");
    expect(story.attrs.block).toBe("story");
    editor.destroy();
  });

  it("preserves cover src + alt round-trip", () => {
    const editor = new Editor({ content: backendDoc, extensions });
    const json = editor.getJSON();
    const cover = (json.content ?? []).find((n) => n.type === "cover") as {
      attrs: { src: string; alt: string; block: string };
    };
    expect(cover.attrs.src).toBe("https://cdn/x.png");
    expect(cover.attrs.alt).toBe("T");
    expect(cover.attrs.block).toBe("cover");
    editor.destroy();
  });
});
