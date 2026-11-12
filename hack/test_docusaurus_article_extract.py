"""Ensure the RedStone guide text survives Docusaurus markup and ignores page chrome."""

import pathlib
import subprocess
import sys
import unittest

SCRIPT = pathlib.Path(__file__).with_name("docusaurus-article-extract.py")

PAGE = """<!doctype html><html><head><script src="/assets/js/main.1a2b.js"></script></head><body>
<nav>Docs Home</nav>
<article><div class="theme-doc-markdown markdown"><header><h1>Guide</h1></header>
<h2 class="anchor" id="s1">1. Connect<a href="#s1" class="hash-link">​</a></h2>
<p>Send   the <code>x-api-key</code>
header.</p>
<ol><li>request</li><li>execute</li></ol>
<ul><li>see <a href="https://example.com/repo">here</a></li></ul>
<ol><li><p>hash it</p><p>then more</p></li><li>
<p>sign it</p></li></ol>
<table><tr><th>op</th><th>meaning</th></tr><tr><td>solve</td><td>bid</td></tr></table>
<div class="codeBlock"><pre class="prism-code language-json codeBlock_x"><code><span class="token-line">{<br></span><span class="token-line">  "op": "subscribe"<br></span><span class="token-line">}</span></code></pre><button>Copy</button></div>
</div><footer>Edit this page</footer></article>
<footer>Copyright</footer></body></html>
"""

EXPECTED = """# Guide

## 1. Connect
Send the x-api-key header.
1. request
2. execute
- see here <https://example.com/repo>
1. hash it
then more
2. sign it
op | meaning
solve | bid
```json
{
  "op": "subscribe"
}
```
"""


def run(page: str) -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(SCRIPT)], input=page, capture_output=True, text=True, check=False)


class DocusaurusArticleTest(unittest.TestCase):
    def test_extracts_article_text(self):
        result = run(PAGE)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, EXPECTED)

    def test_ignores_asset_hashes_outside_article(self):
        self.assertEqual(run(PAGE.replace("main.1a2b.js", "main.9f8e.js")).stdout, EXPECTED)

    def test_fails_without_article(self):
        for page in ("", "<html><body><p>Not found</p></body></html>"):
            with self.subTest(page=page):
                result = run(page)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
