// md_test.js — automated tests for mdRender
// Run: node md_test.js

// Mock browser globals so md.js can attach to "window"
var window = {};

// Load md.js via eval so it binds to our mock window
var fs = require('fs');
var path = require('path');
eval(fs.readFileSync(path.join(__dirname, 'md.js'), 'utf8'));

var mdRender = window.mdRender;
var passed = 0, failed = 0;

function assert(name, condition, detail) {
  if (condition) {
    passed++;
  } else {
    failed++;
    console.error('FAIL: ' + name + (detail ? ' — ' + detail : ''));
  }
}

function assertContains(name, html, substr) {
  assert(name, html.indexOf(substr) !== -1,
    'expected to contain "' + substr + '" but got: ' + html.substring(0, 300));
}

function assertNotContains(name, html, substr) {
  assert(name, html.indexOf(substr) === -1,
    'should NOT contain "' + substr + '" but found in: ' + html.substring(0, 300));
}

// ─── 1. Code Fences ─────────────────────────────────────────────────────────

(function() {
  var input = '```go\nfunc main(){}\n```';
  var html = mdRender(input);
  assertContains('code fence creates <pre>', html, '<pre>');
  assertContains('code fence contains <code>', html, '<code>');
  assertContains('code fence has copy button', html, 'copy-btn');
})();

(function() {
  var input = 'some text ``` not a fence ``` more text';
  var html = mdRender(input);
  assertNotContains('inline triple backticks not a fence', html, '<pre>');
})();

(function() {
  // Multi-line code block should have line numbers
  var input = '```js\nvar a = 1;\nvar b = 2;\n```';
  var html = mdRender(input);
  assertContains('code fence has line numbers', html, 'line-no');
})();

// ─── 2. Inline Code ─────────────────────────────────────────────────────────

(function() {
  var html = mdRender('`hello`');
  assertContains('single backtick inline code', html, '<code>hello</code>');
})();

(function() {
  // Double backtick wraps content without inner backticks
  var html = mdRender('``some code``');
  assertContains('double backtick inline code', html, '<code>some code</code>');
})();

// ─── 3. Table Rendering ─────────────────────────────────────────────────────

(function() {
  var input = '| Col1 | Col2 |\n| --- | --- |\n| a | b |';
  var html = mdRender(input);
  assertContains('basic table has <table>', html, '<table>');
  assertContains('basic table has <thead>', html, '<thead>');
  assertContains('basic table has <tbody>', html, '<tbody>');
  assertContains('basic table header cell', html, '<th');
  assertContains('basic table data cell', html, '<td');
})();

(function() {
  var input = '| col1 | col2 |\n| --- | --- |\n| val\\|ue | other |';
  var html = mdRender(input);
  assertContains('escaped pipe in table cell', html, 'val|ue');
})();

(function() {
  // Right/center alignment
  var input = '| Left | Center | Right |\n| :--- | :---: | ---: |\n| a | b | c |';
  var html = mdRender(input);
  assertContains('table left alignment', html, 'text-align:left');
  assertContains('table center alignment', html, 'text-align:center');
  assertContains('table right alignment', html, 'text-align:right');
})();

(function() {
  // Empty cells should be preserved, not dropped
  var input = '| Cat | Tool | Grade |\n| --- | --- | --- |\n| File | read | A |\n| | write | B |';
  var html = mdRender(input);
  // Row 2 has an empty first cell — it must still produce 3 <td> elements
  // 3 total <tr>: 1 header + 2 body rows
  var matches = html.match(/<tr>/g);
  assert('table row count', matches && matches.length === 3, 'expected 3 <tr> (1 header + 2 body)');
  // Count <td> elements — should be 6 total (3 per row)
  var tds = html.match(/<td[^>]*>/g);
  assert('table empty cell preserved (6 tds)', tds && tds.length === 6,
    'expected 6 <td> but got ' + (tds ? tds.length : 0));
})();

(function() {
  // Rows with fewer cells than header should be padded
  var input = '| A | B | C |\n| --- | --- | --- |\n| only-one |';
  var html = mdRender(input);
  var tds = html.match(/<td[^>]*>/g);
  assert('short row padded to header length', tds && tds.length === 3,
    'expected 3 <td> but got ' + (tds ? tds.length : 0));
})();

// ─── 4. Basic Markdown ──────────────────────────────────────────────────────

(function() {
  var html = mdRender('**bold**');
  assertContains('bold text', html, '<strong>bold</strong>');
})();

(function() {
  var html = mdRender('*italic*');
  assertContains('italic text', html, '<em>italic</em>');
})();

(function() {
  var html = mdRender('# Title');
  assertContains('h1 heading', html, '<h1>Title</h1>');
})();

(function() {
  var html = mdRender('## Subtitle');
  assertContains('h2 heading', html, '<h2>Subtitle</h2>');
})();

(function() {
  var html = mdRender('[text](https://example.com)');
  assertContains('link href', html, 'href="https://example.com"');
  assertContains('link text', html, '>text</a>');
})();

(function() {
  var html = mdRender('---');
  assertContains('horizontal rule', html, '<hr>');
})();

(function() {
  var html = mdRender('- item1\n- item2');
  assertContains('unordered list', html, '<ul>');
  assertContains('unordered list item1', html, '<li>item1</li>');
  assertContains('unordered list item2', html, '<li>item2</li>');
})();

(function() {
  var html = mdRender('1. first\n2. second');
  assertContains('ordered list', html, '<ol>');
  assertContains('ordered list item', html, '<li>first</li>');
})();

(function() {
  // Tables must be wrapped so wide content is horizontally scrollable.
  var html = mdRender('| a | b |\n| --- | --- |\n| 1 | 2 |');
  assertContains('table wrapped in scroll container', html, '<div class="md-table-wrap"');
  assertContains('table copy button present', html, 'class="table-copy-btn"');
  assertContains('table wrapper closed', html, '</table></div>');
})();

(function() {
  // Ordered lists not starting at 1 must emit `start=` so the marker is correct
  // even if the list gets split into multiple <ol> blocks (the "1 1 1" bug).
  var html = mdRender('3. third\n4. fourth');
  assertContains('ol start preserved', html, '<ol start="3">');
  // Lists starting at 1 should NOT have a redundant start attribute.
  var html2 = mdRender('1. one\n2. two');
  assertNotContains('ol start=1 omitted', html2, 'start="1"');
})();

(function() {
  var html = mdRender('> quoted text');
  assertContains('blockquote', html, '<blockquote>');
  assertContains('blockquote content', html, 'quoted text');
})();

// ─── 5. Edge Cases ──────────────────────────────────────────────────────────

(function() {
  var html = mdRender('');
  assert('empty input does not crash', typeof html === 'string');
})();

(function() {
  // mdRender unescapes literal \n from AI runtimes
  var html = mdRender('line1\\nline2');
  assertContains('backslash-n produces line break', html, 'line1');
  assertContains('backslash-n produces line2', html, 'line2');
  // The unescaped newline becomes <br> inside a paragraph
  assertContains('backslash-n renders as <br>', html, '<br>');
})();

(function() {
  var html = mdRender('<script>alert(1)</script>');
  assertNotContains('HTML script tag escaped (no <script>)', html, '<script>');
  assertContains('HTML script tag shows escaped', html, '&lt;script&gt;');
})();

(function() {
  // Ensure backslash-t unescape works
  var html = mdRender('col1\\tcol2');
  assertContains('backslash-t unescaped', html, 'col1\tcol2');
})();

(function() {
  // Code block with >10 lines should be collapsible
  var lines = [];
  for (var i = 0; i < 15; i++) lines.push('line ' + i);
  var input = '```\n' + lines.join('\n') + '\n```';
  var html = mdRender(input);
  assertContains('long code block is collapsed', html, 'collapsed');
  assertContains('long code block has expand button', html, 'expand-btn');
})();

(function() {
  // Code block with <=10 lines should NOT be collapsed
  var lines = [];
  for (var i = 0; i < 5; i++) lines.push('line ' + i);
  var input = '```\n' + lines.join('\n') + '\n```';
  var html = mdRender(input);
  assertNotContains('short code block not collapsed', html, 'collapsed');
})();

(function() {
  // Syntax highlighting for Go keywords
  var input = '```go\nfunc main() {\n\tvar x = 1\n}\n```';
  var html = mdRender(input);
  assertContains('Go keyword highlighted', html, 'hl-kw');
})();

(function() {
  // Paragraphs
  var html = mdRender('para one\n\npara two');
  assertContains('paragraph wrapping first', html, '<p>para one</p>');
  assertContains('paragraph wrapping second', html, '<p>para two</p>');
})();

(function() {
  // Link opens in new tab
  var html = mdRender('[click](http://x.com)');
  assertContains('link target blank', html, 'target="_blank"');
  assertContains('link noopener', html, 'rel="noopener noreferrer"');
})();

// ─── Summary ─────────────────────────────────────────────────────────────────

console.log('\n' + passed + ' passed, ' + failed + ' failed');
process.exit(failed > 0 ? 1 : 0);
