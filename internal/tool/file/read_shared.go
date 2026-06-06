package file

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bearaujus/bmcptools/internal/helper"
)

// ReadLineRange identifies a 1-indexed inclusive line range.
// EndLine may be 0 to read through EOF.
type ReadLineRange struct {
	StartLine int
	EndLine   int
}

// ReadOptions mirrors the line/window controls exposed by read_file.
type ReadOptions struct {
	MaxBytes        int
	IncludeBase64   bool
	StartLine       int
	EndLine         int
	Head            int
	Tail            int
	ShowLineNumbers bool
	Ranges          []ReadLineRange
}

// ReadOutput contains rendered tool text plus basic source metadata.
type ReadOutput struct {
	FileSize int64
	Text     string
}

type lineRenderState struct {
	builder       strings.Builder
	limit         int
	showLineNums  bool
	firstLine     int
	lastLine      int
	returnedLines int
	returnedChars int
	truncated     bool
}

func ParseReadLineRanges(raw any) ([]ReadLineRange, error) {
	rawRanges, ok := raw.([]any)
	if !ok || len(rawRanges) == 0 {
		return nil, fmt.Errorf("ranges must be a non-empty array of [start_line, end_line] pairs")
	}

	ranges := make([]ReadLineRange, 0, len(rawRanges))
	for _, r := range rawRanges {
		pair, ok := r.([]any)
		if !ok || len(pair) < 2 {
			return nil, fmt.Errorf("each range must be a [start_line, end_line] pair")
		}
		start, ok1 := toInt(pair[0])
		end, ok2 := toInt(pair[1])
		if !ok1 || !ok2 || start < 1 || end < 0 {
			return nil, fmt.Errorf("range values must be integers with start_line >= 1 and end_line >= 0")
		}
		ranges = append(ranges, ReadLineRange{StartLine: start, EndLine: end})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].StartLine < ranges[j].StartLine })
	return ranges, nil
}

func ReadPathWithOptions(path string, opts ReadOptions) (ReadOutput, error) {
	if path == "" {
		return ReadOutput{}, fmt.Errorf("path is required")
	}

	limitBytes := opts.MaxBytes
	if limitBytes <= 0 {
		limitBytes = helper.DefaultMaxReadBytes
	}

	f, info, contentType, binary, err := helper.SniffAndOpen(path)
	if err != nil {
		return ReadOutput{}, err
	}
	defer f.Close()

	out := ReadOutput{FileSize: info.Size()}
	if binary {
		text, err := helper.ReadBinaryFile(f, info, contentType, limitBytes, opts.IncludeBase64)
		if err != nil {
			return ReadOutput{}, err
		}
		out.Text = text
		return out, nil
	}

	switch {
	case len(opts.Ranges) > 0:
		out.Text, err = readSharedMultiRange(f, info, opts.Ranges, limitBytes, opts.ShowLineNumbers)
	case opts.Head > 0 && opts.Tail > 0:
		out.Text, err = readSharedHeadThenTail(f, info, opts.Head, opts.Tail, limitBytes, opts.ShowLineNumbers)
	case opts.Head > 0:
		out.Text, err = readSharedLineWindow(f, info, 1, opts.Head, limitBytes, opts.ShowLineNumbers)
	case opts.Tail > 0:
		out.Text, err = readSharedTail(f, info, opts.Tail, limitBytes, opts.ShowLineNumbers)
	case opts.StartLine > 0 || opts.EndLine > 0:
		out.Text, err = readSharedLineWindow(f, info, opts.StartLine, opts.EndLine, limitBytes, opts.ShowLineNumbers)
	case opts.ShowLineNumbers:
		out.Text, err = readSharedFullWithLineNumbers(f, info, limitBytes)
	default:
		text, truncated, readErr := helper.ReadFullText(f, info, limitBytes)
		if readErr != nil {
			return ReadOutput{}, readErr
		}
		if truncated {
			out.Text = text
			return out, nil
		}
		lineCount := helper.CountContentLines(text)
		charCount := utf8.RuneCountInString(text)
		out.Text = fmt.Sprintf("[%s — %s, %s]\n%s",
			info.Name(),
			helper.Pluralize(lineCount, "line"),
			helper.Pluralize(charCount, "char"),
			text,
		)
		return out, nil
	}
	if err != nil {
		return ReadOutput{}, err
	}
	return out, nil
}

func (r *lineRenderState) appendLine(lineNum int, text string) bool {
	if r.firstLine == 0 {
		r.firstLine = lineNum
	}
	r.lastLine = lineNum
	r.returnedLines++
	r.returnedChars += utf8.RuneCountInString(text)
	if r.showLineNums {
		fmt.Fprintf(&r.builder, "%6d|%s\n", lineNum, text)
	} else {
		r.builder.WriteString(text)
		r.builder.WriteByte('\n')
	}
	if r.builder.Len() >= r.limit {
		r.truncated = true
		return false
	}
	return true
}

func readSharedFullWithLineNumbers(f *os.File, info os.FileInfo, limit int) (string, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	state := lineRenderState{limit: limit, showLineNums: true}
	totalLines := 0
	totalKnown := true
	for scanner.Scan() {
		totalLines++
		if !state.appendLine(totalLines, scanner.Text()) {
			totalKnown = false
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}

	if state.returnedLines == 0 {
		return fmt.Sprintf("[%s — 0 lines]", info.Name()), nil
	}

	if state.truncated && info.Size() <= helper.AutoLineCountMaxBytes {
		for scanner.Scan() {
			totalLines++
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read error: %w", err)
		}
		totalKnown = true
	}

	header := formatFullLineHeader(info.Name(), state.returnedLines, totalLines, totalKnown, state.returnedChars)
	if state.truncated {
		state.builder.WriteString(formatTruncationNotice(
			limit,
			info.Size(),
			totalLines,
			totalKnown,
			"Use start_line/end_line to read specific sections.",
		))
	}
	return header + state.builder.String(), nil
}

func readSharedLineWindow(f *os.File, info os.FileInfo, startLine, endLine, limit int, showLineNums bool) (string, error) {
	if startLine < 1 {
		startLine = 1
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	state := lineRenderState{limit: limit, showLineNums: showLineNums}
	lineNum := 0
	reachedEOF := true
	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if endLine > 0 && lineNum > endLine {
			reachedEOF = false
			break
		}
		if !state.appendLine(lineNum, scanner.Text()) {
			reachedEOF = false
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}
	if state.returnedLines == 0 {
		end := "EOF"
		if endLine > 0 {
			end = fmt.Sprintf("line %d", endLine)
		}
		return fmt.Sprintf("[%s] No lines found between line %d and %s", info.Name(), startLine, end), nil
	}

	totalLines, totalKnown, err := resolveTotalLines(f, info, reachedEOF && !state.truncated, lineNum)
	if err != nil {
		return "", err
	}
	header := formatSelectedLineHeader(info.Name(), state, totalLines, totalKnown, "")
	if state.truncated {
		state.builder.WriteString(fmt.Sprintf(
			"\n[TRUNCATED — range output reached max_bytes=%s. Use a smaller line range or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return header + state.builder.String(), nil
}

func readSharedHeadThenTail(f *os.File, info os.FileInfo, headN, tailN, limit int, showLineNums bool) (string, error) {
	if headN < 1 {
		headN = 1
	}
	if tailN < 1 {
		tailN = 1
	}
	if tailN > headN {
		tailN = headN
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	lines := make([]ReadLineRange, 0, 0)
	selected := make([]selectedLine, 0, tailN)
	lineNum := 0
	reachedEOF := true
	for scanner.Scan() {
		lineNum++
		if lineNum > headN {
			reachedEOF = false
			break
		}
		selected = append(selected, selectedLine{number: lineNum, text: scanner.Text()})
		if len(selected) > tailN {
			selected = selected[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}
	_ = lines
	if len(selected) == 0 {
		return fmt.Sprintf("[%s] No lines found in head=%d window", info.Name(), headN), nil
	}

	totalLines, totalKnown, err := resolveTotalLines(f, info, reachedEOF, lineNum)
	if err != nil {
		return "", err
	}
	state := renderSelectedLines(selected, limit, showLineNums)
	header := formatSelectedLineHeader(
		info.Name(),
		state,
		totalLines,
		totalKnown,
		fmt.Sprintf("tail %d of head %d, ", tailN, headN),
	)
	if state.truncated {
		state.builder.WriteString(fmt.Sprintf(
			"\n[TRUNCATED — range output reached max_bytes=%s. Use a smaller line range or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return header + state.builder.String(), nil
}

func readSharedTail(f *os.File, info os.FileInfo, tailN, limit int, showLineNums bool) (string, error) {
	if tailN < 1 {
		tailN = 1
	}

	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	selected := make([]selectedLine, 0, tailN)
	totalLines := 0
	for scanner.Scan() {
		totalLines++
		selected = append(selected, selectedLine{number: totalLines, text: scanner.Text()})
		if len(selected) > tailN {
			selected = selected[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}
	if len(selected) == 0 {
		return fmt.Sprintf("[%s] No lines found in tail=%d window", info.Name(), tailN), nil
	}

	state := renderSelectedLines(selected, limit, showLineNums)
	header := formatSelectedLineHeader(info.Name(), state, totalLines, true, "")
	if state.truncated {
		state.builder.WriteString(fmt.Sprintf(
			"\n[TRUNCATED — range output reached max_bytes=%s. Use a smaller line range or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return header + state.builder.String(), nil
}

func readSharedMultiRange(f *os.File, info os.FileInfo, ranges []ReadLineRange, limit int, showLineNums bool) (string, error) {
	scanner := bufio.NewScanner(f)
	scanBuf := make([]byte, 64*1024)
	scanner.Buffer(scanBuf, scannerBufferLimit(limit))

	var sb strings.Builder
	lineNum := 0
	rangeIdx := 0
	firstInRange := true
	truncated := false
	returnedChars := 0

	for scanner.Scan() {
		lineNum++
		if rangeIdx >= len(ranges) {
			break
		}
		rng := ranges[rangeIdx]
		if lineNum < rng.StartLine {
			continue
		}
		if rng.EndLine > 0 && lineNum > rng.EndLine {
			rangeIdx++
			firstInRange = true
			if rangeIdx >= len(ranges) {
				break
			}
			rng = ranges[rangeIdx]
			if lineNum < rng.StartLine {
				continue
			}
		}
		if firstInRange {
			if rangeIdx > 0 {
				sb.WriteString("\n")
			}
			endStr := "EOF"
			if rng.EndLine > 0 {
				endStr = fmt.Sprintf("%d", rng.EndLine)
			}
			fmt.Fprintf(&sb, "--- Lines %d–%s ---\n", rng.StartLine, endStr)
			firstInRange = false
		}
		if showLineNums {
			fmt.Fprintf(&sb, "%6d|%s\n", lineNum, scanner.Text())
		} else {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		returnedChars += utf8.RuneCountInString(scanner.Text())
		if sb.Len() >= limit {
			truncated = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("[%s] No lines found in specified ranges", info.Name()), nil
	}

	header := fmt.Sprintf("[%s — %s]\n", info.Name(), helper.Pluralize(returnedChars, "char"))
	if truncated {
		sb.WriteString(fmt.Sprintf(
			"\n[TRUNCATED — range output reached max_bytes=%s. Use fewer/smaller ranges or raise max_bytes.]",
			helper.HumanizeBytes(int64(limit)),
		))
	}
	return header + sb.String(), nil
}

type selectedLine struct {
	number int
	text   string
}

func renderSelectedLines(lines []selectedLine, limit int, showLineNums bool) lineRenderState {
	state := lineRenderState{limit: limit, showLineNums: showLineNums}
	for _, line := range lines {
		if !state.appendLine(line.number, line.text) {
			break
		}
	}
	return state
}

func resolveTotalLines(f *os.File, info os.FileInfo, totalKnown bool, knownTotal int) (int, bool, error) {
	if totalKnown {
		return knownTotal, true, nil
	}
	if info.Size() > helper.AutoLineCountMaxBytes {
		return 0, false, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, false, fmt.Errorf("seek error: %w", err)
	}
	total, err := helper.CountLines(f)
	if err != nil {
		return 0, false, fmt.Errorf("read error: %w", err)
	}
	return total, true, nil
}

func formatSelectedLineHeader(name string, state lineRenderState, totalLines int, totalKnown bool, qualifier string) string {
	if totalKnown {
		return fmt.Sprintf("[%s — %slines %d..%d of %s, %s]\n",
			name,
			qualifier,
			state.firstLine,
			state.lastLine,
			helper.Pluralize(totalLines, "line"),
			helper.Pluralize(state.returnedChars, "char"),
		)
	}
	return fmt.Sprintf("[%s — %slines %d..%d, %s]\n",
		name,
		qualifier,
		state.firstLine,
		state.lastLine,
		helper.Pluralize(state.returnedChars, "char"),
	)
}

func formatFullLineHeader(name string, shownLines, totalLines int, totalKnown bool, chars int) string {
	if totalKnown {
		return fmt.Sprintf("[%s — %s, %s]\n",
			name,
			helper.Pluralize(totalLines, "line"),
			helper.Pluralize(chars, "char"),
		)
	}
	return fmt.Sprintf("[%s — first %s shown, %s]\n",
		name,
		helper.Pluralize(shownLines, "line"),
		helper.Pluralize(chars, "char"),
	)
}

func formatTruncationNotice(limit int, fileSize int64, totalLines int, totalKnown bool, hint string) string {
	if totalKnown {
		return fmt.Sprintf(
			"\n[TRUNCATED — showing first %s of %s (%s total). %s]",
			helper.HumanizeBytes(int64(limit)),
			helper.HumanizeBytes(fileSize),
			helper.Pluralize(totalLines, "line"),
			hint,
		)
	}
	return fmt.Sprintf(
		"\n[TRUNCATED — showing first %s of %s. %s]",
		helper.HumanizeBytes(int64(limit)),
		helper.HumanizeBytes(fileSize),
		hint,
	)
}
