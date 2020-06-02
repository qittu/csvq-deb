package readline

import (
	"bytes"
	"sort"
	"strings"
	"unicode"
)

// Caller type for dynamic completion
type DynamicCompleteFunc func(string, string, int) CandidateList

type PrefixCompleterInterface interface {
	Print(prefix string, level int, buf *bytes.Buffer)
	Do(line []rune, pos int, index int) (newLine CandidateList, length int)
	GetName() Candidate
	GetChildren() []PrefixCompleterInterface
	SetChildren(children []PrefixCompleterInterface)
	IsAppendOnly() bool
}

type DynamicPrefixCompleterInterface interface {
	PrefixCompleterInterface
	IsDynamic() bool
	GetDynamicNames(line []rune, origLine []rune, index int) CandidateList
}

type Candidate struct {
	Name               []rune
	FormatAsIdentifier bool
	AppendSpace        bool
}

func (cand Candidate) StringName() string {
	return string(cand.Name)
}

type CandidateList []Candidate

func (l CandidateList) Len() int {
	return len(l)
}

func (l CandidateList) Less(i, j int) bool {
	return strings.Compare(strings.ToUpper(string(l[i].Name)), strings.ToUpper(string(l[j].Name))) < 0
}

func (l CandidateList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l CandidateList) Sort() {
	sort.Sort(l)
}

type PrefixCompleter struct {
	Name               []rune
	Dynamic            bool
	Callback           DynamicCompleteFunc
	Children           []PrefixCompleterInterface
	FormatAsIdentifier bool
	AppendSpace        bool
	AppendOnly         bool
}

func (p *PrefixCompleter) Tree(prefix string) string {
	buf := bytes.NewBuffer(nil)
	p.Print(prefix, 0, buf)
	return buf.String()
}

func Print(p PrefixCompleterInterface, prefix string, level int, buf *bytes.Buffer) {
	cand := p.GetName()
	if strings.TrimSpace(cand.StringName()) != "" {
		buf.WriteString(prefix)
		if level > 0 {
			buf.WriteString("├")
			buf.WriteString(strings.Repeat("─", (level*4)-2))
			buf.WriteString(" ")
		}
		buf.WriteString(cand.StringName() + "\n")
		level++
	}
	for _, ch := range p.GetChildren() {
		ch.Print(prefix, level, buf)
	}
}

func (p *PrefixCompleter) Print(prefix string, level int, buf *bytes.Buffer) {
	Print(p, prefix, level, buf)
}

func (p *PrefixCompleter) IsDynamic() bool {
	return p.Dynamic
}

func (p *PrefixCompleter) GetName() Candidate {
	return Candidate{
		Name:               p.Name,
		FormatAsIdentifier: p.FormatAsIdentifier,
		AppendSpace:        p.AppendSpace,
	}
}

func (p *PrefixCompleter) GetDynamicNames(line []rune, origLine []rune, index int) CandidateList {
	cans := p.Callback(string(line), string(origLine), index)
	for i := range cans {
		cans[i].Name = append(cans[i].Name, ' ')
	}
	return cans
}

func (p *PrefixCompleter) GetChildren() []PrefixCompleterInterface {
	return p.Children
}

func (p *PrefixCompleter) SetChildren(children []PrefixCompleterInterface) {
	p.Children = children
}

func (p *PrefixCompleter) IsAppendOnly() bool {
	return p.AppendOnly
}

func NewPrefixCompleter(pc ...PrefixCompleterInterface) *PrefixCompleter {
	return PcItem("", pc...)
}

func PcItem(name string, pc ...PrefixCompleterInterface) *PrefixCompleter {
	name += " "
	return &PrefixCompleter{
		Name:               []rune(name),
		Dynamic:            false,
		Children:           pc,
		FormatAsIdentifier: false,
		AppendSpace:        true,
		AppendOnly:         false,
	}
}

func PcItemDynamic(callback DynamicCompleteFunc, pc ...PrefixCompleterInterface) *PrefixCompleter {
	return &PrefixCompleter{
		Callback:           callback,
		Dynamic:            true,
		Children:           pc,
		FormatAsIdentifier: false,
		AppendSpace:        true,
		AppendOnly:         false,
	}
}

func (p *PrefixCompleter) Do(line []rune, pos int, index int) (newLine CandidateList, offset int) {
	return doInternal(p, line, pos, line, index)
}

func Do(p PrefixCompleterInterface, line []rune, pos int, index int) (newLine CandidateList, offset int) {
	return doInternal(p, line, pos, line, index)
}

func doInternal(p PrefixCompleterInterface, line []rune, pos int, origLine []rune, index int) (newLine CandidateList, offset int) {
	line = runes.TrimSpaceLeft(line[:pos])
	goNext := false
	var lineCompleter PrefixCompleterInterface
	for _, child := range p.GetChildren() {
		candidates := make(CandidateList, 1)

		if child.IsAppendOnly() {
			line = []rune(LastElement(string(line)))
		}

		childDynamic, ok := child.(DynamicPrefixCompleterInterface)
		if ok && childDynamic.IsDynamic() {
			candidates = childDynamic.GetDynamicNames(line, origLine, index)
		} else {
			candidates[0] = child.GetName()
		}

		for _, candidate := range candidates {
			if len(line) >= len(candidate.Name) {
				if runes.HasPrefixFold(line, candidate.Name, candidate.FormatAsIdentifier) {
					if len(line) == len(candidate.Name) {
						candidate.Name = append(candidate.Name, ' ')
					}
					newLine = append(newLine, candidate)
					offset = len(candidate.Name)
					lineCompleter = child
					goNext = true
				}
			} else {
				if runes.HasPrefixFold(candidate.Name, line, candidate.FormatAsIdentifier) {
					newLine = append(newLine, candidate)
					offset = len(line)
					lineCompleter = child
				}
			}
		}
	}

	if len(newLine) != 1 {
		return
	}

	tmpLine := make([]rune, 0, len(line))
	for i := offset; i < len(line); i++ {
		if line[i] == ' ' {
			continue
		}

		tmpLine = append(tmpLine, line[i:]...)
		return doInternal(lineCompleter, tmpLine, len(tmpLine), origLine, index)
	}

	if goNext {
		return doInternal(lineCompleter, nil, 0, origLine, index)
	}
	return
}

func LastElement(input string) string {
	src := []rune(input)
	buf := new(bytes.Buffer)

	var quote rune = -1

	for i := 0; i < len(src); i++ {
		if -1 < quote {
			switch src[i] {
			case quote:
				quote = -1
				buf.Reset()
			case '\\':
				if i+1 < len(src) && src[i+1] == quote {
					i++
				}
				fallthrough
			default:
				buf.WriteRune(src[i])
			}
			continue
		}

		if 0 < buf.Len() {
			switch {
			case unicode.IsSpace(src[i]) || IsUniqueToken(src[i]):
				buf.Reset()
			default:
				buf.WriteRune(src[i])
			}
			continue
		}

		if unicode.IsSpace(src[i]) || IsUniqueToken(src[i]) {
			continue
		}

		if IsQuotationMark(src[i]) {
			quote = src[i]
		}

		buf.WriteRune(src[i])
	}
	return buf.String()
}

func IsUniqueToken(r rune) bool {
	switch r {
	case ',', '(', ')', ';', '.':
		return true
	}
	return false
}

var RightBracket = map[rune]rune{
	'(': ')',
	'[': ']',
	'{': '}',
}

var LeftBracket = map[rune]rune{
	')': '(',
	']': '[',
	'}': '{',
}

func IsQuotationMark(r rune) bool {
	return r == '\'' || r == '"' || r == '`'
}

func IsBracket(r rune) bool {
	return r == '(' || r == '[' || r == '{'
}

func IsRightBracket(r rune) bool {
	return r == ')' || r == ']' || r == '}'
}

func LiteralIsEnclosed(enclosure rune, line []rune) bool {
	var enclosed = true
	for i := 0; i < len(line); i++ {
		if !enclosed {
			switch line[i] {
			case '\\':
				if i+1 < len(line) && line[i+1] == enclosure {
					i++
				}
			case enclosure:
				enclosed = true
			}
			continue
		}

		if line[i] == enclosure {
			enclosed = false
		}
	}
	return enclosed
}

func BracketIsEnclosed(leftBracket rune, line []rune) bool {
	rightBracket := RightBracket[leftBracket]

	var blockLevel = 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 < len(line) && line[i+1] == rightBracket {
				i++
			}
		case leftBracket:
			blockLevel++
		case rightBracket:
			blockLevel--
		}
	}
	return blockLevel < 1
}

func BracketIsEnclosedByRightBracket(rightBracket rune, line []rune) bool {
	leftBracket := LeftBracket[rightBracket]

	var blockLevel = 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 < len(line) && line[i+1] == rightBracket {
				i++
			}
		case leftBracket:
			blockLevel++
		case rightBracket:
			blockLevel--
		}
	}
	return blockLevel < 1
}
