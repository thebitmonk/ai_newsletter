// Tiptap schema mirroring internal/issuedoc/builder.go.
//
// The document is a top-level "doc" with a "version", "subject", and "title"
// attribute, then a sequence of: cover (atom), optional intro, one or more
// stories. Each schema-defined node carries a `block` attr so the renderer
// can identify it from its HTML representation.

import { Node } from "@tiptap/core";

export const SCHEMA_VERSION = 1;

// Custom top-level doc — replaces the default Document so we can add attrs.
export const IssueDocument = Node.create({
  name: "doc",
  topNode: true,
  content: "cover intro? story+",
  addAttributes() {
    return {
      version: { default: SCHEMA_VERSION },
      subject: { default: "" },
      title: { default: "" },
    };
  },
});

export const CoverNode = Node.create({
  name: "cover",
  group: "block",
  atom: true,
  draggable: false,
  selectable: false,
  addAttributes() {
    return {
      src: { default: "" },
      alt: { default: "" },
      block: { default: "cover" },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-block="cover"]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ["div", { ...HTMLAttributes, "data-block": "cover" }];
  },
});

export const IntroNode = Node.create({
  name: "intro",
  group: "block",
  content: "paragraph+",
  draggable: false,
  addAttributes() {
    return {
      block: { default: "intro" },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-block="intro"]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ["div", { ...HTMLAttributes, "data-block": "intro" }, 0];
  },
});

export const StoryNode = Node.create({
  name: "story",
  group: "block",
  content: "paragraph paragraph",
  draggable: true,
  addAttributes() {
    return {
      storyId: { default: "" },
      sourceUrl: { default: "" },
      block: { default: "story" },
    };
  },
  parseHTML() {
    return [{ tag: 'article[data-block="story"]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return [
      "article",
      {
        ...HTMLAttributes,
        "data-block": "story",
        "data-story-id": HTMLAttributes.storyId,
        "data-source-url": HTMLAttributes.sourceUrl,
      },
      0,
    ];
  },
});
