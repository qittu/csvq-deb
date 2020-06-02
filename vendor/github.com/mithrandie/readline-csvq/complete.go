package readline

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

type AutoCompleter interface {
	Do(line []rune, pos int, index int) (newLine CandidateList, length int)
}

type TabCompleter struct{}

func (t *TabCompleter) Do([]rune, int, int) (CandidateList, int) {
	return CandidateList{{Name: []rune("\t"), FormatAsIdentifier: false, AppendSpace: true}}, 0
}

type opCompleter struct {
	w     io.Writer
	op    *Operation
	width int

	inCompleteMode  bool
	inSelectMode    bool
	candidates      CandidateList
	candidateSource []rune
	candidateOff    int
	candidateChoise int
	candidateColNum int
}

func newOpCompleter(w io.Writer, op *Operation, width int) *opCompleter {
	return &opCompleter{
		w:     w,
		op:    op,
		width: width,
	}
}

func (o *opCompleter) doSelect() {
	if len(o.candidates) == 1 {
		o.op.buf.ReplaceRunes(o.candidates[0].Name, o.candidateOff, o.candidates[0].FormatAsIdentifier, o.candidates[0].AppendSpace)
		o.ExitCompleteMode(false)
		return
	}
	o.nextCandidate(1)
	o.CompleteRefresh()
}

func (o *opCompleter) nextCandidate(i int) {
	o.candidateChoise += i
	o.candidateChoise = o.candidateChoise % len(o.candidates)
	if o.candidateChoise < 0 {
		o.candidateChoise = len(o.candidates) + o.candidateChoise
	}
}

func (o *opCompleter) OnComplete() bool {
	if o.width == 0 {
		return false
	}

	if o.IsInCompleteSelectMode() {
		o.doSelect()
		return true
	}

	rs := o.op.buf.Runes()
	idx := o.op.buf.idx
	if 0 < len(o.op.continuousBuf) {
		rs = append(o.op.continuousBuf, rs...)
		idx = len(o.op.continuousBuf) + idx
	}

	if o.IsInCompleteMode() && o.candidateSource != nil && runes.Equal(rs, o.candidateSource) {
		o.EnterCompleteSelectMode()
		o.doSelect()
		return true
	}

	o.ExitCompleteSelectMode()
	o.candidateSource = rs
	newLines, offset := o.op.cfg.AutoComplete.Do(rs, idx, idx)
	if len(newLines) == 0 {
		o.ExitCompleteMode(false)
		return true
	}

	if !o.IsInCompleteMode() {
		if len(newLines) == 1 {
			o.op.buf.ReplaceRunes(newLines[0].Name, offset, newLines[0].FormatAsIdentifier, newLines[0].AppendSpace)
			o.ExitCompleteMode(false)
			return true
		}
	}

	o.EnterCompleteMode(offset, newLines)
	return true
}

func (o *opCompleter) IsInCompleteSelectMode() bool {
	return o.inSelectMode
}

func (o *opCompleter) IsInCompleteMode() bool {
	return o.inCompleteMode
}

func (o *opCompleter) HandleCompleteSelect(r rune) bool {
	next := true
	switch r {
	case CharEnter, CharCtrlJ:
		next = false
		o.op.buf.ReplaceRunes(o.op.candidates[o.op.candidateChoise].Name, o.op.candidateOff, o.op.candidates[o.op.candidateChoise].FormatAsIdentifier, o.op.candidates[o.op.candidateChoise].AppendSpace)
		o.ExitCompleteMode(false)
	case CharLineStart:
		num := o.candidateChoise % o.candidateColNum
		o.nextCandidate(-num)
	case CharLineEnd:
		num := o.candidateColNum - o.candidateChoise%o.candidateColNum - 1
		o.candidateChoise += num
		if o.candidateChoise >= len(o.candidates) {
			o.candidateChoise = len(o.candidates) - 1
		}
	case CharBackspace:
		o.ExitCompleteSelectMode()
		next = false
	case CharTab, CharForward:
		o.doSelect()
	case CharBell, CharInterrupt:
		o.ExitCompleteMode(true)
		next = false
	case CharNext:
		tmpChoise := o.candidateChoise + o.candidateColNum
		if tmpChoise >= o.getMatrixSize() {
			tmpChoise -= o.getMatrixSize()
		} else if tmpChoise >= len(o.candidates) {
			tmpChoise += o.candidateColNum
			tmpChoise -= o.getMatrixSize()
		}
		o.candidateChoise = tmpChoise
	case CharBackward:
		o.nextCandidate(-1)
	case CharPrev:
		tmpChoise := o.candidateChoise - o.candidateColNum
		if tmpChoise < 0 {
			tmpChoise += o.getMatrixSize()
			if tmpChoise >= len(o.candidates) {
				tmpChoise -= o.candidateColNum
			}
		}
		o.candidateChoise = tmpChoise
	default:
		next = false
		o.ExitCompleteSelectMode()
	}
	if next {
		o.CompleteRefresh()
		return true
	}
	return false
}

func (o *opCompleter) getMatrixSize() int {
	line := len(o.candidates) / o.candidateColNum
	if len(o.candidates)%o.candidateColNum != 0 {
		line++
	}
	return line * o.candidateColNum
}

func (o *opCompleter) OnWidthChange(newWidth int) {
	o.width = newWidth
}

func (o *opCompleter) CompleteRefresh() {
	if !o.inCompleteMode {
		return
	}
	lineCnt := o.op.buf.CursorLineCount()
	colWidth := 0
	for _, c := range o.candidates {
		w := runes.WidthAll(c.Name)
		if w > colWidth {
			colWidth = w
		}
	}
	colWidth++

	// -1 to avoid reach the end of line
	width := o.width - 1
	colNum := width / colWidth
	if colNum != 0 {
		colWidth += (width - (colWidth * colNum)) / colNum
	}

	o.candidateColNum = colNum
	buf := bufio.NewWriter(o.w)
	buf.Write(bytes.Repeat([]byte("\n"), lineCnt))

	colIdx := 0
	lines := 1
	buf.WriteString("\033[J")
	for idx, c := range o.candidates {
		inSelect := idx == o.candidateChoise && o.IsInCompleteSelectMode()
		if inSelect {
			buf.WriteString("\033[30;47m")
		}
		buf.WriteString(c.StringName())
		buf.Write(bytes.Repeat([]byte(" "), colWidth-runes.WidthAll(c.Name)))

		if inSelect {
			buf.WriteString("\033[0m")
		}

		colIdx++
		if colIdx == colNum {
			buf.WriteString("\n")
			lines++
			colIdx = 0
		}
	}

	// move back
	fmt.Fprintf(buf, "\033[%dA\r", lineCnt-1+lines)
	fmt.Fprintf(buf, "\033[%dC", o.op.buf.idx+o.op.buf.PromptLen())
	buf.Flush()
}

func (o *opCompleter) aggCandidate(candidate [][]rune) int {
	offset := 0
	for i := 0; i < len(candidate[0]); i++ {
		for j := 0; j < len(candidate)-1; j++ {
			if i > len(candidate[j]) {
				goto aggregate
			}
			if candidate[j][i] != candidate[j+1][i] {
				goto aggregate
			}
		}
		offset = i
	}
aggregate:
	return offset
}

func (o *opCompleter) EnterCompleteSelectMode() {
	o.inSelectMode = true
	o.candidateChoise = -1
	o.CompleteRefresh()
}

func (o *opCompleter) EnterCompleteMode(offset int, candidates CandidateList) {
	o.inCompleteMode = true
	o.candidates = candidates
	o.candidateOff = offset
	o.CompleteRefresh()
}

func (o *opCompleter) ExitCompleteSelectMode() {
	o.inSelectMode = false
	o.candidates = nil
	o.candidateChoise = -1
	o.candidateOff = -1
	o.candidateSource = nil
}

func (o *opCompleter) ExitCompleteMode(revent bool) {
	o.inCompleteMode = false
	o.ExitCompleteSelectMode()
}
