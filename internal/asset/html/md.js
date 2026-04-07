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
  // Extract fenced code blocks before HTML-escaping so highlighter gets raw code.
  var blocks = [];
  var s = raw.replace(/```(\w*)\n?([\s\S]*?)```/g, function(_, lang, code) {
    var h = highlight(code.replace(/\n$/, ''), lang);
    var idx = blocks.length;
    blocks.push(
      '<pre>' +
      '<button class="copy-btn" onclick="copyCode(this)">Copy</button>' +
      '<code>' + h + '</code>' +
      '</pre>'
    );
    return '\x00BLOCK' + idx + '\x00';
  });

  // Now HTML-escape the rest
  s = s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');

  // Inline code
  s = s.replace(/`([^`\n]+)`/g, '<code>$1</code>');
  // Headings
  s = s.replace(/^(#{1,3}) (.+)$/gm, function(_, h, t) {
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
  // Paragraphs
  s = s.split(/\n{2,}/).map(function(p) {
    p = p.trim(); if (!p) return '';
    if (/^<(h[1-6]|ul|ol|pre|hr|blockquote|\x00BLOCK)/.test(p)) return p;
    return '<p>' + p.replace(/\n/g, '<br>') + '</p>';
  }).filter(Boolean).join('\n');

  // Restore code blocks
  s = s.replace(/\x00BLOCK(\d+)\x00/g, function(_, i) { return blocks[+i]; });
  return s;
}

// ── Copy button handler ──────────────────────────────────────────────────────

function copyCode(btn) {
  var codeEl = btn.nextElementSibling;
  var text = codeEl ? codeEl.textContent : '';
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

global.mdRender = mdRender;
global.copyCode = copyCode;

})(window);
