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
      '<div class="code-head">' +
        '<span class="code-lang">' + (lang ? lang : 'text') + '</span>' +
        '<button class="copy-btn" onclick="copyCode(this)" title="Copy code">' +
          '<svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true"><path fill="currentColor" d="M5.75 1A1.75 1.75 0 0 0 4 2.75v9.5C4 13.216 4.784 14 5.75 14h7.5A1.75 1.75 0 0 0 15 12.25v-9.5A1.75 1.75 0 0 0 13.25 1h-7.5zm-.25 1.75a.25.25 0 0 1 .25-.25h7.5a.25.25 0 0 1 .25.25v9.5a.25.25 0 0 1-.25.25h-7.5a.25.25 0 0 1-.25-.25v-9.5z"/><path fill="currentColor" d="M2.5 5.5A.75.75 0 0 0 1.75 6.25v7A1.75 1.75 0 0 0 3.5 15h7a.75.75 0 0 0 0-1.5h-7a.25.25 0 0 1-.25-.25v-7a.75.75 0 0 0-.75-.75z"/></svg>' +
          '<span class="copy-label">Copy</span>' +
        '</button>' +
      '</div>' +
      '<pre>' +
      '<code>' + numbered + '</code>' +
      '</pre>' +
      (needsCollapse ? '<button class="expand-btn" onclick="toggleCode(this)" data-total="' + lines.length + '" data-hidden="' + (lines.length - COLLAPSE_LINES) + '">▶ Show ' + (lines.length - COLLAPSE_LINES) + ' more lines</button>' : '') +
      '</div>'
    );
    return pre + '\x00BLOCK' + idx + '\x00';
  });

  // Pre-capture raw table source BEFORE any transforms so the copy button
  // yields real markdown. Iterates the same regex used by the renderer below
  // in the same order, so indices line up.
  var rawTables = [];
  (function() {
    var re = /((?:^\|.+\|[ \t]*\n?)+)/gm;
    var m;
    while ((m = re.exec(s)) !== null) {
      rawTables.push(m[1].trim());
    }
  })();

  // Now HTML-escape the rest
  s = s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

  // Inline code — double-backtick first (allows single ` inside), then single
  s = s.replace(/``([^`\n]+)``/g, '<code>$1</code>');
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
  // Ordered lists — capture the starting number so split lists keep counting
  // correctly. (Without `start`, blank lines between items create separate
  // <ol> blocks that all restart at 1, causing "1 1 1".)
  s = s.replace(/((?:^[ \t]*\d+\. .+\n?)+)/gm, function(b) {
    var items = b.match(/^[ \t]*\d+\. (.+)$/gm) || [];
    var firstNum = (b.match(/^[ \t]*(\d+)\. /) || [0, '1'])[1];
    var startAttr = firstNum === '1' ? '' : ' start="' + firstNum + '"';
    return '<ol' + startAttr + '>' + items.map(function(x) {
      return '<li>' + x.replace(/^[ \t]*\d+\. /, '') + '</li>';
    }).join('') + '</ol>';
  });
  // Blockquotes — process BEFORE paragraphs, content inside is recursively rendered
  s = s.replace(/((?:^&gt; ?.+\n?)+)/gm, function(b) {
    var inner = b.replace(/^&gt; ?/gm, '').trimEnd();
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
  // Pre-capture raw table source BEFORE inline transforms have rewritten cell
  // contents into HTML — so the copy button can yield real markdown, not
  // half-rendered HTML. Iterates the same regex over the same string in the
  // same order as the renderer pass below, so indices line up.
  var tableIdx = 0;
  s = s.replace(/((?:^\|.+\|[ \t]*\n?)+)/gm, function(block) {
    var rawSource = rawTables[tableIdx++] || block.trim();
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
    var html = '<div class="md-table-frame" data-md="'+encodeURIComponent(rawSource)+'">'+
      '<div class="md-table-head">'+
        '<span class="md-table-label">table</span>'+
        '<button class="table-copy-btn" type="button" onclick="copyTable(this)" title="Copy table as markdown" aria-label="Copy table">'+
          '<svg viewBox="0 0 16 16" width="12" height="12" aria-hidden="true"><path fill="currentColor" d="M5.75 1A1.75 1.75 0 0 0 4 2.75v9.5C4 13.216 4.784 14 5.75 14h7.5A1.75 1.75 0 0 0 15 12.25v-9.5A1.75 1.75 0 0 0 13.25 1h-7.5zm-.25 1.75a.25.25 0 0 1 .25-.25h7.5a.25.25 0 0 1 .25.25v9.5a.25.25 0 0 1-.25.25h-7.5a.25.25 0 0 1-.25-.25v-9.5z"/><path fill="currentColor" d="M2.5 5.5A.75.75 0 0 0 1.75 6.25v7A1.75 1.75 0 0 0 3.5 15h7a.75.75 0 0 0 0-1.5h-7a.25.25 0 0 1-.25-.25v-7a.75.75 0 0 0-.75-.75z"/></svg>'+
          '<span class="copy-label">Copy</span>'+
        '</button>'+
      '</div>'+
      '<div class="md-table-wrap"><table>';
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
    html += '</tbody></table></div></div>';
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

// ── Copy button handlers ─────────────────────────────────────────────────────

function copyTable(btn) {
  var frame = btn.closest('.md-table-frame') || btn.closest('.md-table-wrap');
  var raw = frame ? frame.getAttribute('data-md') : '';
  try { raw = decodeURIComponent(raw || ''); } catch(e) { raw = raw || ''; }
  clipCopy(raw, function() {
    var label = btn.querySelector('.copy-label');
    if(label) { label.textContent = 'Copied!'; }
    btn.classList.add('copied');
    setTimeout(function(){
      var l = btn.querySelector('.copy-label');
      if(l) { l.textContent = 'Copy'; }
      btn.classList.remove('copied');
    }, 1500);
  });
}

function copyCode(btn) {
  var wrap = btn.closest('.code-wrap');
  var codeEl = wrap ? wrap.querySelector('pre code') : btn.nextElementSibling;
  var lines = codeEl ? codeEl.querySelectorAll('.code-line') : [];
  var text = '';
  if(lines.length) {
    var parts = [];
    for(var i = 0; i < lines.length; i++) {
      var clone = lines[i].cloneNode(true);
      var noEl = clone.querySelector('.line-no');
      if(noEl) noEl.remove();
      parts.push(clone.textContent);
    }
    text = parts.join('\n');
  } else {
    text = codeEl ? codeEl.textContent : '';
  }
  clipCopy(text, function() {
    var label = btn.querySelector('.copy-label');
    if(label) { label.textContent = 'Copied!'; }
    else { btn.textContent = 'Copied!'; }
    btn.classList.add('copied');
    setTimeout(function() {
      var l = btn.querySelector('.copy-label');
      if(l) { l.textContent = 'Copy'; }
      else { btn.textContent = 'Copy'; }
      btn.classList.remove('copied');
    }, 2000);
  });
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

// ── Shared utilities (used by all three HTML templates) ───────────────────────

function fmtSec(s) {
  s = Math.max(0, Math.round(s));
  if(s < 60) return s + 's';
  var m = Math.floor(s/60), sec = s%60;
  if(m < 60) return m + 'm' + (sec > 0 ? ' ' + sec + 's' : '');
  var h = Math.floor(m/60), min = m%60;
  return h + 'h' + (min > 0 ? ' ' + min + 'm' : '');
}

function clipCopy(text, onSuccess) {
  function fallback() {
    var ta = document.createElement('textarea');
    ta.value = text; ta.style.cssText = 'position:fixed;opacity:0';
    document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); onSuccess(); } catch(e) {}
    document.body.removeChild(ta);
  }
  if(navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(onSuccess).catch(fallback);
  } else { fallback(); }
}

function DoubleConfirm() {
  var pending = null;
  var dc = {
    arm: function(btnId, action, label) {
      var btn = document.getElementById(btnId);
      if(!btn || btn.disabled) return;
      if(pending && pending.btnId === btnId) {
        var act = pending.action; dc.reset(); act(); return;
      }
      dc.reset();
      var original = btn.innerHTML;
      btn.innerHTML = label; btn.classList.add('confirming');
      pending = {btnId:btnId, action:action, original:original,
        timer:setTimeout(function(){ dc.reset(); }, 3000)};
    },
    reset: function() {
      if(!pending) return;
      var p = pending; pending = null;
      clearTimeout(p.timer);
      var btn = document.getElementById(p.btnId);
      if(btn){ btn.innerHTML = p.original; btn.classList.remove('confirming'); }
    },
    active: function() { return !!pending; }
  };
  return dc;
}

global.mdRender = mdRender;
global.copyCode = copyCode;
global.copyTable = copyTable;
global.toggleCode = toggleCode;
global.fmtSec = fmtSec;
global.clipCopy = clipCopy;
global.DoubleConfirm = DoubleConfirm;

})(window);
