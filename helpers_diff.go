package main

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

var dmp = diffmatchpatch.New()

// generateDiff returns a standard unified diff of changes between original and
// modified text. ctxLines unchanged lines surround each changed region.
// Returns an empty string when the texts are identical.
// Hunk headers use the full "@@ -a,b +c,d @@" format.
// Uses Myers diff algorithm via go-diff — no line-count limit.
func generateDiff(original, modified string, ctxLines int) string {
	if original == modified {
		return ""
	}

	// Compute line-level diffs using Myers algorithm.
	a, b, lineArray := dmp.DiffLinesToChars(original, modified)
	rawDiffs := dmp.DiffMain(a, b, false)
	lineDiffs := dmp.DiffCharsToLines(rawDiffs, lineArray)

	// Build a flat script with per-line original/new line numbers.
	type scriptLine struct {
		op    byte   // '=' unchanged  '-' removed  '+' added
		text  string
		oLine int    // 1-based line in original (0 for pure insertions)
		nLine int    // 1-based line in new      (0 for pure deletions)
	}
	var script []scriptLine
	oLine, nLine := 0, 0
	for _, d := range lineDiffs {
		lines := strings.Split(d.Text, "\n")
		// DiffLinesToChars always terminates segments with \n; drop the trailing empty.
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		for _, l := range lines {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				oLine++
				nLine++
				script = append(script, scriptLine{'=', l, oLine, nLine})
			case diffmatchpatch.DiffDelete:
				oLine++
				script = append(script, scriptLine{'-', l, oLine, 0})
			case diffmatchpatch.DiffInsert:
				nLine++
				script = append(script, scriptLine{'+', l, 0, nLine})
			}
		}
	}

	// Determine which lines to show (changed ± ctxLines context).
	show := make([]bool, len(script))
	for i, sl := range script {
		if sl.op != '=' {
			from := i - ctxLines
			if from < 0 {
				from = 0
			}
			to := i + ctxLines
			if to >= len(script) {
				to = len(script) - 1
			}
			for k := from; k <= to; k++ {
				show[k] = true
			}
		}
	}

	// Collect hunks — each hunk is a contiguous run of shown lines.
	type hunkSpec struct {
		oStart, nStart int
		oCount, nCount int
		lines          []string
	}
	var hunks []hunkSpec
	var cur *hunkSpec
	lastOLine, lastNLine := 0, 0

	for i, sl := range script {
		if !show[i] {
			if cur != nil {
				hunks = append(hunks, *cur)
				cur = nil
			}
			if sl.oLine > 0 {
				lastOLine = sl.oLine
			}
			if sl.nLine > 0 {
				lastNLine = sl.nLine
			}
			continue
		}

		if cur == nil {
			cur = &hunkSpec{}
			// Determine hunk start: scan ahead within the contiguous shown window
			// to find the first origLine and newLine in this hunk.
			for j := i; j < len(script) && show[j]; j++ {
				if cur.oStart == 0 && script[j].oLine > 0 {
					cur.oStart = script[j].oLine
				}
				if cur.nStart == 0 && script[j].nLine > 0 {
					cur.nStart = script[j].nLine
				}
				if cur.oStart > 0 && cur.nStart > 0 {
					break
				}
			}
			// Fallback for pure-insertion hunk (no orig lines in window).
			if cur.oStart == 0 {
				cur.oStart = lastOLine
			}
			// Fallback for pure-deletion hunk (no new lines in window).
			if cur.nStart == 0 {
				cur.nStart = lastNLine + 1
			}
		}

		switch sl.op {
		case '=':
			cur.lines = append(cur.lines, " "+sl.text)
			cur.oCount++
			cur.nCount++
			lastOLine = sl.oLine
			lastNLine = sl.nLine
		case '-':
			cur.lines = append(cur.lines, "-"+sl.text)
			cur.oCount++
			lastOLine = sl.oLine
		case '+':
			cur.lines = append(cur.lines, "+"+sl.text)
			cur.nCount++
			lastNLine = sl.nLine
		}
	}
	if cur != nil {
		hunks = append(hunks, *cur)
	}

	var sb strings.Builder
	for i, h := range hunks {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.oStart, h.oCount, h.nStart, h.nCount)
		for _, l := range h.lines {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
