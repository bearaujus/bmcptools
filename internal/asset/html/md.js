/* md.js — offline markdown renderer with syntax highlighting and copy support */
(function(global){

// ── Syntax highlighter ──────────────────────────────────────────────────────

var KW = {
  js:   'var|let|const|function|return|if|else|for|while|do|switch|case|break|continue|new|this|class|extends|import|export|default|typeof|instanceof|in|of|null|undefined|true|false|async|await|try|catch|finally|throw|delete|void',
  ts:   'var|let|const|function|return|if|else|for|while|do|switch|case|break|continue|new|this|class|extends|import|export|default|typeof|instanceof|in|of|null|undefined|true|false|async|await|try|catch|finally|throw|delete|void|type|interface|enum|namespace|declare|as|readonly|abstract|implements',
  py:   'def|class|return|if|elif|else|for|while|in|not|and|or|import|from|as|with|try|except|finally|raise|pass|break|continue|lambda|yield|None|True|False|global|nonlocal|del|assert|is|async|await',
  go:   'func|var|const|type|struct|interface|map|chan|go|defer|return|if|else|for|range|switch|case|default|break|continue|select|package|import|nil|true|false|make|new|len|cap|append|copy|delete|close|panic|recover|error|string|int|int64|int32|int16|int8|uint|uint64|bool|byte|rune|float64|float32',
  sh:   'if|then|else|elif|fi|for|do|done|while|until|case|esac|in|function|return|exit|echo|cd|ls|mkdir|rm|cp|mv|cat|grep|sed|awk|export|source|local|set|unset|true|false',
  rs:   'fn|let|mut|const|struct|enum|impl|trait|use|mod|pub|crate|super|self|return|if|else|for|while|loop|match|in|where|type|ref|move|async|await|dyn|true|false|Some|None|Ok|Err',
  java: 'class|interface|enum|extends|implements|import|package|public|private|protected|static|final|new|return|if|else|for|while|do|switch|case|break|continue|try|catch|finally|throw|throws|null|true|false|this|super|void|int|long|double|float|boolean|char|byte|short|String',
  rb:   'def|class|module|end|do|if|elsif|else|unless|while|until|for|in|return|yield|begin|rescue|ensure|raise|true|false|nil|self|super|require|require_relative|include|extend|attr_accessor|attr_reader|attr_writer',
};
KW.javascript = KW.js; KW.typescript = KW.ts; KW.python = KW.py;
KW.golang = KW.go; KW.bash = KW.sh; KW.shell = KW.sh; KW.zsh = KW.sh; KW.rust = KW.rs;

function esc(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function span(cls, s) {
  return '<span class="hl-'+cls+'">'+s+'</span>';
}

function highlight(raw, lang) {
  lang = (lang || '').toLowerCase();
  var keys = KW[lang] || '';
  var kwRe = keys ? new RegExp('^(?:'+keys+')(?![\\w$])') : null;

  var out = '';
  var i = 0;
  var code = raw; // raw is already plain text (not HTML-escaped yet per token)

  while (i < code.length) {
    var ch = code[i];

    // Line comment: // (JS/TS/Go/Java/Rust) or # (Python/Shell/Ruby)
    if (
      (ch === '/' && code[i+1] === '/') ||
      (ch === '#' && lang !== '' && 'py python rb ruby sh bash shell zsh'.indexOf(lang) !== -1)
    ) {
      var nl = code.indexOf('\n', i);
      var line = nl === -1 ? code.slice(i) : code.slice(i, nl);
      out += span('cmt', esc(line));
      i += line.length;
      continue;
    }
    // Block comment /* */
    if (ch === '/' && code[i+1] === '*') {
      var endB = code.indexOf('*/', i+2);
      var blk = endB === -1 ? code.slice(i) : code.slice(i, endB+2);
      out += span('cmt', esc(blk));
      i += blk.length;
      continue;
    }
    // Hash comment for all shell-likes (catch-all)
    if (ch === '#' && !keys) {
      var nl2 = code.indexOf('\n', i);
      var line2 = nl2 === -1 ? code.slice(i) : code.slice(i, nl2);
      out += span('cmt', esc(line2));
      i += line2.length;
      continue;
    }
    // Strings: ", ', `
    if (ch === '"' || ch === "'" || ch === '`') {
      var q = ch;
      var s = q; i++;
      while (i < code.length) {
        var c = code[i]; s += c; i++;
        if (c === '\\' && i < code.length) { s += code[i]; i++; continue; }
        if (c === q) break;
      }
      out += span('str', esc(s));
      continue;
    }
    // Numbers (int, float, hex, binary)
    if (/[0-9]/.test(ch) || (ch === '.' && /[0-9]/.test(code[i+1]||''))) {
      var num = '';
      while (i < code.length && /[0-9._xXa-fA-FoObBnNeE]/.test(code[i])) { num += code[i]; i++; }
      out += span('num', esc(num));
      continue;
    }
    // Identifiers, keywords, function names
    if (/[a-zA-Z_$]/.test(ch)) {
      var word = '';
      while (i < code.length && /[\w$]/.test(code[i])) { word += code[i]; i++; }
      var isKw = kwRe && kwRe.test(word);
      var isFn = code[i] === '(';
      if (isKw) out += span('kw', esc(word));
      else if (isFn) out += span('fn', esc(word));
      else out += esc(word);
      continue;
    }
    // Newlines (preserve as-is)
    if (ch === '\n') { out += '\n'; i++; continue; }
    // Everything else (operators, brackets, spaces)
    out += esc(ch);
    i++;
  }
  return out;
}

// ── Markdown renderer ────────────────────────────────────────────────────────

function mdRender(raw) {
  var COLLAPSE_LINES = 10; // collapse code blocks taller than this

  // Unescape literal \n and \t from AI runtimes FIRST (before code block extraction)
  var s = raw.replace(/\\n/g, '\n').replace(/\\t/g, '\t').replace(/\\"/g, '"');

  // Extract fenced code blocks before HTML-escaping so highlighter gets raw code.
  var blocks = [];
  // Code fences: require ``` at start of a line (^|\n) and a newline after
  // the opening fence. This prevents inline ``` (e.g. inside table cells)
  // from being mis-detected as a code block boundary.
  s = s.replace(/(^|\n)```(\w*)\n([\s\S]*?)```(?=\n|$)/g, function(_, pre, lang, code) {
    code = code.replace(/\n$/, '');
    var h = highlight(code, lang);
    var lines = h.split('\n');
    var numbered = lines.map(function(line, i) {
      return '<span class="code-line"><span class="line-no">' + (i+1) + '</span>' + (line || ' ') + '</span>';
    }).join('');
    var idx = blocks.length;
    var needsCollapse = lines.length > COLLAPSE_LINES;
    var wrapClass = 'code-wrap' + (needsCollapse ? ' collapsed' : '');
    blocks.push(
      '<div class="' + wrapClass + '">' +
      '<pre>' +
      '<button class="copy-btn" onclick="copyCode(this)">Copy</button>' +
      '<code>' + numbered + '</code>' +
      '</pre>' +
      (needsCollapse ? '<button class="expand-btn" onclick="toggleCode(this)" data-total="' + lines.length + '" data-hidden="' + (lines.length - COLLAPSE_LINES) + '">▶ Show ' + (lines.length - COLLAPSE_LINES) + ' more lines</button>' : '') +
      '</div>'
    );
    return pre + '\x00BLOCK' + idx + '\x00';
  });

  // Now HTML-escape the rest
  s = s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

  // Inline code — double-backtick first (allows single ` inside), then single
  s = s.replace(/``([^`]+)``/g, '<code>$1</code>');
  s = s.replace(/`([^`\n]+)`/g, '<code>$1</code>');
  // Headings
  s = s.replace(/^(#{1,6}) (.+)$/gm, function(_, h, t) {
    var n = h.length; return '<h'+n+'>'+t+'</h'+n+'>';
  });
  // Horizontal rule
  s = s.replace(/^---+$/gm, '<hr>');
  // Bold
  s = s.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
  // Italic
  s = s.replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
  // Links
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g,
    '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  // Unordered lists
  s = s.replace(/((?:^[ \t]*[-*] .+\n?)+)/gm, function(b) {
    var items = b.match(/^[ \t]*[-*] (.+)$/gm) || [];
    return '<ul>' + items.map(function(x) {
      return '<li>' + x.replace(/^[ \t]*[-*] /, '') + '</li>';
    }).join('') + '</ul>';
  });
  // Ordered lists
  s = s.replace(/((?:^[ \t]*\d+\. .+\n?)+)/gm, function(b) {
    var items = b.match(/^[ \t]*\d+\. (.+)$/gm) || [];
    return '<ol>' + items.map(function(x) {
      return '<li>' + x.replace(/^[ \t]*\d+\. /, '') + '</li>';
    }).join('') + '</ol>';
  });
  // Blockquotes — process BEFORE paragraphs, content inside is recursively rendered
  s = s.replace(/((?:^&gt; ?.+\n?)+)/gm, function(b) {
    var inner = b.replace(/^&gt; ?/gm, '').trimRight();
    return '<blockquote>' + inner + '</blockquote>';
  });
  // Tables — detect header | separator | rows pattern
  // splitCells handles escaped pipes (\|) so literal | can appear in cell text.
  function splitCells(row) {
    var parts = row.replace(/\\\|/g, '\x01').split('|');
    // Strip the leading/trailing empty parts produced by outer pipes,
    // but preserve empty cells in the middle of the row.
    if (parts.length && !parts[0].trim()) parts.shift();
    if (parts.length && !parts[parts.length-1].trim()) parts.pop();
    return parts.map(function(c){return c.replace(/\x01/g, '|').trim();});
  }
  s = s.replace(/((?:^\|.+\|[ \t]*\n?)+)/gm, function(block) {
    var rows = block.trim().split('\n');
    if (rows.length < 2) return block;
    // Check if second row is a separator (e.g. |---|---|)
    if (!/^\|[\s:]*-{2,}[\s:]*/.test(rows[1])) return block;
    // Parse alignment from separator row
    var seps = rows[1].split('|').filter(function(c){return c.trim();});
    var aligns = seps.map(function(c){
      c = c.trim();
      if (c[0]===':' && c[c.length-1]===':') return 'center';
      if (c[c.length-1]===':') return 'right';
      return 'left';
    });
    var html = '<table>';
    // Header
    var hCells = splitCells(rows[0]);
    html += '<thead><tr>';
    hCells.forEach(function(c,i){
      var a = aligns[i] || 'left';
      html += '<th style="text-align:'+a+'">'+c+'</th>';
    });
    html += '</tr></thead><tbody>';
    // Body rows (skip header and separator)
    var colCount = hCells.length;
    for (var r = 2; r < rows.length; r++) {
      var cells = splitCells(rows[r]);
      html += '<tr>';
      for (var ci = 0; ci < colCount; ci++) {
        var a = aligns[ci] || 'left';
        html += '<td style="text-align:'+a+'">'+(cells[ci]||'')+'</td>';
      }
      html += '</tr>';
    }
    html += '</tbody></table>';
    return html;
  });
  // Paragraphs
  s = s.split(/\n{2,}/).map(function(p) {
    p = p.trim(); if (!p) return '';
    if (/^<(h[1-6]|ul|ol|pre|div|hr|blockquote|table|\x00BLOCK)/.test(p)) return p;
    return '<p>' + p.replace(/\n/g, '<br>') + '</p>';
  }).filter(Boolean).join('\n');

  // Restore code blocks
  s = s.replace(/\x00BLOCK(\d+)\x00/g, function(_, i) { return blocks[+i]; });
  return s;
}

// ── Copy button handler ──────────────────────────────────────────────────────

function copyCode(btn) {
  var codeEl = btn.nextElementSibling;
  // Extract text without line numbers
  var lines = codeEl ? codeEl.querySelectorAll('.code-line') : [];
  var text = '';
  if (lines.length) {
    var parts = [];
    for (var i = 0; i < lines.length; i++) {
      // Clone, remove line-no span, get remaining text
      var clone = lines[i].cloneNode(true);
      var noEl = clone.querySelector('.line-no');
      if (noEl) noEl.remove();
      parts.push(clone.textContent);
    }
    text = parts.join('\n');
  } else {
    text = codeEl ? codeEl.textContent : '';
  }
  function flash() {
    btn.textContent = 'Copied!';
    btn.classList.add('copied');
    setTimeout(function() { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 2000);
  }
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(flash).catch(fallback);
  } else {
    fallback();
  }
  function fallback() {
    var ta = document.createElement('textarea');
    ta.value = text; ta.style.cssText = 'position:fixed;opacity:0';
    document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); flash(); } catch(e) {}
    document.body.removeChild(ta);
  }
}

// ── Expand/collapse handler ─────────────────────────────────────────────────

function toggleCode(btn) {
  var wrap = btn.parentElement;
  var hidden = btn.getAttribute('data-hidden') || '?';
  if (wrap.classList.contains('collapsed')) {
    wrap.classList.remove('collapsed');
    btn.textContent = '▼ Show less';
  } else {
    wrap.classList.add('collapsed');
    btn.textContent = '▶ Show ' + hidden + ' more lines';
    wrap.scrollIntoView({behavior:'smooth', block:'nearest'});
  }
}

global.mdRender = mdRender;
global.copyCode = copyCode;
global.toggleCode = toggleCode;

})(window);
