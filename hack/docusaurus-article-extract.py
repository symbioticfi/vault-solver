#!/usr/bin/env python3
# RedStone publishes its Atom OEV integration guide only as a rendered Docusaurus page; the GitHub repo
# that served the raw markdown is gone. This reads that HTML on stdin and writes the article body
# (the `theme-doc-markdown` div) to stdout as plain text: headings keep a `#` prefix, list items a `- `,
# table cells are joined with ` | ` and code blocks are kept line for line. Navigation, asset hashes and
# copy buttons are left out, so a diff of the output only moves when the guide's text does.
# Used by `make refresh-redstone-guide` and the `docusaurus-html` mode of hack/check-schema-drift.py.
from __future__ import annotations

import html.parser
import re
import sys

BLOCKS = {"p", "div", "ul", "ol", "table", "thead", "tbody", "blockquote", "details", "summary", "hr"}
HEADINGS = {f"h{n}": n for n in range(1, 7)}
SKIP = {"button", "script", "style", "svg"}
VOID = {"br", "hr", "img", "input", "meta", "link", "source", "wbr"}


class Extractor(html.parser.HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.depth = 0  # open elements inside the article body; 0 = outside
        self.skip = 0
        self.pre = 0
        self.lines: list[str] = []
        self.current: list[str] = []
        self.found = False
        self.lists: list[int | None] = []  # per open list: next ordered number, or None for <ul>
        self.links: list[str | None] = []
        self.marker = False  # `current` holds only a list item's prefix, waiting for its text

    def flush(self) -> None:
        text = "".join(self.current)
        if not self.pre:
            text = re.sub(r"\s+", " ", text).strip()
        if text or self.pre:
            self.lines.append(text.rstrip())
        self.current = []
        self.marker = False

    def block(self, keep_marker: bool = False) -> None:
        # A loose list wraps item text in <p>; keep_marker leaves the "1. " in place so the text joins it.
        if self.current and not (keep_marker and self.marker):
            self.flush()

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        classes = (dict(attrs).get("class") or "").split()
        if not self.depth:
            if tag == "div" and "theme-doc-markdown" in classes:
                self.depth, self.found = 1, True
            return
        if tag not in VOID:
            self.depth += 1
        if self.skip or tag in SKIP or "hash-link" in classes:
            if tag not in VOID:
                self.skip += 1
            return
        if tag == "br":
            self.flush() if self.pre else self.current.append(" ")
        elif tag == "pre":
            self.block()
            language = next((c.removeprefix("language-") for c in classes if c.startswith("language-")), "")
            self.lines.append("```" + language)
            self.pre += 1
        elif self.pre:
            return
        elif tag in HEADINGS:
            self.block()
            self.lines.append("")
            self.current.append("#" * HEADINGS[tag] + " ")
        elif tag in ("ul", "ol"):
            self.block()
            self.lists.append(1 if tag == "ol" else None)
        elif tag == "li":
            self.block()
            number = self.lists[-1] if self.lists else None
            self.current.append(f"{number}. " if number else "- ")
            if number:
                self.lists[-1] = number + 1
            self.marker = True
        elif tag == "a":
            href = dict(attrs).get("href") or ""
            self.links.append(href if href.startswith(("http://", "https://")) else None)
        elif tag == "tr":
            self.block()
        elif tag in ("td", "th"):
            if self.current:
                self.current.append(" | ")
        elif tag in BLOCKS:
            self.block(keep_marker=True)

    def handle_endtag(self, tag: str) -> None:
        if not self.depth or tag in VOID:
            return
        self.depth -= 1
        if self.skip:
            self.skip -= 1
            return
        if tag == "pre":
            self.block()
            self.pre -= 1
            self.lines.append("```")
        elif self.pre:
            return
        elif tag == "a":
            if href := (self.links.pop() if self.links else None):
                self.current.append(f" <{href}>")
            return
        if tag in ("ul", "ol") and self.lists:
            self.lists.pop()
        if tag in HEADINGS or tag in BLOCKS or tag in ("li", "tr"):
            self.block()
        if not self.depth:
            self.block()

    def handle_data(self, data: str) -> None:
        if self.depth and not self.skip:
            self.current.append(data)
            if data.strip():
                self.marker = False


def extract(page: str) -> str:
    parser = Extractor()
    parser.feed(page)
    parser.close()
    if not parser.found:
        raise SystemExit("docusaurus-article-extract: no theme-doc-markdown article found in HTML")
    text = "\n".join(line.replace("​", "") for line in parser.lines)
    text = re.sub(r"\n{3,}", "\n\n", text).strip()
    if not text:
        raise SystemExit("docusaurus-article-extract: article body is empty")
    return text + "\n"


if __name__ == "__main__":
    sys.stdout.write(extract(sys.stdin.read()))
